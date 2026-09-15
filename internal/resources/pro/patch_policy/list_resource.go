// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait on the Pro v2
// patch-policies collection. It is a budget for the whole sweep, not for one
// request: the SDK pages the collection in full, so the round-trip count is a
// function of tenant size rather than fixed at one as it was on the classic
// collection this replaced. The page size is 2000, so a tenant needs more than
// 2000 patch policies before a second request is issued at all, and 90s stays
// generous well past that; revisit the constant if that ceiling is ever crossed
// in practice. The list resource schema does not expose a user-overridable
// timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item GET issued when IncludeResource is
// requested (config generation). A single slow item is skipped from the
// generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &PatchPolicyListResource{}
var _ list.ListResourceWithConfigure = &PatchPolicyListResource{}

// NewPatchPolicyListResource returns a list resource for Jamf Pro patch policy
// queries.
func NewPatchPolicyListResource() list.ListResource {
	return &PatchPolicyListResource{}
}

// PatchPolicyListResource implements Terraform query list support. Enumeration
// runs on the Pro v2 patch-policies collection; per-item hydration stays on the
// ProClassic by-ID read, which is the only surface carrying a policy's scope and
// user-interaction sections.
//
// The optional `filter` block is applied client-side via
// filters.ApplyClassicFilter after the full list is fetched: v2 accepts an RSQL
// query on `policyName`, but RSQL string comparison is case-sensitive and this
// resource's contract is a case-insensitive substring, so filtering server-side
// would silently narrow it.
type PatchPolicyListResource struct {
	client    *proclassic.Client
	proClient *pro.Client
}

// Metadata sets the list resource type name.
func (r *PatchPolicyListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_policy"
}

// Configure wires both clients this list resource spans: the Pro client for the
// v2 enumeration and the ProClassic client for the per-item read.
func (r *PatchPolicyListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_policy")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_policy")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.proClient = proClient
}

// ListResourceConfigSchema describes the supported list filters.
func (r *PatchPolicyListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro patch policies. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched against the policy display name." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams patch policy identities back to Terraform.
//
// The Pro v2 collection returns a summary view per policy (id, name, target
// version, deployment method, deployment counts) and no scope or
// user-interaction sections, so when IncludeResource is requested (config
// generation) each policy is fetched individually through the ProClassic by-ID
// read and hydrated through the shared Read state-builder with
// includeUnmanaged=true, populating every wire-present section so the generated
// config is complete rather than identity-only.
//
// A policy the v2 collection enumerated but the classic read cannot return is
// dropped from the result set, because a result carrying no resource body
// generates no configuration and the framework additionally reports it as a
// provider bug. The drop is reported: every skipped policy is collected and one
// trailing diagnostics-only result carries a warning naming them, so a
// generated configuration is never silently short of the collection. That
// matters more here than it did on the classic collection, because enumeration
// and hydration no longer share an id space by construction — the v2 id is
// taken on the wire-confirmed equality with the classic by-id path, and this
// warning is what surfaces a policy where that equality does not hold.
func (r *PatchPolicyListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil || r.proClient == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config PatchPolicyListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.proClient.ListPatchPoliciesV2(listCtx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro patch policies", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, patchPolicyName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)
	var skipped []skippedPatchPolicy

	for _, s := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = s.PolicyName

		id := types.StringValue(s.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, patchPolicyIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetPatchPolicyByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping Jamf Pro patch policy from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				skipped = append(skipped, skippedPatchPolicy{
					id:   id.ValueString(),
					name: s.PolicyName,
					err:  err,
				})
				continue
			}
			state := PatchPolicyResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(patchPolicyTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignPatchPolicyResourceModel(ctx, &state, got, true)...)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro patch policies", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
		"skipped":        len(skipped),
	})

	if len(results) == 0 && len(skipped) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if len(skipped) > 0 {
			push(list.ListResult{
				Diagnostics: diag.Diagnostics{
					diag.NewWarningDiagnostic(
						"Some Jamf Pro patch policies were left out of the results",
						skippedPatchPoliciesWarningDetail(skipped),
					),
				},
			})
		}
	}
}

// skippedPatchPolicy records a policy the Pro v2 collection enumerated but the
// ProClassic by-id read could not return, so the omission can be reported once
// at the end of the stream rather than per item.
type skippedPatchPolicy struct {
	id   string
	name string
	err  error
}

// skippedPatchPoliciesWarningDetail is the operator-facing detail for the
// trailing warning. It names every skipped policy with the error that skipped
// it, and points at the likeliest cause: the results are enumerated on Pro v2
// and read back on ProClassic, so an id the classic path does not recognise is
// the one failure a Terraform-visible message has to explain.
func skippedPatchPoliciesWarningDetail(skipped []skippedPatchPolicy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d patch policy/policies were listed by the Jamf Pro patch-policies collection but could not be read individually, so they carry no configuration and are not in the results:\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(&b, "  - %s (id %s): %s\n", s.name, s.id, s.err)
	}
	b.WriteString("\nEach policy is enumerated on the Pro v2 collection and read back on the ProClassic by-id path. Check that the API integration holds the patch-policies read permission, and that the policy still exists — a policy deleted between the two calls reports the same way.")
	return b.String()
}

// patchPolicyName is the name accessor passed to filters.ApplyClassicFilter.
func patchPolicyName(s pro.PatchPolicyListView) string {
	return s.PolicyName
}
