// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   account.CreateEnvironment
//   account.ListEnvironments (read, update refresh, failure re-check, data sources, list resource)
//   account.UpdateEnvironment
//   account.DeleteEnvironment
//   account.ListTenants      (delete: refuse an environment that still holds tenants)
//
// Deliberately absent: account.AssignTenants. Assigning tenants cannot be undone
// and cannot be read back from the environment, so it belongs to the
// jamfplatform_account_platform_environment_assign_tenants action rather than to
// this resource's lifecycle.
//
// There is no read by identifier, and a request naming an environment that no
// longer exists answers 500 UPSTREAM_ERROR where the specification declares 404
// (wire-probed 2026-10-22: DELETE and PUT on an already-deleted id both did).
// IsNotFoundError therefore never matches, and Update and Delete decide whether
// the environment is gone by listing the collection after a failure.
//
// Status: current. Last reviewed 2026-10-22.

package platform_environment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accounttenant"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create makes a platform environment.
//
// The create response carries the whole stored representation, so there is no
// read-back.
func (r *PlatformEnvironmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PlatformEnvironmentResourceModel
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

	created, err := r.client.CreateEnvironment(createCtx, buildCreateRequest(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Account platform environment", helpers.APIErrorDetail(err))
		return
	}

	assignResourceModel(&plan, created)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, platformEnvironmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Account platform environment", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest representation of the
// platform environment.
//
// The lookup is a scan of the organization's collection matched on the
// identifier, because Jamf Account exposes no read of a single environment. The
// identity branch serves the identity-only refresh Terraform performs when it
// holds an identity and no state. The ordinary branch serves both a normal
// refresh and the read that follows a `terraform import`, where the identifier
// has already been written into state, so nothing here is gated on which path
// it arrived by.
func (r *PlatformEnvironmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PlatformEnvironmentResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform asked for a refresh without state or identity, so the provider cannot tell which platform environment to read.",
			)
			return
		}
		var identity platformEnvironmentIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing platform environment identifier",
				"The resource identity has no 'id', so the provider cannot refresh the platform environment.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(platformEnvironmentTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing platform environment identifier", "Cannot read a Jamf Account platform environment without an identifier.")
		return
	}

	environments, err := r.client.ListEnvironments(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Account platform environments", helpers.APIErrorDetail(err))
		return
	}

	found := findEnvironmentByID(environments, state.ID.ValueString())
	if found == nil {
		tflog.Info(ctx, "Jamf Account platform environment not found, removing from state", map[string]any{"id": state.ID.ValueString()})
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, platformEnvironmentIdentityModel{ID: state.ID})...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.State.RemoveResource(ctx)
		return
	}

	assignResourceModel(&state, found)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, platformEnvironmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update replaces the platform environment's name and type.
//
// The write is a full replacement of those two fields and answers no body, so
// the collection is read afterwards to adopt what Jamf Account stored. A failed
// write is followed by the same read: an environment deleted out from under
// Terraform makes the write answer 500 rather than 404, and the list is what
// tells that apart from a genuine failure. A missing environment cannot be
// dropped from state during an update, so the diagnostic points at a refresh.
func (r *PlatformEnvironmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PlatformEnvironmentResourceModel
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

	id := plan.ID.ValueString()

	if err := r.client.UpdateEnvironment(updateCtx, id, buildUpdateRequest(plan)); err != nil {
		environments, listErr := r.client.ListEnvironments(updateCtx)
		if listErr == nil && findEnvironmentByID(environments, id) == nil {
			resp.Diagnostics.AddError(
				"Jamf Account platform environment no longer exists",
				"The platform environment \""+id+"\" is no longer in this organization, so Terraform cannot update "+
					"it. Run `terraform apply -refresh-only` to drop it from state, then apply to create it again.",
			)
			return
		}
		resp.Diagnostics.AddError("Error updating Jamf Account platform environment", helpers.APIErrorDetail(err))
		return
	}

	environments, err := r.client.ListEnvironments(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Account platform environments", helpers.APIErrorDetail(err))
		return
	}
	found := findEnvironmentByID(environments, id)
	if found == nil {
		resp.Diagnostics.AddError(
			"Jamf Account platform environment no longer exists",
			"Jamf Account accepted the update, but the platform environment is no longer listed. Run "+
				"`terraform apply -refresh-only` to drop it from state.",
		)
		return
	}

	assignResourceModel(&plan, found)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, platformEnvironmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the platform environment.
//
// A platform environment that still holds tenants cannot be deleted, and
// tenants cannot be taken out of one, so the tenant list is read first and a
// non-empty answer refuses the destroy with the tenants named. A failed delete
// is followed by a read of the collection: an environment that is already gone
// answers 500 rather than 404, so absence from the list is what counts as
// success. Anything still listed after a failure is a genuine error.
func (r *PlatformEnvironmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PlatformEnvironmentResourceModel
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

	id := state.ID.ValueString()

	tenants, err := r.client.ListTenants(deleteCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Account tenants", helpers.APIErrorDetail(err))
		return
	}
	if appendHeldTenantsDiagnostics(&resp.Diagnostics, state.Name.ValueString(), accounttenant.InEnvironment(tenants, id)) {
		return
	}

	if err := r.client.DeleteEnvironment(deleteCtx, id); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Account platform environment already deleted", map[string]any{"id": id})
			return
		}
		environments, listErr := r.client.ListEnvironments(deleteCtx)
		if listErr == nil && findEnvironmentByID(environments, id) == nil {
			tflog.Info(ctx, "Jamf Account platform environment already deleted", map[string]any{"id": id})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Account platform environment", helpers.APIErrorDetail(err))
	}
}
