// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

func TestMembershipChanged(t *testing.T) {
	one := stringSet(t, "1")
	oneAgain := stringSet(t, "1")
	two := stringSet(t, "1", "2")
	empty := stringSet(t)
	unmanaged := types.SetNull(types.StringType)

	tests := []struct {
		name    string
		planned types.Set
		current types.Set
		want    bool
	}{
		{name: "a rename leaves the same members on both sides", planned: one, current: oneAgain, want: false},
		{name: "an unmanaged group compares null against null", planned: unmanaged, current: unmanaged, want: false},
		{name: "adding a device is a change", planned: two, current: one, want: true},
		{name: "removing a device is a change", planned: one, current: two, want: true},
		{name: "clearing a group is a change", planned: empty, current: one, want: true},
		{name: "taking over an unmanaged group is a change", planned: one, current: unmanaged, want: true},
		{name: "giving up management is a change", planned: unmanaged, current: one, want: true},
		// An empty set and no set at all mean different things here: one says
		// "hold nobody", the other says "Terraform is not deciding".
		{name: "an empty set differs from an unmanaged one", planned: empty, current: unmanaged, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := membershipChanged(tc.planned, tc.current); got != tc.want {
				t.Errorf("membershipChanged = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestIsKnownSet(t *testing.T) {
	if isKnownSet(types.SetNull(types.StringType)) {
		t.Error("a null set carries no count")
	}
	if isKnownSet(types.SetUnknown(types.StringType)) {
		t.Error("an unknown set carries no count")
	}
	if !isKnownSet(stringSet(t)) {
		t.Error("an empty set is a known count of zero")
	}
	if !isKnownSet(stringSet(t, "1")) {
		t.Error("a populated set is a known count")
	}
}

func TestImpactLabelAndNounMatchTheFamily(t *testing.T) {
	// The alert reads in the admin UI's words, and both come from the shared
	// helper so the four constructs cannot describe themselves differently.
	if got := progroups.Label(deviceType, groupKind); got != "static mobile device group" {
		t.Errorf("label = %q", got)
	}
	if got := progroups.MemberNoun(deviceType); got != "mobile devices" {
		t.Errorf("noun = %q", got)
	}
}
