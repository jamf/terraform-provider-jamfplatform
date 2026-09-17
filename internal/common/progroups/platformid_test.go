// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestGroupTypeFilterValue pins each device kind to the SDK's own groupType
// vocabulary, and so fails if the two branches are ever swapped. Jamf Pro
// matches this filter field case-sensitively and answers a value it does not
// recognise with an empty page rather than an error, so a wrong value here
// nulls platform_id on every group of that kind and reports nothing. The want
// values are the SDK constants, never restated literals — a literal is exactly
// the drift this test exists to catch.
func TestGroupTypeFilterValue(t *testing.T) {
	tests := []struct {
		name string
		dt   DeviceType
		want string
	}{
		{name: "computer", dt: DeviceTypeComputer, want: pro.GroupDtoV1GroupTypeComputer},
		{name: "mobile", dt: DeviceTypeMobile, want: pro.GroupDtoV1GroupTypeMobile},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupTypeFilterValue(tc.dt); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSmartFilterValue pins each group kind to the isSmart the filter expects,
// and so fails if the two branches are ever swapped. A swap is the worst of the
// two failures this file guards: the filter still matches groups, just the
// wrong half of them, so every platform_id resolves to null while the request
// itself succeeds. The want values are bare literals deliberately — the field
// is a quoted JSON boolean for which the SDK generates no vocabulary to borrow.
func TestSmartFilterValue(t *testing.T) {
	tests := []struct {
		name string
		kind GroupKind
		want string
	}{
		{name: "smart", kind: GroupKindSmart, want: "true"},
		{name: "static", kind: GroupKindStatic, want: "false"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := smartFilterValue(tc.kind); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPlatformIDValue pins that a miss is null rather than an empty string.
// The contract the whole bridge rests on is that a group the lookup could not
// resolve carries a null platform_id, plans null and heals on the next refresh;
// rendering types.StringValue("") instead would leave the attribute
// permanently non-null and empty, which no refresh ever repairs. An empty value
// stored in the map counts as a miss for the same reason.
func TestPlatformIDValue(t *testing.T) {
	byJamfProID := map[string]string{
		"1": "5e8f2b7a-0000-4000-8000-000000000001",
		"2": "",
	}

	tests := []struct {
		name      string
		byJamfPro map[string]string
		jamfProID string
		want      string
		wantNull  bool
	}{
		{name: "a hit", byJamfPro: byJamfProID, jamfProID: "1", want: byJamfProID["1"]},
		{name: "a miss", byJamfPro: byJamfProID, jamfProID: "3", wantNull: true},
		{name: "an empty value is a miss", byJamfPro: byJamfProID, jamfProID: "2", wantNull: true},
		{name: "a nil map is a miss", jamfProID: "1", wantNull: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PlatformIDValue(tc.byJamfPro, tc.jamfProID)
			if got.IsUnknown() {
				t.Fatalf("platform_id must never be unknown in state, got %v", got)
			}
			if tc.wantNull {
				if !got.IsNull() {
					t.Fatalf("got %v, want null so the next refresh can heal it", got)
				}
				return
			}
			if got.IsNull() {
				t.Fatalf("got null, want %q", tc.want)
			}
			if got.ValueString() != tc.want {
				t.Fatalf("got %q, want %q", got.ValueString(), tc.want)
			}
		})
	}
}
