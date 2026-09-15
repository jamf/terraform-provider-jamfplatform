// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"maps"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// buildPatchSoftwareTitleCreateInput builds the minimal classic POST payload
// that mints a title. Per the verified wire semantics, name + name_id +
// source_id define the title; the server then populates the full versions
// catalog from the patch definition. Everything else is applied by the
// follow-up v3 merge-patch (see crud.go Create).
func buildPatchSoftwareTitleCreateInput(plan PatchSoftwareTitleResourceModel) *proclassic.PatchSoftwareTitle {
	out := &proclassic.PatchSoftwareTitle{}
	if v := plan.Name.ValueString(); !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		name := v
		out.Name = &name
	}
	if v := plan.NameID.ValueString(); !plan.NameID.IsNull() && !plan.NameID.IsUnknown() {
		nameID := v
		out.NameID = &nameID
	}
	if !plan.SourceID.IsNull() && !plan.SourceID.IsUnknown() {
		sourceID := int(plan.SourceID.ValueInt64())
		out.SourceID = &sourceID
	}
	return out
}

// buildPatchSoftwareTitleConfigurationPatch builds the v3 merge-patch body
// carrying the desired metadata (display name, category, site, notifications).
//
// packages carries the complete desired set of version→package assignments, or
// nil to omit the key entirely. The distinction is load-bearing: the v3
// `packages` array is a full replacement — a version absent from a supplied
// array has its assignment cleared, and an empty array clears every assignment
// — whereas omitting the key leaves the server's assignments untouched
// (wire-probed 2026-09-02). Callers therefore pass nil unless they have the
// live server set to build the replacement from; see unionVersionPackages.
func buildPatchSoftwareTitleConfigurationPatch(plan PatchSoftwareTitleResourceModel, packages map[string]string) *pro.PatchSoftwareTitleConfigurationPatch {
	out := &pro.PatchSoftwareTitleConfigurationPatch{}
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		name := plan.Name.ValueString()
		out.DisplayName = &name
	}
	out.CategoryID = refIDPtr(plan.CategoryID)
	out.SiteID = refIDPtr(plan.SiteID)
	if !plan.WebNotification.IsNull() && !plan.WebNotification.IsUnknown() {
		v := plan.WebNotification.ValueBool()
		out.UiNotifications = &v
	}
	if !plan.EmailNotification.IsNull() && !plan.EmailNotification.IsUnknown() {
		v := plan.EmailNotification.ValueBool()
		out.EmailNotifications = &v
	}
	if packages != nil {
		items := packageItems(packages)
		out.Packages = &items
	}
	return out
}

// refIDPtr maps a configured category or site id onto the v3 wire value, or nil
// so the merge-patch omits the field. The configurations endpoint accepts only
// a positive id or the literal "-1", rejecting anything else outright ("id
// field must be string of positive numeric value or -1"), so every non-positive
// id — including the "0" a title last written through classic
// /patchsoftwaretitles can still carry — is normalised to "-1".
func refIDPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	raw := v.ValueString()
	if raw == "" {
		return nil
	}
	if n, err := strconv.Atoi(raw); err == nil && n <= 0 {
		none := "-1"
		return &none
	}
	return &raw
}

// unionVersionPackages folds the desired version_packages over the live server
// assignments to produce the full replacement array the v3 `packages` field
// requires, preserving the resource's managed-subset contract: only the keys
// Terraform declares are managed, and an assignment made outside Terraform
// survives an apply.
//
// live is the server's current assignment set, planPackages the desired
// Terraform-managed subset, and priorKeys the keys recorded in prior state. A
// key that was in prior state but has been dropped from the plan is an explicit
// unassign, so it is removed from the union; every other live key is carried
// across untouched.
//
// That reading of priorKeys rests on an invariant the read path has to keep:
// every key in state is a key the configuration declared. A Read that hydrated
// undeclared assignments into state would turn each of them into an unassign on
// the first apply, which is #403 — see managedVersionPackages.
func unionVersionPackages(live, planPackages map[string]string, priorKeys []string) map[string]string {
	union := make(map[string]string, len(live)+len(planPackages))
	maps.Copy(union, live)
	for _, k := range priorKeys {
		if _, stillPlanned := planPackages[k]; !stillPlanned {
			delete(union, k)
		}
	}
	maps.Copy(union, planPackages)
	return union
}

// packageItems renders a version→package map as the v3 packages array, sorted
// by version for a deterministic payload.
func packageItems(packages map[string]string) []pro.PatchSoftwareTitlePackages {
	versions := make([]string, 0, len(packages))
	for v := range packages {
		versions = append(versions, v)
	}
	sort.Strings(versions)

	items := make([]pro.PatchSoftwareTitlePackages, 0, len(versions))
	for _, v := range versions {
		version, pkgID := v, packages[v]
		items = append(items, pro.PatchSoftwareTitlePackages{
			Version:   &version,
			PackageID: &pkgID,
		})
	}
	return items
}

// versionPackageMap decodes a model's version_packages attribute into a plain
// map. Returns an empty map for a null or unknown attribute.
func versionPackageMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, diags
	}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	if diags.HasError() {
		return nil, diags
	}
	return out, diags
}

// versionPackageKeys returns the declared version_packages keys from a model's
// map, used as the managed-subset key set for Read reconciliation and Update
// unassign-diffing. Returns nil for null/unknown maps.
func versionPackageKeys(ctx context.Context, m types.Map) ([]string, diag.Diagnostics) {
	elems, diags := versionPackageMap(ctx, m)
	if diags.HasError() {
		return nil, diags
	}
	if len(elems) == 0 {
		return nil, diags
	}
	keys := make([]string, 0, len(elems))
	for k := range elems {
		keys = append(keys, k)
	}
	return keys, diags
}
