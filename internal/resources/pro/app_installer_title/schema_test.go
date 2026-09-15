// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_title

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

const wantTypeName = "jamfplatform_pro_app_installer_title"

func TestAppInstallerTitleDataSource_Metadata(t *testing.T) {
	d := NewAppInstallerTitleDataSource()
	var resp datasource.MetadataResponse
	d.(*AppInstallerTitleDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestAppInstallerTitleDataSource_Schema(t *testing.T) {
	d := NewAppInstallerTitleDataSource()
	var resp datasource.SchemaResponse
	d.(*AppInstallerTitleDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema
	for _, name := range []string{
		"id", "title_name", "publisher", "bundle_id", "version", "short_version",
		"architecture", "minimum_os_version", "language", "availability_date",
		"icon_url", "size_in_bytes", "installer_package_hash",
		"installer_package_hash_type", "launch_daemon_included",
		"notification_available", "package_signing_identity", "suppress_auto_update",
		"original_media_sources", "media_source_type", "installation_path_shared",
		"original_terms_and_conditions", "versions",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if id := s.Attributes["id"]; !id.IsRequired() {
		t.Errorf("id must be required for lookup")
	}
	// version is both a lookup argument and the echoed value.
	v := s.Attributes["version"]
	if !v.IsOptional() || !v.IsComputed() {
		t.Errorf("version must be Optional+Computed, got optional=%v computed=%v", v.IsOptional(), v.IsComputed())
	}
}

func TestAssignTitleDataSource(t *testing.T) {
	got := AssignTitleDataSource(&pro.AppTitleDetails{
		ID:                       "027",
		TitleName:                "Adobe Lightroom Classic",
		Publisher:                "Adobe",
		BundleID:                 "com.adobe.LightroomClassicCC7",
		Version:                  "14.2",
		ShortVersion:             "14.2",
		Architecture:             "universal",
		MinimumOsVersion:         "12.0",
		Language:                 "en",
		AvailabilityDate:         "2026-01-01",
		IconURL:                  "https://example/icon.png",
		SizeInBytes:              123456,
		InstallerPackageHash:     "abc",
		InstallerPackageHashType: "SHA_256",
		LaunchDaemonIncluded:     true,
		NotificationAvailable:    true,
		PackageSigningIdentity:   "Developer ID Installer: Adobe",
		SuppressAutoUpdate:       true,
		MediaSourceType:          "JAMF_SERVER",
		InstallationPathShared:   true,
		OriginalMediaSources: []pro.OriginalMediaSource{
			{Hash: "h1", HashType: "SHA_256", URL: "https://example/media"},
		},
		OriginalTermsAndConditions: []string{"https://example/terms"},
	})

	if got.ID.ValueString() != "027" {
		t.Errorf("id: got %q", got.ID.ValueString())
	}
	if got.SizeInBytes.ValueInt64() != 123456 {
		t.Errorf("size_in_bytes: got %d", got.SizeInBytes.ValueInt64())
	}
	if !got.LaunchDaemonIncluded.ValueBool() {
		t.Errorf("launch_daemon_included should be true")
	}
	if len(got.OriginalMediaSources) != 1 || got.OriginalMediaSources[0].Hash.ValueString() != "h1" {
		t.Errorf("original_media_sources not mapped: %+v", got.OriginalMediaSources)
	}
	if got.MediaSourceType.ValueString() != "JAMF_SERVER" {
		t.Errorf("media_source_type: got %q", got.MediaSourceType.ValueString())
	}
	if !got.InstallationPathShared.ValueBool() {
		t.Errorf("installation_path_shared should be true")
	}
	if len(got.OriginalTermsAndConditions.Elements()) != 1 {
		t.Errorf("original_terms_and_conditions not mapped: %v", got.OriginalTermsAndConditions)
	}
}

func TestAssignTitleVersions(t *testing.T) {
	got := assignTitleVersions(&pro.AppTitleVersionsResult{
		TotalCount: 2,
		Results: []pro.AppTitleVersionAndMediaSourceType{
			{Version: new("11.30.1"), MediaSourceType: "JAMF_SERVER"},
			{Version: new("11.31.0"), MediaSourceType: "JAMF_SERVER"},
		},
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(got))
	}
	if got[0].Version.ValueString() != "11.30.1" || got[0].MediaSourceType.ValueString() != "JAMF_SERVER" {
		t.Errorf("version 0 mismatch: %+v", got[0])
	}
}

// Most titles publish no earlier versions, so the empty case is the common one:
// it must be an empty list, never null, or the Computed attribute has no value.
func TestAssignTitleVersions_EmptyAndNilAreEmptyLists(t *testing.T) {
	for label, in := range map[string]*pro.AppTitleVersionsResult{
		"nil":   nil,
		"empty": {},
	} {
		got := assignTitleVersions(in)
		if got == nil {
			t.Errorf("%s: expected a non-nil empty slice", label)
		}
		if len(got) != 0 {
			t.Errorf("%s: expected no elements, got %d", label, len(got))
		}
	}
}

func TestAssignTitleDataSource_NilSafe(t *testing.T) {
	got := AssignTitleDataSource(nil)
	if !got.ID.IsNull() {
		t.Errorf("nil title should produce null id")
	}
}

// A vendor that publishes no terms and conditions must yield an empty list, not
// null: the attribute is Computed, so null after apply is a framework error.
func TestAssignTitleDataSource_NoTermsIsEmptyList(t *testing.T) {
	got := AssignTitleDataSource(&pro.AppTitleDetails{ID: "027"})
	if got.OriginalTermsAndConditions.IsNull() {
		t.Errorf("original_terms_and_conditions must be an empty list, not null")
	}
	if len(got.OriginalTermsAndConditions.Elements()) != 0 {
		t.Errorf("expected no elements, got %v", got.OriginalTermsAndConditions.Elements())
	}
}
