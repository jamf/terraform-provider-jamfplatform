// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

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

// defaultListTimeout caps how long the list operation will wait on the
// classic /ibeacons endpoint. The list resource schema does not expose a
// user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &IbeaconListResource{}
var _ list.ListResourceWithConfigure = &IbeaconListResource{}

// NewIbeaconListResource returns a list resource for Jamf Pro iBeacon queries.
func NewIbeaconListResource() list.ListResource {
	return &IbeaconListResource{}
}

// IbeaconListResource implements Terraform query list support for Jamf Pro
// iBeacons. Classic /ibeacons accepts no query parameters, so the optional
// `filter` block is applied client-side via filters.ApplyClassicFilter after
// the full list is fetched. Unlike most classic list endpoints, /ibeacons
// returns full Ibeacon objects (including uuid/major/minor) rather than
// minimal id+name rows — the IncludeResource branch can populate every
// schema attribute without a follow-up GET.
type IbeaconListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *IbeaconListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ibeacon"
}

// Configure wires the Jamf ProClassic client into the list resource via the
// shared providerdata.ConfigureProClassic helper.
func (r *IbeaconListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ibeacon")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *IbeaconListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro iBeacons. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams iBeacon identities back to Terraform.
func (r *IbeaconListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config IbeaconListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListIBeacons(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro iBeacons", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.Ibeacon{}
	if resp != nil {
		items = resp.Ibeacons
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, ibeaconListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, ib := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(ib.Name)

		id := helpers.StringValueFromIntPtr(ib.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, ibeaconIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// /ibeacons returns full Ibeacon objects — every Optional/Computed
			// schema attribute can be populated from the list element without
			// a follow-up GET. assignIbeaconResourceModel handles the
			// -1/-1 → IncludeAny derivation.
			state := IbeaconResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(ibeaconTimeoutAttributeTypes),
			}
			ibCopy := ib
			result.Diagnostics.Append(assignIbeaconResourceModel(&state, &ibCopy)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro iBeacons", map[string]any{
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

// ibeaconListItemName is the name accessor passed to filters.ApplyClassicFilter.
func ibeaconListItemName(ib proclassic.Ibeacon) string {
	return helpers.DerefString(ib.Name)
}
