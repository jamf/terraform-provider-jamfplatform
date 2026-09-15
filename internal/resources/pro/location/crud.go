// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateVolumePurchasingLocationV1
//   pro.GetVolumePurchasingLocationV1
//   pro.UpdateVolumePurchasingLocationV1
//   pro.DeleteVolumePurchasingLocationV1
//   pro.ReclaimVolumePurchasingLocationLicensesV1
//   pro.ListVolumePurchasingLocationsV1                  (list resource)
//   pro.ResolveVolumePurchasingLocationV1ByName          (data source name lookup)
// Status: current. Last reviewed 2026-05-25.

package location

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions a new Jamf Pro Volume Purchasing (VPP) location.
//
// The Create flow is: POST → Reclaim → poll-until-`lastSyncTime != ""` → final
// GET. Reclaim runs BEFORE the sync poll so that any
// `clientContextMismatch=true` state inherited from a previously shared
// service token is cleared within seconds of the location being created.
//
// `service_token` is `WriteOnly`, so the plaintext base64 value is pulled
// from `req.Config` (not `req.Plan`). The provider TrimSpaces the supplied
// string before sending (Apple's `.vpptoken` files often carry a trailing
// newline that the Pro API rejects with HTTP 400).
func (r *VolumePurchasingLocationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VolumePurchasingLocationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg VolumePurchasingLocationResourceModel
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

	trimmedToken := strings.TrimSpace(cfg.ServiceToken.ValueString())
	if trimmedToken == "" {
		resp.Diagnostics.AddError(
			"Invalid Volume Purchasing service token",
			"`service_token` is empty after trimming whitespace. Supply the base64 contents of the `.vpptoken` file downloaded from Apple Business Manager / Apple School Manager.",
		)
		return
	}

	createResp, err := r.client.CreateVolumePurchasingLocationV1(createCtx, buildCreateInput(plan, trimmedToken))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro Volume Purchasing location", helpers.APIErrorDetail(err))
		return
	}
	if createResp == nil || createResp.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing Volume Purchasing location ID",
			"Jamf Pro returned success on the POST but did not include a location ID; cannot persist state.",
		)
		return
	}
	id := createResp.ID

	// Reclaim licenses. Soft-fail: log a warning but continue regardless —
	// Reclaim is a best-effort hygiene step that clears `clientContextMismatch`
	// for tokens previously bound to a different Jamf instance; failure here
	// must not block the Create flow.
	if err := r.client.ReclaimVolumePurchasingLocationLicensesV1(createCtx, id); err != nil {
		tflog.Warn(ctx, "Jamf Pro Volume Purchasing reclaim-licenses call failed; continuing with Create", map[string]any{
			"id":    id,
			"error": err.Error(),
		})
	}

	// Poll until Apple's first content sync completes (lastSyncTime non-empty).
	// The first iteration runs immediately so small tenants converge fast.
	loc, err := pollForSyncComplete(createCtx, r.client, id, syncPollInterval, time.Time{})
	if err != nil {
		resp.Diagnostics.AddError("Error waiting for Jamf Pro Volume Purchasing content sync", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignVolumePurchasingLocationResourceModel(ctx, &plan, loc)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingLocationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro Volume Purchasing location", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest VPP location
// representation. Import-time refresh (req.State.Raw is null) sources the ID
// from the identity object so users can `terraform import` by the location ID.
func (r *VolumePurchasingLocationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VolumePurchasingLocationResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this Volume Purchasing location without existing state or identity data, so the provider cannot determine which location to read.",
			)
			return
		}
		var identity volumePurchasingLocationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Volume Purchasing location ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the location.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(volumePurchasingLocationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro Volume Purchasing location without ID.")
		return
	}

	got, err := r.client.GetVolumePurchasingLocationV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Volume Purchasing location not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingLocationIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro Volume Purchasing location", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignVolumePurchasingLocationResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingLocationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro Volume Purchasing location.
//
// If `service_token_wo_version` changed: PATCH with the new token + metadata,
// run Reclaim, then poll until Apple's content sync completes (lastSyncTime
// strictly newer than the prior anchor).
//
// Otherwise (metadata-only change): PATCH the metadata without a `serviceToken`
// field and skip both Reclaim and the sync poll — metadata changes do not
// trigger an Apple-side resync.
func (r *VolumePurchasingLocationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VolumePurchasingLocationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state VolumePurchasingLocationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg VolumePurchasingLocationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Pro Volume Purchasing location without ID.")
		return
	}

	tokenRotated := !plan.ServiceTokenWoVersion.Equal(state.ServiceTokenWoVersion)

	if tokenRotated {
		trimmedToken := strings.TrimSpace(cfg.ServiceToken.ValueString())
		if trimmedToken == "" {
			resp.Diagnostics.AddError(
				"Invalid Volume Purchasing service token",
				"`service_token` is empty after trimming whitespace. Supply the base64 contents of the `.vpptoken` file downloaded from Apple Business Manager / Apple School Manager.",
			)
			return
		}
		if _, err := r.client.UpdateVolumePurchasingLocationV1(updateCtx, id, buildTokenRotationPatch(plan, trimmedToken)); err != nil {
			resp.Diagnostics.AddError("Error rotating Jamf Pro Volume Purchasing service token", helpers.APIErrorDetail(err))
			return
		}

		// Reclaim licenses after a token rotation (soft-fail).
		if err := r.client.ReclaimVolumePurchasingLocationLicensesV1(updateCtx, id); err != nil {
			tflog.Warn(ctx, "Jamf Pro Volume Purchasing reclaim-licenses call failed; continuing with Update", map[string]any{
				"id":    id,
				"error": err.Error(),
			})
		}

		// Wait for the new token's first sync to complete. Anchor on the
		// previously-stored lastSyncTime so we wait for a STRICTLY newer
		// timestamp; if the prior anchor is unparseable or empty the poll
		// helper accepts any non-empty sync time.
		anchor := parseRFC3339OrZero(state.LastSyncTime.ValueString())
		loc, err := pollForSyncComplete(updateCtx, r.client, id, syncPollInterval, anchor)
		if err != nil {
			resp.Diagnostics.AddError("Error waiting for Jamf Pro Volume Purchasing content sync after token rotation", helpers.APIErrorDetail(err))
			return
		}
		resp.Diagnostics.Append(assignVolumePurchasingLocationResourceModel(ctx, &plan, loc)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		// Metadata-only Update: no service_token field on the wire, no
		// Reclaim, no sync poll.
		if _, err := r.client.UpdateVolumePurchasingLocationV1(updateCtx, id, buildMetadataPatch(plan)); err != nil {
			resp.Diagnostics.AddError("Error updating Jamf Pro Volume Purchasing location metadata", helpers.APIErrorDetail(err))
			return
		}
		got, err := r.client.GetVolumePurchasingLocationV1(updateCtx, id)
		if err != nil {
			resp.Diagnostics.AddError("Error reading updated Jamf Pro Volume Purchasing location", helpers.APIErrorDetail(err))
			return
		}
		resp.Diagnostics.Append(assignVolumePurchasingLocationResourceModel(ctx, &plan, got)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingLocationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro Volume Purchasing location.
func (r *VolumePurchasingLocationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VolumePurchasingLocationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro Volume Purchasing location without ID.")
		return
	}

	if err := r.client.DeleteVolumePurchasingLocationV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Volume Purchasing location already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro Volume Purchasing location", helpers.APIErrorDetail(err))
	}
}

// pollForSyncComplete polls `GetVolumePurchasingLocationV1` every `interval`
// until Apple's content sync produces a non-empty `LastSyncTime` (and, when a
// non-zero `anchor` is supplied, until the returned `LastSyncTime` is strictly
// after `anchor`). The first iteration runs immediately so already-synced
// locations return on the first tick without an initial wait.
//
// The poll exits with an error when the surrounding context is cancelled
// (typically the user-configured Create/Update timeout); the error message
// directs operators to raise the timeout on tenants with large catalogs.
func pollForSyncComplete(
	ctx context.Context,
	client *pro.Client,
	id string,
	interval time.Duration,
	anchor time.Time,
) (*pro.VolumePurchasingLocation, error) {
	// First iteration runs immediately (no initial wait). Subsequent iterations
	// honour the interval via a ticker so context cancellation interrupts the
	// gap, not just the GET.
	check := func() (*pro.VolumePurchasingLocation, bool, error) {
		loc, err := client.GetVolumePurchasingLocationV1(ctx, id)
		if err != nil {
			return nil, false, err
		}
		if loc == nil || loc.LastSyncTime == "" {
			return loc, false, nil
		}
		if !anchor.IsZero() {
			if got, parseErr := time.Parse(time.RFC3339, loc.LastSyncTime); parseErr == nil {
				if !got.After(anchor) {
					return loc, false, nil
				}
			}
			// If the returned timestamp is unparseable, fall through and
			// accept any non-empty value as success — better to converge on
			// a wire we cannot parse than to block forever.
		}
		return loc, true, nil
	}

	loc, done, err := check()
	if err != nil {
		return nil, err
	}
	if done {
		return loc, nil
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("VPP content sync did not complete within Create/Update timeout — increase `timeouts { create = \"60m\" }` if your tenant has a large catalog: %w", ctx.Err())
		case <-ticker.C:
			loc, done, err := check()
			if err != nil {
				return nil, err
			}
			if done {
				return loc, nil
			}
		}
	}
}

// parseRFC3339OrZero parses an RFC3339 timestamp and returns time.Zero on any
// parse error (or empty input). Used by Update to anchor the post-rotation
// sync poll on the previously-stored `last_sync_time`; an unparseable anchor
// is treated as "accept any non-empty sync time" rather than failing the
// Update outright.
func parseRFC3339OrZero(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
