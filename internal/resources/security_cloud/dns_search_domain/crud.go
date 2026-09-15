// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.GetDnsSearchDomainV1
//   securitycloud.SetDnsSearchDomainV1
//   securitycloud.ClearDnsSearchDomainV1
//
// Status: current. Last reviewed 2026-08-29.

package dns_search_domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// providerNotConfiguredError is the diagnostic every handler raises when the client
// is missing, kept in one place so the three wordings cannot drift apart.
func providerNotConfiguredError() (string, string) {
	return "Provider not configured",
		"The provider client was not configured. Please ensure the provider block is set up correctly."
}

// Create sets the tenant's search domain, refusing to overwrite a different one.
//
// The preflight read is the whole point. The write is an unconditional upsert that
// reports no conflict, so without reading first a plan that looks like a create
// would silently replace a search domain an administrator set by hand, and two
// Terraform configurations both managing this resource would overwrite each other in
// silence. Refusing, and naming both causes, is the only signal Terraform can give.
//
// A stored value equal to the planned one is adopted rather than refused, because
// the likeliest way to reach it is this handler's own interrupted write. Create
// makes three calls, and the confirming read is the one that can fail after the
// write has already landed — a timeout, a transient 5xx, a dropped connection —
// leaving the tenant configured and Terraform holding no state. The only move left
// is to apply again, which re-enters Create; a preflight testing presence alone
// would refuse the provider's own write and blame an administrator for it. Matching
// on the value keeps the refusal for a genuine conflict and lets a byte-identical
// retry finish.
//
// The adoption above is the fallback, not the plan: a confirming read that fails after
// the write has landed now commits state itself, so the tenant is not left configured
// with Terraform holding nothing. See commitPartialState.
func (r *SearchDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var plan SearchDomainResourceModel
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

	existing, err := r.client.GetDnsSearchDomainV1(createCtx)
	switch {
	case err == nil && existing != nil && existing.Suffix == plan.DomainName.ValueString():
		tflog.Info(ctx, "the Jamf Security Cloud search domain already holds the planned value, adopting it")
	case err == nil && existing != nil && existing.Suffix != "":
		resp.Diagnostics.AddError(
			"Search domain already configured",
			"This tenant already has the search domain \""+existing.Suffix+"\" set, and Jamf Security Cloud "+
				"allows only one. Creating it here would replace that value without warning.\n\n"+
				"If an administrator set that search domain and you want Terraform to manage it, import it:\n\n"+
				"    terraform import jamfplatform_security_cloud_dns_search_domain.<name> "+helpers.SingletonID+
				"\n\nIf your configuration declares this resource more than once, the second block is the cause "+
				"instead, and importing is the wrong answer: there is one search domain per tenant, so remove the "+
				"duplicate and keep a single block.",
		)
		return
	case err != nil && !helpers.IsNotFoundError(err):
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error checking for an existing Jamf Security Cloud search domain", helpers.APIErrorDetail(err))
		}
		return
	}

	if err := r.client.SetDnsSearchDomainV1(createCtx, &securitycloud.SearchDomain{Suffix: plan.DomainName.ValueString()}); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error setting Jamf Security Cloud search domain", helpers.APIErrorDetail(err))
		}
		return
	}

	got, err := r.client.GetDnsSearchDomainV1(createCtx)
	if err != nil {
		commitPartialState(ctx, &plan, resp)
		resp.Diagnostics.AddError(
			"Error reading the Jamf Security Cloud search domain just written",
			"The search domain \""+plan.DomainName.ValueString()+"\" was written to the tenant but could not be "+
				"read back, so Terraform has recorded it under the ID \""+helpers.SingletonID+"\" without "+
				"confirming the stored value. The next plan will refresh it — there is no need to import it, and "+
				"nothing has to be re-created. Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}
	if got == nil {
		commitPartialState(ctx, &plan, resp)
		resp.Diagnostics.AddError(
			"Jamf Security Cloud returned no search domain after writing one",
			"The write succeeded but the confirming read returned an empty response, so the stored value cannot "+
				"be confirmed. Terraform has recorded the planned search domain \""+plan.DomainName.ValueString()+
				"\" under the ID \""+helpers.SingletonID+"\", so the tenant is not left configured without "+
				"Terraform tracking it — the next plan will refresh it, and there is no need to import it.",
		)
		return
	}
	assignSearchDomainResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, searchDomainIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "set Jamf Security Cloud search domain")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// commitPartialState records the planned search domain under the singleton ID when
// the write has landed but the confirming read has not.
//
// Without it a failed read-back leaves the tenant configured and Terraform holding no
// state, which is the case Create's preflight has to forgive on the next apply. The
// value recorded is the planned one rather than the stored one, and that is the point:
// the two are only in doubt because the read failed, and the next refresh settles it.
// Nothing needs nulling for Terraform's benefit — the ID set here is this schema's
// only Computed value.
func commitPartialState(ctx context.Context, plan *SearchDomainResourceModel, resp *resource.CreateResponse) {
	plan.ID = types.StringValue(helpers.SingletonID)
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, searchDomainIdentityModel{ID: plan.ID})...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes Terraform state with the tenant's stored search domain.
//
// An unset search domain answers 404 with SEARCH_DOMAIN_NOT_SET rather than an
// empty 200, so absence is unambiguous and the resource is dropped from state — the
// ordinary contract for a deleted object, and the divergence from
// STYLE_GUIDE §Singleton resources described in the package doc comment.
//
// A successful read carrying no value is treated as the same absence. That is
// defence rather than an observed shape, and it keeps the three handlers agreeing:
// Create's preflight already reads an empty value as nothing configured, so a Read
// committing "" instead would leave the two disagreeing about whether the tenant has
// a search domain at all.
func (r *SearchDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var state SearchDomainResourceModel
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

	got, err := r.client.GetDnsSearchDomainV1(readCtx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud search domain not set, removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud search domain", helpers.APIErrorDetail(err))
		return
	}

	if got == nil || got.Suffix == "" {
		tflog.Info(ctx, "Jamf Security Cloud search domain not set, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	assignSearchDomainResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, searchDomainIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update replaces the tenant's search domain.
//
// No preflight here, unlike Create: state already says this configuration owns the
// value, so overwriting it is the intent rather than an accident.
func (r *SearchDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var plan SearchDomainResourceModel
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

	if err := r.client.SetDnsSearchDomainV1(updateCtx, &securitycloud.SearchDomain{Suffix: plan.DomainName.ValueString()}); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error updating Jamf Security Cloud search domain", helpers.APIErrorDetail(err))
		}
		return
	}

	got, err := r.client.GetDnsSearchDomainV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading the Jamf Security Cloud search domain just written", helpers.APIErrorDetail(err))
		return
	}
	if got == nil {
		resp.Diagnostics.AddError(
			"Jamf Security Cloud returned no search domain after writing one",
			"The write succeeded but the confirming read returned an empty response, so the stored value "+
				"cannot be recorded in state. The search domain may now be set on the tenant without Terraform "+
				"tracking it — import it to reconcile:\n\n"+
				"    terraform import jamfplatform_security_cloud_dns_search_domain.<name> "+helpers.SingletonID,
		)
		return
	}
	assignSearchDomainResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, searchDomainIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Security Cloud search domain")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete clears the tenant's search domain.
//
// A real clear, not the no-op STYLE_GUIDE §Singleton resources describes: the
// endpoint honours the clear and answers 204 whether or not anything was set.
// Destroying this resource removes the search domain for the whole tenant.
//
// Because 204 is the answer either way, a 404 on this path is unexpected, and the
// only one tolerated is the documented SEARCH_DOMAIN_NOT_SET. Every other 404 —
// the gateway's own bare "page not found" for a route it does not serve, say —
// carries no such code and must surface: reading it as "already cleared" would drop
// the resource from state while the search domain stayed live on the tenant.
func (r *SearchDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var state SearchDomainResourceModel
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

	if err := r.client.ClearDnsSearchDomainV1(deleteCtx); err != nil {
		if isSearchDomainNotSet(err) {
			tflog.Info(ctx, "Jamf Security Cloud search domain reported as not set while clearing it", map[string]any{
				"api_error": err.Error(),
			})
			return
		}
		resp.Diagnostics.AddError("Error clearing Jamf Security Cloud search domain", helpers.APIErrorDetail(err))
		return
	}

	tflog.Trace(ctx, "cleared Jamf Security Cloud search domain")
}
