// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// A directory binding is a policy dependency: no scope of its own, but every policy
// referencing it delivers it to that policy's audience.
func (r *DirectoryBindingResource) identifyDependency(_ context.Context, m *DirectoryBindingResourceModel) (id string, name string) {
	if m == nil {
		return "", ""
	}
	return m.ID.ValueString(), m.Name.ValueString()
}

var _ resource.ResourceWithModifyPlan = &DirectoryBindingResource{}

// ModifyPlan reports how many computers this directory binding reaches through the
// policies using it.
func (r *DirectoryBindingResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
	impact.ReportDependencyPlan(ctx, req, resp, impact.DependencyPlanReport{
		Cache: r.impact,
		// Unanchored: any change reaches every policy using it, so no one attribute
		// the figure derives from.
		Path: path.Empty(),
		Kind: impact.DependencyDirectoryBinding,
	}, r.identifyDependency)
}
