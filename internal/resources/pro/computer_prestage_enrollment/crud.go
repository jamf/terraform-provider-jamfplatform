// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateComputerPrestageV3
//   pro.GetComputerPrestageV3
//   pro.UpdateComputerPrestageV3                          (subject to upstream Jamf server PUT-500 bug — handled by isPutSerializerBug + diffPlanAgainstGet workaround)
//   pro.DeleteComputerPrestageV3
//   pro.ResolveComputerPrestageV3ByName                   (data source name lookup)
//   pro.ResolveComputerPrestageV3IDByName                 (data source name lookup ID-only path)
//   pro.ListComputerPrestagesV3                           (list resource + data source name lookup paging)
//   pro.GetComputerPrestageScopeV2                        (folded scope_serial_numbers read)
//   pro.ReplaceComputerPrestageScopeV2                    (folded scope_serial_numbers write)
//
// All endpoint families gated on `profileUuid != ""` via
// `waitForPrestageReady` after Create / before Update. Wire-probe
// 2026-05-28 confirmed that scope reads / writes (and the per-prestage
// PUT update itself) return the bug-shaped `HTTP 500 + empty errors[]`
// — without committing — until Jamf finishes its async post-Create
// setup of the prestage (visible in the admin UI as "information out
// of date"). After the wait, the documented behaviour returns:
// 200 on success, 400 ALREADY_SCOPED / DEVICE_DOES_NOT_EXIST_ON_TOKEN
// for the documented error paths, 409 OPTIMISTIC_LOCK_FAILED on stale
// versionLock.
//
// Status: current. Last reviewed 2026-05-28.

package computer_prestage_enrollment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions a new computer prestage enrollment on Jamf Pro.
func (r *ComputerPrestageEnrollmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg ComputerPrestageEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
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

	post, d := buildPostInput(createCtx, plan, cfg)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	postResp, err := r.client.CreateComputerPrestageV3(createCtx, post)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro computer prestage enrollment", helpers.APIErrorDetail(err))
		return
	}
	if postResp == nil || postResp.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing prestage ID",
			"Jamf Pro returned success on the POST but did not include an ID; cannot persist state.",
		)
		return
	}
	id := postResp.ID
	plan.ID = types.StringValue(id)

	// Refresh via GET (server-canonical values).
	got, err := r.client.GetComputerPrestageV3(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro computer prestage enrollment after create", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignGetToResource(createCtx, &plan, plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Wait for Jamf Pro to finish async setup of the prestage (profile_uuid
	// populates). Scope endpoints + further PUT updates return the
	// bug-shaped HTTP 500 with empty errors[] until this point.
	if d := waitForPrestageReady(createCtx, r.client, id); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}

	// Apply scope if user supplied any serial numbers.
	if !plan.ScopeSerialNumbers.IsNull() && !plan.ScopeSerialNumbers.IsUnknown() {
		if d := applyScope(createCtx, r.client, id, plan.ScopeSerialNumbers); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
	}
	scope, err := r.client.GetComputerPrestageScopeV2(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage scope after create", helpers.APIErrorDetail(err))
		return
	}
	plan.ScopeSerialNumbers = scopeSerialsToSet(scope)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ComputerPrestageEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro computer prestage enrollment", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state from the live Jamf Pro record.
func (r *ComputerPrestageEnrollmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ComputerPrestageEnrollmentResourceModel
	isImport := req.State.Raw.IsNull()
	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this prestage without existing state or identity data; cannot determine which record to read.",
			)
			return
		}
		var identity ComputerPrestageEnrollmentIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError("Missing prestage ID", "Identity 'id' attribute is empty.")
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes)
		seedImportNestedSentinels(&state)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// `ImportStatePassthroughID` stores the ID in the new state and
		// invokes Read with that state — so `req.State.Raw.IsNull()` is
		// FALSE on the post-import Read and the explicit isImport branch
		// above never fires here. Use a Computed-only attribute as the
		// signal instead: `site_id` is always populated by Create from
		// the GET response, so a null value with the ID set can only
		// mean "this is the first Read after ImportStatePassthroughID".
		// In that case seed sentinel empty nested-block pointers so the
		// state-builder rebuilds them — the user's intent for which
		// blocks to manage cannot be inferred yet, and showing nothing
		// would diff against the post-Create state under
		// ImportStateVerify.
		if state.SiteID.IsNull() {
			seedImportNestedSentinels(&state)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro computer prestage enrollment without ID.")
		return
	}

	got, err := r.client.GetComputerPrestageV3(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro computer prestage enrollment not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro computer prestage enrollment", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignGetToResource(readCtx, &state, state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope, err := r.client.GetComputerPrestageScopeV2(readCtx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage scope", helpers.APIErrorDetail(err))
		return
	}
	state.ScopeSerialNumbers = scopeSerialsToSet(scope)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ComputerPrestageEnrollmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies plan against state. PUT is full-replace per wire-probe F3.
// The Jamf server-side response-serializer bug (F4) returns HTTP 500 with an
// empty errors[] body even on successful writes; this function catches that
// specific signature, GETs the record, and verifies via diffPlanAgainstGet
// that every plan field round-tripped.
func (r *ComputerPrestageEnrollmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg ComputerPrestageEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
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
	if id == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update without an ID.")
		return
	}

	// Wait for the prestage to be scope-ready before any PUT. Without
	// this, Update PUTs against a prestage whose async post-Create
	// setup is still in flight return `HTTP 500 + empty errors[]`.
	if d := waitForPrestageReady(updateCtx, r.client, id); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}

	// Pre-PUT GET to source versionLocks.
	preGet, err := r.client.GetComputerPrestageV3(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage before update", helpers.APIErrorDetail(err))
		return
	}

	rotateRecovery := recoveryLockPasswordWoBumped(plan, state)
	rotateAdmin := adminPasswordWoBumped(plan, state)

	put, d := buildPutInput(updateCtx, plan, cfg, preGet, rotateAdmin, rotateRecovery)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	injectVersionLocks(put, preGet)

	_, putErr := r.client.UpdateComputerPrestageV3(updateCtx, id, put)
	putHitServerBug := false
	if putErr != nil {
		if !isPutSerializerBug(putErr) {
			resp.Diagnostics.AddError("Error updating Jamf Pro computer prestage enrollment", helpers.APIErrorDetail(putErr))
			return
		}
		putHitServerBug = true
	}

	// GET-diff to verify the write actually committed (handles both the
	// 500-with-commit and 500-with-silent-rollback flavours of F4b).
	postGet, err := r.client.GetComputerPrestageV3(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage after update", helpers.APIErrorDetail(err))
		return
	}
	if unchanged := diffPlanAgainstGet(updateCtx, plan, postGet); len(unchanged) > 0 {
		if putHitServerBug {
			resp.Diagnostics.AddError(
				"Jamf Pro computer prestage update did not commit",
				fmt.Sprintf("The Jamf Pro PUT endpoint returned HTTP 500 (a known Jamf Pro defect) and the verifying GET shows the write was silently rolled back. %s — most often this is a server-side validation failure on `anchor_certificates` (Jamf Pro validates PEM content). Fix the offending input and re-run `terraform apply`.", fmtUnchangedFields(unchanged)),
			)
			return
		}
		// PUT returned success but state still diverges — schema bug,
		// surface for investigation.
		resp.Diagnostics.AddWarning(
			"Jamf Pro computer prestage update partially applied",
			fmt.Sprintf("PUT returned 200 but a subsequent GET shows the following fields did not round-trip: %s", fmtUnchangedFields(unchanged)),
		)
	} else if putHitServerBug {
		// 500-with-commit. Log a warning so the user (and CI logs)
		// know the upstream Jamf bug fired; the write itself succeeded.
		tflog.Warn(updateCtx, putWorkaroundWarning, map[string]any{"id": id})
	}

	resp.Diagnostics.Append(assignGetToResource(updateCtx, &plan, state, postGet)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Scope reconciliation: PUT replaces the entire serial-number set if
	// the plan differs from the prior state.
	scopeApplyDiags := diag.Diagnostics{}
	if !setsEqual(plan.ScopeSerialNumbers, state.ScopeSerialNumbers) {
		scopeApplyDiags = applyScope(updateCtx, r.client, id, plan.ScopeSerialNumbers)
	}

	// Always re-read scope after the apply attempt (whether or not it
	// errored). Persisting the server's actual scope into plan before
	// returning keeps Terraform's consistency check happy when the
	// scope write failed — otherwise the framework masks our
	// user-facing diagnostic with "Provider produced inconsistent
	// result after apply".
	scope, err := r.client.GetComputerPrestageScopeV2(updateCtx, id)
	if err != nil {
		resp.Diagnostics.Append(scopeApplyDiags...)
		resp.Diagnostics.AddError("Error reading prestage scope after update", helpers.APIErrorDetail(err))
		return
	}
	plan.ScopeSerialNumbers = scopeSerialsToSet(scope)

	if scopeApplyDiags.HasError() {
		resp.Diagnostics.Append(scopeApplyDiags...)
		// Persist the new (parent-updated, scope-unchanged) state so the
		// framework sees a coherent post-apply value alongside our
		// error. Without this resp.State.Set, the framework adds its
		// own "planned X does not correlate with actual" diagnostic
		// that hides the real failure.
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ComputerPrestageEnrollmentIdentityModel{ID: plan.ID})...)
		_ = resp.State.Set(ctx, &plan)
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ComputerPrestageEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the prestage. Server cascade removes the scope assignments.
func (r *ComputerPrestageEnrollmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ComputerPrestageEnrollmentResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete without ID.")
		return
	}

	if err := r.client.DeleteComputerPrestageV3(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro computer prestage enrollment already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro computer prestage enrollment", helpers.APIErrorDetail(err))
	}
}

// waitForPrestageReady blocks until the prestage's server-assigned
// `profileUuid` is non-empty. Verified wire-probe 2026-05-28: immediately
// after POST or after a PUT update the prestage GET returns
// `profileUuid: ""` for several seconds (occasionally tens of seconds
// under load), during which the scope endpoints (and other PUT updates)
// return the bug-shaped `HTTP 500 + {"httpStatus":500,"errors":[]}`
// instead of a documented response. Once `profileUuid` populates, the
// same endpoints return the proper 200/400/409 responses.
//
// The Jamf admin UI surfaces this as a transient "information out of
// date" banner on the prestage; there is no API-level sync-state
// endpoint for computer prestages (mobile_device_prestages has one but
// computer_prestages does not — verified against pro_api.json). The
// provider uses `profileUuid != ""` as the only available readiness
// signal.
func waitForPrestageReady(ctx context.Context, client *pro.Client, id string) diag.Diagnostics {
	var diags diag.Diagnostics
	ticker := time.NewTicker(prestageReadyPollInterval)
	defer ticker.Stop()

	for {
		got, err := client.GetComputerPrestageV3(ctx, id)
		if err != nil {
			diags.AddError(
				"Error polling Jamf Pro computer prestage readiness",
				fmt.Sprintf("Could not fetch prestage %s: %s", id, helpers.APIErrorDetail(err)),
			)
			return diags
		}
		if got != nil && got.ProfileUUID != "" {
			tflog.Trace(ctx, "Jamf Pro computer prestage ready", map[string]any{
				"id":           id,
				"profile_uuid": got.ProfileUUID,
			})
			return diags
		}
		select {
		case <-ctx.Done():
			diags.AddError(
				"Jamf Pro computer prestage readiness wait timed out",
				fmt.Sprintf("Prestage %s did not populate `profileUuid` before the operation timeout fired. The admin UI shows this state as an \"information out of date\" banner. Increase `timeouts.create` (or `timeouts.update`) on the resource if this consistently times out on this tenant.", id),
			)
			return diags
		case <-ticker.C:
		}
	}
}

// seedImportNestedSentinels initialises empty model pointers for every
// Optional-only typed-pointer nested block. Used by the import code path so
// the state-builder rebuilds each block from the GET response instead of
// applying the "user omitted the block" preservation rule (which keeps the
// block nil — correct for normal Create/Update where state reflects the
// user's HCL, wrong for a freshly-imported resource where no user HCL has
// ever been parsed).
func seedImportNestedSentinels(state *ComputerPrestageEnrollmentResourceModel) {
	state.SkipSetupItems = &SkipSetupItemsModel{}
	state.LocationInformation = &LocationInformationModel{}
	state.PurchasingInformation = &PurchasingInformationModel{}
	state.AccountSettings = &AccountSettingsModel{}
}

// applyScope drives a ReplaceComputerPrestageScopeV2 call. Always GETs first
// to source the scope versionLock. Caller must ensure the prestage has
// finished its async post-Create setup (waitForPrestageReady).
//
// Jamf Pro returns `400 ALREADY_SCOPED` (with the offending serial in the
// `description` field) when any serial in the requested set is currently
// scoped to a different PreStage. The provider rewraps that diagnostic
// with workflow guidance — Jamf does not move serials between PreStages
// transparently; the user must remove the serial from the holding
// PreStage first.
func applyScope(ctx context.Context, client *pro.Client, prestageID string, serials types.Set) diag.Diagnostics {
	var diags diag.Diagnostics

	scope, err := client.GetComputerPrestageScopeV2(ctx, prestageID)
	if err != nil {
		diags.AddError("Error reading prestage scope before replace", helpers.APIErrorDetail(err))
		return diags
	}

	planSlice, d := stringSetToSlice(ctx, serials)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	if planSlice == nil {
		planSlice = []string{}
	}
	body := &pro.PrestageScopeUpdate{SerialNumbers: planSlice, VersionLock: scope.VersionLock}

	if _, err := client.ReplaceComputerPrestageScopeV2(ctx, prestageID, body); err != nil {
		summary := "Error replacing prestage scope"
		detail := err.Error()
		if strings.Contains(detail, "ALREADY_SCOPED") {
			summary = "Jamf Pro PreStage scope conflict (serial already assigned)"
			detail += "\n\nJamf Pro enforces single-PreStage-per-serial: at least one serial in `scope_serial_numbers` is currently assigned to a different PreStage. Jamf Pro does not move serials between PreStages transparently: remove the serial from the holding PreStage first (in the same `terraform apply` via `depends_on`, in two separate applies, or via the Jamf Pro admin UI) and re-run."
		}
		diags.AddError(summary, detail)
	}
	return diags
}

func recoveryLockPasswordWoBumped(plan, state ComputerPrestageEnrollmentResourceModel) bool {
	return !plan.RecoveryLockPasswordWoVersion.Equal(state.RecoveryLockPasswordWoVersion)
}

func adminPasswordWoBumped(plan, state ComputerPrestageEnrollmentResourceModel) bool {
	if plan.AccountSettings == nil || state.AccountSettings == nil {
		return plan.AccountSettings != nil
	}
	return !plan.AccountSettings.AdminPasswordWoVersion.Equal(state.AccountSettings.AdminPasswordWoVersion)
}

func setsEqual(a, b types.Set) bool {
	if a.IsNull() && b.IsNull() {
		return true
	}
	if a.IsUnknown() || b.IsUnknown() {
		return false
	}
	return a.Equal(b)
}
