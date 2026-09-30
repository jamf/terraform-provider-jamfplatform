// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.CreateZtnaGroupedGatewayV1
//   securitycloud.GetZtnaGroupedGatewayV1
//   securitycloud.UpdateZtnaGroupedGatewayV1
//   securitycloud.DeleteZtnaGroupedGatewayV1
//   securitycloud.ListZtnaGroupedGatewaysV1 (data sources / list resource)
//   securitycloud.ListUemConnectorsV1 (tenant_ids default, when unset)
//   securitycloud.ResolveZtnaGroupedGatewayV1ByName (singular data source, name lookup)
//
// Status: current. Last reviewed 2026-08-27.

package ztna_grouped_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/securitycloudtenant"
)

// Create creates a new Jamf Security Cloud ZTNA grouped gateway.
//
// A read-back that fails after the create has succeeded still commits state. The group
// exists on the tenant by then, and returning without state would leave it there
// unmanaged, with the retry building a second group over the same member gateways
// rather than converging. What is committed is the plan carrying the new ID — the
// configured values are what the next refresh reconciles, and an errored apply does
// not run Terraform's plan-consistency check. The one value only the read-back could
// have filled is nulled first — see nullUnknownReadBackValues.
func (r *GroupedGatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupedGatewayResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.TenantIDs.IsUnknown() {
		tenantIDs, tenantDiags := r.resolveTenantIDs(ctx)
		resp.Diagnostics.Append(tenantDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.TenantIDs = tenantIDs
	}

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	input, inputDiags := buildGroupedGatewayCreateInput(ctx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateZtnaGroupedGatewayV1(createCtx, input)
	if err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error creating Jamf Security Cloud ZTNA grouped gateway", helpers.APIErrorDetail(err))
		}
		return
	}

	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetZtnaGroupedGatewayV1(createCtx, created.ID)
	if err != nil {
		nullUnknownReadBackValues(&plan)
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, groupedGatewayIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Error reading created Jamf Security Cloud ZTNA grouped gateway",
			"The grouped gateway was created with ID \""+created.ID+"\" but could not be read back, so Terraform "+
				"has recorded its ID and the configured values without its creation timestamp. The next plan will "+
				"refresh it — do not re-create it: a second create would build another group over the same member "+
				"gateways and leave this one unmanaged. Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}
	resp.Diagnostics.Append(assignGroupedGatewayResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, groupedGatewayIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud ZTNA grouped gateway", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// nullUnknownReadBackValues nulls the values only the create read-back could have
// filled, so the partial state committed when it fails is wholly known.
//
// Terraform answers an unknown value in the state a failed apply returns with an
// "invalid result object after apply" error of its own — a provider-bug notice that
// would bury the diagnostic the partial state exists to deliver. created_at is the
// one such value here: it is Computed with no default, so it is Unknown in every
// create plan.
func nullUnknownReadBackValues(plan *GroupedGatewayResourceModel) {
	if plan.CreatedAt.IsUnknown() {
		plan.CreatedAt = types.StringNull()
	}
}

// Read refreshes the Terraform state with the latest grouped gateway
// representation.
func (r *GroupedGatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupedGatewayResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this grouped gateway without existing state or identity data, so the provider cannot determine which group to read.",
			)
			return
		}
		var identity groupedGatewayIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing grouped gateway ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the grouped gateway.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(groupedGatewayTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Security Cloud ZTNA grouped gateway without ID.")
		return
	}

	got, err := r.client.GetZtnaGroupedGatewayV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud ZTNA grouped gateway not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, groupedGatewayIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud ZTNA grouped gateway", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignGroupedGatewayResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, groupedGatewayIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Security Cloud ZTNA grouped gateway.
func (r *GroupedGatewayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan GroupedGatewayResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Security Cloud ZTNA grouped gateway without ID.")
		return
	}

	input, inputDiags := buildGroupedGatewayPatchInput(ctx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateZtnaGroupedGatewayV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error updating Jamf Security Cloud ZTNA grouped gateway", helpers.APIErrorDetail(err))
		}
		return
	}

	got, err := r.client.GetZtnaGroupedGatewayV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Security Cloud ZTNA grouped gateway", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignGroupedGatewayResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, groupedGatewayIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Security Cloud ZTNA grouped gateway.
func (r *GroupedGatewayResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupedGatewayResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Security Cloud ZTNA grouped gateway without ID.")
		return
	}

	if err := r.client.DeleteZtnaGroupedGatewayV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud ZTNA grouped gateway already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		if appendDeleteDiagnostics(&resp.Diagnostics, err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Security Cloud ZTNA grouped gateway", helpers.APIErrorDetail(err))
	}
}

// resolveTenantIDs returns the tenant_ids default: the Security Cloud tenant the provider's
// credentials act on, as a one-element set. See internal/common/securitycloudtenant for where the
// ID comes from and why nothing else is consulted.
//
// ModifyPlan calls this so the plan shows the resolved ID. Create calls it again only when the
// value is still unknown, which is when the provider was not yet configured at plan time.
func (r *GroupedGatewayResource) resolveTenantIDs(ctx context.Context) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	tenantID, err := securitycloudtenant.Resolve(ctx, r.scopeTenantID, func(ctx context.Context) (*securitycloud.ConnectorPage, error) {
		return r.client.ListUemConnectorsV1(ctx)
	})
	if err != nil {
		securitycloudtenant.AppendUnresolved(&diags, path.Root("tenant_ids"), err)
		return types.SetUnknown(types.StringType), diags
	}
	tenantIDs, setDiags := types.SetValueFrom(ctx, types.StringType, []string{tenantID})
	diags.Append(setDiags...)
	return tenantIDs, diags
}
