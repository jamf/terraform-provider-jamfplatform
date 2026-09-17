// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"strings"
	"testing"
)

// TestDeviceGroupAlternative pins that every construct's opening paragraph names
// the right jamfplatform_device_group configuration to use instead, and says
// that a Jamf Pro site is the only reason to prefer this family. The wording is
// rendered from one template precisely so this can be asserted once for all
// sixteen constructs.
func TestDeviceGroupAlternative(t *testing.T) {
	tests := []struct {
		name string
		dt   DeviceType
		kind GroupKind
		want []string
	}{
		{name: "smart computer", dt: DeviceTypeComputer, kind: GroupKindSmart, want: []string{`device_type = "computer"`, `group_type = "smart"`, "computer group"}},
		{name: "static computer", dt: DeviceTypeComputer, kind: GroupKindStatic, want: []string{`device_type = "computer"`, `group_type = "static"`}},
		{name: "smart mobile", dt: DeviceTypeMobile, kind: GroupKindSmart, want: []string{`device_type = "mobile"`, `group_type = "smart"`, "mobile device group"}},
		{name: "static mobile", dt: DeviceTypeMobile, kind: GroupKindStatic, want: []string{`device_type = "mobile"`, `group_type = "static"`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DeviceGroupAlternative(tc.dt, tc.kind)
			for _, want := range append(tc.want, "jamfplatform_device_group", "site") {
				if !strings.Contains(got, want) {
					t.Errorf("does not mention %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestLabelAndMemberNoun pins the admin-UI wording diagnostics and impact alerts
// are written in, so an alert reads the way Jamf Pro's own does.
func TestLabelAndMemberNoun(t *testing.T) {
	labels := map[string]string{
		Label(DeviceTypeComputer, GroupKindSmart):  "smart computer group",
		Label(DeviceTypeComputer, GroupKindStatic): "static computer group",
		Label(DeviceTypeMobile, GroupKindSmart):    "smart mobile device group",
		Label(DeviceTypeMobile, GroupKindStatic):   "static mobile device group",
	}
	if len(labels) != 4 {
		t.Fatalf("the four labels are not distinct: %v", labels)
	}
	for got, want := range labels {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}

	if got := MemberNoun(DeviceTypeComputer); got != "computers" {
		t.Errorf("computer member noun = %q", got)
	}
	if got := MemberNoun(DeviceTypeMobile); got != "mobile devices" {
		t.Errorf("mobile member noun = %q", got)
	}
}
