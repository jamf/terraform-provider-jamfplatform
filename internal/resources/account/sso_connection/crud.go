// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   account.CreateConnection
//   account.GetConnection    (read, create read-back, update read-back)
//   account.ListConnections  (the second half of every read; data sources; list resource)
//   account.UpdateConnection
//   account.DeleteConnection
//
// ListConnections is on the resource's own path, not only the data sources',
// because no single call returns a whole connection: the read of one carries the
// per-provider settings and no products, and the collection read carries the
// products and the consent ticket and no settings. It is also what disambiguates
// a connection Jamf's collection lists but cannot read on its own identifier
// from one that has genuinely gone.
//
// Status: current. Last reviewed 2026-09-02.

package sso_connection

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create makes the connection and reads back the half the create response does
// not carry.
//
// Every attempt is currently refused with an internal failure from Jamf, for
// every payload, in every region — see the package doc. The path is written for
// the fix rather than around the fault, and the diagnostic says whose fault it
// is.
//
// A collection read that fails after the create has succeeded still commits
// state, the same choice dns_zone makes and for a sharper reason here: the
// collection read is a second request, so a rate limit or an expired token is
// enough to lose it, and returning without state would leave a connection nobody
// manages. Jamf does not require connection names to be unique — the same name
// sent twice answers 201 twice — so the next apply would add a second connection
// rather than colliding with the first. What is committed is the plan carrying
// the new identifier and the create response, which settles every Computed
// attribute except the two only the collection carries; those are recorded empty
// and reconciled by the next refresh, and an errored apply does not run
// Terraform's plan-consistency check.
func (r *ConnectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ConnectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, secretDiags := configuredClientSecret(ctx, req.Config)
	resp.Diagnostics.Append(secretDiags...)
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

	request, buildDiags := buildConnectionRequest(createCtx, plan, secret)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateConnection(createCtx, request)
	if err != nil {
		want, identityDiags := connectionIdentityFromPlan(createCtx, plan)
		resp.Diagnostics.Append(identityDiags...)
		orphans, checkErr := r.connectionsCreatedDespite(createCtx, want)
		if len(orphans) > 0 {
			appendOrphanedCreateDiagnostics(&resp.Diagnostics, plan.Name.ValueString(), orphans, err)
			return
		}
		if !appendWriteDiagnostics(&resp.Diagnostics, actionCreate, err) {
			resp.Diagnostics.AddError("Error creating Jamf Account SSO connection", helpers.APIErrorDetail(err))
		}
		if checkErr != nil {
			resp.Diagnostics.AddWarning(
				"Whether a connection was created anyway could not be checked",
				"Jamf Account is known to create a connection even when it reports the create as failed, so "+
					"Terraform read the organization's connection list to look for one — and that read failed "+
					"too. So the failure above is not evidence that nothing was created, and nothing has been "+
					"recorded in state. Check in the Jamf Account console whether a connection named \""+
					plan.Name.ValueString()+"\" exists before applying again: names are not required to be "+
					"unique, so a second apply would add another rather than being refused. Underlying error: "+
					checkErr.Error(),
			)
		}
		return
	}

	plan.ID = types.StringValue(created.ID)

	summary, listErr := r.listSummary(createCtx, created.ID)
	if listErr != nil {
		resp.Diagnostics.Append(assignConnectionResourceModel(&plan, created, nil, false)...)
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, connectionIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Error reading back the created Jamf Account SSO connection",
			"The connection was created with identifier "+created.ID+", but the organization's connection "+
				"list — the only place the enabled products and the consent ticket appear — could not be "+
				"read. Terraform has recorded that identifier and the configured values without confirming "+
				"what Jamf Account stored, and the next plan will refresh it. Do not create it again: Jamf "+
				"does not require connection names to be unique, so a second create would leave two "+
				"connections rather than being refused. Underlying error: "+helpers.APIErrorDetail(listErr),
		)
		return
	}
	if summary == nil {
		appendPartialCollectionDiagnostics(&resp.Diagnostics, created.ID)
	}

	resp.Diagnostics.Append(assignConnectionResourceModel(&plan, created, summary, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, connectionIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Account SSO connection", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest representation of the
// connection.
//
// The identity branch serves the identity-only refresh Terraform performs when
// it holds an identity and no state. Both branches are treated as an adoption
// when `name` is absent, which is what an import leaves: STYLE_GUIDE's usual
// warning that state is not null after an import is why the test is a Required
// attribute rather than the raw state. An adoption takes the stored name and
// fills every settings block; an ordinary refresh does neither, for the reasons
// state_builders.go gives.
//
// A connection built with Microsoft admin consent is refused here, which covers
// both the read after an import and the case of one that grew a consent after
// Terraform adopted it. helpers.go says why it cannot be a managed resource.
func (r *ConnectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ConnectionResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this Jamf Account SSO connection without existing state or identity data, so the provider cannot determine which connection to read.",
			)
			return
		}
		var identity connectionIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !helpers.IsConfiguredValue(identity.ID) || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing connection identifier",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the Jamf Account SSO connection.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(connectionTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if !helpers.IsConfiguredValue(state.ID) || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing connection identifier", "Cannot read a Jamf Account SSO connection without an identifier.")
		return
	}
	id := state.ID.ValueString()
	adopt := state.Name.IsNull() || state.Name.ValueString() == ""

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	found, err := r.client.GetConnection(readCtx, id)
	if err != nil {
		if !helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddError("Error reading Jamf Account SSO connection", helpers.APIErrorDetail(err))
			return
		}
		r.reportMissingConnection(readCtx, ctx, resp, id)
		return
	}

	if appendConsentFlowDiagnostics(&resp.Diagnostics, found) {
		return
	}

	summary, summaryDiags := r.readSummary(readCtx, id)
	resp.Diagnostics.Append(summaryDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(assignConnectionResourceModel(&state, found, summary, adopt)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, connectionIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update replaces the connection's settings.
//
// The whole settings body is sent, because Jamf documents this as a replacement
// rather than a patch — spec-derived, not wire-verified, since the write path is
// refused for every request. Consequently the plan is what is sent, and the
// carry-forward plan modifiers on the optional-and-reported attributes are what
// keep a value the operator left out from being cleared.
//
// A connection built with Microsoft admin consent is refused before any request
// is issued, keyed on the value already in state. That is deterministic and needs
// no guess about how Jamf would answer such a write — a guess that would be
// especially poor here, since every write is currently refused identically
// whatever the reason.
//
// A write that fails is followed by a check for whether the connection is still
// there, and that check has three answers rather than two. Still there means the
// change is merely unconfirmed; gone means the refusal was really a not-found;
// and a check that could not be completed is neither, so it is reported
// alongside the write error rather than passed off as the second — the write
// diagnostic asserts that reading and listing work, which is exactly what a
// failed check disproves.
func (r *ConnectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ConnectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state ConnectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	secret, secretDiags := configuredClientSecret(ctx, req.Config)
	resp.Diagnostics.Append(secretDiags...)
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

	if state.ConsentFlow.ValueBool() {
		appendConsentFlowUpdateDiagnostics(&resp.Diagnostics, state.InternalName.ValueString())
		return
	}

	id := state.ID.ValueString()
	if id == "" {
		resp.Diagnostics.AddError(
			"Missing Jamf Account SSO connection identifier",
			"Terraform has no identifier recorded for this connection, and changing one needs it. Run "+
				"`terraform apply -refresh-only` to populate it, then apply again.",
		)
		return
	}
	plan.ID = state.ID

	request, buildDiags := buildConnectionRequest(updateCtx, plan, secret)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateConnection(updateCtx, id, request)
	if err != nil {
		stillExists, checkErr := r.connectionStillExists(updateCtx, id)
		if stillExists {
			appendUnconfirmedUpdateDiagnostics(&resp.Diagnostics, id, err)
			return
		}
		if !appendWriteDiagnostics(&resp.Diagnostics, actionChange, err) {
			resp.Diagnostics.AddError("Error updating Jamf Account SSO connection", helpers.APIErrorDetail(err))
		}
		if checkErr != nil {
			resp.Diagnostics.AddWarning(
				"Whether the connection still exists could not be checked",
				"Changing connection "+id+" reported a failure, and reading it back to establish whether it is "+
					"still there failed as well — so the refusal above is not evidence that the connection has "+
					"gone, and reading and listing are evidently not working at the moment either. Terraform has "+
					"left the previous values in state. Run `terraform plan -refresh-only` once Jamf Account "+
					"answers again to see what it holds. Underlying error: "+checkErr.Error(),
			)
		}
		return
	}

	summary, summaryDiags := r.readSummary(updateCtx, id)
	resp.Diagnostics.Append(summaryDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(assignConnectionResourceModel(&plan, updated, summary, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, connectionIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Account SSO connection", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the connection.
//
// Removing a connection leaves its domains claimed and verified, which is the
// asymmetry worth knowing: withdrawing a domain shrinks every connection naming
// it, while removing a connection touches no domain. So nothing here has to be
// ordered against the domain resources.
//
// A connection built with Microsoft admin consent is deliberately *not* refused
// here, unlike in Read and Update. Removal works on one, and refusing it would
// leave an operator who adopted one under an earlier provider version unable to
// get rid of it by any means but editing state by hand.
func (r *ConnectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ConnectionResourceModel
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

	id := state.ID.ValueString()
	if id == "" {
		resp.Diagnostics.AddError(
			"Missing Jamf Account SSO connection identifier",
			"Terraform has no identifier recorded for this connection, and removing one needs it. Run "+
				"`terraform apply -refresh-only` to populate it, then destroy again.",
		)
		return
	}

	if err := r.client.DeleteConnection(deleteCtx, id); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Account SSO connection already removed", map[string]any{"id": id})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Account SSO connection", helpers.APIErrorDetail(err))
	}
}

// readSummary returns the collection entry for one connection, and says so when
// there is not one.
//
// Absence is not an error: a connection readable on its own identifier but
// missing from the collection is the mirror image of the disagreement inside
// Jamf that this package guards against, and the read that matters has already
// succeeded, so failing the refresh over the lesser half would be worse. It is
// not silent either. The cost of that absence is that the attributes only the
// collection carries are recorded empty, which in state is indistinguishable
// from a connection that genuinely has none — so the partial read is reported,
// as a warning here in the same way appendGhostConnectionDiagnostics reports the
// disagreement in the other direction as an error.
func (r *ConnectionResource) readSummary(ctx context.Context, id string) (*account.ConnectionSummary, diag.Diagnostics) {
	var diags diag.Diagnostics
	summary, err := r.listSummary(ctx, id)
	if err != nil {
		diags.AddError(
			"Unable to list Jamf Account SSO connections",
			"The connection itself was read, but the organization's connection list — which is the only place "+
				"the enabled products and the consent ticket appear — could not be. Underlying error: "+
				helpers.APIErrorDetail(err),
		)
		return nil, diags
	}
	if summary == nil {
		appendPartialCollectionDiagnostics(&diags, id)
	}
	return summary, diags
}

// listSummary reads the organization's connections and picks out one, handing
// back the read's own error.
//
// It exists so a caller can tell a collection read that failed from a collection
// that simply does not carry the connection: the two are the same value to
// findSummary and mean opposite things to Create, which commits state on the
// first and warns on the second.
func (r *ConnectionResource) listSummary(ctx context.Context, id string) (*account.ConnectionSummary, error) {
	summaries, err := r.client.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	return findSummary(summaries, id), nil
}

// appendPartialCollectionDiagnostics reports a connection that was read on its
// own identifier and that the organization's collection does not carry.
//
// A warning rather than an error, because the connection is there and its
// settings were read: refusing the refresh would strand a resource over the
// lesser half of the read. What it must not do is stay quiet, which would record
// a connection with no products as if Jamf had said it has none.
func appendPartialCollectionDiagnostics(diags *diag.Diagnostics, id string) {
	diags.AddWarning(
		"Jamf Account listed the organization's SSO connections without this one",
		"Connection "+id+" was read on its own identifier, but the organization's connection list does not "+
			"carry it — so the enabled products, which only that list reports, are recorded empty rather than "+
			"as Jamf Account holds them, and the consent ticket is left to whatever the connection's own "+
			"settings carry. That is a disagreement inside Jamf between its own list and the record behind a "+
			"single connection, not a problem with this configuration, and nothing here can fix it. Raise it "+
			"with Jamf Support, quoting the identifier above.",
	)
}

// reportMissingConnection decides what a not-found means.
//
// Two things look identical at the single-connection read and mean opposite
// things. A connection genuinely removed outside Terraform has to be dropped from
// state so the next plan makes it again. A connection Jamf's own collection still
// lists has to be kept, because it exists and planning a fresh create would risk
// a duplicate — that disagreement inside Jamf was observed on a real connection,
// and it is why this branch consults the collection instead of trusting the
// not-found.
func (r *ConnectionResource) reportMissingConnection(readCtx, ctx context.Context, resp *resource.ReadResponse, id string) {
	summaries, err := r.client.ListConnections(readCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to confirm whether the Jamf Account SSO connection still exists",
			"Reading the connection on its identifier reported it missing, and the organization's connection "+
				"list could not be read to confirm that. Terraform has left it in state rather than assuming it "+
				"is gone, because Jamf Account is known to list a connection it cannot read on its own identifier. "+
				"Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}

	if summary := findSummary(summaries, id); summary != nil {
		appendGhostConnectionDiagnostics(&resp.Diagnostics, id, summary.Name)
		return
	}

	tflog.Info(ctx, "Jamf Account SSO connection not found, removing from state", map[string]any{"id": id})
	resp.State.RemoveResource(ctx)
}

// configuredClientSecret reads the write-only client secret out of the
// configuration.
//
// It comes from the configuration rather than the plan because a write-only value
// is stripped from a plan, so the plan would always report it absent — and absent
// is meaningful here: it is how an operator says "keep the stored secret". Only
// the one attribute is read, so an unknown value elsewhere in a nested block
// cannot take the read down with it.
func configuredClientSecret(ctx context.Context, config tfsdk.Config) (types.String, diag.Diagnostics) {
	var secret types.String
	diags := config.GetAttribute(ctx, path.Root("client_secret"), &secret)
	return secret, diags
}

// connectionsCreatedDespite lists connections matching the configured name after a
// create reported an error, so a connection Jamf made anyway is not left
// unmanaged.
//
// Wire-observed 2026-09-02: a POST answering 500 UPSTREAM_ERROR had created the
// connection regardless, and because Jamf appends a random suffix to the stored
// name there is no identifier in the error to look it up by — only the name the
// caller chose. Hence a collection scan.
//
// A read that itself fails hands back its error rather than an empty result, and
// logs it. "Checked, nothing found" and "could not check" mean opposite things to
// an operator deciding whether to apply again, and a caller that could not tell
// them apart would report the second as the first — which is the reading that
// duplicates a connection. Since Jamf allows duplicate names, every match is
// returned and the caller decides — a pre-existing connection of the same name is
// indistinguishable from one this create made, and saying so beats guessing.
func (r *ConnectionResource) connectionsCreatedDespite(ctx context.Context, want connectionIdentity) ([]account.ConnectionSummary, error) {
	if want.name == "" {
		return nil, nil
	}
	all, err := r.client.ListConnections(ctx)
	if err != nil {
		tflog.Warn(ctx, "could not list Jamf Account SSO connections to check whether a failed create took effect anyway", map[string]any{
			"name":  want.name,
			"error": err.Error(),
		})
		return nil, err
	}
	return connectionsMatchingPlan(all, want), nil
}

// connectionStillExists reports whether a connection is present after a change
// reported an error, and whether the question could be answered at all.
//
// A failed update has three outcomes, not two. The connection is gone (deleted
// elsewhere, so the error is really a not-found and the resource should say so);
// or it is still there and Jamf simply will not say whether the change landed; or
// neither read would answer, which is a third thing rather than a negative — the
// change diagnostic asserts that reading and listing work, and an operator told a
// connection was not found when the check itself failed would draw the wrong
// conclusion. So a failed check hands back its error for the caller to report
// alongside the original write error rather than in place of it.
//
// Both reads are logged when they fail, with the exception of a not-found from
// the single read: that is the expected shape of a genuine withdrawal, not
// something gone wrong, so it is recorded at trace level.
func (r *ConnectionResource) connectionStillExists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	_, getErr := r.client.GetConnection(ctx, id)
	if getErr == nil {
		return true, nil
	}
	if helpers.IsNotFoundError(getErr) {
		tflog.Trace(ctx, "Jamf Account SSO connection reported missing on its own identifier after a failed change", map[string]any{"id": id})
	} else {
		tflog.Warn(ctx, "could not read a Jamf Account SSO connection to check whether a failed change left it in place", map[string]any{
			"id":    id,
			"error": getErr.Error(),
		})
	}
	all, err := r.client.ListConnections(ctx)
	if err != nil {
		tflog.Warn(ctx, "could not list Jamf Account SSO connections to check whether a failed change left one in place", map[string]any{
			"id":    id,
			"error": err.Error(),
		})
		return false, err
	}
	for _, candidate := range all {
		if candidate.ID == id {
			return true, nil
		}
	}
	return false, nil
}
