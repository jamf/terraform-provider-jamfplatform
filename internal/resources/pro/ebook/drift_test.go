// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestFlattenEbookGeneral_ReportsDrift pins the wire-authoritative read: an
// echoed value that differs from state must land in state so `terraform plan`
// reports the change. Every field asserted here round-trips through the classic
// /ebooks GET (Jamf Pro 11.31.1, wire-probed 2026-09-06). See issue #387.
func TestFlattenEbookGeneral_ReportsDrift(t *testing.T) {
	t.Parallel()
	state := &EbookGeneralModel{
		Author:         types.StringValue("state author"),
		DeploymentType: types.StringValue("Make Available in Self Service"),
		Version:        types.StringValue("1.0"),
		Free:           types.BoolValue(false),
		CategoryID:     types.StringValue("11"),
		SiteID:         types.StringValue("-1"),
	}
	flattenEbookGeneral(&proclassic.EbookGeneral{
		Name:           new("guide"),
		Author:         new("wire author"),
		DeploymentType: new("Install Automatically/Prompt Users to Install"),
		Version:        new("2.0"),
		Free:           new(true),
		Category:       &proclassic.CategoryObject{ID: new(653), Name: new("Operations")},
		Site:           &proclassic.SiteObject{ID: new(1), Name: new("AGATA")},
	}, state)

	for _, tc := range []struct{ name, want, got string }{
		{"author", "wire author", state.Author.ValueString()},
		{"deployment_type", "Install Automatically/Prompt Users to Install", state.DeploymentType.ValueString()},
		{"version", "2.0", state.Version.ValueString()},
		{"category_id", "653", state.CategoryID.ValueString()},
		{"site_id", "1", state.SiteID.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
	if !state.Free.ValueBool() {
		t.Error("free: wire value must win, got false")
	}
}

// TestFlattenEbook_StickyFieldsIgnoreDrift pins the other half of the #387
// split for this resource: file_type (the server canonicalises the casing) and
// deploy_as_managed (the write does not persist) keep the value already in
// state.
//
// The four self_service notification_* fields are asserted here too, but for a
// different rule: they are echoed only while the tenant-level Self Service
// notifications toggle is on, so with the toggle off — the empty
// EbookSelfService passed in — state must be kept rather than nulled.
// TestFlattenEbookSelfService_NotificationDriftWhenEchoed covers the other
// side.
func TestFlattenEbook_StickyFieldsIgnoreDrift(t *testing.T) {
	t.Parallel()
	general := &EbookGeneralModel{
		FileType:        types.StringValue("ePub"),
		DeployAsManaged: types.BoolValue(false),
	}
	flattenEbookGeneral(&proclassic.EbookGeneral{
		Name:            new("guide"),
		FileType:        new("EPUB"),
		DeployAsManaged: new(true),
	}, general)
	if got := general.FileType.ValueString(); got != "ePub" {
		t.Errorf("file_type: sticky read must keep the caller's casing, got %q", got)
	}
	if general.DeployAsManaged.ValueBool() {
		t.Error("deploy_as_managed: sticky read must keep false")
	}

	ss := &EbookSelfServiceModel{
		NotificationEnabled: types.BoolValue(true),
		NotificationMethod:  types.StringValue("Self Service"),
		NotificationSubject: types.StringValue("state subject"),
		NotificationMessage: types.StringValue("state message"),
	}
	flattenEbookSelfService(&proclassic.EbookSelfService{}, ss)
	for _, tc := range []struct{ name, want, got string }{
		{"notification_method", "Self Service", ss.NotificationMethod.ValueString()},
		{"notification_subject", "state subject", ss.NotificationSubject.ValueString()},
		{"notification_message", "state message", ss.NotificationMessage.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: toggle off, so state must be kept as %q, got %q", tc.name, tc.want, tc.got)
		}
	}
	if !ss.NotificationEnabled.ValueBool() {
		t.Error("notification_enabled: toggle off, so state must be kept as true")
	}
}

// TestFlattenEbookSelfService_NotificationDriftWhenEchoed pins the other side of
// the conditional echo: while the tenant-level Self Service notifications
// toggle is on the classic GET does return the <notification> family, and a
// value that differs from state must then win so drift is reported.
func TestFlattenEbookSelfService_NotificationDriftWhenEchoed(t *testing.T) {
	t.Parallel()
	ss := &EbookSelfServiceModel{
		NotificationEnabled: types.BoolValue(true),
		NotificationMethod:  types.StringValue("Self Service"),
		NotificationSubject: types.StringValue("state subject"),
		NotificationMessage: types.StringValue("state message"),
	}
	flattenEbookSelfService(&proclassic.EbookSelfService{
		Notification:        &proclassic.NotificationValue{Enabled: new(false), Method: new("Self Service and Notification Center")},
		NotificationSubject: new("wire subject"),
		NotificationMessage: new("wire message"),
	}, ss)
	for _, tc := range []struct{ name, want, got string }{
		{"notification_method", "Self Service and Notification Center", ss.NotificationMethod.ValueString()},
		{"notification_subject", "wire subject", ss.NotificationSubject.ValueString()},
		{"notification_message", "wire message", ss.NotificationMessage.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
	if ss.NotificationEnabled.ValueBool() {
		t.Error("notification_enabled: wire false must win over state true")
	}
}
