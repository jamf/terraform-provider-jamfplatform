// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_assignment

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

const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item GET issued when IncludeResource is
// requested (config generation). A single slow item is skipped from the
// generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &VPPAssignmentListResource{}
	_ list.ListResourceWithConfigure = &VPPAssignmentListResource{}
)

func NewVPPAssignmentListResource() list.ListResource {
	return &VPPAssignmentListResource{}
}

type VPPAssignmentListResource struct {
	client *proclassic.Client
}

func (r *VPPAssignmentListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_vpp_assignment"
}

func (r *VPPAssignmentListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_assignment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

func (r *VPPAssignmentListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro VPP assignments. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

func (r *VPPAssignmentListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unconfigured Provider", "The provider has not been configured yet."),
		})
		return
	}

	var config VPPAssignmentListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	apiResp, err := r.client.ListVPPAssignments(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro VPP assignments", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.VppAssignmentsItemVppAssignment{}
	if apiResp != nil {
		items = apiResp.VppAssignments
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, vppAssignmentItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, p := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(p.Name)

		id := helpers.StringValueFromIntPtr(p.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, vppAssignmentIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetVPPAssignmentByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping Jamf Pro VPP assignment from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := VPPAssignmentResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(vppAssignmentTimeoutAttributeTypes),
			}
			assignVPPAssignmentResourceModel(ctx, &state, got, true)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}
		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro VPP assignments", map[string]any{
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

func vppAssignmentItemName(p proclassic.VppAssignmentsItemVppAssignment) string {
	return helpers.DerefString(p.Name)
}
