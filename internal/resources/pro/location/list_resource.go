// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListVolumePurchasingLocationsV1
// Status: current. Last reviewed 2026-05-25.

package location

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait on the
// Pro /v1/volume-purchasing-locations endpoint. The list resource schema does
// not expose a user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &VolumePurchasingLocationListResource{}
	_ list.ListResourceWithConfigure = &VolumePurchasingLocationListResource{}
)

// VolumePurchasingLocationListResource implements Terraform query list
// support for Jamf Pro Volume Purchasing locations. The Pro list endpoint
// supports RSQL, but the provider intentionally uses the shared client-side
// substring matcher for the list resource — the surface mirrors the other
// simple Pro list resources and the dataset is small enough that the extra
// server-side filtering complexity isn't warranted. The list endpoint returns
// the slim `VolumePurchasingLocationListView` shape, which omits the
// `Content` purchased-content slice. When `include_resource = true`, the
// list resource follows up with a per-row `GetVolumePurchasingLocationV1`
// call to populate the content catalog — identity-only listing stays a
// single round trip.
type VolumePurchasingLocationListResource struct {
	client *pro.Client
}

// NewVolumePurchasingLocationListResource returns a list resource for Jamf
// Pro Volume Purchasing location queries.
func NewVolumePurchasingLocationListResource() list.ListResource {
	return &VolumePurchasingLocationListResource{}
}

// Metadata sets the list resource type name.
func (r *VolumePurchasingLocationListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_volume_purchasing_location"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *VolumePurchasingLocationListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_volume_purchasing_location")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *VolumePurchasingLocationListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro Volume Purchasing (VPP) locations. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched. The Jamf Pro list response omits the purchased-content catalog; setting `include_resource = true` triggers a follow-up read per row to populate the `content` catalog. Identity-only listing stays a single round trip." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams VPP location identities back to
// Terraform.
func (r *VolumePurchasingLocationListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config VolumePurchasingLocationListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListVolumePurchasingLocationsV1(listCtx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro Volume Purchasing locations", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, volumePurchasingLocationListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for i := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		item := items[i]

		result := req.NewListResult(ctx)
		result.DisplayName = item.Name

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, volumePurchasingLocationIdentityModel{ID: types.StringValue(item.ID)})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// ListView omits the purchased-content catalog; follow up with a
			// singular GET to populate the `content` attribute. Identity-only
			// listing skips this round trip.
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			full, err := r.client.GetVolumePurchasingLocationV1(itemCtx, item.ID)
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping Volume Purchasing location from generated config after per-item read failure", map[string]any{
					"id":    item.ID,
					"error": err.Error(),
				})
				continue
			}
			state := VolumePurchasingLocationResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(volumePurchasingLocationTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignVolumePurchasingLocationResourceModel(ctx, &state, full)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro Volume Purchasing locations", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
	})

	if len(results) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}

// volumePurchasingLocationListItemName is the name accessor passed to
// filters.ApplyClassicFilter. `VolumePurchasingLocationListView.Name` is a
// plain `string` field (not a pointer), so no nil check is required.
func volumePurchasingLocationListItemName(item pro.VolumePurchasingLocationListView) string {
	return item.Name
}
