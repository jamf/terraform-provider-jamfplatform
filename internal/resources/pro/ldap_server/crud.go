// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateLDAPServerByID   (POST id="0")
//   proclassic.GetLDAPServerByID
//   proclassic.UpdateLDAPServerByID
//   proclassic.DeleteLDAPServerByID
//   proclassic.ListLDAPServers         (data source / list resource)
//   proclassic.GetLDAPServerByName     (data source name lookup)
//
// Uniqueness: connection_settings.display_name, enforced (409), case-, leading-space- and trailing-space-insensitive, accent-sensitive. The plan-time collision check lists with proclassic.ListLDAPServers (collision.go).
//
// Status: current. Last reviewed 2026-05-31.

package ldap_server

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// resolvePassword extracts the plaintext bind password from the config model
// (it is WriteOnly, so it is null in the plan). Returns nil when there is no
// account block or no password — the input builder then omits `<password>`.
func resolvePassword(cfg LdapServerResourceModel) *string {
	if cfg.Connection == nil || cfg.Connection.Account == nil {
		return nil
	}
	return helpers.OptionalStringPointer(cfg.Connection.Account.Password)
}

// Create creates a new Jamf Pro LDAP server. Classic POSTs to id="0"; the
// server allocates the integer ID and returns it at the top-level <id>. We
// then GET to capture server-populated defaults (timeouts, use_wildcards,
// echoed mappings).
func (r *LdapServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan LdapServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg LdapServerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	created, err := r.client.CreateLDAPServerByID(createCtx, "0", buildLdapServerInput(plan, resolvePassword(cfg)))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro LDAP server", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing LDAP server ID",
			"Jamf Pro returned 201 Created with no LDAP server ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetLDAPServerByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro LDAP server", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignLdapServerResourceModel(&plan, got, true)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ldapServerIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro LDAP server", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state. Import-time refresh (req.State.Raw null)
// sources the ID from the identity object.
func (r *LdapServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state LdapServerResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this LDAP server without existing state or identity data, so the provider cannot determine which server to read.",
			)
			return
		}
		var identity ldapServerIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing LDAP server ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the LDAP server.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(ldapServerTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro LDAP server without ID.")
		return
	}

	got, err := r.client.GetLDAPServerByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro LDAP server not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ldapServerIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro LDAP server", helpers.APIErrorDetail(err))
		return
	}

	// Gate the (always-superset) server mappings to the declared shape only on a
	// genuine refresh. `connection` is Required, so a created resource always
	// carries it in prior state; an imported resource (id only, via identity or
	// passthrough) does not — there we populate every mapping sub-block the
	// server returns for full-fidelity import. (req.State.Raw.IsNull() is not a
	// reliable import signal: passthrough import leaves a non-null id-only state.)
	gateMappingsToDeclared := state.Connection != nil
	resp.Diagnostics.Append(assignLdapServerResourceModel(&state, got, gateMappingsToDeclared)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ldapServerIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro LDAP server. Classic /ldapservers applies PUT as a
// partial merge: omitted tags preserve the stored value, set tags overwrite.
// The plaintext `<password>` is sent only when password_wo_version changed —
// otherwise it is omitted so the server retains the stored bind password.
func (r *LdapServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LdapServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state LdapServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg LdapServerResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	// Send the plaintext password only when the rotation trigger changed.
	var password *string
	if passwordWoVersionChanged(plan, state) {
		password = resolvePassword(cfg)
	}

	if err := r.client.UpdateLDAPServerByID(updateCtx, plan.ID.ValueString(), buildLdapServerInput(plan, password)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro LDAP server", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetLDAPServerByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro LDAP server", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignLdapServerResourceModel(&plan, got, true)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ldapServerIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro LDAP server.
func (r *LdapServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LdapServerResourceModel
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

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro LDAP server without ID.")
		return
	}

	if err := r.client.DeleteLDAPServerByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro LDAP server already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro LDAP server", helpers.APIErrorDetail(err))
	}
}

// passwordWoVersionChanged reports whether the WriteOnly password rotation
// trigger (connection.account.password_wo_version) differs between plan and
// state. A nil account on either side is treated as version-null.
func passwordWoVersionChanged(plan, state LdapServerResourceModel) bool {
	planVer := accountWoVersion(plan)
	stateVer := accountWoVersion(state)
	return !planVer.Equal(stateVer)
}
