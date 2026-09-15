// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

func TestAssignDeviceGroupResourceModel(t *testing.T) {
	var state DeviceGroupResourceModel

	assignDeviceGroupResourceModel(&state, &securitycloud.Group{
		ID:   "57497e81-d499-4f99-8fe8-8f262d0f5b8f",
		Name: "Executives",
	})

	if got := state.ID.ValueString(); got != "57497e81-d499-4f99-8fe8-8f262d0f5b8f" {
		t.Errorf("ID = %q, want the group ID", got)
	}
	if got := state.Name.ValueString(); got != "Executives" {
		t.Errorf("Name = %q, want %q", got, "Executives")
	}
}

// TestAssignDeviceGroupResourceModel_EmptyIDDoesNotClobber pins the ID guard. A
// caller that already holds the ID — Create, after the POST — must not have it
// blanked by a response that omits it.
func TestAssignDeviceGroupResourceModel_EmptyIDDoesNotClobber(t *testing.T) {
	state := DeviceGroupResourceModel{ID: types.StringValue("kept")}

	assignDeviceGroupResourceModel(&state, &securitycloud.Group{ID: "", Name: "Executives"})

	if got := state.ID.ValueString(); got != "kept" {
		t.Errorf("ID = %q, want the prior value to survive an empty echo", got)
	}
}

// TestAssignDeviceGroupResourceModel_NameIsTakenFromTheServer pins that the read
// is authoritative for the name. The plan-time validator stops the whitespace the
// server would trim, but if the server ever normalises something else, state must
// carry what was stored rather than what was sent.
func TestAssignDeviceGroupResourceModel_NameIsTakenFromTheServer(t *testing.T) {
	state := DeviceGroupResourceModel{Name: types.StringValue("what we sent")}

	assignDeviceGroupResourceModel(&state, &securitycloud.Group{ID: "abc", Name: "what was stored"})

	if got := state.Name.ValueString(); got != "what was stored" {
		t.Errorf("Name = %q, want the server's value", got)
	}
}

func TestAssignDeviceGroupDataSourceModel(t *testing.T) {
	var state DeviceGroupDataSourceModel

	assignDeviceGroupDataSourceModel(&state, &securitycloud.Group{ID: "abc", Name: "Executives"})

	if got := state.ID.ValueString(); got != "abc" {
		t.Errorf("ID = %q, want %q", got, "abc")
	}
	if got := state.Name.ValueString(); got != "Executives" {
		t.Errorf("Name = %q, want %q", got, "Executives")
	}
}

// TestBuildDeviceGroupsResultModel covers the one asymmetry in the plural data
// source: the implicit "Default Group" arrives with no `id` key, so its ID decodes
// as the empty string. Reporting that as "" would assert a value the API does not
// have, so it becomes null and built_in becomes true.
func TestBuildDeviceGroupsResultModel(t *testing.T) {
	tests := []struct {
		name        string
		item        securitycloud.GroupListItem
		wantIDNull  bool
		wantID      string
		wantBuiltIn bool
	}{
		{
			name:        "stored group",
			item:        securitycloud.GroupListItem{ID: "abc", Name: "Executives"},
			wantID:      "abc",
			wantBuiltIn: false,
		},
		{
			name:        "built-in group has no identifier",
			item:        securitycloud.GroupListItem{ID: "", Name: defaultGroupName},
			wantIDNull:  true,
			wantBuiltIn: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildDeviceGroupsResultModel(tc.item)

			if got.ID.IsNull() != tc.wantIDNull {
				t.Errorf("ID.IsNull() = %v, want %v", got.ID.IsNull(), tc.wantIDNull)
			}
			if !tc.wantIDNull && got.ID.ValueString() != tc.wantID {
				t.Errorf("ID = %q, want %q", got.ID.ValueString(), tc.wantID)
			}
			if got.BuiltIn.ValueBool() != tc.wantBuiltIn {
				t.Errorf("BuiltIn = %v, want %v", got.BuiltIn.ValueBool(), tc.wantBuiltIn)
			}
			if got.Name.ValueString() != tc.item.Name {
				t.Errorf("Name = %q, want %q", got.Name.ValueString(), tc.item.Name)
			}
			if got.BuiltIn.IsNull() {
				t.Error("BuiltIn must always be a known value, never null")
			}
		})
	}
}

// TestManageableGroups pins the filter the list resource applies. The built-in
// group is identified by its missing ID rather than by its name, so the rule holds
// even if Jamf ever labels it differently.
func TestManageableGroups(t *testing.T) {
	tests := []struct {
		name  string
		items []securitycloud.GroupListItem
		want  []string
	}{
		{
			name:  "nil list",
			items: nil,
			want:  []string{},
		},
		{
			name: "built-in group dropped, order preserved",
			items: []securitycloud.GroupListItem{
				{ID: "a", Name: "Alpha"},
				{ID: "", Name: defaultGroupName},
				{ID: "b", Name: "Bravo"},
			},
			want: []string{"a", "b"},
		},
		{
			name: "an unidentified entry under any name is dropped",
			items: []securitycloud.GroupListItem{
				{ID: "", Name: "Some Other Implicit Group"},
				{ID: "a", Name: "Alpha"},
			},
			want: []string{"a"},
		},
		{
			name: "every entry manageable",
			items: []securitycloud.GroupListItem{
				{ID: "a", Name: "Alpha"},
				{ID: "b", Name: "Bravo"},
			},
			want: []string{"a", "b"},
		},
		{
			name:  "only the built-in group",
			items: []securitycloud.GroupListItem{{ID: "", Name: defaultGroupName}},
			want:  []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := manageableGroups(tc.items)

			if len(got) != len(tc.want) {
				t.Fatalf("got %d groups, want %d (%v)", len(got), len(tc.want), got)
			}
			for i, id := range tc.want {
				if got[i].ID != id {
					t.Errorf("groups[%d].ID = %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}
