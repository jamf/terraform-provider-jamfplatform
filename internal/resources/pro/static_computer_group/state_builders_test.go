// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

//go:fix inline

func TestAssignStaticComputerGroupResourceState(t *testing.T) {
	state := StaticComputerGroupResourceModel{
		ID:          types.StringValue("41"),
		Name:        types.StringValue("stale"),
		Description: types.StringValue("stale"),
		SiteID:      types.StringValue("9"),
	}
	assignStaticComputerGroupResourceState(&state, &pro.StaticComputerGroup{
		ID:          "41",
		Name:        "Lab Macs",
		Description: new("Ground floor"),
		SiteID:      new("7"),
	})

	if state.Name.ValueString() != "Lab Macs" {
		t.Errorf("name = %q, want %q", state.Name.ValueString(), "Lab Macs")
	}
	if state.Description.ValueString() != "Ground floor" {
		t.Errorf("description = %q, want %q", state.Description.ValueString(), "Ground floor")
	}
	if state.SiteID.ValueString() != "7" {
		t.Errorf("site_id = %q, want %q", state.SiteID.ValueString(), "7")
	}
}

// TestAssignStaticComputerGroupResourceState_NormalisesTheNoSiteSentinel pins
// that every shape Jamf Pro uses for "no site" collapses to one value, so a
// refresh cannot flip state between them.
func TestAssignStaticComputerGroupResourceState_NormalisesTheNoSiteSentinel(t *testing.T) {
	for _, wire := range []*string{nil, new(""), new(progroups.NoSiteID)} {
		state := StaticComputerGroupResourceModel{}
		assignStaticComputerGroupResourceState(&state, &pro.StaticComputerGroup{ID: "1", Name: "n", SiteID: wire})
		if state.SiteID.ValueString() != progroups.NoSiteID {
			t.Errorf("site_id = %q for wire value %v, want %q", state.SiteID.ValueString(), wire, progroups.NoSiteID)
		}
	}
}

// TestAssignStaticComputerGroupResourceState_Description walks every pairing of
// what Jamf Pro reports against what state already holds, because description is
// Optional+Computed and the two ends of the table fail in opposite ways. An
// authored empty string must survive — the schema tells an operator to write one
// to clear the note, every write is a full replace so the read echoes the empty
// string back, and nulling it there aborts the apply with Terraform Core's
// inconsistent-result error. An unauthored one must land null, so that an import
// and a post-apply refresh of the same group agree.
func TestAssignStaticComputerGroupResourceState_Description(t *testing.T) {
	for _, tc := range []struct {
		name  string
		wire  *string
		prior types.String
		want  types.String
	}{
		{name: "absent against an unauthored note", wire: nil, prior: types.StringNull(), want: types.StringNull()},
		{name: "empty against an unauthored note", wire: new(""), prior: types.StringNull(), want: types.StringNull()},
		{name: "empty against an authored empty note", wire: new(""), prior: types.StringValue(""), want: types.StringValue("")},
		{name: "absent against an authored empty note", wire: nil, prior: types.StringValue(""), want: types.StringValue("")},
		{name: "empty against an authored note", wire: new(""), prior: types.StringValue("noted"), want: types.StringNull()},
		{name: "a note against an unauthored one", wire: new("noted"), prior: types.StringNull(), want: types.StringValue("noted")},
		{name: "a note against an authored empty one", wire: new("noted"), prior: types.StringValue(""), want: types.StringValue("noted")},
		{name: "a changed note", wire: new("noted"), prior: types.StringValue("stale"), want: types.StringValue("noted")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := StaticComputerGroupResourceModel{Description: tc.prior}
			assignStaticComputerGroupResourceState(&state, &pro.StaticComputerGroup{ID: "1", Name: "n", Description: tc.wire})
			if !state.Description.Equal(tc.want) {
				t.Errorf("description = %v, want %v", state.Description, tc.want)
			}
		})
	}
}

// TestAssignStaticComputerGroupResourceState_ImportAndRefreshAgree pins the
// consequence the reconcile has to preserve: the same response, read once with
// nothing in state and once after an apply, lands the same value.
func TestAssignStaticComputerGroupResourceState_ImportAndRefreshAgree(t *testing.T) {
	for _, wire := range []*string{nil, new(""), new("noted")} {
		onImport := StaticComputerGroupResourceModel{}
		assignStaticComputerGroupResourceState(&onImport, &pro.StaticComputerGroup{ID: "1", Name: "n", Description: wire})

		afterApply := StaticComputerGroupResourceModel{Description: onImport.Description}
		assignStaticComputerGroupResourceState(&afterApply, &pro.StaticComputerGroup{ID: "1", Name: "n", Description: wire})

		if !onImport.Description.Equal(afterApply.Description) {
			t.Errorf("import landed %v and a refresh landed %v for the same response", onImport.Description, afterApply.Description)
		}
	}
}

func TestAssignStaticComputerGroupResourceState_NilResponseChangesNothing(t *testing.T) {
	state := StaticComputerGroupResourceModel{Name: types.StringValue("kept")}
	assignStaticComputerGroupResourceState(&state, nil)
	if state.Name.ValueString() != "kept" {
		t.Errorf("name = %q, want it untouched", state.Name.ValueString())
	}
}

func TestAssignStaticComputerGroupDataSourceState(t *testing.T) {
	data := StaticComputerGroupDataSourceModel{}
	assignStaticComputerGroupDataSourceState(&data, &pro.StaticComputerGroup{
		ID:     "41",
		Name:   "Lab Macs",
		SiteID: nil,
	})
	if data.ID.ValueString() != "41" || data.Name.ValueString() != "Lab Macs" {
		t.Errorf("id/name = %q/%q, want 41/Lab Macs", data.ID.ValueString(), data.Name.ValueString())
	}
	if data.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("site_id = %q, want %q", data.SiteID.ValueString(), progroups.NoSiteID)
	}
	if !data.Description.IsNull() {
		t.Errorf("description = %v, want null when Jamf Pro reports none", data.Description)
	}
}

func TestFlattenStaticComputerGroupMembership(t *testing.T) {
	group := &proclassic.ComputerGroup{
		Computers: &proclassic.ComputerGroupComputers{
			Computer: &[]proclassic.ComputerGroupComputersComputerItem{
				{ID: new(11), Name: new("mac-a")},
				{ID: new(12), Name: new("mac-b")},
				{Name: new("no-id")},
			},
		},
	}
	got := flattenStaticComputerGroupMembership(group)
	if len(got) != 2 || got[0] != "11" || got[1] != "12" {
		t.Errorf("got %v, want [11 12] with the identifier-less entry dropped", got)
	}
}

// TestFlattenStaticComputerGroupMembership_EmptyShapes pins that every way Jamf
// Pro reports a memberless group comes back as nil, which the callers read as
// "the group is empty" rather than "nothing was read".
func TestFlattenStaticComputerGroupMembership_EmptyShapes(t *testing.T) {
	for name, group := range map[string]*proclassic.ComputerGroup{
		"nil group":     nil,
		"no block":      {},
		"empty block":   {Computers: &proclassic.ComputerGroupComputers{}},
		"empty listing": {Computers: &proclassic.ComputerGroupComputers{Computer: &[]proclassic.ComputerGroupComputersComputerItem{}}},
	} {
		if got := flattenStaticComputerGroupMembership(group); len(got) != 0 {
			t.Errorf("%s: got %v, want no members", name, got)
		}
	}
}

// TestMembershipSetValue_NilBecomesEmpty pins the difference that decides
// whether an apply succeeds: a nil slice would make a null set, which collides
// with a configuration that asked for an empty one.
func TestMembershipSetValue_NilBecomesEmpty(t *testing.T) {
	got, diags := membershipSetValue(context.Background(), nil)
	if diags.HasError() {
		t.Fatalf("diagnostics: %v", diags)
	}
	if got.IsNull() {
		t.Fatal("a nil membership must render as an empty set, not a null one")
	}
	if len(got.Elements()) != 0 {
		t.Errorf("got %v, want an empty set", got)
	}
}

// TestMembershipOwnershipGate walks the table Read implements through the shared
// helper, because getting it wrong is silent: an unmanaged membership that
// leaked into state would plan as a removal on the next apply, and an import
// that adopted an empty one would break ImportStateVerify.
func TestMembershipOwnershipGate(t *testing.T) {
	ctx := context.Background()
	empty, _ := membershipSetValue(ctx, nil)
	wire, _ := membershipSetValue(ctx, []string{"11"})

	for _, tc := range []struct {
		name           string
		prior          types.Set
		wire           types.Set
		firstHydration bool
		wireMembers    int
		wantNull       bool
		wantElements   int
	}{
		{name: "managed adopts the wire", prior: stringSet(t, "99"), wire: wire, wireMembers: 1, wantElements: 1},
		{name: "managed adopts an empty wire", prior: stringSet(t, "99"), wire: empty, wireMembers: 0, wantElements: 0},
		{name: "unmanaged stays null", prior: types.SetNull(types.StringType), wire: wire, wireMembers: 1, wantNull: true},
		{name: "import adopts members", prior: types.SetNull(types.StringType), wire: wire, firstHydration: true, wireMembers: 1, wantElements: 1},
		{name: "import of an empty group stays null", prior: types.SetNull(types.StringType), wire: empty, firstHydration: true, wireMembers: 0, wantNull: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := scope.RefreshManagedSet(tc.prior, tc.wire, tc.firstHydration && tc.wireMembers > 0)
			if got.IsNull() != tc.wantNull {
				t.Fatalf("null = %v, want %v", got.IsNull(), tc.wantNull)
			}
			if !tc.wantNull && len(got.Elements()) != tc.wantElements {
				t.Errorf("got %d elements, want %d", len(got.Elements()), tc.wantElements)
			}
		})
	}
}
