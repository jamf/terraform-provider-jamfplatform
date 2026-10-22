// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetCloudDistributionPointV1
//   pro.CreateCloudDistributionPointV1
//   pro.UpdateCloudDistributionPointV1
//   pro.DeleteCloudDistributionPointV1
//
// Status: current. Last reviewed 2026-05-29.

package cloud_distribution_point

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create enables the cloud distribution point (POST — the server's None→type
// transition). The API rejects POST when a cloud distribution point is already
// configured ("<TYPE> is already configured."); that case is mapped to an
// import-guidance error.
func (r *CloudDistributionPointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, config CloudDistributionPointResourceModel
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

	input, err := buildCloudDistributionPointInput(plan, config)
	if err != nil {
		resp.Diagnostics.AddError("Invalid cloud distribution point configuration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.CreateCloudDistributionPointV1(createCtx, input)
	if err != nil {
		if isAlreadyConfiguredError(err) {
			resp.Diagnostics.AddError(
				"Cloud distribution point already configured",
				"A cloud distribution point is already configured on this Jamf Pro tenant, so it cannot be created. "+
					"Import the existing object instead:\n\n"+
					"  terraform import jamfplatform_pro_cloud_distribution_point.<name> singleton\n\n"+
					"Original error: "+helpers.APIErrorDetail(err),
			)
			return
		}
		resp.Diagnostics.AddError("Error creating Jamf Pro cloud distribution point", helpers.APIErrorDetail(err))
		return
	}

	// The server makes a sole cloud distribution point principal on create and
	// ignores a configured master = false, so it reads back true. A PATCH sets the
	// configured value (wire-verified 2026-10-22: PATCH master=false on the sole
	// JCDS point reads back false), so state matches config.
	if !plan.Master.IsNull() && !plan.Master.IsUnknown() && (got.Master == nil || *got.Master != plan.Master.ValueBool()) {
		master := plan.Master.ValueBool()
		patched, err := r.client.UpdateCloudDistributionPointV1(createCtx, &pro.CloudDistributionPoint{
			CdnType:  got.CdnType,
			Username: got.Username,
			Master:   &master,
		})
		if err != nil {
			resp.Diagnostics.AddError("Error setting master on new cloud distribution point", helpers.APIErrorDetail(err))
			return
		}
		got = patched
	}

	assignCloudDistributionPointResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, cloudDistributionPointIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro cloud distribution point")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state from the API. The GET always returns HTTP 200; a
// cdn_type of "NONE" means the cloud distribution point has been disabled
// (destroyed or drifted away), so the resource is removed from state.
func (r *CloudDistributionPointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state CloudDistributionPointResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(cloudDistributionPointTimeoutAttributeTypes)
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

	got, err := r.client.GetCloudDistributionPointV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro cloud distribution point", helpers.APIErrorDetail(err))
		return
	}

	if strings.EqualFold(got.CdnType, cdnTypeNone) {
		tflog.Trace(ctx, "cloud distribution point is NONE (disabled); removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	assignCloudDistributionPointResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, cloudDistributionPointIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies a PATCH (merge-patch). cdn_type is RequiresReplace, so any
// type change routes through Delete+Create instead — Update only ever mutates
// fields within the existing type. cdn_type is nevertheless mandatory in every
// PATCH body (the API rejects a body without it), so the input builder always
// emits it.
func (r *CloudDistributionPointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, config CloudDistributionPointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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

	input, err := buildCloudDistributionPointInput(plan, config)
	if err != nil {
		resp.Diagnostics.AddError("Invalid cloud distribution point configuration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.UpdateCloudDistributionPointV1(updateCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro cloud distribution point", helpers.APIErrorDetail(err))
		return
	}

	assignCloudDistributionPointResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, cloudDistributionPointIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Pro cloud distribution point")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete disables the cloud distribution point (DELETE → cdn_type "NONE"). This
// is a real remote operation, not a state-only removal: destroying the resource
// turns off cloud distribution for the tenant.
func (r *CloudDistributionPointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state CloudDistributionPointResourceModel
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

	if err := r.client.DeleteCloudDistributionPointV1(deleteCtx); err != nil {
		// An already-absent cloud distribution point is the delete's objective.
		// The transport no longer treats DELETE→404 as success, so handle it here.
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro cloud distribution point already removed")
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro cloud distribution point", helpers.APIErrorDetail(err))
		return
	}
	tflog.Trace(ctx, "deleted (disabled) Jamf Pro cloud distribution point")
}

// isAlreadyConfiguredError reports whether a Create error is the server's
// "<TYPE> is already configured." rejection (HTTP 400, code INVALID_FIELD),
// which signals the singleton already exists and should be imported.
func isAlreadyConfiguredError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "is already configured")
}
