// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.CreateZtnaGatewayV1
//   securitycloud.GetZtnaGatewayV1 (read, create/update read-back, readiness wait)
//   securitycloud.UpdateZtnaGatewayV1
//   securitycloud.DeleteZtnaGatewayV1
//   securitycloud.ListZtnaGatewaysV1 (data sources / list resource)
//   securitycloud.ResolveZtnaGatewayV1ByName (singular data source, name lookup)
//
// Status: current. Last reviewed 2026-08-27.

package ztna_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Security Cloud ZTNA gateway.
//
// The create response carries only the new ID, so the gateway is read back for
// the server-assigned parts — the status block, and the dedicated egress
// addresses on an internet gateway.
//
// A gateway is not usable the moment it is created: the 2026-08-31 probe measured
// 275 seconds before one reported itself operational. For the forms that reach that
// state the apply waits for it, so a completed create leaves a gateway that is ready
// rather than one still being built — see waitForGatewayState for the measurements
// and gatewayWaitTarget for which forms qualify and what each waits for. The wait's last read is what gets
// recorded, which is why readBackGateway exists rather than a second read here: when
// the wait exhausts the budget, another read on the same context could only fail, and
// an exhausted wait must stay a warning on a successful apply rather than becoming an
// error that taints a gateway the account is already paying for.
//
// A read-back that fails after the create has succeeded still commits state. The
// gateway exists on the tenant by then, and returning without state would leave it
// running unmanaged: nothing stops Jamf Security Cloud provisioning a second gateway
// on the retry, which would take another dedicated IP address from the account's
// allotment and leave the first one to be found in the admin UI. What is committed is
// the plan carrying the new ID — the configured values are what the next refresh
// reconciles, and an errored apply does not run Terraform's plan-consistency check.
// Values only the read-back could have filled are nulled first — see
// nullUnknownReadBackValues.
func (r *GatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config GatewayResourceModel
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

	input, inputDiags := buildGatewayCreateInput(ctx, plan, config)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateZtnaGatewayV1(createCtx, input)
	if err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error creating Jamf Security Cloud ZTNA gateway", helpers.APIErrorDetail(err))
		}
		return
	}

	plan.ID = types.StringValue(created.ID)

	var got *securitycloud.Gateway
	if want, wait := gatewayWaitTarget(&plan, gatewayWaitCreate); wait {
		observed, lastState, reached := waitForGatewayState(createCtx, r.client.GetZtnaGatewayV1, created.ID, want, gatewayStatusPollInterval, gatewayAbandonState(want), gatewayDownDwell)
		if !reached {
			appendGatewayWaitWarning(&resp.Diagnostics, gatewayWaitCreate, want, lastState)
		}
		got = observed
	}

	got, err = readBackGateway(createCtx, r.client.GetZtnaGatewayV1, created.ID, got)
	if err != nil {
		nullUnknownReadBackValues(&plan)
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gatewayIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Error reading created Jamf Security Cloud ZTNA gateway",
			"The gateway was created with ID \""+created.ID+"\" but could not be read back, so Terraform has "+
				"recorded its ID and the configured values without the status or dedicated egress "+
				"addresses. The next plan will refresh those — do not re-create it: nothing prevents a second "+
				"gateway being provisioned alongside this one, which would consume another dedicated IP address "+
				"from the account's allotment and leave this one running unmanaged. Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}
	resp.Diagnostics.Append(assignGatewayResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gatewayIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud ZTNA gateway", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// nullUnknownReadBackValues nulls the values only the create read-back could have
// filled, so the partial state committed when it fails is wholly known.
//
// Terraform answers an unknown value in the state a failed apply returns with an
// "invalid result object after apply" error of its own — a provider-bug notice that
// would bury the diagnostic the partial state exists to deliver. All four values here
// are Computed with no default, so each is Unknown in every create plan: the status
// block, the dedicated egress addresses, and the authentication method each tunnel
// endpoint reports. `enabled` is Optional and Computed but carries a default, so the
// framework has already resolved it at plan time.
func nullUnknownReadBackValues(plan *GatewayResourceModel) {
	if plan.Status.IsUnknown() {
		plan.Status = types.ObjectNull(statusAttributeTypes)
	}
	if plan.DedicatedEgressIPAddresses.IsUnknown() {
		plan.DedicatedEgressIPAddresses = types.ListNull(types.StringType)
	}
	if plan.IPSec == nil {
		return
	}
	if plan.IPSec.JamfSide != nil && plan.IPSec.JamfSide.AuthMethod.IsUnknown() {
		plan.IPSec.JamfSide.AuthMethod = types.StringNull()
	}
	if plan.IPSec.CustomerSide != nil && plan.IPSec.CustomerSide.AuthMethod.IsUnknown() {
		plan.IPSec.CustomerSide.AuthMethod = types.StringNull()
	}
}

// Read refreshes the Terraform state with the latest gateway representation.
func (r *GatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GatewayResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this gateway without existing state or identity data, so the provider cannot determine which gateway to read.",
			)
			return
		}
		var identity gatewayIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing gateway ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the gateway.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(gatewayTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Security Cloud ZTNA gateway without ID.")
		return
	}

	got, err := r.client.GetZtnaGatewayV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud ZTNA gateway not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gatewayIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud ZTNA gateway", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignGatewayResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gatewayIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Security Cloud ZTNA gateway.
//
// The prior state is needed as well as the plan and config: it carries the
// pre-shared key's rotation trigger, which is what decides whether the key goes
// on the wire at all.
//
// The readiness wait runs unconditionally on the forms that qualify, with no attempt
// to work out from the plan whether this particular change re-provisions anything. It
// does not need to: an update that changes only a name or a contact leaves the gateway
// already operational, and the wait costs exactly one read in that case, which is the
// read this path had to make anyway.
func (r *GatewayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config GatewayResourceModel
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

	if plan.ID.IsNull() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Security Cloud ZTNA gateway without ID.")
		return
	}

	input, inputDiags := buildGatewayPatchInput(ctx, plan, state, config)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateZtnaGatewayV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error updating Jamf Security Cloud ZTNA gateway", helpers.APIErrorDetail(err))
		}
		return
	}

	var got *securitycloud.Gateway
	if want, wait := gatewayWaitTarget(&plan, gatewayWaitUpdate); wait {
		observed, lastState, reached := waitForGatewayState(updateCtx, r.client.GetZtnaGatewayV1, plan.ID.ValueString(), want, gatewayStatusPollInterval, gatewayAbandonState(want), gatewayDownDwell)
		if !reached {
			appendGatewayWaitWarning(&resp.Diagnostics, gatewayWaitUpdate, want, lastState)
		}
		got = observed
	}

	got, err := readBackGateway(updateCtx, r.client.GetZtnaGatewayV1, plan.ID.ValueString(), got)
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Security Cloud ZTNA gateway", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignGatewayResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gatewayIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Security Cloud ZTNA gateway.
func (r *GatewayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GatewayResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Security Cloud ZTNA gateway without ID.")
		return
	}

	if err := r.client.DeleteZtnaGatewayV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud ZTNA gateway already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		if appendDeleteDiagnostics(&resp.Diagnostics, err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Security Cloud ZTNA gateway", helpers.APIErrorDetail(err))
	}
}
