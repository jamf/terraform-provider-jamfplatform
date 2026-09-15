// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// liveConfigJSON is a real GET /pro/v3/patch-software-title-configurations/147
// body captured on Jamf Pro 11.31.1 (title "8x8 Work", name_id 285, patch
// source "Jamf") with one version assigned, a real category, no site and both
// notifications off. Decoding a captured body rather than hand-building the SDK
// struct is what catches a field the SDK has mis-tagged: every value below has
// to survive the wire names Jamf actually sends.
const liveConfigJSON = `{
  "id" : "147",
  "jamfOfficial" : true,
  "displayName" : "8x8 Work",
  "categoryId" : "58",
  "siteId" : "-1",
  "uiNotifications" : false,
  "emailNotifications" : false,
  "softwareTitleId" : "147",
  "extensionAttributes" : [ ],
  "softwareTitleName" : "8x8 Work",
  "softwareTitleNameId" : "285",
  "softwareTitlePublisher" : "8x8",
  "patchSourceName" : "Jamf",
  "patchSourceEnabled" : true,
  "packages" : [ {
    "packageId" : "1",
    "version" : "8.33.2.2",
    "displayName" : "gen-pkg-datajar-reissue-fv-images.pkg"
  } ]
}`

// liveDefinitionsJSON is a real GET
// /pro/v3/patch-software-title-configurations/147/definitions fragment under
// the default sort (absoluteOrderId:asc), which Jamf orders newest-first.
const liveDefinitionsJSON = `{ "totalCount": 84, "results": [
  {"version":"8.36.2.3","releaseDate":"2026-08-04T07:46:54Z","standalone":true,"minimumOperatingSystem":"12.0","rebootRequired":false,"killApps":[{"appName":"8x8 Work"}],"absoluteOrderId":"0"},
  {"version":"8.35.2.6","releaseDate":"2026-07-06T06:46:10Z","standalone":true,"minimumOperatingSystem":"12.0","rebootRequired":false,"killApps":[{"appName":"8x8 Work"}],"absoluteOrderId":"1"}
] }`

func decodeLiveConfig(t *testing.T) *pro.PatchSoftwareTitleConfiguration {
	t.Helper()
	var c pro.PatchSoftwareTitleConfiguration
	if err := json.Unmarshal([]byte(liveConfigJSON), &c); err != nil {
		t.Fatalf("unmarshal live configuration: %v", err)
	}
	return &c
}

func decodeLiveDefinitions(t *testing.T) []pro.PatchSoftwareTitleDefinition {
	t.Helper()
	var d pro.PatchSoftwareTitleDefinitions
	if err := json.Unmarshal([]byte(liveDefinitionsJSON), &d); err != nil {
		t.Fatalf("unmarshal live definitions: %v", err)
	}
	return d.Results
}

func stateStrings(t *testing.T, l types.List) []string {
	t.Helper()
	out := []string{}
	if diags := l.ElementsAs(context.Background(), &out, false); diags.HasError() {
		t.Fatalf("read list elements: %v", diags)
	}
	return out
}

func stateMap(t *testing.T, m types.Map) map[string]string {
	t.Helper()
	out := map[string]string{}
	if diags := m.ElementsAs(context.Background(), &out, false); diags.HasError() {
		t.Fatalf("read map elements: %v", diags)
	}
	return out
}

// TestLiveWireUnmarshal pins the decode contract against a captured v3 body.
// The configuration reports ids as strings and its notification flags as bare
// bools, so a mis-tagged SDK field lands as a zero value that reads as real
// state — "no category assigned", "notifications off" — rather than as an
// error. Decoding the captured body is the only place that is caught.
func TestLiveWireUnmarshal(t *testing.T) {
	c := decodeLiveConfig(t)

	if c.ID != "147" {
		t.Errorf("id: %q", c.ID)
	}
	if c.DisplayName != "8x8 Work" {
		t.Errorf("displayName: %q", c.DisplayName)
	}
	if c.SoftwareTitleNameID != "285" {
		t.Errorf("softwareTitleNameId: %q", c.SoftwareTitleNameID)
	}
	if c.CategoryID != "58" {
		t.Errorf("categoryId: %q", c.CategoryID)
	}
	if c.SiteID != "-1" {
		t.Errorf("siteId: %q", c.SiteID)
	}
	if c.UiNotifications || c.EmailNotifications {
		t.Errorf("notifications: ui=%v email=%v", c.UiNotifications, c.EmailNotifications)
	}
	if c.PatchSourceName != "Jamf" {
		t.Errorf("patchSourceName: %q", c.PatchSourceName)
	}
	if len(c.Packages) != 1 {
		t.Fatalf("expected 1 package assignment, got %d", len(c.Packages))
	}
	pkg := c.Packages[0]
	if pkg.Version == nil || *pkg.Version != "8.33.2.2" {
		t.Errorf("package version: %v", pkg.Version)
	}
	if pkg.PackageID == nil || *pkg.PackageID != "1" {
		t.Errorf("package id: %v", pkg.PackageID)
	}
	if len(c.ExtensionAttributes) != 0 {
		t.Errorf("expected no extension attributes, got %d", len(c.ExtensionAttributes))
	}
}

// TestDefinitionVersions_PreservesServerOrder pins that the version catalogue
// is passed through in the order /definitions returns it. The endpoint's
// default absoluteOrderId:asc sort is newest-first, which is the order
// available_versions documents and the order a user reads to pick the version
// to assign a package to; sorting the strings would put 8.35 above 8.36 and
// quietly invert that.
func TestDefinitionVersions_PreservesServerOrder(t *testing.T) {
	got := definitionVersions(decodeLiveDefinitions(t))

	want := []string{"8.36.2.3", "8.35.2.6"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

// TestDefinitionVersions_SkipsEmptyVersions guards against an empty string
// reaching available_versions, where it would look like a selectable version a
// package could be assigned to.
func TestDefinitionVersions_SkipsEmptyVersions(t *testing.T) {
	got := definitionVersions([]pro.PatchSoftwareTitleDefinition{
		{Version: "1.0"},
		{Version: ""},
		{Version: "0.9"},
	})
	if len(got) != 2 || got[0] != "1.0" || got[1] != "0.9" {
		t.Errorf("expected [1.0 0.9], got %v", got)
	}
}

// TestAssignResourceModel_ManagedSubsetReconcile pins the read half of the
// managed-subset contract: version_packages is rebuilt from the keys the user
// declared, so a declared key whose server-side package is gone drops out
// (surfacing the drift as a diff) while a version the server has assigned but
// the user never declared stays out of state entirely.
func TestAssignResourceModel_ManagedSubsetReconcile(t *testing.T) {
	c := decodeLiveConfig(t)
	declared := []string{"8.33.2.2", "8.30.0.0"}
	state := PatchSoftwareTitleResourceModel{}

	diags := assignPatchSoftwareTitleResourceModel(context.Background(), &state, c, definitionVersions(decodeLiveDefinitions(t)), declared)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if state.ID.ValueString() != "147" {
		t.Errorf("id: %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "8x8 Work" {
		t.Errorf("name: %q", state.Name.ValueString())
	}
	if state.NameID.ValueString() != "285" {
		t.Errorf("name_id: %q", state.NameID.ValueString())
	}
	if state.CategoryID.ValueString() != "58" {
		t.Errorf("category_id: %q", state.CategoryID.ValueString())
	}
	if state.SiteID.ValueString() != "-1" {
		t.Errorf("site_id: %q", state.SiteID.ValueString())
	}
	if state.WebNotification.ValueBool() || state.EmailNotification.ValueBool() {
		t.Errorf("notifications: web=%v email=%v", state.WebNotification.ValueBool(), state.EmailNotification.ValueBool())
	}

	if state.VersionPackages.IsNull() {
		t.Fatalf("version_packages must be non-null when a declared key is assigned")
	}
	vp := stateMap(t, state.VersionPackages)
	if len(vp) != 1 || vp["8.33.2.2"] != "1" {
		t.Errorf("expected {8.33.2.2:1}, got %+v", vp)
	}

	av := stateStrings(t, state.AvailableVersions)
	if len(av) != 2 || av[0] != "8.36.2.3" || av[1] != "8.35.2.6" {
		t.Errorf("available_versions wrong: %+v", av)
	}
}

// TestAssignResourceModel_NoDeclaredKeysYieldsNullMap pins that a config with
// no version_packages block reads back null rather than an empty map. The
// attribute is Optional-only, so an empty map in state against an absent block
// in config is a permanent diff.
func TestAssignResourceModel_NoDeclaredKeysYieldsNullMap(t *testing.T) {
	state := PatchSoftwareTitleResourceModel{}
	diags := assignPatchSoftwareTitleResourceModel(context.Background(), &state, decodeLiveConfig(t), nil, nil)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.VersionPackages.IsNull() {
		t.Errorf("expected null version_packages when no keys declared, got %v", state.VersionPackages)
	}
}

// TestAssignResourceModel_DeclaredKeyDroppedWhenPackageGone pins the drift case
// where every declared key has lost its package: the map collapses to null, not
// to an empty map, for the same round-trip reason.
func TestAssignResourceModel_DeclaredKeyDroppedWhenPackageGone(t *testing.T) {
	state := PatchSoftwareTitleResourceModel{}
	diags := assignPatchSoftwareTitleResourceModel(context.Background(), &state, decodeLiveConfig(t), nil, []string{"8.32.2.10"})
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.VersionPackages.IsNull() {
		t.Errorf("a declared key with no server package must drop → null map, got %v", state.VersionPackages)
	}
}

// TestAssignResourceModel_LeavesSourceIDUntouched pins the one attribute the v3
// read must not write. The configuration names its patch source but never
// numbers it, so deriving source_id here would need a catalogue lookup on every
// read; instead whatever state holds is kept, which is always correct because
// source_id is RequiresReplace. A read that zeroed or nulled it would force a
// replacement of a live title on the next plan.
func TestAssignResourceModel_LeavesSourceIDUntouched(t *testing.T) {
	state := PatchSoftwareTitleResourceModel{SourceID: types.Int64Value(1)}
	diags := assignPatchSoftwareTitleResourceModel(context.Background(), &state, decodeLiveConfig(t), nil, nil)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.SourceID.IsNull() || state.SourceID.ValueInt64() != 1 {
		t.Errorf("source_id must survive the read untouched, got %v", state.SourceID)
	}
}

// TestAssignResourceModel_PreservesIDWhenResponseOmitsIt pins that an id
// already in state is not overwritten by an empty one. The Update path assigns
// straight from the PATCH response, so an id the server left out of that body
// would otherwise blank the resource's own identifier.
func TestAssignResourceModel_PreservesIDWhenResponseOmitsIt(t *testing.T) {
	state := PatchSoftwareTitleResourceModel{ID: types.StringValue("42")}
	api := &pro.PatchSoftwareTitleConfiguration{DisplayName: "refreshed"}

	diags := assignPatchSoftwareTitleResourceModel(context.Background(), &state, api, nil, nil)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "42" {
		t.Errorf("expected ID preserved as 42, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "refreshed" {
		t.Errorf("expected the response's display name to land, got %q", state.Name.ValueString())
	}
}

// TestAssignResourceModel_NilAPIIsNoop pins that a nil configuration leaves
// state alone rather than blanking every attribute, so a caller that has
// already raised a diagnostic cannot also corrupt state on its way out.
func TestAssignResourceModel_NilAPIIsNoop(t *testing.T) {
	state := PatchSoftwareTitleResourceModel{ID: types.StringValue("7"), Name: types.StringValue("Keep")}
	diags := assignPatchSoftwareTitleResourceModel(context.Background(), &state, nil, nil, nil)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "7" || state.Name.ValueString() != "Keep" {
		t.Errorf("expected state unchanged, got id=%q name=%q", state.ID.ValueString(), state.Name.ValueString())
	}
}

// TestAssignDataSourceModel_FullServerView pins the deliberate difference from
// the resource: a data source has no prior state to scope against, so it
// surfaces every assignment the configuration reports rather than a managed
// subset. source_id comes from the caller, which resolved it from the patch
// source name.
func TestAssignDataSourceModel_FullServerView(t *testing.T) {
	state := PatchSoftwareTitleDataSourceModel{}

	diags := assignPatchSoftwareTitleDataSourceModel(context.Background(), &state, decodeLiveConfig(t), definitionVersions(decodeLiveDefinitions(t)), types.Int64Value(1))
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if state.ID.ValueString() != "147" {
		t.Errorf("id: %q", state.ID.ValueString())
	}
	if state.NameID.ValueString() != "285" {
		t.Errorf("name_id: %q", state.NameID.ValueString())
	}
	if state.SourceID.ValueInt64() != 1 {
		t.Errorf("source_id must come from the caller, got %v", state.SourceID)
	}
	if state.CategoryID.ValueString() != "58" || state.SiteID.ValueString() != "-1" {
		t.Errorf("category/site: %q / %q", state.CategoryID.ValueString(), state.SiteID.ValueString())
	}
	if state.VersionPackages.IsNull() {
		t.Fatalf("version_packages must be populated from the server view")
	}
	vp := stateMap(t, state.VersionPackages)
	if len(vp) != 1 || vp["8.33.2.2"] != "1" {
		t.Errorf("expected {8.33.2.2:1}, got %+v", vp)
	}
	av := stateStrings(t, state.AvailableVersions)
	if len(av) != 2 || av[0] != "8.36.2.3" {
		t.Errorf("available_versions wrong: %+v", av)
	}
}

// TestAssignDataSourceModel_NilAPIIsNoop pins the same defensive no-op as the
// resource path, so a failed lookup cannot half-populate a data source result.
func TestAssignDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := PatchSoftwareTitleDataSourceModel{ID: types.StringValue("7")}
	diags := assignPatchSoftwareTitleDataSourceModel(context.Background(), &state, nil, nil, types.Int64Null())
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "7" {
		t.Errorf("expected state unchanged, got id=%q", state.ID.ValueString())
	}
}

// TestRefIDValue_NoAssignmentSentinel pins the read side of the v3 id
// vocabulary. category_id and site_id are Optional+Computed, so state has to
// hold a value that a config of "-1" round-trips against; collapsing every
// non-positive spelling the server can report — "-1", an omitted field, and the
// "0" a classic-era title still carries — onto the one sentinel is what stops a
// permanent diff.
func TestRefIDValue_NoAssignmentSentinel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "omitted field", in: "", want: "-1"},
		{name: "classic zero", in: "0", want: "-1"},
		{name: "sentinel", in: "-1", want: "-1"},
		{name: "other negative", in: "-7", want: "-1"},
		{name: "real id", in: "58", want: "58"},
		{name: "non-numeric", in: "abc", want: "abc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := refIDValue(tc.in)
			if got.IsNull() {
				t.Fatalf("must never be null — the attribute is Optional+Computed")
			}
			if got.ValueString() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got.ValueString())
			}
		})
	}
}

// TestAssignedPackagesByVersion_SkipsIncompleteEntries pins that a package
// entry missing either half of the pair is ignored. A nil or empty version
// would key the map on "", and a nil or empty package id would read as an
// assignment to package "" — both of which would then be sent back as a real
// assignment by the full-replacement patch.
func TestAssignedPackagesByVersion_SkipsIncompleteEntries(t *testing.T) {
	ver, pkg, empty := "1.0", "5", ""
	got := assignedPackagesByVersion([]pro.PatchSoftwareTitlePackages{
		{Version: &ver, PackageID: &pkg},
		{Version: &ver, PackageID: nil},
		{Version: nil, PackageID: &pkg},
		{Version: &empty, PackageID: &pkg},
		{Version: &ver, PackageID: &empty},
	})

	if len(got) != 1 || got["1.0"] != "5" {
		t.Errorf("expected {1.0:5}, got %+v", got)
	}
}

// TestManagedVersionPackages_NoDeclaredKeysIsNull pins the null-versus-empty
// choice at its source: the helper returns a null map both when nothing is
// declared and when nothing declared still has a package, so neither case
// writes an empty map against an absent config block.
func TestManagedVersionPackages_NoDeclaredKeysIsNull(t *testing.T) {
	assigned := map[string]string{"1.0": "5"}

	for _, tc := range []struct {
		name     string
		declared []string
	}{
		{name: "nothing declared", declared: nil},
		{name: "declared key has no package", declared: []string{"9.9"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := managedVersionPackages(context.Background(), tc.declared, assigned)
			if diags.HasError() {
				t.Fatalf("diags: %v", diags)
			}
			if !got.IsNull() {
				t.Errorf("expected a null map, got %v", got)
			}
		})
	}
}

// TestMatchSourceIDs pins the exact-name match used to recover source_id from
// the patch source name v3 reports. The match must be exact and must return
// every hit rather than the first: the internal and external catalogues have
// separate id spaces, so a name in both is ambiguous and the caller has to be
// able to see that instead of writing a guessed number into a RequiresReplace
// attribute.
func TestMatchSourceIDs(t *testing.T) {
	id1, id2, id3 := 1, 2, 3
	jamf, jamfLower, other := "Jamf", "jamf", "Other"
	sources := []proclassic.IDName{
		{ID: &id1, Name: &jamf},
		{ID: &id2, Name: &jamfLower},
		{ID: &id3, Name: &other},
		{ID: nil, Name: &jamf},
		{ID: &id1, Name: nil},
	}

	tests := []struct {
		name  string
		query string
		want  []int
	}{
		{name: "exact match only", query: "Jamf", want: []int{1}},
		{name: "case sensitive", query: "jamf", want: []int{2}},
		{name: "no match", query: "Missing", want: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchSourceIDs(sources, tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("position %d: expected %d, got %d", i, tc.want[i], got[i])
				}
			}
		})
	}
}

// TestMatchSourceIDs_ReportsEveryDuplicate pins the ambiguity signal: two
// entries sharing a name must both come back so resolveSourceID can refuse
// rather than silently take the first.
func TestMatchSourceIDs_ReportsEveryDuplicate(t *testing.T) {
	id4, id9 := 4, 9
	name := "Jamf"
	got := matchSourceIDs([]proclassic.IDName{
		{ID: &id4, Name: &name},
		{ID: &id9, Name: &name},
	}, "Jamf")

	if len(got) != 2 || got[0] != 4 || got[1] != 9 {
		t.Errorf("expected [4 9], got %v", got)
	}
}

// TestOrEmpty pins that a nil version catalogue lands as an empty list rather
// than null. available_versions is Computed with no plan modifier, so a null
// where the framework expects a value after apply is an "inconsistent result"
// error rather than a diff.
func TestOrEmpty(t *testing.T) {
	if got := orEmpty(nil); got == nil || len(got) != 0 {
		t.Errorf("expected a non-nil empty slice, got %v", got)
	}
	if got := orEmpty([]string{"1.0"}); len(got) != 1 || got[0] != "1.0" {
		t.Errorf("expected [1.0], got %v", got)
	}
}

// TestAssignedVersionPackagesValue pins the null-versus-empty choice on the
// whole-server-view path the list resource streams from. version_packages is
// Optional-only with a minimum of one entry, so a title with no assignments has
// to arrive as an unset attribute: an empty map is the one shape the schema
// refuses, and emitting it made `terraform plan -generate-config-out` produce
// configuration this provider then rejected.
func TestAssignedVersionPackagesValue(t *testing.T) {
	ctx := context.Background()
	ver, pkg := "1.0", "5"

	t.Run("assignments populate the map", func(t *testing.T) {
		got, diags := assignedVersionPackagesValue(ctx, []pro.PatchSoftwareTitlePackages{
			{Version: &ver, PackageID: &pkg},
		})
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if got.IsNull() {
			t.Fatalf("expected a populated map, got null")
		}
		if m := stateMap(t, got); len(m) != 1 || m["1.0"] != "5" {
			t.Errorf("expected {1.0:5}, got %+v", m)
		}
	})

	for _, tc := range []struct {
		name string
		pkgs []pro.PatchSoftwareTitlePackages
	}{
		{name: "no assignments reported", pkgs: nil},
		{name: "every assignment unreadable", pkgs: []pro.PatchSoftwareTitlePackages{{Version: nil, PackageID: nil}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diags := assignedVersionPackagesValue(ctx, tc.pkgs)
			if diags.HasError() {
				t.Fatalf("diags: %v", diags)
			}
			if !got.IsNull() {
				t.Fatalf("expected a null map, got %v", got)
			}
			if elemType := got.ElementType(ctx); elemType != types.StringType {
				t.Errorf("expected a typed null with element type %v, got %v", types.StringType, elemType)
			}
		})
	}
}

// TestManagedVersionPackages_NeverHydratesUndeclaredAssignments pins #403 so a
// later uniform-hydration sweep cannot reintroduce it. An import declares
// nothing, and adopting the server's assignment set here makes every one of them
// a prior key Update reads as an explicit unassign — so the first apply after an
// import strips every package the configuration does not mention. The cosmetic
// addition diff #391 wanted removed is the price, and it applies as a no-op.
func TestManagedVersionPackages_NeverHydratesUndeclaredAssignments(t *testing.T) {
	assigned := map[string]string{"8.32.2.10": "111", "8.33.0.0": "222"}
	got, diags := managedVersionPackages(context.Background(), nil, assigned)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if !got.IsNull() {
		t.Fatalf("version_packages must stay null when nothing is declared, got %v", got)
	}
	if elemType := got.ElementType(context.Background()); elemType != types.StringType {
		t.Errorf("expected a typed null with element type %v, got %v", types.StringType, elemType)
	}
}
