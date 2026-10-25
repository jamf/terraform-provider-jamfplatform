// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ModifyPlan decides whether the icon needs re-uploading, and leaves
// icon_source_hash unresolved on every plan that uploads one.
//
// Unlike the icon and branding-image stores, a customization has a real update
// endpoint, so a new icon is delivered in place rather than by replacement.
// icon_source_hash is what tells Update to upload: a plan value that differs
// from state makes it re-upload, and the two Computed values the upload settles
// — the hash and branding_settings.icon_url — go Unknown together so apply can
// supply both.
//
// Where the hash comes from depends on the source, because the two differ in
// whether two reads of the same source answer with the same bytes:
//
//   - A create leaves icon_source_hash Unknown. Create hashes the exact bytes
//     it uploads, so a source that answers two reads differently can no longer
//     plan one value and apply another (issue #373, reproduced on this resource
//     on 2026-09-04). A local path is still opened and hashed here and the hash
//     discarded, so an unreadable one fails at plan rather than part-way
//     through an apply.
//   - A local path on an existing customization is read and hashed here, and a
//     differing hash re-uploads. Local bytes are stable, so the operator sees
//     the re-upload in terraform plan.
//   - An http(s):// URL on an existing customization is not fetched here.
//     Remote content is not stable between reads, so hashing it would propose
//     an upload on plans where nothing had changed. The URL string is compared
//     instead, which does not detect content published behind an unchanged URL.
//
// A plan that leaves icon_source unset, or carries it as unknown, returns
// before any of that: prior state's hash then carries forward and Update
// uploads nothing.
//
// The destroy path returns before touching the source; a destroy needs no bytes.
func (r *EnrollmentCustomizationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	r.reportNameCollision(ctx, req, resp)

	var plan EnrollmentCustomizationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.IconSource.IsNull() || plan.IconSource.IsUnknown() || plan.IconSource.ValueString() == "" {
		return
	}

	source := plan.IconSource.ValueString()
	localSource := !files.URLSource(source)

	var hash string
	if localSource {
		hashed, err := files.HashLocalSource(ctx, source)
		if err != nil {
			resp.Diagnostics.AddError("Error reading enrollment customization icon source during plan", helpers.APIErrorDetail(err))
			return
		}
		hash = hashed
	}

	if req.State.Raw.IsNull() {
		return
	}

	var state EnrollmentCustomizationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if localSource {
		if hash == state.IconSourceHash.ValueString() {
			return
		}
		planIconUpload(ctx, resp)
		return
	}

	if source == state.IconSource.ValueString() {
		return
	}
	planIconUpload(ctx, resp)
}

// planIconUpload marks the two values an upload settles as unresolved, which is
// both the signal Update reads to re-upload and what lets apply commit the hash
// of the bytes it actually sent.
//
// icon_source_hash goes Unknown rather than carrying the hash computed at plan
// time, so that the value committed to state is always one Update read off the
// upload. Attribute plan modifiers run before this, so the Unknown wins over
// UseStateForUnknown.
func planIconUpload(ctx context.Context, resp *resource.ModifyPlanResponse) {
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("icon_source_hash"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("branding_settings").AtName("icon_url"), types.StringUnknown())...)
}
