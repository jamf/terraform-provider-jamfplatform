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

// TestAssignSettingsResourceModel_ManagementUsernameEmptyString pins #445:
// Jamf Pro echoes an empty management username as "", so an authored "" must
// survive the read while an unset or unknown value stays null.
func TestAssignSettingsResourceModel_ManagementUsernameEmptyString(t *testing.T) {
	for name, tc := range map[string]struct {
		incoming types.String
		wantNull bool
	}{
		"authored empty": {incoming: types.StringValue(""), wantNull: false},
		"unknown":        {incoming: types.StringUnknown(), wantNull: true},
		"null":           {incoming: types.StringNull(), wantNull: true},
	} {
		t.Run(name, func(t *testing.T) {
			state := &UserInitiatedEnrollmentSettingsResourceModel{ManagementUsername: tc.incoming}
			assignSettingsResourceModel(state, &pro.EnrollmentSettingsV4{ManagementUsername: ""})
			if got := state.ManagementUsername; got.IsNull() != tc.wantNull || got.ValueString() != "" {
				t.Errorf("management_username = %s, want null=%t", got, tc.wantNull)
			}
		})
	}
}

// TestSiteIDValue_CollapsesNoSiteEchoes proves Jamf Pro's "0" for an access
// group with no site reads back as the single user-facing "-1".
func TestSiteIDValue_CollapsesNoSiteEchoes(t *testing.T) {
	cases := []struct {
		name string
		in   *string
		want types.String
	}{
		{"nil stays null", nil, types.StringNull()},
		{"zero collapses", new("0"), types.StringValue("-1")},
		{"minus one stays", new("-1"), types.StringValue("-1")},
		{"real site stays", new("42"), types.StringValue("42")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := siteIDValue(tc.in); !got.Equal(tc.want) {
				t.Fatalf("siteIDValue = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSiteDiffers_TreatsZeroAsNoSite proves a configured "-1" does not plan an
// update against a group Jamf Pro reports with "0".
func TestSiteDiffers_TreatsZeroAsNoSite(t *testing.T) {
	cur := pro.EnrollmentAccessGroupPreview{SiteID: new("0")}
	if siteDiffers(accessGroupModel{SiteID: types.StringValue("-1")}, cur) {
		t.Fatal("-1 against a stored 0 must not differ")
	}
	if !siteDiffers(accessGroupModel{SiteID: types.StringValue("5")}, cur) {
		t.Fatal("a real site against a stored 0 must differ")
	}
}
