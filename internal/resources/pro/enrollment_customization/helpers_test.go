// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestBuildPanelIndex_GroupsByType(t *testing.T) {
	panels := []pro.GetEnrollmentCustomizationPanel{
		{ID: 1, Type: panelTypeText, Rank: 0},
		{ID: 2, Type: panelTypeSso, Rank: 1},
		{ID: 3, Type: panelTypeText, Rank: 1},
		{ID: 4, Type: panelTypeLdap, Rank: 2},
		{ID: 5, Type: "unknown-future-type"},
	}
	idx := buildPanelIndex(panels)
	if len(idx.Text) != 2 {
		t.Fatalf("text count = %d, want 2", len(idx.Text))
	}
	if len(idx.Sso) != 1 {
		t.Fatalf("sso count = %d, want 1", len(idx.Sso))
	}
	if len(idx.Ldap) != 1 {
		t.Fatalf("ldap count = %d, want 1", len(idx.Ldap))
	}
}

func TestIndexByID_SkipsEmptyKeys(t *testing.T) {
	type item struct {
		id string
	}
	items := []item{{id: "1"}, {id: ""}, {id: "2"}}
	m := indexByID(items, func(i item) string { return i.id })
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if _, ok := m["1"]; !ok {
		t.Fatalf("missing entry for id=1")
	}
	if _, ok := m[""]; ok {
		t.Fatalf("empty id should not be indexed")
	}
}
