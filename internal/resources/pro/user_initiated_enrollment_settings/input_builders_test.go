// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestMerge_PreservesReenrollmentFields proves the /v4 read-merge-write
// round-trips the six Re-enrollment fields this resource does not own.
func TestMerge_PreservesReenrollmentFields(t *testing.T) {
	got := pro.EnrollmentSettingsV4{
		FlushPolicyHistory:              new(true),
		FlushLocationInformation:        new(true),
		FlushLocationHistoryInformation: new(false),
		FlushExtensionAttributes:        new(true),
		FlushSoftwareUpdatePlans:        new(false),
		FlushMDMCommandsOnReenroll:      new("DELETE_EVERYTHING"),
		ManagementUsername:              "lapsadmin",
		LaunchSelfService:               new(true),
	}
	// Plan only sets restrict_reenrollment; everything else unset (unknown).
	plan := UserInitiatedEnrollmentSettingsResourceModel{
		RestrictReenrollment: types.BoolValue(true),
	}

	body := mergeEnrollmentSettingsInput(got, plan)

	if body.FlushPolicyHistory == nil || !*body.FlushPolicyHistory {
		t.Error("flushPolicyHistory not preserved")
	}
	if body.FlushMDMCommandsOnReenroll == nil || *body.FlushMDMCommandsOnReenroll != "DELETE_EVERYTHING" {
		t.Error("flushMdmCommandsOnReenroll not preserved")
	}
	if body.FlushSoftwareUpdatePlans == nil || *body.FlushSoftwareUpdatePlans {
		t.Error("flushSoftwareUpdatePlans not preserved")
	}
}

// TestMerge_OwnedFieldsOverwrittenWhenKnown proves a known plan value overrides
// the GET value, while an unset (unknown) plan value falls back to GET.
func TestMerge_OwnedFieldsOverwrittenWhenKnown(t *testing.T) {
	got := pro.EnrollmentSettingsV4{
		LaunchSelfService:    new(true), // server default true
		RestrictReenrollment: new(false),
		ManagementUsername:   "lapsadmin",
	}
	plan := UserInitiatedEnrollmentSettingsResourceModel{
		RestrictReenrollment: types.BoolValue(true), // overwrite
		// LaunchSelfService unset → unknown → keep GET (true)
		// ManagementUsername unset → unknown → keep GET (lapsadmin)
	}

	body := mergeEnrollmentSettingsInput(got, plan)

	if body.RestrictReenrollment == nil || !*body.RestrictReenrollment {
		t.Error("restrict_reenrollment should be overwritten to true")
	}
	if body.LaunchSelfService == nil || !*body.LaunchSelfService {
		t.Error("launch_self_service should fall back to GET value true")
	}
	if body.ManagementUsername != "lapsadmin" {
		t.Errorf("management_username should fall back to GET, got %q", body.ManagementUsername)
	}
}

// TestMerge_ManagementUsernameOverwrite proves a known plan username overrides.
func TestMerge_ManagementUsernameOverwrite(t *testing.T) {
	got := pro.EnrollmentSettingsV4{ManagementUsername: "lapsadmin"}
	plan := UserInitiatedEnrollmentSettingsResourceModel{
		ManagementUsername: types.StringValue("newadmin"),
	}
	body := mergeEnrollmentSettingsInput(got, plan)
	if body.ManagementUsername != "newadmin" {
		t.Errorf("expected newadmin, got %q", body.ManagementUsername)
	}
}

// TestMerge_NeverResendsGetCerts proves the GET's cert objects and detail
// echoes are nilled so they are omitted from the PUT (preserve semantics).
func TestMerge_NeverResendsGetCerts(t *testing.T) {
	got := pro.EnrollmentSettingsV4{
		MDMSigningCertificate:               &pro.CertificateIdentityV2{Filename: new("x.p12")},
		MDMSigningCertificateDetails:        &pro.CertificateDetails{Subject: new("CN=x")},
		DeveloperCertificateIdentity:        &pro.CertificateIdentityV2{Filename: new("d.p12")},
		DeveloperCertificateIdentityDetails: &pro.CertificateDetails{Subject: new("CN=d")},
		PersonalDeviceEnrollmentType:        new("USERENROLLMENT"),
	}
	body := mergeEnrollmentSettingsInput(got, UserInitiatedEnrollmentSettingsResourceModel{})

	if body.MDMSigningCertificate != nil || body.MDMSigningCertificateDetails != nil {
		t.Error("MDM cert object/details must be nil in the PUT body")
	}
	if body.DeveloperCertificateIdentity != nil || body.DeveloperCertificateIdentityDetails != nil {
		t.Error("developer cert object/details must be nil in the PUT body")
	}
	if body.PersonalDeviceEnrollmentType != nil {
		t.Error("deprecated personalDeviceEnrollmentType must be omitted from the PUT")
	}
}

// TestBuildCertificateIdentity decodes base64 keystore bytes + password.
func TestBuildCertificateIdentity(t *testing.T) {
	cfg := &certificateModel{
		KeystoreFile:     types.StringValue("aGVsbG8="), // "hello"
		KeystorePassword: types.StringValue("pw"),
		KeystoreFileName: types.StringValue("k.p12"),
	}
	id, err := buildCertificateIdentity(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.IdentityKeystore == nil || string(*id.IdentityKeystore) != "hello" {
		t.Error("keystore bytes not decoded")
	}
	if id.KeystorePassword == nil || *id.KeystorePassword != "pw" {
		t.Error("password not set")
	}
	if id.Filename == nil || *id.Filename != "k.p12" {
		t.Error("filename not set")
	}
}

// TestBuildCertificateIdentity_DefaultsFilename verifies a filename is always
// sent even when keystore_file_name is unset — Jamf Pro requires it (dev-cert
// PUT 500s without it; MDM cert is not ingested without it).
func TestBuildCertificateIdentity_DefaultsFilename(t *testing.T) {
	cfg := &certificateModel{
		KeystoreFile:     types.StringValue("aGVsbG8="),
		KeystorePassword: types.StringValue("pw"),
		// KeystoreFileName intentionally unset (null).
	}
	id, err := buildCertificateIdentity(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Filename == nil || *id.Filename != defaultKeystoreFilename {
		t.Errorf("Filename = %v, want default %q", id.Filename, defaultKeystoreFilename)
	}
}

// TestBuildAccessGroupInput maps a planned group; the directory group id is the
// caller-resolved value (not the Computed model field); create omits the server
// id, update carries it.
func TestBuildAccessGroupInput(t *testing.T) {
	m := accessGroupModel{
		LdapServerID:                types.StringValue("7"),
		Name:                        types.StringValue("Admins"),
		SiteID:                      types.StringValue("-1"),
		EnterpriseEnrollmentEnabled: types.BoolValue(true),
		RequireEula:                 types.BoolValue(false),
	}
	out := buildAccessGroupInput(m, "37158")
	if out.ID != nil {
		t.Error("create payload should omit id")
	}
	if out.GroupID != "37158" {
		t.Errorf("GroupID = %q, want resolved 37158", out.GroupID)
	}
	if out.LdapServerID != "7" || out.Name != "Admins" {
		t.Error("name/server fields not mapped")
	}
	if out.EnterpriseEnrollmentEnabled == nil || !*out.EnterpriseEnrollmentEnabled {
		t.Error("enterprise toggle not mapped")
	}

	m.ID = types.StringValue("13")
	if got := buildAccessGroupInput(m, "37158"); got.ID == nil || *got.ID != "13" {
		t.Error("update payload should carry id")
	}
}
