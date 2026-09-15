// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.UploadDeviceEnrollmentTokenV1
//   pro.GetDeviceEnrollmentV1
//   pro.UpdateDeviceEnrollmentV1
//   pro.ReplaceDeviceEnrollmentTokenV1
//   pro.DeleteDeviceEnrollmentV1
//   pro.GetLatestDeviceEnrollmentSyncV1                    (Create / token-rotation sync wait)
//
// The metadata update has never been probed for merge-versus-replace. Omitting
// site_id or supervision_identity_id preserves the stored value because both
// attributes carry UseStateForUnknown and so re-send the prior value, not
// because the endpoint was seen to merge an omitted field. Probe it before
// relying on merge behaviour here.
//
// Status: current. Last reviewed 2026-05-28.

package automated_device_enrollment

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions a new Jamf Pro Automated Device Enrollment (ADE) instance.
//
// The Jamf Pro API splits creation across two endpoints: an initial token
// upload that allocates the instance ID, followed by a metadata PUT that sets
// the user-visible name and any site / supervision-identity associations.
// This Create runs both steps and, if the metadata PUT fails, calls Delete on
// the partially-created instance so Terraform either fully succeeds or leaves
// no resource behind.
//
// `server_token` is `WriteOnly`, so the plaintext base64 value is pulled from
// `req.Config` (not `req.Plan`). The provider TrimSpaces the supplied string
// and then base64-decodes it to raw bytes before calling the SDK.
func (r *AutomatedDeviceEnrollmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AutomatedDeviceEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg AutomatedDeviceEnrollmentResourceModel
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

	decoded, decodeDiags := decodeServerToken(cfg.ServerToken.ValueString())
	resp.Diagnostics.Append(decodeDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	uploadResp, err := r.client.UploadDeviceEnrollmentTokenV1(createCtx, buildCreateTokenInput(plan, decoded))
	if err != nil {
		resp.Diagnostics.AddError("Error uploading Jamf Pro Automated Device Enrollment token", helpers.APIErrorDetail(err))
		return
	}
	if uploadResp == nil || uploadResp.ID == "" {
		resp.Diagnostics.AddError(
			"Upload response missing Automated Device Enrollment ID",
			"Jamf Pro returned success on the token upload but did not include an instance ID; cannot persist state.",
		)
		return
	}
	id := uploadResp.ID

	// Step 2: rename + set optional associations via UpdateDeviceEnrollmentV1.
	// If this fails we must roll back the partially-created instance so Create
	// is atomic from Terraform's perspective.
	if _, err := r.client.UpdateDeviceEnrollmentV1(createCtx, id, buildMetadataInput(plan)); err != nil {
		tflog.Warn(ctx, "Jamf Pro Automated Device Enrollment metadata PUT failed, rolling back upload", map[string]any{
			"id":    id,
			"error": err.Error(),
		})
		if rbErr := r.client.DeleteDeviceEnrollmentV1(createCtx, id); rbErr != nil {
			tflog.Warn(ctx, "Jamf Pro Automated Device Enrollment rollback delete failed", map[string]any{
				"id":    id,
				"error": rbErr.Error(),
			})
		}
		resp.Diagnostics.AddError(
			"Error finalising Jamf Pro Automated Device Enrollment instance",
			fmt.Sprintf("Token upload succeeded but the follow-up metadata PUT failed; the partial instance was deleted. Underlying error: %s", helpers.APIErrorDetail(err)),
		)
		return
	}

	// Wait for the first Apple ADE sync to complete before declaring
	// Create successful. Downstream resources (computer prestages,
	// scope assignments) assume the device list is known to Jamf;
	// returning before the sync finishes lets the caller hit
	// `context deadline exceeded` on the very next API call against
	// the freshly-created instance.
	if d := waitForAdeSync(createCtx, r.client, id); d.HasError() {
		resp.Diagnostics.Append(d...)
		return
	}

	got, err := r.client.GetDeviceEnrollmentV1(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro Automated Device Enrollment instance", helpers.APIErrorDetail(err))
		return
	}
	assignAutomatedDeviceEnrollmentResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, automatedDeviceEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro Automated Device Enrollment instance", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest ADE instance
// representation. Import-time refresh (req.State.Raw is null) sources the ID
// from the identity object so users can `terraform import` by the instance ID.
func (r *AutomatedDeviceEnrollmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AutomatedDeviceEnrollmentResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this Automated Device Enrollment instance without existing state or identity data, so the provider cannot determine which instance to read.",
			)
			return
		}
		var identity automatedDeviceEnrollmentIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Automated Device Enrollment ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the instance.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(automatedDeviceEnrollmentTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro Automated Device Enrollment instance without ID.")
		return
	}

	got, err := r.client.GetDeviceEnrollmentV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Automated Device Enrollment instance not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, automatedDeviceEnrollmentIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro Automated Device Enrollment instance", helpers.APIErrorDetail(err))
		return
	}

	assignAutomatedDeviceEnrollmentResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, automatedDeviceEnrollmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro Automated Device Enrollment instance. The
// `server_token` is rotated via `ReplaceDeviceEnrollmentTokenV1` only when the
// user bumps `server_token_wo_version` (matches the WriteOnly pattern in
// `jamfplatform_pro_directory_binding`). Metadata is refreshed on every
// Update via `UpdateDeviceEnrollmentV1`.
func (r *AutomatedDeviceEnrollmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AutomatedDeviceEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state AutomatedDeviceEnrollmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg AutomatedDeviceEnrollmentResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Pro Automated Device Enrollment instance without ID.")
		return
	}

	if !plan.ServerTokenWoVersion.Equal(state.ServerTokenWoVersion) {
		decoded, decodeDiags := decodeServerToken(cfg.ServerToken.ValueString())
		resp.Diagnostics.Append(decodeDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if _, err := r.client.ReplaceDeviceEnrollmentTokenV1(updateCtx, id, buildCreateTokenInput(plan, decoded)); err != nil {
			resp.Diagnostics.AddError("Error rotating Jamf Pro Automated Device Enrollment token", helpers.APIErrorDetail(err))
			return
		}
		// Token rotation triggers a fresh Apple ADE sync; wait for it
		// to finish so the next downstream call lands on a synced
		// instance (mirrors Create behaviour).
		if d := waitForAdeSync(updateCtx, r.client, id); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
	}

	if _, err := r.client.UpdateDeviceEnrollmentV1(updateCtx, id, buildMetadataInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Automated Device Enrollment instance metadata", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetDeviceEnrollmentV1(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro Automated Device Enrollment instance", helpers.APIErrorDetail(err))
		return
	}
	assignAutomatedDeviceEnrollmentResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, automatedDeviceEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro Automated Device Enrollment instance.
func (r *AutomatedDeviceEnrollmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AutomatedDeviceEnrollmentResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro Automated Device Enrollment instance without ID.")
		return
	}

	if err := r.client.DeleteDeviceEnrollmentV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Automated Device Enrollment instance already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro Automated Device Enrollment instance", helpers.APIErrorDetail(err))
	}
}

// waitForAdeSync polls the per-instance latest-sync endpoint until the sync
// state reaches `SUCCESSFUL` (terminal success), surfaces a failure-shaped
// state as a diagnostic error, or the surrounding context deadline fires.
//
// Apple's ADE round-trip is the bottleneck — uploading the .p7m to Jamf is
// sub-second but Jamf then has to fetch the device list from Apple, which
// can take a few minutes on cold tokens. Downstream resources (computer
// prestages, scope assignments) assume the device list is known, so the
// ADE Create / token-rotation Update must block until the sync completes.
//
// Terminal-success value sourced from a live wire-probe against a healthy
// tenant ADE instance (2026-05-28). Any state whose name contains "FAIL"
// is treated as terminal failure; everything else is treated as in-flight
// and polled again.
func waitForAdeSync(ctx context.Context, client *pro.Client, id string) diag.Diagnostics {
	var diags diag.Diagnostics
	ticker := time.NewTicker(syncPollInterval)
	defer ticker.Stop()

	for {
		sync, err := client.GetLatestDeviceEnrollmentSyncV1(ctx, id)
		if err != nil {
			// 404 is expected briefly immediately after upload — the
			// sync record may not exist yet. Tolerate and re-poll.
			if !helpers.IsNotFoundError(err) {
				diags.AddError(
					"Error polling Jamf Pro Automated Device Enrollment sync status",
					fmt.Sprintf("Could not fetch sync status for instance %s: %s", id, helpers.APIErrorDetail(err)),
				)
				return diags
			}
		} else if sync != nil {
			switch {
			case sync.SyncState == adeSyncStateSuccessful:
				tflog.Trace(ctx, "Jamf Pro Automated Device Enrollment sync completed", map[string]any{
					"id":        id,
					"timestamp": sync.Timestamp,
				})
				return diags
			case strings.Contains(sync.SyncState, "FAIL"):
				diags.AddError(
					"Jamf Pro Automated Device Enrollment sync failed",
					fmt.Sprintf("Sync state for instance %s is %q (timestamp %s). Inspect the ADE instance in the Jamf Pro admin UI to diagnose the failure.", id, sync.SyncState, sync.Timestamp),
				)
				return diags
			default:
				tflog.Trace(ctx, "Jamf Pro Automated Device Enrollment sync in progress", map[string]any{
					"id":         id,
					"sync_state": sync.SyncState,
				})
			}
		}

		select {
		case <-ctx.Done():
			diags.AddError(
				"Jamf Pro Automated Device Enrollment sync wait timed out",
				fmt.Sprintf("Sync for instance %s did not reach state %q before the operation timeout fired. Increase the `timeouts.create` (or `timeouts.update`) on the resource if Apple ADE sync is consistently slow on this tenant.", id, adeSyncStateSuccessful),
			)
			return diags
		case <-ticker.C:
		}
	}
}

// decodeServerToken TrimSpaces the user-supplied base64 string and decodes it
// into the raw `[]byte` shape expected by `pro.DeviceEnrollmentToken.EncodedToken`.
// A decode failure surfaces as a `Diagnostics` error so callers can
// short-circuit before hitting the SDK.
func decodeServerToken(raw string) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		diags.AddError(
			"Invalid Automated Device Enrollment server token",
			"`server_token` is empty after trimming whitespace. Supply the base64-encoded contents of the `.p7m` token downloaded from Apple Business Manager / Apple School Manager.",
		)
		return nil, diags
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		diags.AddError(
			"Invalid Automated Device Enrollment server token",
			fmt.Sprintf("`server_token` is not valid base64: %s. Supply the base64-encoded contents of the `.p7m` token downloaded from Apple Business Manager / Apple School Manager.", helpers.APIErrorDetail(err)),
		)
		return nil, diags
	}
	return decoded, diags
}
