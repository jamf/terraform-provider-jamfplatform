// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

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
// /policies endpoint.
const defaultListTimeout = 120 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &PolicyListResource{}
var _ list.ListResourceWithConfigure = &PolicyListResource{}

// NewPolicyListResource returns a list resource for Jamf Pro policy queries.
func NewPolicyListResource() list.ListResource {
	return &PolicyListResource{}
}

// PolicyListResource implements Terraform query list support for Jamf Pro
// policies. Classic /policies has no RSQL — the optional `filter` block is
// applied client-side via filters.ApplyClassicFilter. List items carry only
// id + name on the wire, so when IncludeResource is requested (config
// generation) each policy is fetched individually and hydrated through the
// shared Read state-builder with includeUnmanaged=true, populating every
// wire-present section (general, scope, self_service, scripts, packages, …)
// so the generated config is complete rather than general-only.
type PolicyListResource struct {
	client *proclassic.Client
}

// PolicyListResourceModel is the config model for policy list queries.
type PolicyListResourceModel struct {
	Filter *filters.ClassicFilterModel `tfsdk:"filter"`
}

// Metadata sets the list resource type name.
func (r *PolicyListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_policy"
}

// Configure wires the Jamf ProClassic client into the list resource.
func (r *PolicyListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_policy")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *PolicyListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro policies. Supply an optional case-insensitive `name_substring` filter applied locally after the full list is fetched. List entries surface as identity-only (id and display name); full policy detail requires a per-policy read." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams policy identities back to Terraform.
func (r *PolicyListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config PolicyListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListPolicies(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro policies", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.IDName{}
	if resp != nil {
		items = resp.Policies
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, policyListItemName)

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
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, policyIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetPolicyByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping Jamf Pro policy from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := PolicyResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(policyTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignPolicyResourceModel(ctx, &state, got, true)...)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro policies", map[string]any{
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

// policyListItemName is the name accessor passed to filters.ApplyClassicFilter.
func policyListItemName(p proclassic.IDName) string {
	return helpers.DerefString(p.Name)
}
