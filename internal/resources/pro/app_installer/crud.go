// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateAppInstallerDeploymentV1 (POST /v1/app-installers/deployments → {href,id})
//   pro.GetAppInstallerDeploymentV1    (GET  /v1/app-installers/deployments/{id})
//   pro.UpdateAppInstallerDeploymentV1 (PUT  /v1/app-installers/deployments/{id})
//   pro.DeleteAppInstallerDeploymentV1 (DELETE)
//   pro.ListAppInstallerTitlesV1       (whole catalog: name → id and id → name, cached)
//   pro.ListAppInstallerDeploymentsV1  (list resource / plural data source / name lookup / collision check in collision.go)
//
// Uniqueness: name, not enforced; duplicates stored; checked only with
// include_unenforced. The strict comparison ignores case, trailing spaces and
// accents.
//
// Status: current. Last reviewed 2026-09-03.
//
// Create returns only {href,id}; state is built from a follow-up GET (mirrors
// blueprint / user_initiated_enrollment_settings). The notificationSettings and
// selfServiceSettings nested blocks are FULL-REPLACE — any field omitted from
// the request body is reset to the server default — so the input builder always
// emits a complete block when one is managed. The catalog title is referenced by
// name: Create/Update resolve app_title_name → id, and Read reverse-resolves the
// id back to the name (the GET returns only the id). Both directions answer from
// one unfiltered catalog list cached per provider instance, and the forward match
// is decided on byte equality because Jamf Pro's own name filter is a
// case-insensitive glob — see name_lookup.go. selected_version is
// Computed-only: the server derives it from update_behavior, answering "" while
// the behaviour is AUTOMATIC and the current version once it is MANUAL.

package app_installer

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new App Installer deployment, then refreshes state from a
// follow-up GET (the POST returns only {href,id}).
func (r *AppInstallerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan AppInstallerResourceModel
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

	appTitleID, titleDiags := resolveAppTitleID(createCtx, catalogOrNil(r.titles), plan.AppTitleName.ValueString())
	resp.Diagnostics.Append(titleDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateAppInstallerDeploymentV1(createCtx, buildAppInstallerInput(plan, appTitleID))
	if err != nil {
		resp.Diagnostics.AddError("Error creating App Installer deployment", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing deployment ID",
			"Jamf Pro returned 201 Created with no deployment ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetAppInstallerDeploymentV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created App Installer deployment", helpers.APIErrorDetail(err))
		return
	}
	assignAppInstallerResourceModel(&plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created App Installer deployment", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest deployment representation.
func (r *AppInstallerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state AppInstallerResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this deployment without existing state or identity data, so the provider cannot determine which deployment to read.",
			)
			return
		}
		var identity appInstallerIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing deployment ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the deployment.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(appInstallerTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read App Installer deployment without ID.")
		return
	}

	got, err := r.client.GetAppInstallerDeploymentV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "App Installer deployment not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading App Installer deployment", helpers.APIErrorDetail(err))
		return
	}

	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): name is schema-Required and always
	// populated in genuinely managed state, so state.Name.IsNull() can only
	// mean this Read call is doing first-time import hydration. Hydrate the
	// wire-present optional notification_settings/self_service_settings
	// sections in that case; subsequent Reads revert to only refreshing
	// sections the current state already tracks.
	firstHydration := state.Name.IsNull()
	assignAppInstallerResourceModel(&state, got, firstHydration)

	// Reverse-resolve app_title_id → app_title_name. The deployment GET returns
	// only the ID, so the name has to come from the cached catalog snapshot, and
	// the two Read paths need opposite failure handling. A routine refresh keeps the name
	// already in state rather than failing over a transient catalog error. An
	// import has no such value to keep, and app_title_name is Required, so a
	// failed lookup there would write null into a Required attribute with no
	// diagnostic — reporting a successful import that later surfaces as an
	// unexplained in-place update and fails ImportStateVerify. So a failed
	// lookup is an error on the import path only.
	if name, ok := titleNameForID(readCtx, catalogOrNil(r.titles), state.AppTitleID.ValueString()); ok {
		state.AppTitleName = types.StringValue(name)
	} else if firstHydration {
		resp.Diagnostics.AddError(
			"Unable to resolve the App Catalog title name",
			fmt.Sprintf("Importing this deployment needs app_title_id %q resolved to its App Catalog title name, and the catalog lookup failed. "+
				"Retry once the catalog is reachable; if the title has been withdrawn from the App Catalog, the deployment cannot be managed by title name.",
				state.AppTitleID.ValueString()),
		)
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates an App Installer deployment, then refreshes state from a GET
// (PUT responses on Pro endpoints are routinely lossy for server-derived
// fields).
func (r *AppInstallerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AppInstallerResourceModel
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

	appTitleID, titleDiags := resolveAppTitleID(updateCtx, catalogOrNil(r.titles), plan.AppTitleName.ValueString())
	resp.Diagnostics.Append(titleDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateAppInstallerDeploymentV1(updateCtx, plan.ID.ValueString(), buildAppInstallerInput(plan, appTitleID)); err != nil {
		resp.Diagnostics.AddError("Error updating App Installer deployment", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetAppInstallerDeploymentV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated App Installer deployment", helpers.APIErrorDetail(err))
		return
	}
	assignAppInstallerResourceModel(&plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an App Installer deployment.
func (r *AppInstallerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AppInstallerResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete App Installer deployment without ID.")
		return
	}

	if err := r.client.DeleteAppInstallerDeploymentV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "App Installer deployment already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting App Installer deployment", helpers.APIErrorDetail(err))
	}
}
