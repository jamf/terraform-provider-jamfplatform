// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package class

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignClassResourceModel populates a resource model from a Class response.
// Server is authoritative; the username sets preserve the configured casing
// (Jamf Pro canonicalises usernames) and every membership collection preserves
// the prior null-vs-empty shape when the server returns nothing.
func assignClassResourceModel(ctx context.Context, state *ClassResourceModel, c *proclassic.Class) diag.Diagnostics {
	var diags diag.Diagnostics
	if c == nil {
		return diags
	}

	if c.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(c.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(c.Name)
	state.Description = helpers.ReconcileOptionalStringPointer(c.Description, state.Description)
	state.Source = helpers.StringPointerValueOrNull(c.Source)

	siteID, siteName := scope.FlattenSiteObject(c.Site)
	state.SiteID = helpers.ReconcileOptionalStringPointer(siteID, state.SiteID)
	state.SiteName = helpers.StringPointerValueOrNull(siteName)

	var d diag.Diagnostics
	state.Students, d = reconcileStringSet(ctx, studentUsernames(c), state.Students, true)
	diags.Append(d...)
	state.Teachers, d = reconcileStringSet(ctx, teacherUsernames(c), state.Teachers, true)
	diags.Append(d...)
	state.StudentGroupIDs, d = reconcileStringSet(ctx, studentGroupIDs(c), state.StudentGroupIDs, false)
	diags.Append(d...)
	state.TeacherGroupIDs, d = reconcileStringSet(ctx, teacherGroupIDs(c), state.TeacherGroupIDs, false)
	diags.Append(d...)
	state.MobileDeviceGroupIDs, d = reconcileStringSet(ctx, mobileDeviceGroupIDs(c), state.MobileDeviceGroupIDs, false)
	diags.Append(d...)

	state.StudentIDs, d = computedIDSet(ctx, studentIDInts(c))
	diags.Append(d...)
	state.TeacherIDs, d = computedIDSet(ctx, teacherIDInts(c))
	diags.Append(d...)

	return diags
}

// assignClassDataSourceModel populates a data source model from a Class response.
// Read-only: every membership collection is surfaced directly (null when empty),
// with no reconcile against a prior value.
func assignClassDataSourceModel(ctx context.Context, state *ClassDataSourceModel, c *proclassic.Class) diag.Diagnostics {
	var diags diag.Diagnostics
	if c == nil {
		return diags
	}

	if c.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(c.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(c.Name)
	state.Description = helpers.StringPointerValueOrNull(c.Description)
	state.Source = helpers.StringPointerValueOrNull(c.Source)

	siteID, siteName := scope.FlattenSiteObject(c.Site)
	state.SiteID = helpers.StringPointerValueOrNull(siteID)
	state.SiteName = helpers.StringPointerValueOrNull(siteName)

	var d diag.Diagnostics
	state.Students, d = stringSetOrNull(ctx, studentUsernames(c))
	diags.Append(d...)
	state.Teachers, d = stringSetOrNull(ctx, teacherUsernames(c))
	diags.Append(d...)
	state.StudentGroupIDs, d = stringSetOrNull(ctx, studentGroupIDs(c))
	diags.Append(d...)
	state.TeacherGroupIDs, d = stringSetOrNull(ctx, teacherGroupIDs(c))
	diags.Append(d...)
	state.MobileDeviceGroupIDs, d = stringSetOrNull(ctx, mobileDeviceGroupIDs(c))
	diags.Append(d...)
	state.StudentIDs, d = computedIDSet(ctx, studentIDInts(c))
	diags.Append(d...)
	state.TeacherIDs, d = computedIDSet(ctx, teacherIDInts(c))
	diags.Append(d...)

	return diags
}

// stringSetOrNull builds a set from values, returning a null set when empty.
func stringSetOrNull(ctx context.Context, values []string) (types.Set, diag.Diagnostics) {
	if len(values) == 0 {
		return types.SetNull(types.StringType), nil
	}
	return types.SetValueFrom(ctx, types.StringType, values)
}

// --- inner-collection accessors (nil-safe) ---

func studentUsernames(c *proclassic.Class) []string {
	if c.Students == nil {
		return nil
	}
	return derefStringSlice(c.Students.Student)
}

func teacherUsernames(c *proclassic.Class) []string {
	if c.Teachers == nil {
		return nil
	}
	return derefStringSlice(c.Teachers.Teacher)
}

func studentGroupIDs(c *proclassic.Class) []string {
	if c.StudentGroupIds == nil {
		return nil
	}
	return idStringsFromIntSlice(c.StudentGroupIds.ID)
}

func teacherGroupIDs(c *proclassic.Class) []string {
	if c.TeacherGroupIds == nil {
		return nil
	}
	return idStringsFromIntSlice(c.TeacherGroupIds.ID)
}

func mobileDeviceGroupIDs(c *proclassic.Class) []string {
	if c.MobileDeviceGroupIds == nil {
		return nil
	}
	return idStringsFromIntSlice(c.MobileDeviceGroupIds.ID)
}

func studentIDInts(c *proclassic.Class) *[]int {
	if c.StudentIds == nil {
		return nil
	}
	return c.StudentIds.ID
}

func teacherIDInts(c *proclassic.Class) *[]int {
	if c.TeacherIds == nil {
		return nil
	}
	return c.TeacherIds.ID
}
