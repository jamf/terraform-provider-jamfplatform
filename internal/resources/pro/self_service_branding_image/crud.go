// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.UploadBrandingImageV1
//   pro.DownloadBrandingImageV1
//
// Not adopted: there is no branding-image metadata GET, update, or delete
// endpoint. Read confirms existence via the byte download; Update is a
// refresh-only no-op (content changes route through RequiresReplace); Delete
// removes from Terraform state only.
//
// Status: current. Last reviewed 2026-06-10.

package self_service_branding_image

import (
	"context"
	"io"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create uploads an image, derives its ID from the returned URL, and records the
// hash of the bytes it uploaded.
//
// The source is hashed by streaming it once and then rewound for the upload, so
// source_hash always describes what was actually sent without the bytes being
// buffered. The rewound *os.File stays an io.Seeker, which is what lets the SDK
// rewind it and retry a 429; handing it a plain reader would forfeit the
// retry. Computing the hash at plan time instead meant reading the source
// twice, and a source that answers two reads with different bytes then planned
// one hash and applied another, which Terraform rejects as an inconsistent plan
// (issue #373).
func (r *SelfServiceBrandingImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServiceBrandingImageResourceModel
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

	file, filename, cleanup, err := files.OpenUploadSource(createCtx, plan.ImageFileSource.ValueString(), files.DefaultMaxBytes)
	if err != nil {
		resp.Diagnostics.AddError("Error opening branding image source", helpers.APIErrorDetail(err))
		return
	}
	defer cleanup()

	hash, err := files.HashStreamSHA256(file)
	if err != nil {
		resp.Diagnostics.AddError("Error reading branding image source", helpers.APIErrorDetail(err))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		resp.Diagnostics.AddError("Error rewinding branding image source", helpers.APIErrorDetail(err))
		return
	}

	uploaded, err := r.client.UploadBrandingImageV1(createCtx, filename, file)
	if err != nil {
		resp.Diagnostics.AddError("Error uploading Jamf Pro branding image", helpers.APIErrorDetail(err))
		return
	}

	if err := assignUploadedImage(&plan, uploaded); err != nil {
		resp.Diagnostics.AddError("Error processing branding image upload response", helpers.APIErrorDetail(err))
		return
	}
	plan.SourceHash = types.StringValue(hash)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceBrandingImageIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro Self Service branding image", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read confirms the image still exists by downloading its bytes (the only
// branding-image GET available). A NotFound removes the resource from state.
// On import (no source_hash in state) it computes the canonical hash from the
// downloaded bytes so subsequent plans are stable.
func (r *SelfServiceBrandingImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SelfServiceBrandingImageResourceModel
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// If state didn't supply an ID, fall back to identity (import path).
	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this branding image without existing state or identity data, so the provider cannot determine which image to read.",
			)
			return
		}
		var identity selfServiceBrandingImageIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing branding image ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the branding image.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(selfServiceBrandingImageTimeoutAttributeTypes)
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	data, err := r.client.DownloadBrandingImageV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro branding image not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro branding image", helpers.APIErrorDetail(err))
		return
	}

	// Populate source_hash when missing (post-import / recovery). On normal
	// refresh the hash is already set and we keep it.
	if state.SourceHash.IsNull() || state.SourceHash.IsUnknown() || state.SourceHash.ValueString() == "" {
		state.SourceHash = types.StringValue(files.ComputeContentSHA256(data))
		tflog.Info(ctx, "branding image Read populated source_hash from server bytes", map[string]any{
			"id":          state.ID.ValueString(),
			"source_hash": state.SourceHash.ValueString(),
		})
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceBrandingImageIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a refresh-only no-op. Content changes route through replacement via
// ModifyPlan's RequiresReplace (Jamf Pro has no branding-image update
// endpoint). Update only runs for in-place diffs that do NOT change
// source_hash — e.g. after import when the user assigns image_file_source to a
// local path whose bytes match the imported hash. The computed id/url/hash are
// carried forward by UseStateForUnknown, so Update just persists the plan.
func (r *SelfServiceBrandingImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServiceBrandingImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceBrandingImageIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the branding image from Terraform state only. No delete
// endpoint exists; the image record persists on the tenant.
func (r *SelfServiceBrandingImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SelfServiceBrandingImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Warn(ctx, "jamfplatform_pro_self_service_branding_image delete: no delete API; image removed from state only",
		map[string]any{"id": state.ID.ValueString()})
}
