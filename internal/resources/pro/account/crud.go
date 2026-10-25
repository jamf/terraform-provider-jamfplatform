// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateAccountV1               (POST /accounts; sets accountType incl. FEDERATED)
//   pro.GetAccountV1                  (base-field read)
//   pro.UpdateAccountV1               (PUT base fields)
//   pro.DeleteAccountV1
//   pro.ResolveAccountV1IDByName      (data source username lookup)
//   proclassic.GetAccountByUserID     (Custom privilege grid read; live grid before every privilege write)
//   proclassic.UpdateAccountByUserID  (Custom privilege grid write; a sent <privileges> replaces the whole grid — merged client-side)
//   proclassic.ListAccounts           (privilege-catalog discovery, ModifyPlan)
//
// Uniqueness: username and email, each enforced (400 INVALID_REQUEST_PARAMETER_VALUE "Duplicated user account"), case-, trailing-space- and accent-insensitive, a leading space significant. The plan-time collision check lists with pro.ListAccountsV1 (collision.go).
//
// Status: current. Last reviewed 2026-09-06.

package account

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a Jamf Pro admin account via the Pro API, then (for a Custom +
// Full Access account) writes the privilege grid via the classic API through
// writeCustomPrivileges, which merges against whatever grid the new account
// already carries. If the classic step fails, the just-created account is
// deleted so no half-built account is left behind. Privileges are trusted from
// the plan (not re-read).
func (r *AccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg AccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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

	created, err := r.proClient.CreateAccountV1(createCtx, buildProUserAccount(plan, helpers.OptionalStringPointer(cfg.Password)))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro account", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError("Create response missing account ID", "Jamf Pro returned success with no account ID; cannot persist state.")
		return
	}
	plan.ID = helpers.StringPointerValueOrNull(created.ID)
	id := plan.ID.ValueString()

	if custPrivApplicable(plan.PrivilegeSet, plan.AccessLevel) {
		d := r.writeCustomPrivileges(createCtx, id, plan, "Error writing Jamf Pro account privileges")
		if d.HasError() {
			if delErr := r.proClient.DeleteAccountV1(createCtx, id); delErr != nil {
				tflog.Error(ctx, "failed to roll back account after privilege write failure", map[string]any{"id": id, "delete_error": delErr.Error()})
			}
			resp.Diagnostics.Append(d...)
			return
		}
		resp.Diagnostics.Append(d...)
	}

	got, err := r.proClient.GetAccountV1(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro account", helpers.APIErrorDetail(err))
		return
	}
	assignProBaseFields(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accountIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Trace(ctx, "created Jamf Pro account", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state: base fields from Pro, the Custom privilege grid from
// classic (intersect-on-read) when the account is Custom + Full Access.
func (r *AccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AccountResourceModel
	stateAbsent := req.State.Raw.IsNull()

	if stateAbsent {
		if req.Identity == nil {
			resp.Diagnostics.AddError("Missing resource identity", "Terraform requested a refresh without existing state or identity data.")
			return
		}
		var identity accountIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError("Missing account ID", "The resource identity did not include an 'id' attribute.")
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(accountTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	priorUsername := types.StringNull()
	if !stateAbsent {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("username"), &priorUsername)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	hydrating := importHydration(stateAbsent, priorUsername)

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro account without ID.")
		return
	}
	id := state.ID.ValueString()

	got, err := r.proClient.GetAccountV1(readCtx, id)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro account not found, removing from state", map[string]any{"id": id})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accountIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro account", helpers.APIErrorDetail(err))
		return
	}
	assignProBaseFields(&state, got)

	if custPrivApplicable(state.PrivilegeSet, state.AccessLevel) {
		classicGot, err := r.classicClient.GetAccountByUserID(readCtx, id)
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro account privileges", helpers.APIErrorDetail(err))
			return
		}
		resp.Diagnostics.Append(assignClassicPrivileges(readCtx, &state, classicGot, hydrating)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		state.Privileges = nil
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accountIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies base-field changes via Pro PUT and Custom privilege changes via
// the classic API through writeCustomPrivileges. Each side is called only when
// it has something to send, so a privilege-only edit skips the Pro PUT and a
// plan with no declared privilege category skips the classic write.
func (r *AccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg AccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
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

	id := plan.ID.ValueString()
	passwordRotated := !plan.PasswordWOVersion.Equal(state.PasswordWOVersion)

	if baseFieldsChanged(plan, state) || passwordRotated {
		var password *string
		if passwordRotated {
			password = helpers.OptionalStringPointer(cfg.Password)
		}
		if _, err := r.proClient.UpdateAccountV1(updateCtx, id, buildProUserAccount(plan, password)); err != nil {
			resp.Diagnostics.AddError("Error updating Jamf Pro account", helpers.APIErrorDetail(err))
			return
		}
	}

	if custPrivApplicable(plan.PrivilegeSet, plan.AccessLevel) {
		resp.Diagnostics.Append(r.writeCustomPrivileges(updateCtx, id, plan, "Error updating Jamf Pro account privileges")...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	got, err := r.proClient.GetAccountV1(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro account", helpers.APIErrorDetail(err))
		return
	}
	assignProBaseFields(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accountIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro account via the Pro API.
func (r *AccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AccountResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro account without ID.")
		return
	}

	if err := r.proClient.DeleteAccountV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro account already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro account", helpers.APIErrorDetail(err))
	}
}

// writeCustomPrivileges sends the Custom privilege grid for account id through
// the classic API as a read-merge-write: the live grid is read first and every
// category the plan declares replaces the server's, while undeclared categories
// are re-sent as the server holds them. That is what makes "omit a category and
// the server keeps it" true — the endpoint replaces the whole grid on any sent
// <privileges> (wire-probed 2026-09-06, Jamf Pro 11.31.1, issue #385), so a
// partial body would empty the rest. Nothing is sent, and nothing read, when the
// plan declares no category at all; the server then keeps its grid untouched.
// errSummary is the diagnostic summary for a failed write, so Create and Update
// read distinctly in the output.
func (r *AccountResource) writeCustomPrivileges(ctx context.Context, id string, plan AccountResourceModel, errSummary string) diag.Diagnostics {
	var diags diag.Diagnostics
	if !managesPrivileges(plan) {
		return diags
	}
	live, err := r.classicClient.GetAccountByUserID(ctx, id)
	if err != nil {
		diags.AddError("Error reading Jamf Pro account privileges before write", helpers.APIErrorDetail(err))
		return diags
	}
	classicInput, d := buildClassicPrivileges(ctx, plan, live)
	diags.Append(d...)
	if diags.HasError() || classicInput == nil {
		return diags
	}
	if err := r.classicClient.UpdateAccountByUserID(ctx, id, classicInput); err != nil {
		diags.AddError(errSummary, helpers.APIErrorDetail(err))
	}
	return diags
}

// baseFieldsChanged reports whether any Pro-owned base field differs between
// plan and state (account_type is RequiresReplace, so it is not compared here).
func baseFieldsChanged(plan, state AccountResourceModel) bool {
	return !plan.Username.Equal(state.Username) ||
		!plan.FullName.Equal(state.FullName) ||
		!plan.EmailAddress.Equal(state.EmailAddress) ||
		!plan.AccessLevel.Equal(state.AccessLevel) ||
		!plan.PrivilegeSet.Equal(state.PrivilegeSet) ||
		!plan.AccessStatus.Equal(state.AccessStatus) ||
		!plan.LdapServerID.Equal(state.LdapServerID) ||
		!plan.SiteID.Equal(state.SiteID) ||
		!plan.ForcePasswordChange.Equal(state.ForcePasswordChange)
}
