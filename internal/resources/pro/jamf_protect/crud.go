// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.RegisterJamfProtectV1        (POST /v1/jamf-protect/register → 201, full settings body)
//   pro.GetJamfProtectSettingsV1     (GET  /v1/jamf-protect; 404 when unregistered)
//   pro.UpdateJamfProtectSettingsV1  (PUT  /v1/jamf-protect; request carries only autoInstall)
//   pro.UnregisterJamfProtectV1      (DELETE /v1/jamf-protect; idempotent 204)
//   pro.SyncJamfProtectPlansV1       (POST /v1/jamf-protect/plans/sync; fire-and-forget 204)
//   pro.ListJamfProtectPlansV1       (plural plans data source)
//
// Not adopted:
//   pro.ListJamfProtectDeploymentTasksV1 / pro.RetryJamfProtectDeploymentTasksV1
//     — operational deployment-task plumbing, not a declarative configuration surface.
//   pro.ListJamfProtectHistoryV1 / pro.CreateJamfProtectHistoryNoteV1
//     — object history, convention-wide exclusion.
//
// Status: current. Last reviewed 2026-06-10.
//
// The register POST response is byte-identical to a subsequent GET, so Create
// needs no GET-after-write. POSTing over an existing registration overwrites
// it in place (new registrationId, autoInstall preserved) and a failed
// credential check leaves the old registration intact, which makes the
// in-place re-register on Update safe. Create and Update both finish with a
// fire-and-forget plans sync; a sync failure is a WARNING (the registration
// itself succeeded), never an error.

package jamf_protect

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// syncPlans fires the fire-and-forget plans sync that follows every
// successful register / settings write. A failure is reported as a warning —
// the registration or update itself already succeeded, and the catalog can be
// re-synced from the Jamf Pro UI (Sync Plans) or on the next apply.
func (r *JamfProtectResource) syncPlans(ctx context.Context, diags *diag.Diagnostics) {
	if err := r.client.SyncJamfProtectPlansV1(ctx); err != nil {
		diags.AddWarning(
			"Jamf Protect plans sync failed",
			"The Jamf Protect registration succeeded, but the follow-up plans sync returned an error. "+
				"Re-run `terraform apply`, or trigger Sync Plans from Settings → Jamf apps → Jamf Protect in the Jamf Pro UI. "+
				"Original error: "+err.Error(),
		)
		return
	}
	tflog.Trace(ctx, "synced Jamf Protect plans")
}

// Create registers Jamf Pro with the Jamf Protect instance. The 201 response
// body is authoritative state (byte-identical to a GET). When the plan pins
// auto_install to a value the register response does not already carry, a
// follow-up PUT applies it. Create finishes with a fire-and-forget plans sync.
func (r *JamfProtectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, config JamfProtectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
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

	got, err := r.client.RegisterJamfProtectV1(createCtx, buildJamfProtectRegistrationInput(plan, config))
	if err != nil {
		resp.Diagnostics.AddError("Error registering Jamf Protect", helpers.APIErrorDetail(err))
		return
	}

	// auto_install is Optional+Computed: when configured and differing from
	// the register response (the server preserves a prior value on overwrite,
	// false on a fresh register), assert it with the one PUT-mutable field.
	if want := helpers.OptionalBoolPointer(plan.AutoInstall); want != nil && *want != got.AutoInstall {
		got, err = r.client.UpdateJamfProtectSettingsV1(createCtx, buildJamfProtectSettingsInput(want))
		if err != nil {
			resp.Diagnostics.AddError("Error updating Jamf Protect auto_install after registering", helpers.APIErrorDetail(err))
			return
		}
	}

	r.syncPlans(createCtx, &resp.Diagnostics)

	assignJamfProtectResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfProtectIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "registered Jamf Protect")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state from the API. A 404 means the tenant is not registered
// (unregistered out of band or never registered), so the resource is removed
// from state.
func (r *JamfProtectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state JamfProtectResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(jamfProtectTimeoutAttributeTypes)
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

	got, err := r.client.GetJamfProtectSettingsV1(readCtx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Trace(ctx, "Jamf Protect not registered; removing from state")
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Protect registration", helpers.APIErrorDetail(err))
		return
	}

	assignJamfProtectResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfProtectIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update re-registers in place when a registration field changed (api_url,
// client_id, or a password_wo_version bump) — the server overwrites the
// existing registration without unregistering first, and a failed credential
// check leaves the old registration intact. The re-register response is
// authoritative; auto_install is PUT afterwards only when the preserved value
// differs from the plan (mirrors Create). On a settings-only change the PUT
// alone runs. Both paths finish with a fire-and-forget plans sync.
func (r *JamfProtectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, state, config JamfProtectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
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

	reRegister := !plan.APIURL.Equal(state.APIURL) ||
		!plan.ClientID.Equal(state.ClientID) ||
		!plan.PasswordWoVersion.Equal(state.PasswordWoVersion)

	var got *pro.ProtectSettingsResponse
	if reRegister {
		var err error
		got, err = r.client.RegisterJamfProtectV1(updateCtx, buildJamfProtectRegistrationInput(plan, config))
		if err != nil {
			resp.Diagnostics.AddError("Error re-registering Jamf Protect", helpers.APIErrorDetail(err))
			return
		}
		tflog.Trace(ctx, "re-registered Jamf Protect in place")
	}

	want := helpers.OptionalBoolPointer(plan.AutoInstall)
	if want == nil {
		// auto_install unresolved in the plan (cannot normally happen —
		// UseStateForUnknown fills it from prior state on update); carry the
		// prior state value.
		current := state.AutoInstall.ValueBool()
		want = &current
	}
	// PUT auto_install when it is what changed (settings-only path) or when
	// the value the re-register preserved differs from the plan — mirrors
	// Create's conditional PUT.
	if got == nil || *want != got.AutoInstall {
		updated, err := r.client.UpdateJamfProtectSettingsV1(updateCtx, buildJamfProtectSettingsInput(want))
		if err != nil {
			resp.Diagnostics.AddError("Error updating Jamf Protect settings", helpers.APIErrorDetail(err))
			return
		}
		got = updated
	}

	r.syncPlans(updateCtx, &resp.Diagnostics)

	assignJamfProtectResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfProtectIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Protect registration")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete unregisters Jamf Pro from the Jamf Protect instance. The DELETE is
// idempotent (204 even when already unregistered); a 404 is tolerated for the
// same reason. Configuration profiles already created from Protect plans
// remain in Jamf Pro, and the synced plans catalog persists.
func (r *JamfProtectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state JamfProtectResourceModel
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

	if err := r.client.UnregisterJamfProtectV1(deleteCtx); err != nil {
		// An already-absent registration is the delete's objective.
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Protect already unregistered")
			return
		}
		resp.Diagnostics.AddError("Error unregistering Jamf Protect", helpers.APIErrorDetail(err))
		return
	}
	tflog.Trace(ctx, "unregistered Jamf Protect")
}
