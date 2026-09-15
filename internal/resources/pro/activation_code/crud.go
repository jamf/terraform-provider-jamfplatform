// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.GetActivationCode       (deprecated 2026-07-14; no successor read is published)
//   proclassic.UpdateActivationCode    (deprecated 2026-07-14; successor pro.UpdateActivationCodeV1
//                                        deliberately not adopted. PUT, XML — known server bug:
//                                        commits the write but returns HTTP 500; tolerated via
//                                        read-back verification in applyActivationCode. PI-1401.)
//
// Status: deprecated by Jamf 2026-07-14; no migration is possible and none is planned — the read has
// no successor, so the resource and the data source both carry a whole-schema DeprecationMessage and
// are removed with the endpoint. See deprecation.go for the reasoning behind the six SA1019
// suppressions. Last reviewed 2026-09-15.

package activation_code

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the activation code singleton. The ProClassic
// API has no Create endpoint for this object, so this funnels into Update against the
// plan, then reads back to capture authoritative state.
func (r *ActivationCodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ActivationCodeResourceModel
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

	got, applyDiags := r.applyActivationCode(createCtx, plan)
	resp.Diagnostics.Append(applyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	assignActivationCodeResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, activationCodeIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro activation code")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest activation code from the ProClassic API.
func (r *ActivationCodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ActivationCodeResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(activationCodeTimeoutAttributeTypes)
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

	//nolint:staticcheck // SA1019: the Classic /activationcode endpoint is deprecated with no replacement read — see deprecation.go.
	got, err := r.client.GetActivationCode(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro activation code", helpers.APIErrorDetail(err))
		return
	}

	assignActivationCodeResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, activationCodeIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new activation code and organization name to the ProClassic API.
// Both fields are always written together (see buildActivationCodeInput).
func (r *ActivationCodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ActivationCodeResourceModel
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

	got, applyDiags := r.applyActivationCode(updateCtx, plan)
	resp.Diagnostics.Append(applyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	assignActivationCodeResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, activationCodeIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the ProClassic API. The activation code is a license secret that
// must persist on the tenant — there is no remote delete, and blanking it would disable
// the tenant. Terraform removes the resource from state on its own after this returns.
func (r *ActivationCodeResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro activation code from Terraform state (singleton — no remote delete; license code persists on tenant)")
}
