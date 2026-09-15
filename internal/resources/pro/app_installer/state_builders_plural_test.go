// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func sampleEntries() []pro.AppTitleDeploymentSummary {
	return []pro.AppTitleDeploymentSummary{
		{
			ID:             "177",
			Name:           "Jamf Composer",
			Enabled:        true,
			DeploymentType: "SELF_SERVICE",
			UpdateBehavior: "AUTOMATIC",
			App: &pro.AppTitleDeploymentSummaryApp{
				ID:                  "Composer",
				BundleID:            new("com.jamfsoftware.Composer"),
				LatestVersion:       new("16.0.4"),
				SelectedVersion:     "",
				DeployedVersion:     "16.0.4",
				MediaSourceType:     "JAMF_SERVER",
				TitleAvailableInAis: true,
				VersionRemoved:      false,
			},
			Site:             &pro.AppInstallersSite{ID: "-1", Name: new("None")},
			Category:         &pro.AppInstallersCategory{ID: "58", Name: new("Productivity")},
			ComputerStatuses: &pro.AppInstallersInstallationSummary{Available: 3, Failed: 1, InProgress: 0, Installed: 5, Unqualified: 2},
		},
		{
			ID:             "200",
			Name:           "Adobe Lightroom Classic",
			DeploymentType: "INSTALL_AUTOMATICALLY",
			UpdateBehavior: "MANUAL",
		},
	}
}

func TestFilterAndMapDeployments_Expanded(t *testing.T) {
	got := FilterAndMapDeployments(sampleEntries(), types.StringNull())
	if len(got) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(got))
	}
	d := got[0]
	if d.ID.ValueString() != "177" || d.UpdateBehavior.ValueString() != "AUTOMATIC" {
		t.Errorf("entry 0 scalars mismatch: %+v", d)
	}
	if d.App == nil || d.App.BundleID.ValueString() != "com.jamfsoftware.Composer" {
		t.Errorf("entry 0 app mismatch: %+v", d.App)
	}
	if !d.App.TitleAvailableInAis.ValueBool() {
		t.Errorf("entry 0 title_available_in_ais should be true")
	}
	if d.Category == nil || d.Category.Name.ValueString() != "Productivity" {
		t.Errorf("entry 0 category mismatch: %+v", d.Category)
	}
	if d.SmartGroup != nil {
		t.Errorf("entry 0 smart_group should be nil (absent ref)")
	}
	if d.ComputerStatuses == nil || d.ComputerStatuses.Installed.ValueInt64() != 5 {
		t.Errorf("entry 0 computer_statuses mismatch: %+v", d.ComputerStatuses)
	}
}

func TestFilterAndMapDeployments_NilNestedRefs(t *testing.T) {
	got := FilterAndMapDeployments(sampleEntries(), types.StringNull())
	d := got[1]
	if d.App != nil || d.Site != nil || d.Category != nil || d.SmartGroup != nil || d.ComputerStatuses != nil {
		t.Errorf("entry 1 absent nested refs must map to nil: %+v", d)
	}
}

func TestFilterAndMapDeployments_Substring(t *testing.T) {
	got := FilterAndMapDeployments(sampleEntries(), types.StringValue("lightroom"))
	if len(got) != 1 || got[0].ID.ValueString() != "200" {
		t.Fatalf("substring filter mismatch: %+v", got)
	}
}

func TestFilterAndMapDeployments_EmptyNotNil(t *testing.T) {
	got := FilterAndMapDeployments(nil, types.StringNull())
	if got == nil {
		t.Fatalf("expected non-nil empty slice")
	}
}
