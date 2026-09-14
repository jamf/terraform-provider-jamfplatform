// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func idSet(ids ...string) types.Set {
	vals := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		vals = append(vals, types.StringValue(id))
	}
	return types.SetValueMust(types.StringType, vals)
}

func nullSet() types.Set { return types.SetNull(types.StringType) }

func TestBuildEbookGeneral(t *testing.T) {
	m := &EbookGeneralModel{
		Name:            types.StringValue("Field Guide"),
		Author:          types.StringValue("J. Doe"),
		URL:             types.StringValue("https://example.org/guide.pdf"),
		DeploymentType:  types.StringValue(deploymentTypeSelfService),
		DeployAsManaged: types.BoolValue(true),
		Free:            types.BoolValue(false),
		FileType:        types.StringValue("PDF"),
		Version:         types.StringValue("1.0"),
		CategoryID:      types.StringValue("7"),
		SiteID:          types.StringValue("3"),
	}
	g := buildEbookGeneral(m)
	if g.Name == nil || *g.Name != "Field Guide" {
		t.Errorf("name not mapped: %+v", g.Name)
	}
	if g.URL == nil || *g.URL != "https://example.org/guide.pdf" {
		t.Errorf("url not mapped")
	}
	if g.DeploymentType == nil || *g.DeploymentType != deploymentTypeSelfService {
		t.Errorf("deployment_type not mapped")
	}
	if g.FileType == nil || *g.FileType != "PDF" {
		t.Errorf("file_type not mapped")
	}
	if g.DeployAsManaged == nil || !*g.DeployAsManaged {
		t.Errorf("deploy_as_managed not mapped")
	}
	if g.Category == nil || g.Category.ID == nil || *g.Category.ID != 7 {
		t.Errorf("category id not mapped: %+v", g.Category)
	}
	if g.Site == nil || g.Site.ID == nil || *g.Site.ID != 3 {
		t.Errorf("site id not mapped: %+v", g.Site)
	}
}

func TestBuildEbookNotification(t *testing.T) {
	if n := buildEbookNotification(types.BoolNull(), types.StringNull()); n != nil {
		t.Errorf("expected nil notification when unconfigured, got %+v", n)
	}
	n := buildEbookNotification(types.BoolValue(true), types.StringValue("Self Service"))
	if n == nil || n.Enabled == nil || !*n.Enabled || n.Method == nil || *n.Method != "Self Service" {
		t.Fatalf("notification legs not assembled: %+v", n)
	}
}

func emptyEbookScope() *EbookScopeModel {
	return &EbookScopeModel{
		Targets: &EbookScopeTargetsModel{
			AllComputers:         types.BoolNull(),
			AllMobileDevices:     types.BoolNull(),
			AllJssUsers:          types.BoolNull(),
			ComputerIDs:          nullSet(),
			ComputerGroupIDs:     nullSet(),
			MobileDeviceIDs:      nullSet(),
			MobileDeviceGroupIDs: nullSet(),
			BuildingIDs:          nullSet(),
			DepartmentIDs:        nullSet(),
			UserIDs:              nullSet(),
			UserGroupIDs:         nullSet(),
			ClassIDs:             nullSet(),
		},
	}
}

func TestBuildEbookScope_Collapse(t *testing.T) {
	ctx := context.Background()
	s, diags := buildEbookScope(ctx, emptyEbookScope())
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if s != nil {
		t.Errorf("expected nil scope for empty model, got %+v", s)
	}
}

func TestBuildEbookScope_DualTargetUnionAndClasses(t *testing.T) {
	ctx := context.Background()
	m := emptyEbookScope()
	m.Targets.AllComputers = types.BoolValue(false)
	m.Targets.ComputerIDs = idSet("11", "12")
	m.Targets.ComputerGroupIDs = idSet("5")
	m.Targets.MobileDeviceIDs = idSet("21")
	m.Targets.MobileDeviceGroupIDs = idSet("6", "7")
	m.Targets.BuildingIDs = idSet("3")
	m.Targets.UserIDs = idSet("31")
	m.Targets.ClassIDs = idSet("41", "42")
	m.Limitations = &EbookScopeLimitationsModel{
		NetworkSegmentIDs:                idSet("2"),
		DirectoryServiceOrLocalUserNames: idSetNames("alice"),
		DirectoryServiceUserGroupNames:   nullSet(),
	}
	m.Exclusions = &EbookScopeExclusionsModel{
		ComputerIDs:                      nullSet(),
		ComputerGroupIDs:                 nullSet(),
		MobileDeviceIDs:                  idSet("99"),
		MobileDeviceGroupIDs:             nullSet(),
		BuildingIDs:                      nullSet(),
		DepartmentIDs:                    nullSet(),
		UserIDs:                          nullSet(),
		UserGroupIDs:                     nullSet(),
		NetworkSegmentIDs:                nullSet(),
		DirectoryServiceOrLocalUserNames: idSetNames("bob"),
		DirectoryServiceUserGroupNames:   idSetNames("Admins"),
	}

	s, diags := buildEbookScope(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if s == nil {
		t.Fatalf("expected non-nil scope")
	}
	if s.Computers == nil || s.Computers.Computer == nil || len(*s.Computers.Computer) != 2 {
		t.Errorf("computers not mapped: %+v", s.Computers)
	}
	if s.MobileDevices == nil || s.MobileDevices.MobileDevice == nil || len(*s.MobileDevices.MobileDevice) != 1 {
		t.Errorf("mobile_devices not mapped: %+v", s.MobileDevices)
	}
	if s.MobileDeviceGroups == nil || s.MobileDeviceGroups.MobileDeviceGroup == nil || len(*s.MobileDeviceGroups.MobileDeviceGroup) != 2 {
		t.Errorf("mobile_device_groups not mapped")
	}
	if s.Classes == nil || s.Classes.Class == nil || len(*s.Classes.Class) != 2 {
		t.Errorf("classes not mapped: %+v", s.Classes)
	}
	if s.JssUsers == nil || s.JssUsers.User == nil || len(*s.JssUsers.User) != 1 {
		t.Errorf("jss_users (user_ids) not mapped")
	}
	if s.Limitations == nil || s.Limitations.Users == nil || s.Limitations.Users.User == nil || len(*s.Limitations.Users.User) != 1 {
		t.Errorf("limitations users (names) not mapped")
	}
	if s.Exclusions == nil || s.Exclusions.MobileDevices == nil {
		t.Errorf("exclusions mobile_devices not mapped")
	}
	if s.Exclusions.UserGroups == nil || s.Exclusions.UserGroups.UserGroup == nil || len(*s.Exclusions.UserGroups.UserGroup) != 1 {
		t.Errorf("exclusions user_groups (names) not mapped")
	}
}

func TestBuildEbookInput_IconStampedIntoBothBlocks(t *testing.T) {
	ctx := context.Background()
	plan := EbookResourceModel{
		General: &EbookGeneralModel{
			Name: types.StringValue("Field Guide"),
			URL:  types.StringValue("https://example.org/guide.pdf"),
		},
		SelfService: &EbookSelfServiceModel{
			IconID: types.StringValue("805"),
			Categories: []EbookSelfServiceCategoryModel{
				{ID: types.StringValue("3"), DisplayIn: types.BoolValue(true), FeatureIn: types.BoolValue(false)},
			},
		},
	}
	out, diags := buildEbookInput(ctx, plan)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if out.SelfService == nil || out.SelfService.SelfServiceIcon == nil || out.SelfService.SelfServiceIcon.ID == nil || *out.SelfService.SelfServiceIcon.ID != 805 {
		t.Errorf("self_service icon id not mapped: %+v", out.SelfService)
	}
	// The icon must also be stamped into <general> (it round-trips under both).
	if out.General == nil || out.General.SelfServiceIcon == nil || out.General.SelfServiceIcon.ID == nil || *out.General.SelfServiceIcon.ID != 805 {
		t.Errorf("general icon id not stamped: %+v", out.General)
	}
	if out.SelfService.SelfServiceCategories == nil || out.SelfService.SelfServiceCategories.Category == nil || len(*out.SelfService.SelfServiceCategories.Category) != 1 {
		t.Fatalf("categories not mapped")
	}
	c := (*out.SelfService.SelfServiceCategories.Category)[0]
	if c.ID == nil || *c.ID != 3 || c.DisplayIn == nil || !*c.DisplayIn {
		t.Errorf("category fields not mapped: %+v", c)
	}
}

// idSetNames is an alias for idSet used where the elements are names rather than
// numeric IDs, for readability in the scope tests.
func idSetNames(names ...string) types.Set { return idSet(names...) }

// TestBuildEbookSelfService_CategoryIDOnlySendsDisplayIn guards the config
// shape that silently stored nothing. See helpers.SelfServiceCategoryDisplayIn.
func TestBuildEbookSelfService_CategoryIDOnlySendsDisplayIn(t *testing.T) {
	t.Parallel()
	ss := buildEbookSelfService(&EbookSelfServiceModel{
		Categories: []EbookSelfServiceCategoryModel{{ID: types.StringValue("64")}},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil {
		t.Fatal("expected a category on the wire")
	}
	cats := *ss.SelfServiceCategories.Category
	if len(cats) != 1 || cats[0].DisplayIn == nil || !*cats[0].DisplayIn {
		t.Fatalf("display_in must be sent as true for an id-only category, got %+v", cats)
	}
}

// TestEbookScopeClassesCleared_SingleRequestPaths pins every shape that must
// NOT cost a second write: no payload, no scope, an unmanaged class category,
// and a declared-empty one (the clear gesture, which a single request applies).
func TestEbookScopeClassesCleared_SingleRequestPaths(t *testing.T) {
	empty := []proclassic.IDName{}
	for _, tc := range []struct {
		name    string
		payload *proclassic.EbookPost
	}{
		{"nil payload", nil},
		{"no scope", &proclassic.EbookPost{General: &proclassic.EbookPostGeneral{}}},
		{"scope without classes", &proclassic.EbookPost{Scope: &proclassic.EbookPostScope{
			Departments: &proclassic.EbookScopeDepartments{Department: &[]proclassic.IDName{{ID: new(9)}}},
		}}},
		{"classes declared empty", &proclassic.EbookPost{Scope: &proclassic.EbookPostScope{
			Classes: &proclassic.EbookScopeClasses{Class: &empty},
		}}},
		{"classes wrapper without members", &proclassic.EbookPost{Scope: &proclassic.EbookPostScope{
			Classes: &proclassic.EbookScopeClasses{},
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ebookScopeClassesCleared(tc.payload); got != nil {
				t.Errorf("expected a single-request path, got a first-pass payload: %+v", got.Scope)
			}
		})
	}
}

// TestEbookScopeClassesCleared_FirstPassCarriesTheChange proves the first of the
// two requests is the caller's own body with only <classes> emptied: every
// other category is carried unchanged, and the caller's payload — the one that
// goes out second — is not mutated.
func TestEbookScopeClassesCleared_FirstPassCarriesTheChange(t *testing.T) {
	depts := []proclassic.IDName{{ID: new(9)}, {ID: new(10)}}
	classes := []proclassic.IDName{{ID: new(41)}, {ID: new(42)}}
	payload := &proclassic.EbookPost{
		General: &proclassic.EbookPostGeneral{Name: new("Field Guide")},
		Scope: &proclassic.EbookPostScope{
			AllComputers: new(false),
			Departments:  &proclassic.EbookScopeDepartments{Department: &depts},
			Classes:      &proclassic.EbookScopeClasses{Class: &classes},
			Limitations:  &proclassic.EbookScopeLimitations{},
		},
	}

	first := ebookScopeClassesCleared(payload)
	if first == nil {
		t.Fatalf("expected a first-pass payload for a scope carrying classes")
	}
	if first.Scope.Classes == nil || first.Scope.Classes.Class == nil || len(*first.Scope.Classes.Class) != 0 {
		t.Errorf("first pass must carry an explicitly empty classes category, got %+v", first.Scope.Classes)
	}
	if first.Scope.Departments != payload.Scope.Departments || first.Scope.Limitations != payload.Scope.Limitations {
		t.Errorf("first pass must carry every other scope category unchanged")
	}
	if first.General != payload.General || first.Scope.AllComputers != payload.Scope.AllComputers {
		t.Errorf("first pass must carry the rest of the body unchanged")
	}
	if payload.Scope.Classes == nil || payload.Scope.Classes.Class == nil || len(*payload.Scope.Classes.Class) != 2 {
		t.Errorf("the caller's payload was mutated: %+v", payload.Scope.Classes)
	}
	if len(classes) != 2 || classes[0].ID == nil || *classes[0].ID != 41 {
		t.Errorf("the caller's class slice was mutated: %+v", classes)
	}
}

// TestEbookScopeClassesCleared_EmitsAnExplicitEmptyElement checks the gesture
// that reaches the wire. The scope subtree replaces wholesale, so the first
// request has to land <classes></classes> — an omitted element would leave the
// body's meaning to the endpoint's merge rules rather than stating the clear.
func TestEbookScopeClassesCleared_EmitsAnExplicitEmptyElement(t *testing.T) {
	classes := []proclassic.IDName{{ID: new(41)}}
	payload := &proclassic.EbookPost{Scope: &proclassic.EbookPostScope{
		Departments: &proclassic.EbookScopeDepartments{Department: &[]proclassic.IDName{{ID: new(9)}}},
		Classes:     &proclassic.EbookScopeClasses{Class: &classes},
	}}

	first, err := xml.Marshal(ebookScopeClassesCleared(payload).Scope)
	if err != nil {
		t.Fatalf("marshal first pass: %v", err)
	}
	if got := string(first); !strings.Contains(got, "<classes></classes>") || strings.Contains(got, "<class>") {
		t.Errorf("first pass body must carry an empty classes element and no members: %s", got)
	}

	second, err := xml.Marshal(payload.Scope)
	if err != nil {
		t.Fatalf("marshal second pass: %v", err)
	}
	if got := string(second); !strings.Contains(got, "<class><id>41</id></class>") {
		t.Errorf("second pass body must carry the class members: %s", got)
	}
}
