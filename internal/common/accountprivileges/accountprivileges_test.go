// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package accountprivileges

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func mustSet(t *testing.T, vals ...string) types.Set {
	t.Helper()
	s, diags := NewStringSet(vals)
	if diags.HasError() {
		t.Fatalf("NewStringSet(%v): %v", vals, diags)
	}
	return s
}

func setStrings(t *testing.T, s types.Set) []string {
	t.Helper()
	if s.IsNull() {
		return nil
	}
	var out []string
	if d := s.ElementsAs(context.Background(), &out, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	sort.Strings(out)
	return out
}

func TestGroupPrivilegesRoundTrip(t *testing.T) {
	in := map[string][]string{
		"jss_objects":  {"Read Computers", "Update Computers"},
		"jss_settings": {"Read Activation Code"},
		"jss_actions":  {},
	}
	got := FromGroupPrivileges(ToGroupPrivileges(in))
	for k, want := range in {
		sort.Strings(want)
		gv := got[k]
		sort.Strings(gv)
		if !reflect.DeepEqual(want, gv) && (len(want) != 0 || len(gv) != 0) {
			t.Errorf("category %s: want %v, got %v", k, want, gv)
		}
	}
}

func TestAccountPrivilegesRoundTrip(t *testing.T) {
	in := map[string][]string{
		"jss_objects": {"Read Scripts"},
		"recon":       {"Use Recon"},
	}
	got := FromAccountPrivileges(ToAccountPrivileges(in))
	if !reflect.DeepEqual(got["jss_objects"], []string{"Read Scripts"}) {
		t.Errorf("jss_objects: %v", got["jss_objects"])
	}
	if !reflect.DeepEqual(got["recon"], []string{"Use Recon"}) {
		t.Errorf("recon: %v", got["recon"])
	}
	if _, ok := got["jss_settings"]; ok {
		t.Errorf("absent category jss_settings should not appear: %v", got)
	}
}

func TestToAccountPrivilegesEmptyMapIsNil(t *testing.T) {
	if ToAccountPrivileges(nil) != nil {
		t.Error("nil map should yield nil privileges")
	}
	if ToGroupPrivileges(map[string][]string{}) != nil {
		t.Error("empty map should yield nil privileges")
	}
}

func TestIntersectIntoState_DropsServerAdded(t *testing.T) {
	// Declared {Update Buildings}; server expanded to add Read Activation Code
	// in a different category. Declared category keeps only what it declared;
	// the undeclared (null) jss_settings category stays null.
	prior := &Model{JamfProServerObjects: mustSet(t, "Update Buildings")}
	server := map[string][]string{
		"jss_objects":  {"Update Buildings"},
		"jss_settings": {"Read Activation Code"},
	}
	out, diags := IntersectIntoState(context.Background(), prior, server)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if got := setStrings(t, out.JamfProServerObjects); !reflect.DeepEqual(got, []string{"Update Buildings"}) {
		t.Errorf("jss_objects: %v", got)
	}
	if !out.JamfProServerSettings.IsNull() {
		t.Errorf("undeclared jss_settings should stay null, got %v", out.JamfProServerSettings)
	}
}

func TestIntersectIntoState_PreservesRemovalIntent(t *testing.T) {
	// Prior declared {A,B}; user is removing B (so config will be {A}). On the
	// read after a write of {A}, the server still has {A} (+maybe deps we don't
	// declare). Intersect with declared {A} keeps {A}; B is gone — removal works.
	prior := &Model{JamfProServerObjects: mustSet(t, "A")}
	server := map[string][]string{"jss_objects": {"A", "ServerDep"}}
	out, _ := IntersectIntoState(context.Background(), prior, server)
	if got := setStrings(t, out.JamfProServerObjects); !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("expected [A] (ServerDep dropped), got %v", got)
	}
}

func TestIntersectIntoState_DriftShrinksWhenServerLosesDeclared(t *testing.T) {
	// Declared {A,B}; server now only has {A} (B removed out-of-band). State
	// becomes {A}, so the next plan re-adds B (drift corrected).
	prior := &Model{JamfProServerObjects: mustSet(t, "A", "B")}
	server := map[string][]string{"jss_objects": {"A"}}
	out, _ := IntersectIntoState(context.Background(), prior, server)
	if got := setStrings(t, out.JamfProServerObjects); !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("expected [A], got %v", got)
	}
}

func TestIntersectIntoState_ImportMaterialisesFullGrid(t *testing.T) {
	server := map[string][]string{"jss_objects": {"A", "B"}}
	out, _ := IntersectIntoState(context.Background(), nil, server)
	if got := setStrings(t, out.JamfProServerObjects); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("import should materialise full grid, got %v", got)
	}
	if !out.JamfProServerSettings.IsNull() {
		t.Errorf("absent category should be null on import, got %v", out.JamfProServerSettings)
	}
}

// TestIntersectIntoState_ImportDedupesServerDuplicates guards the genconfig /
// import path: the classic /accounts endpoint can echo the same privilege
// string more than once within a category, and types.SetValue rejects duplicate
// elements with a hard "Duplicate Set Element" error. NewStringSet must collapse
// the duplicates so hydration succeeds.
func TestIntersectIntoState_ImportDedupesServerDuplicates(t *testing.T) {
	server := map[string][]string{
		"jss_objects": {"Create/Read/Update Cloud Distribution Point", "Create/Read/Update Cloud Distribution Point", "Read Buildings"},
	}
	out, diags := IntersectIntoState(context.Background(), nil, server)
	if diags.HasError() {
		t.Fatalf("IntersectIntoState with duplicate server values: %v", diags)
	}
	if got, want := setStrings(t, out.JamfProServerObjects), []string{"Create/Read/Update Cloud Distribution Point", "Read Buildings"}; !reflect.DeepEqual(got, want) {
		t.Errorf("duplicate server privileges should collapse, got %v want %v", got, want)
	}
}

// TestCategorizedSets_DedupesWithinCategoryAndUnion is the regression guard for
// issue #290: the account_privileges data source projected the discovered
// catalog into state with its own set builder that called types.SetValue
// directly, so the classic Administrator grid's within-category duplicates
// (Create/Read/Update Cloud Distribution Point, Read/Update Computer Check-In —
// each echoed twice) produced a hard "Duplicate Set Element" error at apply and
// the data source never landed in state. CategorizedSets must collapse the
// duplicates in every category and in the flat union.
func TestCategorizedSets_DedupesWithinCategoryAndUnion(t *testing.T) {
	catalog := map[string][]string{
		"jss_objects": {
			"Create Cloud Distribution Point", "Create Cloud Distribution Point",
			"Read Cloud Distribution Point", "Read Cloud Distribution Point",
			"Update Cloud Distribution Point", "Update Cloud Distribution Point",
			"Read Buildings",
		},
		"jss_settings": {
			"Read Computer Check-In", "Read Computer Check-In",
			"Update Computer Check-In", "Update Computer Check-In",
		},
	}
	sets, all, diags := CategorizedSets(catalog)
	if diags.HasError() {
		t.Fatalf("CategorizedSets with duplicate wire values: %v", diags)
	}

	wantObjects := []string{
		"Create Cloud Distribution Point",
		"Read Buildings",
		"Read Cloud Distribution Point",
		"Update Cloud Distribution Point",
	}
	if got := setStrings(t, sets["jss_objects"]); !reflect.DeepEqual(got, wantObjects) {
		t.Errorf("jss_objects: got %v want %v", got, wantObjects)
	}
	wantSettings := []string{"Read Computer Check-In", "Update Computer Check-In"}
	if got := setStrings(t, sets["jss_settings"]); !reflect.DeepEqual(got, wantSettings) {
		t.Errorf("jss_settings: got %v want %v", got, wantSettings)
	}

	// Absent categories still yield an empty (non-null) set.
	if s := sets["recon"]; s.IsNull() || len(s.Elements()) != 0 {
		t.Errorf("absent category recon should be empty non-null set, got %v", s)
	}

	// The flat union carries every distinct privilege exactly once.
	wantAll := append(append([]string(nil), wantObjects...), wantSettings...)
	sort.Strings(wantAll)
	if got := setStrings(t, all); !reflect.DeepEqual(got, wantAll) {
		t.Errorf("all union: got %v want %v", got, wantAll)
	}
}

func TestModelToMap_OmitsNullCategories(t *testing.T) {
	m := &Model{JamfProServerObjects: mustSet(t, "Read Computers")}
	got, diags := m.ToMap(context.Background())
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if _, ok := got["jss_objects"]; !ok {
		t.Errorf("declared category missing: %v", got)
	}
	if _, ok := got["jss_settings"]; ok {
		t.Errorf("null category should be omitted: %v", got)
	}
}

// TestMergeGrid_DeclaredReplacesUndeclaredCarried is the write-side contract
// behind issue #385: a declared category wins over the live value, and a
// category the configuration does not declare is carried from the live grid,
// server-injected dependency privileges included, so the full-replace PUT cannot
// empty it.
func TestMergeGrid_DeclaredReplacesUndeclaredCarried(t *testing.T) {
	declared := &Model{JamfProServerObjects: mustSet(t, "Read Buildings", "Read Departments")}
	server := map[string][]string{
		"jss_objects":  {"Read Computers"},
		"jss_settings": {"Read License Information", "Read SMTP Server"},
		"jss_actions":  {"Send Computer Remote Lock Command"},
	}
	got, diags := MergeGrid(context.Background(), declared, server)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	want := map[string][]string{
		"jss_objects":  {"Read Buildings", "Read Departments"},
		"jss_settings": {"Read License Information", "Read SMTP Server"},
		"jss_actions":  {"Send Computer Remote Lock Command"},
	}
	for k, w := range want {
		g := got[k]
		sort.Strings(g)
		if !reflect.DeepEqual(g, w) {
			t.Errorf("category %s: got %v want %v", k, g, w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("merged grid has %d categories, want %d: %v", len(got), len(want), got)
	}
	if !reflect.DeepEqual(server["jss_objects"], []string{"Read Computers"}) {
		t.Errorf("server grid mutated: %v", server["jss_objects"])
	}
}

// TestMergeGrid_DeclaredEmptyClears keeps a declared [] as a present key with an
// empty slice, so ToGroupPrivileges emits an empty element and the category is
// cleared rather than carried from the live grid.
func TestMergeGrid_DeclaredEmptyClears(t *testing.T) {
	declared := &Model{JamfProServerActions: mustSet(t)}
	server := map[string][]string{"jss_actions": {"Send Computer Remote Lock Command"}}
	got, diags := MergeGrid(context.Background(), declared, server)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	v, ok := got["jss_actions"]
	if !ok {
		t.Fatalf("declared empty category dropped from merged grid: %v", got)
	}
	if v == nil || len(v) != 0 {
		t.Errorf("declared empty category should be a non-nil empty slice, got %#v", v)
	}
	if grid := ToGroupPrivileges(got); grid == nil || grid.JssActions == nil {
		t.Errorf("empty category should still be emitted as an element: %+v", grid)
	}
}

// TestMergeGrid_NilServer covers Create, where nothing exists yet: the merged
// grid is exactly the declared categories.
func TestMergeGrid_NilServer(t *testing.T) {
	declared := &Model{Recon: mustSet(t, "Use Recon")}
	got, diags := MergeGrid(context.Background(), declared, nil)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !reflect.DeepEqual(got, map[string][]string{"recon": {"Use Recon"}}) {
		t.Errorf("got %v", got)
	}
}

// TestMergeGrid_NilDeclared returns a copy of the live grid when nothing is
// declared, so a caller that does reach it with no configuration re-sends the
// server's own grid rather than an empty one.
func TestMergeGrid_NilDeclared(t *testing.T) {
	server := map[string][]string{"casper_admin": {"Use Casper Admin"}}
	got, diags := MergeGrid(context.Background(), nil, server)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !reflect.DeepEqual(got, server) {
		t.Errorf("got %v want %v", got, server)
	}
}

// fakeDiscoverer serves a fixed account/group topology for Discover tests.
type fakeDiscoverer struct {
	groups map[int]*proclassic.Group
	users  map[int]*proclassic.Account
}

func (f fakeDiscoverer) ListAccounts(_ context.Context) (*proclassic.Accounts, error) {
	var gs []proclassic.AccountsGroupsGroupItem
	for id := range f.groups {
		gs = append(gs, proclassic.AccountsGroupsGroupItem{ID: &id})
	}
	var us []proclassic.AccountsUsersUserItem
	for id := range f.users {
		us = append(us, proclassic.AccountsUsersUserItem{ID: &id})
	}
	return &proclassic.Accounts{
		Groups: &proclassic.AccountsGroups{Group: &gs},
		Users:  &proclassic.AccountsUsers{User: &us},
	}, nil
}

func (f fakeDiscoverer) GetAccountGroupByID(_ context.Context, id string) (*proclassic.Group, error) {
	n, _ := strconv.Atoi(id)
	if g, ok := f.groups[n]; ok {
		return g, nil
	}
	return nil, fmt.Errorf("not found")
}

func (f fakeDiscoverer) GetAccountByUserID(_ context.Context, id string) (*proclassic.Account, error) {
	n, _ := strconv.Atoi(id)
	if u, ok := f.users[n]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("not found")
}

func adminGroup(id int) *proclassic.Group {
	set := administratorPrivilegeSet
	return &proclassic.Group{
		ID:           &id,
		PrivilegeSet: &set,
		Privileges: &proclassic.GroupPrivileges{
			JssObjects: &proclassic.GroupPrivilegesJssObjects{Privilege: &[]string{"Read Computers", "Update Computers"}},
		},
	}
}

func TestDiscover_PrefersAdminGroup(t *testing.T) {
	custom := "Custom"
	f := fakeDiscoverer{
		groups: map[int]*proclassic.Group{
			1: {ID: new(1), PrivilegeSet: &custom},
			8: adminGroup(8),
		},
	}
	cat, err := Discover(context.Background(), f)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !cat.Contains("Read Computers") || !cat.Contains("Update Computers") {
		t.Errorf("catalog missing expected privileges: %v", cat.All())
	}
	if cat.Contains("Nonexistent Privilege") {
		t.Error("catalog should not contain bogus privilege")
	}
}

func TestDiscover_NoAdminErrors(t *testing.T) {
	custom := "Custom"
	f := fakeDiscoverer{groups: map[int]*proclassic.Group{1: {ID: new(1), PrivilegeSet: &custom}}}
	if _, err := Discover(context.Background(), f); err == nil {
		t.Error("expected error when no Administrator exists")
	}
}

func TestValidate_RejectsUnknownWithSuggestion(t *testing.T) {
	cat := catalogFromMap(map[string][]string{"jss_objects": {"Read Computers", "Update Computers"}})
	m := &Model{JamfProServerObjects: mustSet(t, "Read Computer")} // typo: missing trailing s
	diags := Validate(context.Background(), cat, m, path.Root("privileges"))
	if !diags.HasError() {
		t.Fatal("expected an error for unknown privilege")
	}
	if detail := diags[0].Detail(); !contains(detail, "Did you mean") || !contains(detail, "Read Computers") {
		t.Errorf("expected fuzzy suggestion, got: %s", detail)
	}
}

func TestValidate_AcceptsKnown(t *testing.T) {
	cat := catalogFromMap(map[string][]string{"jss_objects": {"Read Computers"}})
	m := &Model{JamfProServerObjects: mustSet(t, "Read Computers")}
	if diags := Validate(context.Background(), cat, m, path.Root("privileges")); diags.HasError() {
		t.Errorf("unexpected error: %v", diags)
	}
}

func TestLevenshtein(t *testing.T) {
	if d := levenshtein("Read Computers", "read computers"); d != 0 {
		t.Errorf("case-insensitive distance should be 0, got %d", d)
	}
	if d := levenshtein("abc", "abd"); d != 1 {
		t.Errorf("want 1, got %d", d)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestFilterToCatalog covers the import repair for a preset privilege_set. Jamf
// Pro expands "Auditor" into a privilege list that can name privileges the same
// tenant will not grant, and adopting one hands the resource's own Validate a
// config it refuses — so an imported group would produce a plan that cannot run.
func TestFilterToCatalog(t *testing.T) {
	ctx := context.Background()
	catalog := &Catalog{valid: map[string]struct{}{
		"Read Computers":   {},
		"Read Cache":       {},
		"Read SMTP Server": {},
	}}

	var m Model
	objects, diags := NewStringSet([]string{"Read Computers", "Read Knobs"})
	if diags.HasError() {
		t.Fatalf("building the objects set: %v", diags)
	}
	settings, diags := NewStringSet([]string{"Read Cache", "Read SMTP Server"})
	if diags.HasError() {
		t.Fatalf("building the settings set: %v", diags)
	}
	*m.setPtr("jss_objects") = objects
	*m.setPtr("jss_settings") = settings

	out, diags := FilterToCatalog(ctx, &m, catalog)
	if diags.HasError() {
		t.Fatalf("FilterToCatalog: %v", diags)
	}

	got, d := declaredStrings(ctx, *out.setPtr("jss_objects"))
	if d.HasError() {
		t.Fatalf("reading back the objects set: %v", d)
	}
	if !reflect.DeepEqual(got, []string{"Read Computers"}) {
		t.Errorf("jss_objects = %v, want the ungrantable privilege dropped", got)
	}

	kept, d := declaredStrings(ctx, *out.setPtr("jss_settings"))
	if d.HasError() {
		t.Fatalf("reading back the settings set: %v", d)
	}
	sort.Strings(kept)
	if !reflect.DeepEqual(kept, []string{"Read Cache", "Read SMTP Server"}) {
		t.Errorf("jss_settings = %v, want both grantable privileges kept", kept)
	}

	// An untouched category stays null rather than becoming an empty set, which
	// is what "this category is unmanaged" means everywhere else here.
	if !out.setPtr("jss_actions").IsNull() {
		t.Errorf("jss_actions = %v, want null", *out.setPtr("jss_actions"))
	}
}

// TestFilterToCatalog_NilCatalogKeepsEverything pins the discovery-failure path:
// the grid is left as read rather than silently emptied.
func TestFilterToCatalog_NilCatalogKeepsEverything(t *testing.T) {
	ctx := context.Background()
	var m Model
	objects, diags := NewStringSet([]string{"Read Computers", "Read Knobs"})
	if diags.HasError() {
		t.Fatalf("building the objects set: %v", diags)
	}
	*m.setPtr("jss_objects") = objects

	out, diags := FilterToCatalog(ctx, &m, nil)
	if diags.HasError() {
		t.Fatalf("FilterToCatalog: %v", diags)
	}
	got, d := declaredStrings(ctx, *out.setPtr("jss_objects"))
	if d.HasError() {
		t.Fatalf("reading back the objects set: %v", d)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"Read Computers", "Read Knobs"}) {
		t.Errorf("jss_objects = %v, want the unfiltered set", got)
	}
}
