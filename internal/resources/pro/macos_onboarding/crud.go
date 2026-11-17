// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetOnboardingV1
//   pro.UpdateOnboardingV1
//   pro.ListOnboardingEligibleAppsV1                  (eligible-items data source)
//   pro.ListOnboardingEligibleConfigurationProfilesV1 (eligible-items data source)
//   pro.ListOnboardingEligiblePoliciesV1              (eligible-items data source)
//
// Not adopted (intentional): pro.ListOnboardingHistoryV1, pro.CreateOnboardingHistoryNoteV1,
// pro.ExportOnboardingHistoryV1 — onboarding history and export are convention-wide
// exclusions (audit/reporting concerns, not configuration).
//
// Status: current.

package macos_onboarding

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the macOS Onboarding settings singleton. The
// Jamf Pro API has no Create endpoint for this object — one record per tenant already
// exists — so this funnels into Update against the plan, then reads back to capture
// authoritative state.
//
// Unlike the login-page singleton, both managed fields (enabled, onboarding_items) are
// Required, so there is no omitted-field adoption to perform — the plan always carries
// the complete state. The PUT is full-replace (wire-probed): the body is the entire
// item list. The post-write GET picks up server-derived echoes and the priority-sorted
// canonical order.
func (r *OnboardingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan OnboardingResourceModel
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

	items, diags := onboardingItemsFromList(createCtx, plan.OnboardingItems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateOnboardingV1(createCtx, buildOnboardingInput(plan.Enabled.ValueBool(), items)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro macOS Onboarding settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetOnboardingV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro macOS Onboarding settings after write", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignOnboardingResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, onboardingIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro macOS Onboarding settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *OnboardingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state OnboardingResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(onboardingTimeoutAttributeTypes)
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

	got, err := r.client.GetOnboardingV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro macOS Onboarding settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignOnboardingResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, onboardingIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create. The PUT
// is full-replace, so the entire item list from the plan is sent. Authoritative state
// comes from a follow-up GET.
func (r *OnboardingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan OnboardingResourceModel
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

	items, diags := onboardingItemsFromList(updateCtx, plan.OnboardingItems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateOnboardingV1(updateCtx, buildOnboardingInput(plan.Enabled.ValueBool(), items)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro macOS Onboarding settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetOnboardingV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro macOS Onboarding settings after update", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignOnboardingResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, onboardingIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the
// record persists on the tenant. Terraform removes the resource from state on its own
// after this handler returns. No SDK call is made and no diagnostics are emitted.
func (r *OnboardingResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro macOS Onboarding settings from Terraform state (singleton — no remote delete)")
}
