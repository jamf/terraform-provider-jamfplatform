// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.GetDnsCustomHostnameMappingsV1
//   securitycloud.ReplaceDnsCustomHostnameMappingsV1
//   securitycloud.ClearDnsCustomHostnameMappingsV1
//
// Status: current.

package dns_hostname_mappings

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// providerNotConfiguredError is the diagnostic every handler raises when the client
// is missing, kept in one place so the wordings cannot drift apart.
func providerNotConfiguredError() (string, string) {
	return "Provider not configured",
		"The provider client was not configured. Please ensure the provider block is set up correctly."
}

// Create writes the tenant's hostname mappings, refusing to discard existing ones.
//
// The preflight read is the whole point. PUT is an unconditional full replace that
// reports no conflict, so without reading first a plan that looks like a create would
// silently discard mappings an administrator added by hand, and two Terraform
// configurations both managing this resource would overwrite each other in silence.
// Refusing and pointing at import is the only signal Terraform can give.
//
// It compares against the planned mappings rather than testing for presence, so that a
// stored set equal to the plan is adopted and only a differing one is refused. That is
// what makes a create retryable after its own confirming read has failed: the tenant is
// then configured with no state to show it, and refusing the retry would leave the
// operator importing the provider's own work. See storedMappingsMatchPlan.
//
// A not-found is treated as nothing configured rather than as a failure. The endpoint
// answers an empty set with a 200 and no results, so this branch should be
// unreachable — but Read already treats a 404 as absence, and having the two disagree
// would mean a create that fails on a tenant a refresh handles fine.
//
// The adoption above is the fallback, not the plan: when the write has landed and only
// the confirming read has failed, state is committed here anyway, so the tenant is not
// left holding mappings Terraform knows nothing about. That is what write's return
// value reports — the diagnostics alone cannot distinguish a write that never happened
// from a read-back that failed after one did.
func (r *HostnameMappingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var plan HostnameMappingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	existing, err := r.client.GetDnsCustomHostnameMappingsV1(createCtx)
	if err != nil && !helpers.IsNotFoundError(err) {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error checking for existing Jamf Security Cloud hostname mappings", helpers.APIErrorDetail(err))
		}
		return
	}

	storedCount := storedMappingCount(existing)
	matchesPlan, matchDiags := storedMappingsMatchPlan(createCtx, plan.Mappings, existing)
	resp.Diagnostics.Append(matchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	switch {
	case storedCount > 0 && matchesPlan:
		tflog.Info(ctx, "the stored Jamf Security Cloud hostname mappings already match the plan, adopting them")
	case storedCount > 0:
		resp.Diagnostics.AddError(
			"Hostname mappings already configured",
			"This tenant already has "+strconv.Itoa(storedCount)+" hostname mapping(s) that differ from this "+
				"configuration, and this resource owns the entire set — creating it here would discard them "+
				"without warning. If they were added in the admin UI or by another tool, import them instead of "+
				"recreating them:\n\n"+
				"    terraform import jamfplatform_security_cloud_dns_hostname_mappings.<name> "+helpers.SingletonID+
				"\n\nIf instead this configuration declares this resource more than once, remove the duplicate "+
				"block: there is one mapping set per tenant, so a second instance can only overwrite the first.",
		)
		return
	}

	written := r.write(createCtx, ctx, &plan, &resp.Diagnostics, "creating")
	if resp.Diagnostics.HasError() {
		if written {
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, hostnameMappingsIdentityModel{ID: plan.ID})...)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		}
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, hostnameMappingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud hostname mappings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the tenant's stored hostname mappings.
//
// An empty mapping set is a 200 with no results rather than a 404, so absence has to
// be recognised from the payload: an empty set means every mapping this resource owned
// is gone, which is the deleted state and drops the resource from state. That differs
// from the sibling search domain resource, whose absence is a 404 — a reason to probe
// each construct rather than inherit either answer.
func (r *HostnameMappingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var state HostnameMappingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := r.client.GetDnsCustomHostnameMappingsV1(readCtx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud hostname mappings not found, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud hostname mappings", helpers.APIErrorDetail(err))
		return
	}

	if len(got.Results) == 0 {
		tflog.Info(ctx, "Jamf Security Cloud hostname mappings are empty, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(assignHostnameMappingsResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, hostnameMappingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update replaces the tenant's hostname mappings.
//
// No preflight here, unlike Create: state already says this configuration owns the
// set, so replacing it wholesale is the intent rather than an accident.
func (r *HostnameMappingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var plan HostnameMappingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	r.write(updateCtx, ctx, &plan, &resp.Diagnostics, "updating")
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, hostnameMappingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Security Cloud hostname mappings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the tenant's hostname mappings.
//
// A real clear, not the no-op STYLE_GUIDE §Singleton resources describes: the endpoint
// honours DELETE and answers 204 whether or not anything was stored, so an
// already-empty set needs no special case. Destroying this resource removes every
// hostname mapping the tenant has.
func (r *HostnameMappingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var state HostnameMappingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if err := r.client.ClearDnsCustomHostnameMappingsV1(deleteCtx); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud hostname mappings already cleared")
			return
		}
		resp.Diagnostics.AddError("Error clearing Jamf Security Cloud hostname mappings", helpers.APIErrorDetail(err))
		return
	}

	tflog.Trace(ctx, "cleared Jamf Security Cloud hostname mappings")
}

// write sends the planned mapping set and reads it back into the model.
//
// Create and Update share this because the endpoint is a full replace and the two
// differ only in Create's preflight: a separate copy in each would be two places for
// the read-back to be forgotten. The read-back is not optional — the server dedupes
// addresses and returns the set in an order of its own, so the plan is not what is
// stored.
//
// callCtx carries the operation timeout; logCtx is the untimed context, so a trace
// line still lands if the call itself times out.
//
// The return value reports whether the replace call landed, not whether everything
// worked: a caller that has to decide between committing partial state and committing
// none cannot tell those apart from the diagnostics. The singleton ID is set as soon
// as the write succeeds, before the read-back, so that partial state carries it — it
// is a provider constant, not a server-assigned value, so there is nothing to wait
// for.
func (r *HostnameMappingsResource) write(callCtx, logCtx context.Context, plan *HostnameMappingsResourceModel, diags *diag.Diagnostics, verb string) bool {
	input, inputDiags := buildMappingsWriteInput(callCtx, plan.Mappings)
	diags.Append(inputDiags...)
	if diags.HasError() {
		return false
	}

	if err := r.client.ReplaceDnsCustomHostnameMappingsV1(callCtx, &input); err != nil {
		if !appendWriteDiagnostics(diags, err) && !appendDuplicateHostnameHint(diags, err) {
			diags.AddError("Error "+verb+" Jamf Security Cloud hostname mappings", helpers.APIErrorDetail(err))
		}
		return false
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	got, err := r.client.GetDnsCustomHostnameMappingsV1(callCtx)
	if err != nil {
		diags.AddError(
			"Error reading the Jamf Security Cloud hostname mappings just written",
			"The mapping set was written to the tenant but could not be read back, so Terraform has recorded the "+
				"configured mappings under the ID \""+helpers.SingletonID+"\" without confirming what was "+
				"stored: Jamf Security Cloud dedupes addresses and returns its own order. The next plan will refresh "+
				"them: there is no need to import them, and nothing has to be re-created. Underlying error: "+
				helpers.APIErrorDetail(err),
		)
		return true
	}

	diags.Append(assignHostnameMappingsResourceModel(logCtx, plan, got)...)
	return true
}
