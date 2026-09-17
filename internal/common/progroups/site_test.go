// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestSiteIDForWrite pins that a site id is always emitted, and that the two
// shapes meaning "the operator said nothing" both become the no-site sentinel
// rather than the empty string Jamf Pro rejects.
func TestSiteIDForWrite(t *testing.T) {
	tests := []struct {
		name    string
		planned types.String
		want    string
	}{
		{name: "a real site", planned: types.StringValue("1"), want: "1"},
		{name: "the sentinel", planned: types.StringValue(NoSiteID), want: NoSiteID},
		{name: "null becomes the sentinel", planned: types.StringNull(), want: NoSiteID},
		{name: "unknown becomes the sentinel", planned: types.StringUnknown(), want: NoSiteID},
		{name: "empty becomes the sentinel", planned: types.StringValue(""), want: NoSiteID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SiteIDForWrite(tc.planned)
			if got == nil {
				t.Fatal("site id was omitted; every write in this family must emit one")
			}
			if *got != tc.want {
				t.Fatalf("got %q, want %q", *got, tc.want)
			}
		})
	}
}

// TestSiteIDForState pins that every way Jamf Pro says "no site" collapses to
// one value. The endpoints are inconsistent about it — "-1" from a computer GET,
// JSON null from the PUT response for the same group — and letting that through
// would flip a no-site group's state between refreshes.
func TestSiteIDForState(t *testing.T) {
	sentinel := NoSiteID
	empty := ""
	real := "1"

	tests := []struct {
		name string
		wire *string
		want string
	}{
		{name: "absent", wire: nil, want: NoSiteID},
		{name: "empty string", wire: &empty, want: NoSiteID},
		{name: "the sentinel", wire: &sentinel, want: NoSiteID},
		{name: "a real site", wire: &real, want: "1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SiteIDForState(tc.wire)
			if got.IsNull() || got.IsUnknown() {
				t.Fatalf("site_id must always be known in state, got %v", got)
			}
			if got.ValueString() != tc.want {
				t.Fatalf("got %q, want %q", got.ValueString(), tc.want)
			}
		})
	}
}
