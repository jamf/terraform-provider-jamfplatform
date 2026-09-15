// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignSettingsResourceModel_MapsFields verifies the GET → state mapping.
func TestAssignSettingsResourceModel_MapsFields(t *testing.T) {
	s := &pro.EnrollmentSettingsV4{
		InstallSingleProfile:             new(true),
		RestrictReenrollment:             new(false),
		MacOsEnterpriseEnrollmentEnabled: new(true),
		ManagementUsername:               "lapsadmin",
		PersonalDeviceEnrollmentType:     new("USERENROLLMENT"),
	}
	state := &UserInitiatedEnrollmentSettingsResourceModel{}
	assignSettingsResourceModel(state, s)

	if !state.SkipCertificateInstallation.ValueBool() {
		t.Error("skip_certificate_installation should be true")
	}
	if state.RestrictReenrollment.ValueBool() {
		t.Error("restrict_reenrollment should be false")
	}
	if state.ManagementUsername.ValueString() != "lapsadmin" {
		t.Error("management_username not mapped")
	}
	if state.PersonalDeviceEnrollmentType.ValueString() != "USERENROLLMENT" {
		t.Error("personal_device_enrollment_type not mapped")
	}
}

// TestApplyCertDetails_PopulatesWhenAuthored proves details fill subject/serial
// only when the user authored the block, and keystore_file_name is preserved.
func TestApplyCertDetails_PopulatesWhenAuthored(t *testing.T) {
	s := &pro.EnrollmentSettingsV4{
		MDMSigningCertificateDetails: &pro.CertificateDetails{
			Subject:      new("CN=mdm"),
			SerialNumber: new("123"),
		},
	}
	state := &UserInitiatedEnrollmentSettingsResourceModel{
		MdmSigningCertificate: &certificateModel{
			KeystoreFileName: types.StringValue("preserved.p12"),
		},
	}
	assignSettingsResourceModel(state, s)

	if state.MdmSigningCertificate.Subject.ValueString() != "CN=mdm" {
		t.Error("subject not populated from details")
	}
	if state.MdmSigningCertificate.SerialNumber.ValueString() != "123" {
		t.Error("serial_number not populated from details")
	}
	if state.MdmSigningCertificate.KeystoreFileName.ValueString() != "preserved.p12" {
		t.Error("keystore_file_name should be preserved from prior state, not echoed")
	}
}

// TestAssignSettingsResourceModel_NoCertInjection proves a tenant cert is never
// injected when the user did not author the block.
func TestAssignSettingsResourceModel_NoCertInjection(t *testing.T) {
	s := &pro.EnrollmentSettingsV4{
		MDMSigningCertificateDetails: &pro.CertificateDetails{Subject: new("CN=mdm")},
	}
	state := &UserInitiatedEnrollmentSettingsResourceModel{}
	assignSettingsResourceModel(state, s)
	if state.MdmSigningCertificate != nil {
		t.Error("must not inject a cert block the user did not author")
	}
}

// TestAssignAccessGroupsState_BuildsSet verifies the access-group set assembly.
func TestAssignAccessGroupsState_BuildsSet(t *testing.T) {
	groups := []pro.EnrollmentAccessGroupPreview{
		{ID: new("1"), GroupID: "-1", LdapServerID: "-1", Name: "All Directory Service Users", SiteID: new("-1"), RequireEula: new(true)},
		{ID: new("13"), GroupID: "31", LdapServerID: "7", Name: "Admins"},
	}
	set, diags := assignAccessGroupsState(context.Background(), groups)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if len(set.Elements()) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(set.Elements()))
	}
}
