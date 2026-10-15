// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// A package is a policy dependency: no scope of its own, but every policy
// referencing it delivers it to that policy's audience.
func (r *PackageResource) identifyDependency(_ context.Context, m *PackageResourceModel) (id string, name string) {
	if m == nil {
		return "", ""
	}
	return m.ID.ValueString(), m.DisplayName.ValueString()
}

var _ resource.ResourceWithModifyPlan = &PackageResource{}

// ModifyPlan marks the hash attributes Unknown when the uploaded file's bytes
// changed (see planUploadedContentChange), then reports how many computers this
// package reaches through the policies using it.
func (r *PackageResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUploadedContentChange(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	impact.ReportDependencyPlan(ctx, req, resp, impact.DependencyPlanReport{
		Cache: r.impact,
		// Unanchored: any change reaches every policy using it, so no one attribute
		// the figure derives from.
		Path: path.Empty(),
		Kind: impact.DependencyPackage,
	}, r.identifyDependency)
}
