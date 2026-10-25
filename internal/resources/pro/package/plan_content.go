// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
)

// planUploadedContentChange leaves every attribute Jamf Pro derives from the
// uploaded bytes Unknown when the local file behind package_file_source no
// longer matches the digest recorded in state.
//
// resetIfSourceChangedString watches the source PATH, and a path is unchanged
// when the file at it is replaced in place — the usual way a package is
// updated. Without this the plan carried the old hashes forward, Update
// re-uploaded (it compares digests itself), Jamf Pro recomputed them, and
// Terraform refused the result as inconsistent. A change to the file alone
// planned nothing at all, because no attribute differed.
//
// Only a local path is hashed. A URL source is fetched during apply, where the
// bytes hashed are the bytes uploaded; fetching it here would download the
// binary on every plan. Content changing behind an unchanged URL is therefore
// not detected at plan time — Update still re-uploads when it runs for another
// reason.
//
// The values are left Unknown rather than set to the local digest: the server's
// figure after the upload is the truth, and a file edited between plan and apply
// would otherwise reproduce the same inconsistency.
func planUploadedContentChange(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var source types.String
	if diags := req.Plan.GetAttribute(ctx, path.Root("package_file_source"), &source); diags.HasError() {
		return
	}
	if !isConfiguredString(source) || files.URLSource(source.ValueString()) {
		return
	}
	var stateSha3, stateSha256, stateMD5, stateSize types.String
	for name, dst := range map[string]*types.String{"sha3_512": &stateSha3, "sha256": &stateSha256, "md5": &stateMD5, "size": &stateSize} {
		if diags := req.State.GetAttribute(ctx, path.Root(name), dst); diags.HasError() {
			return
		}
	}

	file, _, cleanup, err := files.OpenUploadSource(ctx, source.ValueString(), files.DefaultMaxBytes)
	if err != nil {
		tflog.Debug(ctx, "package content check skipped: source not readable at plan time", map[string]any{"err": err.Error()})
		return
	}
	defer cleanup()

	local, localSize, err := hashStream(file)
	if err != nil {
		tflog.Debug(ctx, "package content check skipped: source not hashable at plan time", map[string]any{"err": err.Error()})
		return
	}
	recorded := fileDigests{sha3: stateSha3.ValueString(), sha256: stateSha256.ValueString(), md5: stateMD5.ValueString()}
	if recorded.sameContent(local, stateSize.ValueString(), localSize) {
		return
	}

	var diags diag.Diagnostics
	for _, name := range []string{"sha3_512", "sha256", "md5", "hash_value", "size"} {
		diags.Append(resp.Plan.SetAttribute(ctx, path.Root(name), types.StringUnknown())...)
	}
	resp.Diagnostics.Append(diags...)
}
