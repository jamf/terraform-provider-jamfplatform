// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListReturnToServiceConfigurationsV1
// Status: current. Last reviewed 2026-06-13.

package return_to_service

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

// defaultListTimeout caps how long the list operation waits on the Pro
// /v1/return-to-service endpoint.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &ReturnToServiceListResource{}
var _ list.ListResourceWithConfigure = &ReturnToServiceListResource{}

// NewReturnToServiceListResource returns a list resource for Return to Service
// configuration queries.
func NewReturnToServiceListResource() list.ListResource {
	return &ReturnToServiceListResource{}
}

// ReturnToServiceListResource implements Terraform query list support. The Pro
// list endpoint returns full configuration objects, so `include_resource`
// hydrates every attribute directly from the list element — no per-row follow-up
// GET. The optional `filter` block applies a case-insensitive display-name
// substring match client-side after the full list is fetched.
type ReturnToServiceListResource struct {
	client *pro.Client
}

// ReturnToServiceListResourceModel is the config model for list queries.
type ReturnToServiceListResourceModel struct {
	Filter *filters.ClassicFilterModel `tfsdk:"filter"`
}

// Metadata sets the list resource type name.
func (r *ReturnToServiceListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_return_to_service"
}

// Configure wires the Jamf Pro client into the list resource.
func (r *ReturnToServiceListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_return_to_service")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *ReturnToServiceListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro Return to Service configurations. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams Return to Service configuration identities
// back to Terraform.
func (r *ReturnToServiceListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config ReturnToServiceListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	listResp, err := r.client.ListReturnToServiceConfigurationsV1(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro Return to Service configurations", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []pro.ReturnToServiceConfiguration{}
	if listResp != nil {
		items = listResp.Results
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, returnToServiceListItemName)

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

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, returnToServiceIdentityModel{ID: types.StringValue(item.ID)})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The list response carries the full object; hydrate every attribute
			// directly — no follow-up GET.
			state := ReturnToServiceResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(returnToServiceTimeoutAttributeTypes),
			}
			assignReturnToServiceResourceModel(&state, &item)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro Return to Service configurations", map[string]any{
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

// returnToServiceListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func returnToServiceListItemName(c pro.ReturnToServiceConfiguration) string {
	return c.DisplayName
}
