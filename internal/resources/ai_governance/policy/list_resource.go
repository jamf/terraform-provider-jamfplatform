// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPolicySort is the sort expression every list read sends, so the streamed order is
// deterministic across runs rather than left to the platform's default.
const defaultPolicySort = "name:asc"

// policyTimeoutAttributeTypes defines the object attribute types for the resource's timeouts block,
// needed to build a null value when a list result carries full resource state.
var policyTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

var (
	_ list.ListResource              = &PolicyListResource{}
	_ list.ListResourceWithConfigure = &PolicyListResource{}
)

// NewPolicyListResource returns a list resource for AI Governance policy queries.
func NewPolicyListResource() list.ListResource {
	return &PolicyListResource{}
}

// PolicyListResource implements Terraform query and bulk-import support for AI Governance policies.
type PolicyListResource struct {
	client *aigovernance.Client
}

// Metadata sets the list resource type name.
func (r *PolicyListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_governance_policy"
}

// Configure wires the AI Governance client into the list resource.
func (r *PolicyListResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected List Resource Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_ai_governance_policy", providerdata.AIGovernanceScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = aigovernance.New(pd.Client)
}

// ListResourceConfigSchema describes the list configuration.
func (r *PolicyListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf AI Governance policies, for `terraform query` and for importing existing policies " +
			"in bulk. Archived policies are never returned." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"schema_drift_only": listschema.BoolAttribute{
				Description: "When true, return only policies whose settings schema version is behind the one " +
					"the platform now offers for their tool. These are the policies worth reviewing after a tool " +
					"publishes a new schema.",
				Optional: true,
			},
		},
	}
}

// policyListConfigModel is the list resource's configuration.
type policyListConfigModel struct {
	SchemaDriftOnly types.Bool `tfsdk:"schema_drift_only"`
}

// List executes the query and streams policy identities back to Terraform.
//
// Filling in full resource state costs one read per policy: the listing omits the settings, so there
// is no way to answer IncludeResource from the list alone. The reads are serial, which keeps a query
// over a large tenant inside the platform's request budget, but they are bounded only when the caller
// sets a limit — the listing itself is always fetched in full, and an unlimited query with
// IncludeResource reads every policy on the tenant.
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

	var config policyListConfigModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	summaries, err := r.client.ListPolicies(ctx, []string{defaultPolicySort}, config.SchemaDriftOnly.ValueBool())
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list AI policies", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(summaries)) {
		maxResults = int64(len(summaries))
	}

	results := make([]list.ListResult, 0, maxResults)
	for _, summary := range summaries {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = summary.Name

		id := types.StringValue(summary.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, policyIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			if !r.appendResourceState(ctx, &result, summary.ID) {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed AI Governance policies", map[string]any{
		"limit":    req.Limit,
		"returned": len(results),
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

// appendResourceState fills one list result's resource state, reporting whether it succeeded.
func (r *PolicyListResource) appendResourceState(ctx context.Context, result *list.ListResult, id string) bool {
	detail, err := r.client.GetPolicy(ctx, id)
	if err != nil {
		result.Diagnostics.AddError("Unable to read AI policy "+id, helpers.APIErrorDetail(err))
		return false
	}

	state := policyModel{Timeouts: helpers.NewResourceTimeoutsNullValue(policyTimeoutAttributeTypes)}
	if err := applyPolicyToState(&state, detail); err != nil {
		result.Diagnostics.AddError("Unable to read AI policy settings", helpers.APIErrorDetail(err))
		return false
	}
	state.Publish = resolvePublish(state.Publish)

	result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
	return !result.Diagnostics.HasError()
}
