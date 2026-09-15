// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package availabletitles

import (
	"encoding/xml"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestDataSourceAttributes_HasExpectedFields(t *testing.T) {
	attrs := DataSourceAttributes()
	for _, name := range []string{"name_id", "app_name", "current_version", "publisher", "last_modified"} {
		a, ok := attrs[name]
		if !ok {
			t.Errorf("missing attribute %q", name)
			continue
		}
		if !a.IsComputed() || a.IsOptional() || a.IsRequired() {
			t.Errorf("%q must be computed-only", name)
		}
	}
}

func TestMapTitles_NilAndEmptyYieldNonNilEmpty(t *testing.T) {
	cases := map[string]*proclassic.PatchAvailableTitles{
		"nil response":           nil,
		"nil AvailableTitles":    {},
		"nil AvailableTitle ptr": {AvailableTitles: &proclassic.PatchAvailableTitlesAvailableTitles{}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			got := MapTitles(in)
			if got == nil {
				t.Fatalf("expected non-nil slice, got nil")
			}
			if len(got) != 0 {
				t.Errorf("expected empty slice, got %d", len(got))
			}
		})
	}
}

// TestMapTitles_FromLiveWire pins the decode + flatten against the real
// /patchavailabletitles wire shape (free-form name_id values included).
func TestMapTitles_FromLiveWire(t *testing.T) {
	wire := `<patch_available_titles><size>2</size><available_titles>` +
		`<available_title><name_id>518</name_id><last_modified>2026-03-05T15:31:49Z</last_modified><current_version>16.0.4</current_version><publisher>Jamf</publisher><app_name>Jamf Composer</app_name></available_title>` +
		`<available_title><name_id>com.cisco.anyconnect.gui</name_id><last_modified>2024-01-10T22:12:48Z</last_modified><current_version>4.10.07073</current_version><publisher>Cisco</publisher><app_name>Cisco AnyConnect Secure Mobility Client</app_name></available_title>` +
		`</available_titles></patch_available_titles>`

	var p proclassic.PatchAvailableTitles
	if err := xml.Unmarshal([]byte(wire), &p); err != nil {
		t.Fatalf("unmarshal live wire: %v", err)
	}

	got := MapTitles(&p)
	if len(got) != 2 {
		t.Fatalf("expected 2 titles, got %d", len(got))
	}
	if got[0].NameID.ValueString() != "518" || got[0].AppName.ValueString() != "Jamf Composer" {
		t.Errorf("title[0] mismatch: name_id=%q app_name=%q", got[0].NameID.ValueString(), got[0].AppName.ValueString())
	}
	if got[0].CurrentVersion.ValueString() != "16.0.4" || got[0].Publisher.ValueString() != "Jamf" {
		t.Errorf("title[0] version/publisher mismatch: %q / %q", got[0].CurrentVersion.ValueString(), got[0].Publisher.ValueString())
	}
	if got[1].NameID.ValueString() != "com.cisco.anyconnect.gui" {
		t.Errorf("title[1] reverse-DNS name_id mismatch: %q", got[1].NameID.ValueString())
	}
	if got[1].LastModified.ValueString() != "2024-01-10T22:12:48Z" {
		t.Errorf("title[1] last_modified mismatch: %q", got[1].LastModified.ValueString())
	}
}
