// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignPackageResourceModel_NilResponse(t *testing.T) {
	var state PackageResourceModel
	diags := assignPackageResourceModel(&state, nil)
	if !diags.HasError() {
		t.Fatalf("expected error diag on nil response")
	}
}

func TestAssignPackageResourceModel_ServerEmptyStringsCollapseToNull(t *testing.T) {
	// Per §12.2 the server returns "" for every server-defaulted *string
	// field on a fresh record. reconcileOptionalStringPointer should
	// collapse those to null in state because the user did not configure
	// them.
	pkgID := "100"
	state := PackageResourceModel{}
	resp := &pro.Package{
		ID:                  &pkgID,
		PackageName:         "MyApp",
		FileName:            "MyApp.pkg",
		CategoryID:          "-1",
		Priority:            10,
		Info:                new(""),
		Notes:               new(""),
		Md5:                 new(""),
		Sha256:              new(""),
		Sha3512:             new(""),
		HashType:            new("SHA_512"), // server default pre-upload
		HashValue:           new(""),
		Size:                new(""),
		OsRequirements:      new(""),
		Manifest:            new(""),
		ManifestFileName:    new(""),
		InstallLanguage:     new("en_US"),
		ParentPackageID:     new("-1"),
		SelfHealingAction:   new("nothing"),
		CloudTransferStatus: new(""),
		Format:              new(""),
	}

	if diags := assignPackageResourceModel(&state, resp); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	// Required-style fields must always reflect.
	if state.ID.ValueString() != pkgID {
		t.Errorf("ID = %v, want %v", state.ID.ValueString(), pkgID)
	}
	if state.DisplayName.ValueString() != "MyApp" {
		t.Errorf("DisplayName = %v", state.DisplayName.ValueString())
	}
	if state.FileName.ValueString() != "MyApp.pkg" {
		t.Errorf("FileName = %v", state.FileName.ValueString())
	}

	// Server-defaulted strings the user did not configure must land on null
	// (their server value was "").
	nullStrings := map[string]types.String{
		"Info":                state.Info,
		"Notes":               state.Notes,
		"Md5":                 state.Md5,
		"Sha256":              state.Sha256,
		"Sha3512":             state.Sha3512,
		"HashValue":           state.HashValue,
		"Size":                state.Size,
		"OsRequirements":      state.OSRequirements,
		"Manifest":            state.Manifest,
		"ManifestFileName":    state.ManifestFileName,
		"CloudTransferStatus": state.CloudTransferStatus,
		"Format":              state.Format,
	}
	for name, v := range nullStrings {
		if !v.IsNull() {
			t.Errorf("%s must be null when server returned \"\", got %q", name, v.ValueString())
		}
	}

	// Server-defaulted strings with non-empty server values land directly.
	for name, want := range map[string]string{
		"InstallLanguage":   "en_US",
		"ParentPackageID":   "-1",
		"SelfHealingAction": "nothing",
		"HashType":          "SHA_512",
	} {
		var got string
		switch name {
		case "InstallLanguage":
			got = state.InstallLanguage.ValueString()
		case "ParentPackageID":
			got = state.ParentPackageID.ValueString()
		case "SelfHealingAction":
			got = state.SelfHealingAction.ValueString()
		case "HashType":
			got = state.HashType.ValueString()
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}

	if state.Priority.ValueInt64() != 10 {
		t.Errorf("Priority = %d, want 10", state.Priority.ValueInt64())
	}
}

func TestAssignPackageResourceModel_BoolPointerNilCollapsesToNull(t *testing.T) {
	pkgID := "200"
	state := PackageResourceModel{}
	resp := &pro.Package{
		ID:          &pkgID,
		PackageName: "X",
		FileName:    "X.pkg",
		CategoryID:  "-1",
		Priority:    10,
		// FillExistingUsers / Swu / Indexed / SelfHealNotify all nil → null
	}
	if diags := assignPackageResourceModel(&state, resp); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	for name, v := range map[string]types.Bool{
		"FillExistingUsers":         state.FillExistingUsers,
		"AvailableInSoftwareUpdate": state.AvailableInSoftwareUpdate,
		"Indexed":                   state.Indexed,
		"SelfHealNotify":            state.SelfHealNotify,
	} {
		if !v.IsNull() {
			t.Errorf("%s must be null when server returned nil pointer, got %v", name, v.ValueBool())
		}
	}
}

func TestAssignPackageDataSourceModel_NilResponse(t *testing.T) {
	var state PackageDataSourceModel
	diags := assignPackageDataSourceModel(&state, nil)
	if !diags.HasError() {
		t.Fatalf("expected error diag on nil response")
	}
}

func TestAssignPackageDataSourceModel_BasicFields(t *testing.T) {
	pkgID := "300"
	state := PackageDataSourceModel{}
	resp := &pro.Package{
		ID:               &pkgID,
		PackageName:      "DSPkg",
		FileName:         "DSPkg.pkg",
		CategoryID:       "9",
		Priority:         11,
		FillUserTemplate: true,
		RebootRequired:   false,
		Info:             new("info-body"),
		Md5:              new("abc"),
		Swu:              new(true),
	}
	if diags := assignPackageDataSourceModel(&state, resp); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if state.DisplayName.ValueString() != "DSPkg" {
		t.Errorf("DisplayName not set")
	}
	if state.CategoryID.ValueString() != "9" {
		t.Errorf("CategoryID not set: %v", state.CategoryID.ValueString())
	}
	if !state.FillUserTemplate.ValueBool() {
		t.Errorf("FillUserTemplate not set")
	}
	if !state.AvailableInSoftwareUpdate.ValueBool() {
		t.Errorf("AvailableInSoftwareUpdate not set from Swu pointer")
	}
	if state.Md5.ValueString() != "abc" {
		t.Errorf("Md5 not set")
	}
}
