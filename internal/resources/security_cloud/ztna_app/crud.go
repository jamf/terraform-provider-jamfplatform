// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.CreateZtnaAppV1
//   securitycloud.GetZtnaAppV1
//   securitycloud.UpdateZtnaAppV1
//   securitycloud.DeleteZtnaAppV1
//   securitycloud.ListZtnaAppsV1 (data sources / list resource / collision check in collision.go)
//
// Uniqueness: hostnames, tenant-wide, enforced (409 HOSTNAME_CONFLICT), case-insensitive and accent-sensitive. The name is not a key.
//
// Status: current. Last reviewed 2026-10-25.

package ztna_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Security Cloud ZTNA access policy application.
//
// The create response carries only the new ID and a canonical URL, so the
// application is read back to populate state with the stored representation — which
// matters here because the server re-orders and de-duplicates every collection, and
// folds host names to lower case.
//
// A read-back that fails after the create has succeeded still commits state. The
// application exists on the tenant by then, and returning without state would orphan
// it — worse here than on the sibling constructs, because host names, address ranges
// and predefined definitions belong to only one application per tenant, so the retry
// an operator reaches for is refused by this provider's own diagnostics, describing an
// object they never knowingly created. What is committed is the plan carrying the new
// ID: the configured values are what the next refresh will reconcile, and an errored
// apply does not run Terraform's plan-consistency check, so a collection the server
// re-ordered or de-duplicated is not a fault at that point. Values only the read-back
// could have filled are nulled first — see nullUnknownReadBackValues.
func (r *ZtnaAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ZtnaAppResourceModel
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

	input, inputDiags := buildAppCreateInput(ctx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateZtnaAppV1(createCtx, input)
	if err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err, !plan.PredefinedAppID.IsNull()) {
			resp.Diagnostics.AddError("Error creating Jamf Security Cloud access policy application", helpers.APIErrorDetail(err))
		}
		return
	}

	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetZtnaAppV1(createCtx, created.ID)
	if err != nil {
		nullUnknownReadBackValues(&plan)
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ztnaAppIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Error reading created Jamf Security Cloud access policy application",
			"The application was created with ID \""+created.ID+"\" but could not be read back, so Terraform has "+
				"recorded its ID and the configured values without confirming what was stored. The next plan will "+
				"refresh it — do not re-create it: host names, address ranges and predefined definitions belong to "+
				"only one application per tenant, so a second create would be refused. Underlying error: "+
				helpers.APIErrorDetail(err),
		)
		return
	}
	resp.Diagnostics.Append(assignAppResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ztnaAppIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud access policy application", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// nullUnknownReadBackValues nulls the values only the create read-back could have
// filled, so the partial state committed when it fails is wholly known.
//
// Terraform answers an unknown value in the state a failed apply returns with an
// "invalid result object after apply" error of its own — a provider-bug notice that
// would bury the diagnostic the partial state exists to deliver. app_type is the one
// such value here: it is Computed with no default, so it is Unknown in every create
// plan. The security cards are Optional and Computed but carry defaults, so the
// framework has already resolved them at plan time.
func nullUnknownReadBackValues(plan *ZtnaAppResourceModel) {
	if plan.AppType.IsUnknown() {
		plan.AppType = types.StringNull()
	}
}

// Read refreshes the Terraform state with the latest application representation.
//
// On import the security blocks come back empty, and deliberately so: each one is
// Optional-only and the read path fills a card only when state already carries it,
// so an imported application starts with no security block and the operator adopts
// the cards they want to manage. The server always holds all three, and adopting
// them all would put values in state the configuration never wrote.
func (r *ZtnaAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ZtnaAppResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this access policy application without existing state or "+
					"identity data, so the provider cannot determine which application to read.",
			)
			return
		}
		var identity ztnaAppIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing application ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the "+
					"access policy application.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(ztnaAppTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Security Cloud access policy application without ID.")
		return
	}

	got, err := r.client.GetZtnaAppV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud access policy application not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ztnaAppIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud access policy application", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignAppResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ztnaAppIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Security Cloud ZTNA access policy application.
//
// The update returns no body, so the application is read back — the same reason
// Create does.
func (r *ZtnaAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ZtnaAppResourceModel
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

	if plan.ID.IsNull() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Security Cloud access policy application without ID.")
		return
	}

	input, inputDiags := buildAppPatchInput(ctx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateZtnaAppV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err, !plan.PredefinedAppID.IsNull()) {
			resp.Diagnostics.AddError("Error updating Jamf Security Cloud access policy application", helpers.APIErrorDetail(err))
		}
		return
	}

	got, err := r.client.GetZtnaAppV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Security Cloud access policy application", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAppResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ztnaAppIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Security Cloud ZTNA access policy application.
//
// Nothing in Jamf Security Cloud references an application, so unlike a gateway
// there is no propagation-blocked case to translate — a delete either succeeds or
// the application was already gone.
func (r *ZtnaAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ZtnaAppResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Security Cloud access policy application without ID.")
		return
	}

	if err := r.client.DeleteZtnaAppV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud access policy application already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Security Cloud access policy application", helpers.APIErrorDetail(err))
	}
}
