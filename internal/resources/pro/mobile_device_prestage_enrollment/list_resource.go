// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListMobileDevicePrestagesV3
//
// Status: current. Last reviewed 2026-05-30.

package mobile_device_prestage_enrollment

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

const defaultListTimeout = 60 * time.Second

// defaultItemReadTimeout caps each per-item scope read performed during config
// generation, independent of the list-fetch budget.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &MobileDevicePrestageEnrollmentListResource{}
	_ list.ListResourceWithConfigure = &MobileDevicePrestageEnrollmentListResource{}
)

// MobileDevicePrestageEnrollmentListResource implements Terraform query list
// support for Jamf Pro Mobile Device PreStage Enrollments. The Pro list
// endpoint accepts no RSQL filter, so the optional `filter` block is applied
// client-side after the full list is fetched. The list response carries the
// full `GetMobileDevicePrestageV3` shape per row, so when IncludeResource=true
// the row is populated from the same response — no N+1 follow-up GET is
// required, except scope: the list response omits scope assignments, so when
// IncludeResource is requested (config generation) each row's scope is fetched
// with GetMobileDevicePrestageScopeV2 and ScopeSerialNumbers is populated from
// it (an empty set when no serials are assigned). This captures the real
// serials for a faithful import and gives the set a defined element type — an
// uninitialised types.Set has none and fails config generation with a value
// conversion error. The per-item scope read uses its own timeout, decoupled
// from the list-fetch budget; a row whose scope read fails is dropped from the
// generated config rather than aborting the whole type.
type MobileDevicePrestageEnrollmentListResource struct {
	client *pro.Client
}

// NewMobileDevicePrestageEnrollmentListResource returns a list resource.
func NewMobileDevicePrestageEnrollmentListResource() list.ListResource {
	return &MobileDevicePrestageEnrollmentListResource{}
}

// Metadata sets the list resource type name.
func (r *MobileDevicePrestageEnrollmentListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_prestage_enrollment"
}

// Configure wires the Jamf Pro client into the list resource.
func (r *MobileDevicePrestageEnrollmentListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_prestage_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *MobileDevicePrestageEnrollmentListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro Mobile Device PreStage Enrollments. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched. The Jamf Pro list response carries every attribute, so `include_resource = true` does not require a follow-up read per item." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams prestage identities back to Terraform.
func (r *MobileDevicePrestageEnrollmentListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config MobileDevicePrestageEnrollmentListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListMobileDevicePrestagesV3(listCtx, nil)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro mobile device prestage enrollments", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, mobileDevicePrestageListItemName)

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
		result.DisplayName = item.DisplayName

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, MobileDevicePrestageEnrollmentIdentityModel{ID: types.StringValue(item.ID)})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := MobileDevicePrestageEnrollmentResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignGetToResource(ctx, &state, state, &item)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			scopeCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			scope, err := r.client.GetMobileDevicePrestageScopeV2(scopeCtx, item.ID)
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping mobile device prestage enrollment from generated config after scope read failure", map[string]any{
					"id":    item.ID,
					"error": err.Error(),
				})
				continue
			}
			state.ScopeSerialNumbers = scopeSerialsToSet(scope)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro Mobile Device PreStage Enrollments", map[string]any{
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

// mobileDevicePrestageListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func mobileDevicePrestageListItemName(item pro.GetMobileDevicePrestageV3) string {
	return item.DisplayName
}
