// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetManagedSoftwareUpdateFeatureToggleV1        (Read; poll source for Create/Update)
//   pro.UpdateManagedSoftwareUpdateFeatureToggleV1     (Create/Update — async PUT)
//   pro.GetManagedSoftwareUpdateFeatureToggleStatusV1  (read-only; error enrichment on settle timeout)
//
// Not adopted as CRUD:
//   pro.AbandonManagedSoftwareUpdateFeatureToggleV1    (POST /abandon — break-glass; jamfplatform_pro_managed_software_update_abandon action)
//
// The PUT is asynchronous: the server applies the toggle on a background process and the
// PUT echo is stale (wire-probed 2026-06-13 — the echo returned the OLD value while an
// immediate GET returned the new one). Create/Update therefore PUT, then poll
// GET /feature-toggle until `enabled` converges to the requested value (timeout-bounded),
// and only then write state. Convergence on the GET is the authoritative signal — the
// /status endpoint is read-only here, consulted solely to enrich a settle-timeout error.
//
// Status: current. Last reviewed 2026-06-13.

package managed_software_updates

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// settlePollInterval is the delay between GETs while waiting for the async toggle to settle.
const settlePollInterval = 2 * time.Second

// toggleClient is the subset of *pro.Client the async apply/poll uses. It exists so the
// poll-to-settle logic can be unit-tested with a fake (the live PUT is asynchronous and the
// reference singleton resources are synchronous, so there is no shared poll test harness).
type toggleClient interface {
	GetManagedSoftwareUpdateFeatureToggleV1(ctx context.Context) (*pro.ManagedSoftwareUpdatePlanToggle, error)
	UpdateManagedSoftwareUpdateFeatureToggleV1(ctx context.Context, request *pro.ManagedSoftwareUpdatePlanToggle) (*pro.ManagedSoftwareUpdatePlanToggle, error)
	GetManagedSoftwareUpdateFeatureToggleStatusV1(ctx context.Context) (*pro.ManagedSoftwareUpdatePlanToggleStatusWrapper, error)
}

// Create handles initial provisioning of the singleton. The API has no Create endpoint —
// one record per tenant always exists — so this adopts the live value (preserving `enabled`
// when omitted), PUTs, polls to settle, then reads back.
func (r *ManagedSoftwareUpdateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ManagedSoftwareUpdateResourceModel
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

	// Adopt the live value as the merge base so the feature keeps its current state rather
	// than being flipped to false on the write when the user omits `enabled`.
	current, err := r.client.GetManagedSoftwareUpdateFeatureToggleV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Managed Software Updates feature", helpers.APIErrorDetail(err))
		return
	}

	got, err := applyAndSettle(createCtx, r.client, buildManagedSoftwareUpdateInput(plan, current), settlePollInterval)
	if err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro Managed Software Updates feature", helpers.APIErrorDetail(err))
		return
	}
	assignManagedSoftwareUpdateResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, managedSoftwareUpdateIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Managed Software Updates feature")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest feature state from the Jamf Pro API.
func (r *ManagedSoftwareUpdateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ManagedSoftwareUpdateResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(managedSoftwareUpdateTimeoutAttributeTypes)
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

	got, err := r.client.GetManagedSoftwareUpdateFeatureToggleV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Managed Software Updates feature", helpers.APIErrorDetail(err))
		return
	}

	assignManagedSoftwareUpdateResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, managedSoftwareUpdateIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new value to the Jamf Pro API. Same async PUT + poll-to-settle as
// Create; the merge base is nil because UseStateForUnknown has already carried an omitted
// `enabled` into the plan as a known prior value.
func (r *ManagedSoftwareUpdateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ManagedSoftwareUpdateResourceModel
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

	got, err := applyAndSettle(updateCtx, r.client, buildManagedSoftwareUpdateInput(plan, nil), settlePollInterval)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Managed Software Updates feature", helpers.APIErrorDetail(err))
		return
	}
	assignManagedSoftwareUpdateResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, managedSoftwareUpdateIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the feature
// state persists on the tenant. Terraform removes the resource from state on its own after
// this handler returns. No SDK call is made and no diagnostics are emitted.
func (r *ManagedSoftwareUpdateResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Managed Software Updates feature from Terraform state (singleton — no remote delete)")
}

// applyAndSettle PUTs the toggle then waits for the asynchronous change to take effect.
// The PUT echo is stale, so the returned value comes from the GET that observes
// convergence — never from the PUT response.
func applyAndSettle(ctx context.Context, client toggleClient, body *pro.ManagedSoftwareUpdatePlanToggle, interval time.Duration) (*pro.ManagedSoftwareUpdatePlanToggle, error) {
	if _, err := client.UpdateManagedSoftwareUpdateFeatureToggleV1(ctx, body); err != nil {
		return nil, err
	}
	return pollToggleSettled(ctx, client, body.Toggle, interval)
}

// pollToggleSettled polls GET /feature-toggle until `toggle` equals the requested value,
// bounded by ctx. When desired already matches (a no-op apply, or omit-on-create adopt) it
// returns on the first GET. On ctx expiry it returns a settle-timeout error enriched with
// the background process's last reported status.
func pollToggleSettled(ctx context.Context, client toggleClient, desired bool, interval time.Duration) (*pro.ManagedSoftwareUpdatePlanToggle, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// lastErr keeps the most recent GET failure. A transient error mid-flip is fine to
	// retry, but if GETs keep failing until ctx expires we must surface that error rather
	// than masking a 4xx/5xx behind a generic settle-timeout message.
	var lastErr error

	for {
		got, err := client.GetManagedSoftwareUpdateFeatureToggleV1(ctx)
		switch {
		case err == nil && got.Toggle == desired:
			return got, nil
		case err != nil:
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return nil, settleTimeoutError(client, desired, lastErr)
		case <-ticker.C:
		}
	}
}

// settleTimeoutError builds the timeout diagnostic. When the poll kept failing it surfaces
// that error (so a persistent 4xx/5xx is not masked as a timeout); otherwise it best-effort
// enriches the message with the last reported status of the relevant background process
// (toggleOn when enabling, toggleOff when disabling) and points at the break-glass abandon
// action. It uses a fresh short-lived context because the caller's context has expired.
func settleTimeoutError(client toggleClient, desired bool, lastErr error) error {
	if lastErr != nil {
		return fmt.Errorf(
			"failed to read the Managed Software Updates feature while waiting for enabled=%t to settle: %w",
			desired, lastErr,
		)
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	detail := ""
	if wrapper, err := client.GetManagedSoftwareUpdateFeatureToggleStatusV1(statusCtx); err == nil && wrapper != nil {
		st := wrapper.ToggleOff
		if desired {
			st = wrapper.ToggleOn
		}
		if st != nil {
			detail = fmt.Sprintf(" Last reported background status: state=%q, exitState=%q, exitMessage=%q.", st.State, st.ExitState, st.ExitMessage)
		}
	}

	return fmt.Errorf(
		"timed out waiting for the Managed Software Updates feature to settle to enabled=%t.%s "+
			"If the change is stuck, invoke the jamfplatform_pro_managed_software_update_abandon action to force-stop the process, then retry. "+
			"You can also raise the create/update timeout on this resource",
		desired, detail,
	)
}
