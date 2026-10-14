// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func idSet(ids ...string) types.Set {
	vals := make([]attr.Value, 0, len(ids))
	for _, id := range ids {
		vals = append(vals, types.StringValue(id))
	}
	return types.SetValueMust(types.StringType, vals)
}

func TestBuildMacAppGeneral(t *testing.T) {
	m := &MacAppGeneralModel{
		Name:           types.StringValue("iMovie"),
		Version:        types.StringValue("10.4"),
		BundleID:       types.StringValue("com.apple.iMovieApp"),
		URL:            types.StringValue("https://apps.apple.com/app/id408981434"),
		IsFree:         types.BoolValue(true),
		DeploymentType: types.StringValue(deploymentTypeAutomatic),
		CategoryID:     types.StringValue("7"),
		SiteID:         types.StringValue("3"),
	}
	g := buildMacAppGeneral(m)
	if g.Name == nil || *g.Name != "iMovie" {
		t.Errorf("name not mapped: %+v", g.Name)
	}
	if g.Version == nil || *g.Version != "10.4" {
		t.Errorf("version not mapped")
	}
	if g.BundleID == nil || *g.BundleID != "com.apple.iMovieApp" {
		t.Errorf("bundle_id not mapped")
	}
	if g.DeploymentType == nil || *g.DeploymentType != deploymentTypeAutomatic {
		t.Errorf("deployment_type not mapped")
	}
	if g.IsFree == nil || !*g.IsFree {
		t.Errorf("is_free not mapped")
	}
	if g.Category == nil || g.Category.ID == nil || *g.Category.ID != 7 {
		t.Errorf("category id not mapped: %+v", g.Category)
	}
	if g.Site == nil || g.Site.ID == nil || *g.Site.ID != 3 {
		t.Errorf("site id not mapped: %+v", g.Site)
	}
}

func TestBuildMacAppNotification(t *testing.T) {
	// Neither configured → nil (omitted).
	if n := buildMacAppNotification(types.BoolNull(), types.StringNull()); n != nil {
		t.Errorf("expected nil notification when unconfigured, got %+v", n)
	}
	// Both configured → both legs set.
	n := buildMacAppNotification(types.BoolValue(true), types.StringValue("Self Service"))
	if n == nil || n.Enabled == nil || !*n.Enabled || n.Method == nil || *n.Method != "Self Service" {
		t.Fatalf("notification legs not assembled: %+v", n)
	}
}

func TestBuildMacAppScope_TargetsAndCollapse(t *testing.T) {
	ctx := context.Background()

	// Empty model collapses to nil so <scope> is omitted entirely.
	empty := &scope.ComputerScopeModelNoIbeacons{
		Targets: &scope.ComputerScopeTargetsModel{
			AllComputers:     types.BoolNull(),
			AllJssUsers:      types.BoolNull(),
			ComputerIDs:      types.SetNull(types.StringType),
			ComputerGroupIDs: types.SetNull(types.StringType),
			BuildingIDs:      types.SetNull(types.StringType),
			DepartmentIDs:    types.SetNull(types.StringType),
			UserIDs:          types.SetNull(types.StringType),
			UserGroupIDs:     types.SetNull(types.StringType),
		},
	}
	s, diags := buildMacAppScope(ctx, empty)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if s != nil {
		t.Errorf("expected nil scope for empty model, got %+v", s)
	}

	// Targets map into the SDK sub-blocks.
	m := &scope.ComputerScopeModelNoIbeacons{
		Targets: &scope.ComputerScopeTargetsModel{
			AllComputers:     types.BoolValue(false),
			AllJssUsers:      types.BoolNull(),
			ComputerIDs:      idSet("11", "12"),
			ComputerGroupIDs: idSet("5"),
			BuildingIDs:      types.SetNull(types.StringType),
			DepartmentIDs:    types.SetNull(types.StringType),
			UserIDs:          types.SetNull(types.StringType),
			UserGroupIDs:     types.SetNull(types.StringType),
		},
		Limitations: &scope.ComputerScopeLimitationsModelNoIbeacons{
			NetworkSegmentIDs:                idSet("2"),
			DirectoryServiceOrLocalUserNames: types.SetNull(types.StringType),
			DirectoryServiceUserGroupNames:   types.SetNull(types.StringType),
		},
	}
	s, diags = buildMacAppScope(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if s == nil || s.Computers == nil || s.Computers.Computer == nil || len(*s.Computers.Computer) != 2 {
		t.Fatalf("computers not mapped: %+v", s)
	}
	if s.ComputerGroups == nil || s.ComputerGroups.ComputerGroup == nil || len(*s.ComputerGroups.ComputerGroup) != 1 {
		t.Errorf("computer_groups not mapped")
	}
	if s.Limitations == nil || s.Limitations.NetworkSegments == nil {
		t.Errorf("limitations network_segments not mapped")
	}
}

func TestBuildMacAppVpp_Collapse(t *testing.T) {
	// Only computed counts present (no writable fields) → nil.
	m := &MacAppVppModel{
		AssignVppDeviceBasedLicenses: types.BoolNull(),
		VppAdminAccountID:            types.StringNull(),
		TotalVppLicenses:             types.Int64Value(10),
	}
	if v := buildMacAppVpp(m); v != nil {
		t.Errorf("expected nil vpp when no writable fields, got %+v", v)
	}

	// Writable field present → built, counts never written.
	m = &MacAppVppModel{
		AssignVppDeviceBasedLicenses: types.BoolValue(true),
		VppAdminAccountID:            types.StringValue("4"),
		TotalVppLicenses:             types.Int64Value(10),
	}
	v := buildMacAppVpp(m)
	if v == nil || v.AssignVppDeviceBasedLicenses == nil || !*v.AssignVppDeviceBasedLicenses {
		t.Fatalf("assign not mapped: %+v", v)
	}
	if v.VppAdminAccountID == nil || *v.VppAdminAccountID != 4 {
		t.Errorf("vpp_admin_account_id not mapped")
	}
	if v.TotalVppLicenses != nil {
		t.Errorf("computed counts must never be written, got %+v", v.TotalVppLicenses)
	}
}

func TestBuildMacAppSelfService_CategoriesAndIcon(t *testing.T) {
	m := &MacAppSelfServiceModel{
		InstallButtonText:   types.StringValue("Install"),
		NotificationEnabled: types.BoolValue(true),
		NotificationMethod:  types.StringValue("Self Service"),
		SelfServiceIcon:     &MacAppSelfServiceIconModel{ID: types.StringValue("9")},
		SelfServiceCategories: []MacAppSelfServiceCategoryModel{
			{ID: types.StringValue("3"), DisplayIn: types.BoolValue(true), FeatureIn: types.BoolValue(false)},
		},
	}
	ss := buildMacAppSelfService(m)
	if ss.Notification == nil || ss.Notification.Method == nil || *ss.Notification.Method != "Self Service" {
		t.Errorf("notification not assembled")
	}
	if ss.SelfServiceIcon == nil || ss.SelfServiceIcon.ID == nil || *ss.SelfServiceIcon.ID != 9 {
		t.Errorf("icon id not mapped: %+v", ss.SelfServiceIcon)
	}
	if ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil || len(*ss.SelfServiceCategories.Category) != 1 {
		t.Fatalf("categories not mapped")
	}
	c := (*ss.SelfServiceCategories.Category)[0]
	if c.ID == nil || *c.ID != 3 {
		t.Errorf("category id not mapped: %+v", c.ID)
	}
}

// TestBuildMacAppSelfService_CategoryIDOnlySendsDisplayIn guards the config
// shape that silently stored nothing. See helpers.SelfServiceCategoryDisplayIn.
func TestBuildMacAppSelfService_CategoryIDOnlySendsDisplayIn(t *testing.T) {
	t.Parallel()
	ss := buildMacAppSelfService(&MacAppSelfServiceModel{
		SelfServiceCategories: []MacAppSelfServiceCategoryModel{{ID: types.StringValue("64")}},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil {
		t.Fatal("expected a category on the wire")
	}
	cats := *ss.SelfServiceCategories.Category
	if len(cats) != 1 || cats[0].DisplayIn == nil || !*cats[0].DisplayIn {
		t.Fatalf("display_in must be sent as true for an id-only category, got %+v", cats)
	}
}

// TestBuildMacAppSelfService_CategoryNameNeverSent pins that a category entry
// carries only its id and flags. Jamf Pro ignores the name, and a name carried
// from another element would misdescribe the entry. See categoryName.
func TestBuildMacAppSelfService_CategoryNameNeverSent(t *testing.T) {
	t.Parallel()
	ss := buildMacAppSelfService(&MacAppSelfServiceModel{
		SelfServiceCategories: []MacAppSelfServiceCategoryModel{{ID: types.StringValue("64"), Name: types.StringValue("Other category")}},
	})
	if ss == nil || ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil || len(*ss.SelfServiceCategories.Category) != 1 {
		t.Fatal("expected one category on the wire")
	}
	if got := (*ss.SelfServiceCategories.Category)[0].Name; got != nil {
		t.Errorf("category name must not be sent, got %q", *got)
	}
}
