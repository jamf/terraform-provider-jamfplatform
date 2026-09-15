// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.UploadIconV1
//   pro.GetIconV1
//   pro.DownloadIconV1
//
// Status: current. Last reviewed 2026-05-24.

package icon

import (
	"bytes"
	"context"
	"io"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create uploads an icon to Jamf Pro and stores the resulting ID and URL in
// state, along with the hash of the bytes it uploaded.
//
// The source is hashed by streaming it once and then rewound for the upload, so
// source_hash always describes what was actually sent without the bytes being
// buffered. The rewound *os.File stays an io.Seeker, which is what lets the SDK
// precompute Content-Length and retry a 429; handing it a plain reader would
// forfeit both. Computing the hash at plan time instead meant reading the source
// twice, and a source that answers two reads with different bytes — Apple's
// iTunes artwork CDN does — then planned one hash and applied another, which
// Terraform rejects as an inconsistent plan (issue #373).
func (r *IconResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IconResourceModel
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

	file, filename, cleanup, err := files.OpenUploadSource(createCtx, plan.IconFileSource.ValueString(), files.DefaultMaxBytes)
	if err != nil {
		resp.Diagnostics.AddError("Error opening icon source", helpers.APIErrorDetail(err))
		return
	}
	defer cleanup()

	hash, err := files.HashStreamSHA256(file)
	if err != nil {
		resp.Diagnostics.AddError("Error reading icon source", helpers.APIErrorDetail(err))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		resp.Diagnostics.AddError("Error rewinding icon source", helpers.APIErrorDetail(err))
		return
	}

	iconResp, err := r.client.UploadIconV1(createCtx, filename, file)
	if err != nil {
		resp.Diagnostics.AddError("Error uploading Jamf Pro icon", helpers.APIErrorDetail(err))
		return
	}

	assignIconResourceModel(&plan, iconResp)
	plan.SourceHash = types.StringValue(hash)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, iconIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro icon", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest icon representation.
//
// Reading state:
//   - Normal refresh: load state from req.State.
//   - Import: req.State.Raw may be null, in which case the resource ID is
//     read from req.Identity. We do not rely on Raw.IsNull() alone — the
//     Plugin Framework's import flow can populate req.State with the ID
//     before calling Read, leaving Raw non-null but other attributes null.
//
// After loading, GetIconV1 refreshes the URL. State missing a source_hash then
// has one computed from the icon's own bytes, downloaded from the tenant. That
// covers the first Read after an import, which arrives with an ID and nothing
// else, and recovery from a state file that lost the value. An ordinary refresh
// already holds the hash and downloads nothing.
//
// icon_file_source cannot be recovered this way — a path or URL is not
// something a refresh can infer — so whatever state held stays, which is null
// after an import and a user's source on every refresh after that.
func (r *IconResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IconResourceModel

	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if state.ID.IsNull() || state.ID.IsUnknown() || state.ID.ValueString() == "" {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this icon without existing state or identity data, so the provider cannot determine which icon to read.",
			)
			return
		}
		var identity iconIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing icon ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the icon.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(iconTimeoutAttributeTypes)
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro icon without ID.")
		return
	}

	iconResp, err := r.client.GetIconV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro icon not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro icon", helpers.APIErrorDetail(err))
		return
	}

	assignIconResourceModel(&state, iconResp)

	if state.SourceHash.IsNull() || state.SourceHash.IsUnknown() || state.SourceHash.ValueString() == "" {
		data, dlErr := downloadIconBytes(readCtx, r.client, state.ID.ValueString(), iconResp.URL)
		if dlErr != nil {
			tflog.Warn(ctx, "icon Read: could not compute source_hash from server", map[string]any{
				"id":  state.ID.ValueString(),
				"err": dlErr.Error(),
			})
			state.SourceHash = types.StringNull()
		} else {
			state.SourceHash = types.StringValue(files.ComputeContentSHA256(data))
			tflog.Info(ctx, "icon Read populated source_hash from server bytes", map[string]any{
				"id":          state.ID.ValueString(),
				"source_hash": state.SourceHash.ValueString(),
			})
		}
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, iconIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update refreshes state without touching the icon. Jamf Pro has no icon update
// endpoint, so a new image only ever arrives by replacement, and Update is left
// with the diffs that leave the image alone: re-pointing icon_file_source at
// byte-identical content, and the first apply after an import, which gives state
// a source for the first time.
func (r *IconResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IconResourceModel
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

	got, err := r.client.GetIconV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro icon", helpers.APIErrorDetail(err))
		return
	}
	assignIconResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, iconIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro icon from Terraform state.
//
// No DeleteIconV1 endpoint exists. The icon record persists on the tenant;
// this operation removes it from Terraform state only.
func (r *IconResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Warn(ctx, "jamfplatform_pro_icon delete: no DeleteIconV1 API; icon removed from state only",
		map[string]any{"id": state.ID.ValueString()})
}

// downloadIconBytes fetches the raw bytes of a Jamf Pro icon. It tries
// iconURL (CDN download via files.DownloadCapped) first; if that fails or
// the URL is empty it falls back to DownloadIconV1 (/v1/icon/download/{id}),
// which may return HTTP 500 on some tenants.
func downloadIconBytes(ctx context.Context, client *pro.Client, id, iconURL string) ([]byte, error) {
	if iconURL != "" {
		var buf bytes.Buffer
		if _, _, err := files.DownloadCapped(ctx, iconURL, &buf, files.DefaultMaxBytes); err == nil {
			return buf.Bytes(), nil
		}
	}
	return client.DownloadIconV1(ctx, id, 0, "")
}
