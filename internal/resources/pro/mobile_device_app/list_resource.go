// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation waits on the classic
// /mobiledeviceapplications endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &MobileAppListResource{}
var _ list.ListResourceWithConfigure = &MobileAppListResource{}

// NewMobileAppListResource returns a list resource for Jamf Pro mobile device app queries.
func NewMobileAppListResource() list.ListResource {
	return &MobileAppListResource{}
}

// MobileAppListResource implements Terraform query list support for Jamf Pro
// mobile device apps. Classic /mobiledeviceapplications has no RSQL — the
// optional `filter` block is applied client-side via filters.ApplyClassicFilter.
// List items carry only id + name on the wire, so when IncludeResource is
// requested (config generation) each app is fetched individually and hydrated
// through the shared Read state-builder with includeUnmanaged=true, populating
// every wire-present section (general, scope, self_service, vpp,
// app_configuration) so the generated config is complete rather than
// general-only.
type MobileAppListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *MobileAppListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_app"
}

// Configure wires the Jamf ProClassic client into the list resource.
func (r *MobileAppListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *MobileAppListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro mobile device apps. Supply an optional case-insensitive `name_substring` filter applied locally after the full list is fetched. List entries surface as identity-only (id and display name); full detail requires a per-app read." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams app identities back to Terraform.
func (r *MobileAppListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config MobileAppListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListMobileDeviceApplications(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro mobile device apps", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.MobileDeviceApplicationsItemMobileDeviceApplication{}
	if resp != nil {
		items = resp.MobileDeviceApplications
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, mobileAppListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, a := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(a.Name)

		id := helpers.StringValueFromIntPtr(a.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, mobileAppIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetMobileDeviceApplicationByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping mobile device application from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := MobileAppResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(mobileAppTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignMobileAppResourceModel(ctx, &state, got, true)...)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro mobile device apps", map[string]any{
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

// mobileAppListItemName is the name accessor passed to filters.ApplyClassicFilter.
func mobileAppListItemName(a proclassic.MobileDeviceApplicationsItemMobileDeviceApplication) string {
	return helpers.DerefString(a.Name)
}
