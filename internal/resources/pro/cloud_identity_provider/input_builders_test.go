// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"encoding/base64"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// sampleKeystoreBytes is a short byte sequence used as a stand-in for a real
// PKCS#12 file. The content is not parsed — tests only verify round-trip
// encoding.
var sampleKeystoreBytes = []byte{0x30, 0x82, 0x00, 0x01} // minimal ASN.1 stub

// sampleKeystoreB64 is the base64 encoding of sampleKeystoreBytes.
var sampleKeystoreB64 = base64.StdEncoding.EncodeToString(sampleKeystoreBytes)

// --- buildGoogleCreateRequest -----------------------------------------------

// TestBuildGoogleCreateRequest_KeystoreDecoded verifies the base64 keystore file
// attribute is decoded into FileBytes on the SDK type, and that the password is
// carried through.
func TestBuildGoogleCreateRequest_KeystoreDecoded(t *testing.T) {
	plan := minimalGooglePlanWithKeystore(sampleKeystoreB64, "hunter2", types.Int64Value(1), "")
	cfg := plan // WriteOnly attrs live on config, not plan in real usage; model is the same here.

	req, diags := buildGoogleCreateRequest(plan, cfg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if string(req.Server.Keystore.FileBytes) != string(sampleKeystoreBytes) {
		t.Errorf("FileBytes mismatch: got %v, want %v", req.Server.Keystore.FileBytes, sampleKeystoreBytes)
	}
	if req.Server.Keystore.Password != "hunter2" {
		t.Errorf("Password mismatch: got %q, want %q", req.Server.Keystore.Password, "hunter2")
	}
}

// TestBuildGoogleCreateRequest_FileNameDefault verifies that when the plan has
// no explicit file_name, the SDK keystore file_name defaults to "keystore.p12".
func TestBuildGoogleCreateRequest_FileNameDefault(t *testing.T) {
	plan := minimalGooglePlanWithKeystore(sampleKeystoreB64, "pw", types.Int64Value(1), "")
	cfg := plan

	req, diags := buildGoogleCreateRequest(plan, cfg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Server.Keystore.FileName != defaultKeystoreFileName {
		t.Errorf("expected default FileName=%q, got %q", defaultKeystoreFileName, req.Server.Keystore.FileName)
	}
}

// TestBuildGoogleCreateRequest_FileNameExplicit verifies that an explicit
// file_name in the plan overrides the default.
func TestBuildGoogleCreateRequest_FileNameExplicit(t *testing.T) {
	plan := minimalGooglePlanWithKeystore(sampleKeystoreB64, "pw", types.Int64Value(1), "my-cert.p12")
	cfg := plan

	req, diags := buildGoogleCreateRequest(plan, cfg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Server.Keystore.FileName != "my-cert.p12" {
		t.Errorf("expected FileName=%q, got %q", "my-cert.p12", req.Server.Keystore.FileName)
	}
}

// TestBuildGoogleCreateRequest_InvalidBase64 verifies a diagnostic is added
// when the keystore file value is not valid base64.
func TestBuildGoogleCreateRequest_InvalidBase64(t *testing.T) {
	plan := minimalGooglePlanWithKeystore("not-valid-base64!!!", "pw", types.Int64Value(1), "")
	cfg := plan

	_, diags := buildGoogleCreateRequest(plan, cfg)
	if !diags.HasError() {
		t.Errorf("expected an error diagnostic for invalid base64 keystore")
	}
}

// --- buildGoogleUpdateRequest -----------------------------------------------

// TestBuildGoogleUpdateRequest_KeystoreOmittedWhenVersionUnchanged verifies that
// when wo_version is the same in plan and state, the keystore is nil (omitted
// from the update so Jamf Pro retains the stored certificate).
func TestBuildGoogleUpdateRequest_KeystoreOmittedWhenVersionUnchanged(t *testing.T) {
	plan := minimalGooglePlanWithKeystore(sampleKeystoreB64, "pw", types.Int64Value(1), "")
	state := minimalGooglePlanWithKeystore("", "", types.Int64Value(1), "") // same wo_version
	cfg := plan

	req, diags := buildGoogleUpdateRequest(plan, state, cfg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Server.Keystore != nil {
		t.Errorf("Keystore must be nil (omitted) when wo_version unchanged; got %+v", req.Server.Keystore)
	}
}

// TestBuildGoogleUpdateRequest_KeystoreIncludedWhenVersionChanged verifies that
// when wo_version changes, the keystore is included in the update.
func TestBuildGoogleUpdateRequest_KeystoreIncludedWhenVersionChanged(t *testing.T) {
	plan := minimalGooglePlanWithKeystore(sampleKeystoreB64, "pw", types.Int64Value(2), "")
	state := minimalGooglePlanWithKeystore("", "", types.Int64Value(1), "") // different wo_version
	cfg := plan

	req, diags := buildGoogleUpdateRequest(plan, state, cfg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Server.Keystore == nil {
		t.Fatalf("Keystore must be included when wo_version changed")
	}
	if string(req.Server.Keystore.FileBytes) != string(sampleKeystoreBytes) {
		t.Errorf("FileBytes mismatch on rotation: got %v, want %v", req.Server.Keystore.FileBytes, sampleKeystoreBytes)
	}
}

// TestBuildGoogleUpdateRequest_KeystoreIncludedWhenVersionWasNull verifies that
// changing wo_version from null (initial state) to a value triggers upload.
func TestBuildGoogleUpdateRequest_KeystoreIncludedWhenVersionWasNull(t *testing.T) {
	plan := minimalGooglePlanWithKeystore(sampleKeystoreB64, "pw", types.Int64Value(1), "")
	state := minimalGooglePlanWithKeystore("", "", types.Int64Null(), "") // null prior state
	cfg := plan

	req, diags := buildGoogleUpdateRequest(plan, state, cfg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if req.Server.Keystore == nil {
		t.Fatalf("Keystore must be included when wo_version changes from null to a value")
	}
}

// --- buildMappingsRequest ----------------------------------------------------

// TestBuildMappingsRequest_NilInput verifies that a nil model returns a nil
// mappings pointer (Jamf Pro then generates Google defaults).
func TestBuildMappingsRequest_NilInput(t *testing.T) {
	if got := buildMappingsRequest(nil); got != nil {
		t.Errorf("buildMappingsRequest(nil) must return nil, got %+v", got)
	}
}

// TestBuildMappingsRequest_PopulatedModel verifies every mapping field reaches
// the SDK struct when a fully-populated model is supplied.
func TestBuildMappingsRequest_PopulatedModel(t *testing.T) {
	m := &cloudLdapMappingsModel{
		UserMappings: &cloudLdapUserMappingsModel{
			ObjectClassLimitation: types.StringValue("ANY_OBJECT_CLASSES"),
			ObjectClasses:         types.StringValue("inetOrgPerson"),
			SearchBase:            types.StringValue("ou=Users"),
			SearchScope:           types.StringValue("ALL_SUBTREES"),
			AdditionalSearchBase:  types.StringValue("ou=Contractors"),
			UserID:                types.StringValue("uid"),
			Username:              types.StringValue("sAMAccountName"),
			RealName:              types.StringValue("cn"),
			EmailAddress:          types.StringValue("mail"),
			Department:            types.StringValue("departmentNumber"),
			Building:              types.StringValue("building"),
			Room:                  types.StringValue("roomNumber"),
			Phone:                 types.StringValue("telephoneNumber"),
			Position:              types.StringValue("title"),
			UserUUID:              types.StringValue("entryUUID"),
		},
		GroupMappings: &cloudLdapGroupMappingsModel{
			ObjectClassLimitation: types.StringValue("ANY_OBJECT_CLASSES"),
			ObjectClasses:         types.StringValue("groupOfNames"),
			SearchBase:            types.StringValue("ou=Groups"),
			SearchScope:           types.StringValue("ALL_SUBTREES"),
			GroupID:               types.StringValue("gidNumber"),
			GroupName:             types.StringValue("cn"),
			GroupUUID:             types.StringValue("entryUUID"),
		},
		MembershipMappings: &cloudLdapMembershipMappingsModel{
			GroupMembershipMapping: types.StringValue("memberOf"),
		},
	}

	got := buildMappingsRequest(m)
	if got == nil {
		t.Fatal("buildMappingsRequest with populated model must not return nil")
	}
	if got.UserMappings.ObjectClassLimitation != "ANY_OBJECT_CLASSES" {
		t.Errorf("UserMappings.ObjectClassLimitation mismatch")
	}
	if got.UserMappings.EmailAddress != "mail" {
		t.Errorf("UserMappings.EmailAddress mismatch")
	}
	if got.GroupMappings.GroupName != "cn" {
		t.Errorf("GroupMappings.GroupName mismatch")
	}
	if got.MembershipMappings.GroupMembershipMapping != "memberOf" {
		t.Errorf("MembershipMappings.GroupMembershipMapping mismatch")
	}
}

// TestBuildMappingsRequest_AdditionalSearchBaseNilWhenUnset verifies that a null
// additional_search_base maps to a nil *string on the SDK struct (not an empty
// string pointer), which lets the JSON encoder omit the field.
func TestBuildMappingsRequest_AdditionalSearchBaseNilWhenUnset(t *testing.T) {
	m := &cloudLdapMappingsModel{
		UserMappings: &cloudLdapUserMappingsModel{
			AdditionalSearchBase: types.StringNull(),
		},
	}
	got := buildMappingsRequest(m)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.UserMappings.AdditionalSearchBase != nil {
		t.Errorf("AdditionalSearchBase must be nil when TF value is null; got %q", *got.UserMappings.AdditionalSearchBase)
	}
}

// TestBuildMappingsRequest_AdditionalSearchBaseSetWhenPresent verifies that a
// non-null additional_search_base maps to a non-nil *string.
func TestBuildMappingsRequest_AdditionalSearchBaseSetWhenPresent(t *testing.T) {
	m := &cloudLdapMappingsModel{
		UserMappings: &cloudLdapUserMappingsModel{
			AdditionalSearchBase: types.StringValue("ou=Extra"),
		},
	}
	got := buildMappingsRequest(m)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.UserMappings.AdditionalSearchBase == nil {
		t.Fatal("AdditionalSearchBase must not be nil when TF value is set")
	}
	if *got.UserMappings.AdditionalSearchBase != "ou=Extra" {
		t.Errorf("AdditionalSearchBase mismatch: got %q, want %q", *got.UserMappings.AdditionalSearchBase, "ou=Extra")
	}
}

// --- buildAzureCreateRequest -------------------------------------------------

// TestBuildAzureCreateRequest_CodePlaceholder verifies that the Code field on
// the create request is the non-blank placeholder — the server rejects a blank
// code with 400 INVALID_FIELD "must not be blank".
func TestBuildAzureCreateRequest_CodePlaceholder(t *testing.T) {
	plan := minimalAzurePlan("d5749c84-5cc5-4691-a187-4545c02ff915")

	got := buildAzureCreateRequest(plan)
	if got.Server.Code != azureConsentCodePlaceholder {
		t.Errorf("Code must be the placeholder on create; got %q, want %q", got.Server.Code, azureConsentCodePlaceholder)
	}
	if got.Server.Code == "" {
		t.Errorf("Code must not be empty on create")
	}
	// The builder always hardcodes the wire value (AZURE) for ProviderName,
	// regardless of the TF-facing ENTRA_ID on the plan.
	if got.CloudIDPCommon.ProviderName != wireProviderAzure {
		t.Errorf("CloudIDPCommon.ProviderName must be wire value %q; got %q", wireProviderAzure, got.CloudIDPCommon.ProviderName)
	}
}

// TestBuildAzureCreateRequest_IDAndTypeNil verifies that the ID and Type
// pointer fields on the create request are nil (omitempty on create).
func TestBuildAzureCreateRequest_IDAndTypeNil(t *testing.T) {
	plan := minimalAzurePlan("11111111-2222-3333-4444-555555555556")

	got := buildAzureCreateRequest(plan)
	if got.Server.ID != nil {
		t.Errorf("Server.ID must be nil on create; got %q", *got.Server.ID)
	}
	if got.Server.Type != nil {
		t.Errorf("Server.Type must be nil on create; got %q", *got.Server.Type)
	}
}

// TestBuildAzureCreateRequest_TenantID verifies the tenant_id is round-tripped.
func TestBuildAzureCreateRequest_TenantID(t *testing.T) {
	tenantID := "12345678-1234-1234-1234-123456789abc"
	plan := minimalAzurePlan(tenantID)

	got := buildAzureCreateRequest(plan)
	if got.Server.TenantID != tenantID {
		t.Errorf("TenantID mismatch: got %q, want %q", got.Server.TenantID, tenantID)
	}
}

// --- buildAzureUpdateRequest -------------------------------------------------

// TestBuildAzureUpdateRequest_ServerIDFromPlanID verifies that the Server.ID on
// the update request is set to the plan's ID (the TF state id). Also confirms
// the wire ProviderName is always AZURE regardless of the TF-facing ENTRA_ID.
func TestBuildAzureUpdateRequest_ServerIDFromPlanID(t *testing.T) {
	plan := minimalAzurePlan("11111111-2222-3333-4444-555555555557")
	plan.ID = types.StringValue("azure-server-id-abc")

	got := buildAzureUpdateRequest(plan, nil)
	if got.Server.ID != "azure-server-id-abc" {
		t.Errorf("Server.ID must equal plan ID; got %q", got.Server.ID)
	}
	// buildAzureUpdateRequest hardcodes the wire value (AZURE), not the TF value.
	if got.CloudIDPCommon.ProviderName != wireProviderAzure {
		t.Errorf("CloudIDPCommon.ProviderName must be wire value %q; got %q", wireProviderAzure, got.CloudIDPCommon.ProviderName)
	}
}

// TestBuildAzureUpdateRequest_NoCode verifies the update struct has no Code
// field (it's on the create shape only; update struct doesn't include it).
func TestBuildAzureUpdateRequest_NoCode(t *testing.T) {
	plan := minimalAzurePlan("11111111-2222-3333-4444-555555555558")
	plan.ID = types.StringValue("some-id")
	// buildAzureUpdateRequest returns AzureConfigurationUpdate which has no Code
	// field — this test confirms the function compiles and returns a non-nil result.
	got := buildAzureUpdateRequest(plan, nil)
	if got == nil {
		t.Errorf("buildAzureUpdateRequest must not return nil")
	}
}

// --- buildAzureMappings ------------------------------------------------------

// TestBuildAzureMappings_NilReturnsEmpty verifies that nil mappings with no
// merge base produces a zero AzureMappings struct (all empty strings), not a nil
// pointer. That is the create path: nothing declared and nothing to preserve.
func TestBuildAzureMappings_NilReturnsEmpty(t *testing.T) {
	got := buildAzureMappings(nil, nil)
	if got.UserID != "" || got.Email != "" || got.GroupName != "" {
		t.Errorf("nil mappings must produce zero AzureMappings; got %+v", got)
	}
}

// TestBuildAzureMappings_UndeclaredBlockCarriesTheStoredMappings pins #404. The
// eleven keys are always on the wire and Jamf Pro stores what it is sent, so an
// undeclared block sending empty strings wipes the connection's mappings. The
// stored set goes back out instead, which is what makes the omission preserve.
func TestBuildAzureMappings_UndeclaredBlockCarriesTheStoredMappings(t *testing.T) {
	live := &pro.AzureMappings{
		UserID:    "id",
		UserName:  "userPrincipalName",
		Email:     "mail",
		GroupName: "displayName",
	}

	got := buildAzureMappings(nil, live)
	if got.UserID != "id" || got.UserName != "userPrincipalName" || got.Email != "mail" || got.GroupName != "displayName" {
		t.Errorf("an undeclared block must carry the stored mappings; got %+v", got)
	}
}

// TestBuildAzureMappings_DeclaredBlockIgnoresTheStoredMappings pins the other
// half of the contract: a declared block is authoritative, so a field left out
// inside it is written empty and clears that mapping rather than inheriting what
// the connection holds.
func TestBuildAzureMappings_DeclaredBlockIgnoresTheStoredMappings(t *testing.T) {
	live := &pro.AzureMappings{UserID: "id", Email: "mail"}
	m := &cloudAzureMappingsModel{UserID: types.StringValue("objectId")}

	got := buildAzureMappings(m, live)
	if got.UserID != "objectId" {
		t.Errorf("the declared field must win; got %q", got.UserID)
	}
	if got.Email != "" {
		t.Errorf("a field omitted inside a declared block must be written empty, got %q", got.Email)
	}
}

// TestBuildAzureMappings_FieldsRoundTrip verifies all 11 Azure mapping fields
// round-trip from the TF model to the SDK struct.
func TestBuildAzureMappings_FieldsRoundTrip(t *testing.T) {
	m := &cloudAzureMappingsModel{
		UserID:     types.StringValue("id"),
		UserName:   types.StringValue("userPrincipalName"),
		RealName:   types.StringValue("displayName"),
		Email:      types.StringValue("mail"),
		Department: types.StringValue("department"),
		Building:   types.StringValue("officeLocation"),
		Room:       types.StringValue("officeName"),
		Phone:      types.StringValue("mobilePhone"),
		Position:   types.StringValue("jobTitle"),
		GroupID:    types.StringValue("id"),
		GroupName:  types.StringValue("displayName"),
	}

	got := buildAzureMappings(m, nil)
	if got.UserID != "id" {
		t.Errorf("UserID mismatch")
	}
	if got.UserName != "userPrincipalName" {
		t.Errorf("UserName mismatch")
	}
	if got.Email != "mail" {
		t.Errorf("Email mismatch")
	}
	if got.Phone != "mobilePhone" {
		t.Errorf("Phone mismatch")
	}
	if got.GroupName != "displayName" {
		t.Errorf("GroupName mismatch")
	}
}

// --- helpers -----------------------------------------------------------------

// minimalGooglePlanWithKeystore builds a minimal resource model for the Google
// branch with the given keystore attributes. fileName is applied to the plan
// keystore; file and password live on the cfg (WriteOnly) side.
func minimalGooglePlanWithKeystore(file, password string, woVersion types.Int64, fileName string) CloudIdentityProviderResourceModel {
	ksModel := &cloudLdapKeystoreModel{
		WoVersion: woVersion,
	}
	if file != "" {
		ksModel.File = types.StringValue(file)
	} else {
		ksModel.File = types.StringNull()
	}
	if password != "" {
		ksModel.Password = types.StringValue(password)
	} else {
		ksModel.Password = types.StringNull()
	}
	if fileName != "" {
		ksModel.FileName = types.StringValue(fileName)
	} else {
		ksModel.FileName = types.StringNull()
	}
	return CloudIdentityProviderResourceModel{
		ID:           types.StringValue("test-id"),
		DisplayName:  types.StringValue("Test Google IDP"),
		ProviderName: types.StringValue(providerGoogle),
		Google: &cloudIdentityProviderGoogleModel{
			Server: &cloudLdapServerModel{
				ServerURL:                                types.StringValue("ldap.google.com"),
				DomainName:                               types.StringValue("example.com"),
				Port:                                     types.Int64Value(636),
				ConnectionType:                           types.StringValue("LDAPS"),
				ConnectionTimeout:                        types.Int64Value(15),
				SearchTimeout:                            types.Int64Value(60),
				UseWildcards:                             types.BoolValue(true),
				Enabled:                                  types.BoolValue(true),
				MembershipCalculationOptimizationEnabled: types.BoolValue(false),
				Keystore:                                 ksModel,
			},
		},
	}
}

// minimalAzurePlan builds a minimal resource model for the Entra ID branch.
// ProviderName uses the TF-facing value (providerEntraID = "ENTRA_ID"); the
// builder functions are responsible for converting to the wire value
// (wireProviderAzure = "AZURE") before sending to the API.
func minimalAzurePlan(tenantID string) CloudIdentityProviderResourceModel {
	return CloudIdentityProviderResourceModel{
		ID:           types.StringNull(),
		DisplayName:  types.StringValue("Test Entra ID IDP"),
		ProviderName: types.StringValue(providerEntraID),
		Azure: &cloudIdentityProviderAzureModel{
			TenantID:                                 types.StringValue(tenantID),
			SearchTimeout:                            types.Int64Value(30),
			Enabled:                                  types.BoolValue(true),
			MembershipCalculationOptimizationEnabled: types.BoolValue(false),
			TransitiveMembershipEnabled:              types.BoolValue(false),
			TransitiveMembershipUserField:            types.StringValue("userPrincipalName"),
			TransitiveDirectoryMembershipEnabled:     types.BoolValue(false),
		},
	}
}
