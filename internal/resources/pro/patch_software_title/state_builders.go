// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignPatchSoftwareTitleResourceModel populates a resource model from a v3
// patch software title configuration plus the version catalog, which lives on
// the separate /definitions sub-resource rather than the configuration body.
// declaredKeys is the managed subset of software_version keys the caller
// declared (plan keys on Create/Update, prior state keys on Read):
// version_packages is rebuilt from only those keys by looking each up in the
// configuration's package assignments. A declared key whose package is gone
// server-side is dropped from the map, surfacing the drift. An import declares
// nothing, so it hydrates no assignments — see managedVersionPackages.
//
// source_id is deliberately left untouched. The v3 configuration names its
// patch source (patchSourceName) but never numbers it, and a title's source
// cannot change once minted, so whatever is already in state stays correct.
// Import is the one case with nothing in state to keep, and Read resolves the
// number from the name there — see resolveSourceID.
func assignPatchSoftwareTitleResourceModel(ctx context.Context, state *PatchSoftwareTitleResourceModel, s *pro.PatchSoftwareTitleConfiguration, availableVersions, declaredKeys []string) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}
	if s.ID != "" {
		state.ID = types.StringValue(s.ID)
	}
	state.Name = types.StringValue(s.DisplayName)
	if s.SoftwareTitleNameID != "" {
		state.NameID = types.StringValue(s.SoftwareTitleNameID)
	}

	state.CategoryID = refIDValue(s.CategoryID)
	state.SiteID = refIDValue(s.SiteID)
	state.WebNotification = types.BoolValue(s.UiNotifications)
	state.EmailNotification = types.BoolValue(s.EmailNotifications)

	availList, d := types.ListValueFrom(ctx, types.StringType, orEmpty(availableVersions))
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.AvailableVersions = availList

	vp, d := managedVersionPackages(ctx, declaredKeys, assignedPackagesByVersion(s.Packages))
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.VersionPackages = vp

	return diags
}

// assignPatchSoftwareTitleDataSourceModel populates a data source model. The
// data source surfaces the full server view, so version_packages is built from
// every assignment the configuration reports (no managed-subset gating — there
// is no prior state to scope against). sourceID is resolved by the caller from
// the configuration's patch source name.
func assignPatchSoftwareTitleDataSourceModel(ctx context.Context, state *PatchSoftwareTitleDataSourceModel, s *pro.PatchSoftwareTitleConfiguration, availableVersions []string, sourceID types.Int64) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}
	state.ID = types.StringValue(s.ID)
	state.Name = types.StringValue(s.DisplayName)
	state.NameID = types.StringValue(s.SoftwareTitleNameID)
	state.SourceID = sourceID

	state.CategoryID = refIDValue(s.CategoryID)
	state.SiteID = refIDValue(s.SiteID)
	state.WebNotification = types.BoolValue(s.UiNotifications)
	state.EmailNotification = types.BoolValue(s.EmailNotifications)

	availList, d := types.ListValueFrom(ctx, types.StringType, orEmpty(availableVersions))
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.AvailableVersions = availList

	vpMap, d := types.MapValueFrom(ctx, types.StringType, assignedPackagesByVersion(s.Packages))
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.VersionPackages = vpMap

	return diags
}

// refIDValue maps a v3 category or site id onto its Terraform value. The
// configurations endpoint reports "not assigned" as the string "-1" and rejects
// any other non-positive id on a write, so that sentinel round-trips as-is.
// Two other shapes are normalised to it defensively: an empty string (a field
// the server omitted), and the literal "0" — which no v3 write can produce but
// a title last written through the classic /patchsoftwaretitles endpoint can
// still carry, because that endpoint cleared a category by storing wire id 0.
func refIDValue(id string) types.String {
	if id == "" {
		return types.StringValue("-1")
	}
	if n, err := strconv.Atoi(id); err == nil && n <= 0 {
		return types.StringValue("-1")
	}
	return types.StringValue(id)
}

// definitionVersions returns every version string the patch source publishes for
// the title, preserving server order. The /definitions default sort is
// absoluteOrderId:asc, which Jamf orders newest-first — the same order the
// classic <versions> block used, so available_versions needs no re-sorting.
func definitionVersions(defs []pro.PatchSoftwareTitleDefinition) []string {
	out := make([]string, 0, len(defs))
	for i := range defs {
		if defs[i].Version != "" {
			out = append(out, defs[i].Version)
		}
	}
	return out
}

// assignedPackagesByVersionCounted returns a software_version → package_id map
// for every package assignment the configuration reports, plus the number of
// reported assignments it could not read (no version or no package id, either
// missing or empty).
//
// The count exists because this fold also feeds a write: Update sends the folded
// set as the v3 `packages` array, which is a FULL REPLACEMENT, so an assignment
// missing from the fold is cleared server-side. Dropping one silently would
// break the promise version_packages makes, that assignments made outside
// Terraform survive an apply — hence a caller that writes must be able to say
// how many it is about to clear.
func assignedPackagesByVersionCounted(pkgs []pro.PatchSoftwareTitlePackages) (map[string]string, int) {
	out := map[string]string{}
	unreadable := 0
	for i := range pkgs {
		if pkgs[i].Version == nil || pkgs[i].PackageID == nil {
			unreadable++
			continue
		}
		if *pkgs[i].Version == "" || *pkgs[i].PackageID == "" {
			unreadable++
			continue
		}
		out[*pkgs[i].Version] = *pkgs[i].PackageID
	}
	return out, unreadable
}

// assignedPackagesByVersion returns a software_version → package_id map for
// every package assignment the configuration reports, discarding the ones it
// cannot read. It is the read-path form, where a discard only shapes state; a
// caller folding the map into a write wants assignedPackagesByVersionCounted so
// the discards can be reported.
func assignedPackagesByVersion(pkgs []pro.PatchSoftwareTitlePackages) map[string]string {
	out, _ := assignedPackagesByVersionCounted(pkgs)
	return out
}

// managedVersionPackages builds the version_packages map from only the declared
// keys: each is looked up in the server's assigned set and included if still
// present. A declared key with no server-side package is dropped (surfaces
// drift). When no keys are declared the map is null (matches an unset config),
// and that holds on a first-time import hydration too.
//
// # Why import does not adopt the assigned set
//
// It did, briefly (#391 via #400), so that a declared version_packages did not
// plan as an addition on the first plan after an import. That adoption unassigns
// packages (#403). Update reads the managed subset out of PRIOR STATE and hands
// it to unionVersionPackages, which reads a prior key missing from the plan as
// an explicit unassign — correct only while every prior key is a key the
// configuration declared. Hydration breaks that invariant: the prior keys become
// the server's assignments, so the settle plan's apply clears every version the
// configuration does not mention, and a full replacement clears what it omits.
//
// The cost of not adopting is the cosmetic diff #391 set out to remove: a
// declared version_packages plans as an addition against the null in state, and
// applying it re-sends assignments the title already holds. That fold preserves
// the live set, so the apply is a no-op on the server.
func managedVersionPackages(ctx context.Context, declaredKeys []string, assigned map[string]string) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(declaredKeys) == 0 {
		return types.MapNull(types.StringType), diags
	}
	managed := map[string]string{}
	for _, k := range declaredKeys {
		if pkg, ok := assigned[k]; ok {
			managed[k] = pkg
		}
	}
	if len(managed) == 0 {
		return types.MapNull(types.StringType), diags
	}
	m, d := types.MapValueFrom(ctx, types.StringType, managed)
	diags.Append(d...)
	return m, diags
}

// stringsFromList reads a Terraform list of strings back into a Go slice,
// yielding nil for a null or unknown list. It is how a caller recovers the value
// already held for a Computed list attribute when the read that would refresh it
// failed — see readAvailableVersionsBestEffort.
func stringsFromList(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := l.ElementsAs(ctx, &out, false)
	return out, diags
}

// orEmpty substitutes an empty slice for nil so a Computed list attribute lands
// as an empty list rather than null.
func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// assignedVersionPackagesValue folds every package assignment the configuration
// reports into a version_packages value, yielding a null map when the title has
// none. version_packages is Optional-only with a minimum of one entry, so an
// empty map is not a legal configuration value — a title with no assignments has
// to be represented by an unset attribute, or generated configuration would
// carry the one shape the schema refuses. It is the whole-server-view form, for
// callers with no declared subset to scope against; a caller holding declared
// keys wants managedVersionPackages.
//
// The null is typed rather than the zero-value types.Map, which is an
// untyped/DynamicPseudoType map and fails the schema type check.
func assignedVersionPackagesValue(ctx context.Context, pkgs []pro.PatchSoftwareTitlePackages) (types.Map, diag.Diagnostics) {
	assigned := assignedPackagesByVersion(pkgs)
	if len(assigned) == 0 {
		return types.MapNull(types.StringType), nil
	}
	return types.MapValueFrom(ctx, types.StringType, assigned)
}
