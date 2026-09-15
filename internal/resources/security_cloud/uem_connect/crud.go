// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	securitycloud.CreateUemConnectorV1
//	securitycloud.GetUemConnectorV1
//	securitycloud.UpdateUemConnectorSyncSettingsV1
//	securitycloud.EnableUemConnectorV1
//	securitycloud.DeleteUemConnectorV1
//	securitycloud.ListUemConnectorsV1 (data source)
//
// Status: current. Last reviewed 2026-08-31.
//
// CreateUemConnectorV1 takes a vendor-discriminated union as of spec v1882: the
// body is securitycloud.ConnectorCreateRequestBody and the Jamf Pro variant is
// fully typed rather than the generic request with three fields grafted on from
// the wire. Re-probed on the review date — the OAuth form's nested
// deviceSyncAuth clears field validation and fails at the connection test
// (UEM_CONNECTION_FAILED), so the shape reaches the server intact.
//
// Two SDK methods this resource deliberately does not call, both wire-verified
// redundant on 2026-08-28:
//
//	GetUemConnectorSyncSettingsV1  returns a response byte-identical to
//	                              GetUemConnectorV1, so it is an alias rather
//	                              than a narrower read.
//	DisableUemConnectorV1          is exactly EnableUemConnectorV1 with
//	                              enabled=false, and the PUT is idempotent, so
//	                              one code path serves both states.
//
// Three more on the namespace belong to no construct here, and are recorded so
// their absence reads as a decision rather than an oversight:
//
//	CancelUemConnectorSyncV1       cancelling a run started by the synchronize
//	                              action would need the transaction ID that
//	                              action does not return, and Terraform has no
//	                              shape for "undo the action I just invoked".
//	ListUemConnectorSyncRunsV1     the sync run history, including the device
//	                              counts the connector record omits. A data
//	                              source over it would report values that change
//	                              with no configuration change; the data source's
//	                              latest_sync covers the current run.
//	DeployActivationProfileToUemV1 vulnerability management enrolment, which the
//	                              package doc records as unmanageable — the
//	                              deploy has no read to reconcile against.
//
// The settings write is a full replacement: an omitted field is reset to Jamf's
// default rather than left alone. Create and Update therefore always send the
// complete desired state — see buildSyncSettingsInput.
package uem_connect

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates the Jamf Security Cloud UEM Connect integration.
//
// Three writes, because the API splits the integration across three endpoints:
// the connection is created, then its sync settings are written, then its
// enablement is set. The create endpoint accepts only the connection details.
//
// A failure after the first write leaves a real integration on the tenant, and
// the diagnostics say so: silently reporting "create failed" would send the user
// to re-apply into the one-per-tenant conflict instead of converging. The ID and
// identity are committed to state before the follow-up writes for the same reason,
// which is also what covers a failed read-back at the end: state already holds the
// integration by then, so that path reports what happened rather than orphaning it.
// Values only the trailing read-back can fill are nulled before the early commit —
// see nullUnknownReadBackValues.
func (r *UEMConnectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan UEMConnectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config UEMConnectResourceModel
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

	input, inputDiags := buildConnectorCreateInput(plan, writeOnlyClientSecret(config))
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateUemConnectorV1(createCtx, input)
	if err != nil {
		if !appendCreateDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error creating Jamf Security Cloud UEM Connect integration", helpers.APIErrorDetail(err))
		}
		return
	}

	plan.ID = types.StringValue(created.ID)
	nullUnknownReadBackValues(&plan)
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, uemConnectIdentityModel{ID: plan.ID})...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applySettings(createCtx, &resp.Diagnostics, created.ID, plan, "created") {
		return
	}

	got, err := r.client.GetUemConnectorV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading created Jamf Security Cloud UEM Connect integration",
			"The integration was created with ID \""+created.ID+"\" and its settings applied, but reading it back "+
				"failed, so Terraform has recorded its ID and the configured values without confirming what was "+
				"stored. The next plan will refresh it — do not re-create it: Jamf Security Cloud allows one UEM "+
				"Connect integration per tenant, so a second create would be refused. Underlying error: "+
				helpers.APIErrorDetail(err),
		)
		return
	}
	resp.Diagnostics.Append(assignUEMConnectResourceModel(&plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, uemConnectIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud UEM Connect integration", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// nullUnknownReadBackValues nulls the values only the trailing read-back can fill, so
// that the state committed before it is wholly known.
//
// Terraform answers an unknown value in the state a failed apply returns with an
// "invalid result object after apply" error of its own — a provider-bug notice that
// would bury the diagnostic the early commit exists to deliver. The server address is
// Unknown on the platform_tenant form, where the tenant resolves it. The mapping
// fields are Optional and Computed with no default, and UseNonNullStateForUnknown has
// no prior state to hold on to during a create, so each one the configuration leaves
// out arrives Unknown as well. Both mapping blocks are checked for nil first because
// each is a pointer, absent when the configuration omits it.
func nullUnknownReadBackValues(plan *UEMConnectResourceModel) {
	if plan.UEMServerURL.IsUnknown() {
		plan.UEMServerURL = types.StringNull()
	}
	if mapping := plan.UserDataFieldMapping; mapping != nil {
		if mapping.DeviceName.IsUnknown() {
			mapping.DeviceName = types.StringNull()
		}
		if mapping.UserName.IsUnknown() {
			mapping.UserName = types.StringNull()
		}
		if mapping.UserID.IsUnknown() {
			mapping.UserID = types.StringNull()
		}
		if mapping.PhoneNumber.IsUnknown() {
			mapping.PhoneNumber = types.StringNull()
		}
		if email := mapping.Email; email != nil {
			if email.Source.IsUnknown() {
				email.Source = types.StringNull()
			}
			if email.OnlyIfEmailMissing.IsUnknown() {
				email.OnlyIfEmailMissing = types.BoolNull()
			}
		}
	}
	if group := plan.GroupMembershipMapping; group != nil && group.Enabled.IsUnknown() {
		group.Enabled = types.BoolNull()
	}
}

// Read refreshes Terraform state with the stored integration.
func (r *UEMConnectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state UEMConnectResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this UEM Connect integration without existing state or identity "+
					"data, so the provider cannot determine which integration to read.",
			)
			return
		}
		var identity uemConnectIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing UEM Connect integration ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the "+
					"integration.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(uemConnectTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// An import arrives carrying only the identifier: Terraform writes it in
	// ImportState and then calls Read with nothing else set. req.State.Raw is
	// therefore NOT null on that path, so the isImport flag above cannot be the
	// signal — a Required attribute being null is. Everything else null while the
	// ID is present happens on no other path.
	//
	// It matters because the optional blocks are only populated for a managed
	// block, and on import every block is null. Without lifting the gate an
	// imported integration would come back looking unconfigured.
	importing := isImport || state.UEMVendor.IsNull()

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := r.client.GetUemConnectorV1(readCtx, state.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			tflog.Debug(ctx, "Jamf Security Cloud UEM Connect integration no longer exists; removing from state",
				map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud UEM Connect integration", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignUEMConnectResourceModel(&state, got, importing)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, uemConnectIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the sync settings and enablement.
//
// The connection itself has no update operation, so every attribute describing it
// forces replacement and never reaches here.
func (r *UEMConnectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UEMConnectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state UEMConnectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	updateTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	if !r.applySettings(updateCtx, &resp.Diagnostics, plan.ID.ValueString(), plan, "updated") {
		return
	}

	got, err := r.client.GetUemConnectorV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Security Cloud UEM Connect integration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignUEMConnectResourceModel(&plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Security Cloud UEM Connect integration", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the integration.
//
// Jamf Security Cloud keeps the API role and integration it provisioned on the
// Jamf Pro side, and reuses them if an integration is created against that tenant
// again — so a replace does not leave credentials accumulating there, though it
// does mint a fresh client secret. Devices already synced into Jamf Security Cloud
// are not removed by this.
func (r *UEMConnectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state UEMConnectResourceModel
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

	if err := r.client.DeleteUemConnectorV1(deleteCtx, state.ID.ValueString()); err != nil {
		if isNotFound(err) {
			tflog.Debug(ctx, "Jamf Security Cloud UEM Connect integration already gone",
				map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Security Cloud UEM Connect integration", helpers.APIErrorDetail(err))
		return
	}

	tflog.Trace(ctx, "deleted Jamf Security Cloud UEM Connect integration", map[string]any{"id": state.ID.ValueString()})
}

// applySettings writes the sync settings and then the enablement, shared by Create
// and Update. It reports whether both succeeded.
//
// The order matters on create: a sync triggered by the enablement write should see
// the settings the user asked for rather than Jamf's defaults.
//
// phase names what the caller had just done, so a failure here can say what state
// the tenant is in — "created but not configured" needs a different response from
// the user than "unchanged".
func (r *UEMConnectResource) applySettings(ctx context.Context, diags *diag.Diagnostics, id string, plan UEMConnectResourceModel, phase string) bool {
	settings, settingsDiags := buildSyncSettingsInput(plan)
	diags.Append(settingsDiags...)
	if diags.HasError() {
		return false
	}

	if err := r.client.UpdateUemConnectorSyncSettingsV1(ctx, id, settings); err != nil {
		if !appendUpdateDiagnostics(diags, err) {
			diags.AddError(
				"Error writing Jamf Security Cloud UEM Connect settings",
				"The integration was "+phase+" but its settings could not be written, so it may be running with "+
					"Jamf Security Cloud's defaults. Re-run to converge. Reported: "+helpers.APIErrorDetail(err),
			)
		}
		return false
	}

	enabled := plan.Enabled.ValueBool()
	if err := r.client.EnableUemConnectorV1(ctx, id, &securitycloud.EnablementRequest{Enabled: enabled}); err != nil {
		if !appendUpdateDiagnostics(diags, err) {
			diags.AddError(
				"Error setting Jamf Security Cloud UEM Connect enablement",
				"The integration was "+phase+" and its settings written, but its enabled state could not be set. "+
					"Re-run to converge. Reported: "+helpers.APIErrorDetail(err),
			)
		}
		return false
	}

	return true
}

// writeOnlyClientSecret reads the OAuth client secret out of the configuration.
//
// A WriteOnly attribute is null in the plan by design — that is what keeps it out
// of state — so the value has to come from req.Config, and only on the write that
// sends it.
func writeOnlyClientSecret(config UEMConnectResourceModel) string {
	if config.OAuth == nil || config.OAuth.ClientSecret.IsNull() || config.OAuth.ClientSecret.IsUnknown() {
		return ""
	}
	return config.OAuth.ClientSecret.ValueString()
}
