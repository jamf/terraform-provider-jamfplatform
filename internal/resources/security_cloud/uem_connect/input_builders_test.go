// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// planWithDefaults returns a resource model as the framework would hand it over
// after applying the schema defaults, which is what Create and Update actually see.
func planWithDefaults() UEMConnectResourceModel {
	return UEMConnectResourceModel{
		UEMVendor:                     types.StringValue(vendorJamfPro),
		Enabled:                       types.BoolValue(true),
		ScheduledSyncEnabled:          types.BoolValue(true),
		SyncRefreshIntervalMinutes:    types.Int64Value(defaultSyncRefreshIntervalMinutes),
		UEMAutoDeleteBehaviour:        types.StringValue(defaultUEMAutoDeleteBehaviour),
		DeviceRiskUEMSignalingEnabled: types.BoolValue(false),
		DisableSyncOnAuthError:        types.BoolValue(true),
		ConcurrentDeviceSyncEnabled:   types.BoolValue(true),
	}
}

// jamfProVariant unwraps the create envelope to the Jamf Pro variant, failing the
// test if the envelope does not select it.
//
// It checks the envelope's own discriminator, not just that the pointer is set:
// the SDK marshals whichever variant `vendor` names, so an envelope whose
// discriminator is wrong emits a bare `{"vendor":...}` and silently discards
// everything the builder assembled. A test reaching straight for body.JAMFPRO
// would pass through that.
func jamfProVariant(t *testing.T, body *securitycloud.ConnectorCreateRequestBody) *securitycloud.JamfProConnectorCreateRequest {
	t.Helper()
	if body == nil {
		t.Fatal("no create body was built")
	}
	if body.Vendor != securitycloud.ConnectorCreateRequestBodyVendorJamfPro {
		t.Fatalf("envelope vendor = %q, want JAMF_PRO; the SDK marshals by this field, so any other value drops the variant", body.Vendor)
	}
	if body.JAMFPRO == nil {
		t.Fatal("the envelope selects JAMF_PRO but carries no Jamf Pro variant")
	}
	return body.JAMFPRO
}

// TestBuildCreateInput_MarshalsTheVariant pins that the assembled request actually
// reaches the wire.
//
// The two tests below assert on the Go struct, which is not the same claim: the
// create body is a discriminated union, and the SDK's MarshalJSON emits the variant
// the envelope's `vendor` names or, failing that, an object carrying nothing but
// that field. A builder that filled the variant and left the envelope's
// discriminator empty would satisfy every field assertion and still send a request
// with no credentials, no strategy and no URL in it. Marshaling is the only place
// that shows up.
//
// The credentials are asserted here by value rather than only by key: a
// JamfProCredentials whose fields are dropped or swapped still marshals to a
// deviceSyncAuth object, so a key-presence check passes on a body carrying the
// wrong client ID.
func TestBuildCreateInput_MarshalsTheVariant(t *testing.T) {
	plan := planWithDefaults()
	plan.UEMServerURL = types.StringValue("https://example.jamfcloud.com")
	plan.OAuth = &OAuthModel{
		ClientID:     types.StringValue("client-id"),
		ClientSecret: types.StringNull(),
	}

	body, diags := buildConnectorCreateInput(plan, "the-secret")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	for _, key := range []string{"vendor", "authStrategy", "url", "deviceSyncAuth"} {
		if _, ok := got[key]; !ok {
			t.Errorf("%q is absent from the marshaled body — the envelope did not select the variant: %s", key, encoded)
		}
	}
	if got["vendor"] != securitycloud.ConnectorCreateRequestBodyVendorJamfPro {
		t.Errorf("vendor = %v", got["vendor"])
	}

	credentials, ok := got["deviceSyncAuth"].(map[string]any)
	if !ok {
		t.Fatalf("deviceSyncAuth did not marshal as an object: %s", encoded)
	}
	if credentials["clientId"] != "client-id" {
		t.Errorf("deviceSyncAuth.clientId = %v, want the plan value", credentials["clientId"])
	}
	if credentials["clientSecret"] != "the-secret" {
		t.Errorf("deviceSyncAuth.clientSecret = %v, want the config value", credentials["clientSecret"])
	}
}

// TestBuildCreateInput_PlatformTenantForm pins the platform-tenant request shape,
// including that no credentials are sent.
func TestBuildCreateInput_PlatformTenantForm(t *testing.T) {
	plan := planWithDefaults()
	plan.PlatformTenant = &PlatformTenantModel{TenantID: types.StringValue("ff584e5b")}

	body, diags := buildConnectorCreateInput(plan, "")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	input := jamfProVariant(t, body)

	if input.AuthStrategy != securitycloud.JamfProConnectorCreateRequestAuthStrategyM2m {
		t.Errorf("authStrategy = %q, want M2M", input.AuthStrategy)
	}
	if input.TenantID == nil || *input.TenantID != "ff584e5b" {
		t.Errorf("tenantId = %v", input.TenantID)
	}
	if input.DeviceSyncAuth != nil {
		t.Errorf("credentials were sent for the platform-tenant form: %+v", input.DeviceSyncAuth)
	}
}

// TestBuildCreateInput_PlatformTenantSendsNoURL pins that the address is left empty
// rather than invented. Jamf Security Cloud ignores whatever is sent here and
// derives the address from the tenant, and it accepts an empty string
// (wire-verified 2026-08-28) — so sending nothing is both honest and accepted.
func TestBuildCreateInput_PlatformTenantSendsNoURL(t *testing.T) {
	plan := planWithDefaults()
	plan.PlatformTenant = &PlatformTenantModel{TenantID: types.StringValue("ff584e5b")}
	plan.UEMServerURL = types.StringUnknown()

	body, diags := buildConnectorCreateInput(plan, "")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if input := jamfProVariant(t, body); input.URL != "" {
		t.Errorf("url = %q, want empty", input.URL)
	}
}

// TestBuildCreateInput_OAuthForm pins the OAuth request shape, and in particular
// that the secret comes from the separately-passed config value rather than from
// the plan, where a write-only attribute is always null.
func TestBuildCreateInput_OAuthForm(t *testing.T) {
	plan := planWithDefaults()
	plan.UEMServerURL = types.StringValue("https://example.jamfcloud.com")
	plan.OAuth = &OAuthModel{
		ClientID:     types.StringValue("client-id"),
		ClientSecret: types.StringNull(),
	}

	body, diags := buildConnectorCreateInput(plan, "the-secret")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	input := jamfProVariant(t, body)

	if input.AuthStrategy != securitycloud.JamfProConnectorCreateRequestAuthStrategyJamfProOauth {
		t.Errorf("authStrategy = %q, want JAMF_PRO_OAUTH", input.AuthStrategy)
	}
	if input.TenantID != nil {
		t.Errorf("tenantId was sent for the OAuth form: %v", *input.TenantID)
	}
	if input.DeviceSyncAuth == nil {
		t.Fatal("no credentials were sent")
	}
	if input.DeviceSyncAuth.ClientSecret == nil {
		t.Error("no clientSecret was sent")
	} else if *input.DeviceSyncAuth.ClientSecret != "the-secret" {
		t.Errorf("clientSecret = %q, want the config value", *input.DeviceSyncAuth.ClientSecret)
	}
	if input.DeviceSyncAuth.ClientID == nil {
		t.Error("no clientId was sent")
	} else if *input.DeviceSyncAuth.ClientID != "client-id" {
		t.Errorf("clientId = %q, want the plan value", *input.DeviceSyncAuth.ClientID)
	}
	if input.URL != "https://example.jamfcloud.com" {
		t.Errorf("url = %q", input.URL)
	}
}

// TestBuildCreateInput_NoAuthBlockErrors covers the case the config validator
// should already have stopped. It errors rather than sending a request the server
// answers with a 500.
func TestBuildCreateInput_NoAuthBlockErrors(t *testing.T) {
	_, diags := buildConnectorCreateInput(planWithDefaults(), "")
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic when neither auth block is set")
	}
}

// TestBuildSyncSettings_SendsEveryField is the load-bearing test for this file. The
// settings write is a full replacement, so any field the builder omits is reset to
// Jamf's default — silently reverting configuration the user did not touch. Every
// scalar must therefore be present on every write, with one deliberate exception:
// deviceUnmanagedThreshold, which Jamf Security Cloud discards for a JAMF_PRO
// connector, so there is no configuration to revert and sending it only invited a
// plan/apply mismatch.
func TestBuildSyncSettings_SendsEveryField(t *testing.T) {
	plan := planWithDefaults()
	plan.SyncRefreshIntervalMinutes = types.Int64Value(720)
	plan.DeviceRiskUEMSignalingEnabled = types.BoolValue(true)
	plan.DisableSyncOnAuthError = types.BoolValue(false)
	plan.ConcurrentDeviceSyncEnabled = types.BoolValue(false)
	plan.ScheduledSyncEnabled = types.BoolValue(false)

	input, diags := buildSyncSettingsInput(plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if input.Vendor != vendorJamfPro {
		t.Errorf("vendor = %q; a mismatched vendor is answered with a 500", input.Vendor)
	}
	if input.Scheduled == nil || *input.Scheduled {
		t.Error("scheduled was omitted or wrong")
	}
	if input.RefreshRateMinutes == nil || *input.RefreshRateMinutes != 720 {
		t.Error("refreshRateMinutes was omitted or wrong")
	}
	if input.DeviceUnmanagedThreshold != nil {
		t.Error("deviceUnmanagedThreshold was sent; it is the one field this builder must omit, " +
			"because Jamf Security Cloud discards it for a JAMF_PRO connector and echoing it back " +
			"as state breaks every apply")
	}
	if input.DeviceRiskTagging == nil || !*input.DeviceRiskTagging {
		t.Error("deviceRiskTagging was omitted or wrong")
	}
	if input.DisableSyncOnAuthError == nil || *input.DisableSyncOnAuthError {
		t.Error("disableSyncOnAuthError was omitted or wrong")
	}
	if input.ConcurrentSyncEnabled == nil || *input.ConcurrentSyncEnabled {
		t.Error("concurrentSyncEnabled was omitted or wrong — this is the field the SDK used to be unable to send")
	}
	if input.GroupSettings == nil {
		t.Error("groupSettings was omitted; it is the one field the server preserves, so relying on that would " +
			"make the write's effect depend on a quirk")
	}
}

// TestBuildSyncSettings_TranslatesAutoDeleteBehaviour pins that the wire value is
// sent, not this resource's vocabulary.
func TestBuildSyncSettings_TranslatesAutoDeleteBehaviour(t *testing.T) {
	plan := planWithDefaults()
	plan.UEMAutoDeleteBehaviour = types.StringValue("keep_deleted_or_retired")

	input, diags := buildSyncSettingsInput(plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if input.AutoDeviceDeletion != securitycloud.SyncSettingsAutoDeviceDeletionDisabled {
		t.Errorf("autoDeviceDeletion = %q, want DISABLED", input.AutoDeviceDeletion)
	}
}

// TestBuildSyncSettings_UnknownAutoDeleteBehaviourErrors covers the value the
// validator should have rejected.
func TestBuildSyncSettings_UnknownAutoDeleteBehaviourErrors(t *testing.T) {
	plan := planWithDefaults()
	plan.UEMAutoDeleteBehaviour = types.StringValue("not-a-behaviour")

	if _, diags := buildSyncSettingsInput(plan); !diags.HasError() {
		t.Fatal("expected an error diagnostic")
	}
}

// TestBuildDeviceFieldMappings_OmittedBlockSendsEmptyObject pins the equivalence
// the schema documents: leaving the block out asks for Jamf's defaults, which is
// what the admin UI's "Use default data field mapping" checkbox does.
func TestBuildDeviceFieldMappings_OmittedBlockSendsEmptyObject(t *testing.T) {
	out := buildDeviceFieldMappings(nil)

	if out.DeviceNameMapping != nil || out.UserNameMapping != nil || out.UserIDMapping != nil ||
		out.PhoneNumberMapping != nil || out.UserEmailMapping != nil {
		t.Errorf("expected every member unset, got %+v", out)
	}
}

// TestBuildDeviceFieldMappings_PartialBlock pins that an unset field within a
// configured block is omitted rather than sent as an empty string, which the server
// would refuse as an invalid enum value.
func TestBuildDeviceFieldMappings_PartialBlock(t *testing.T) {
	out := buildDeviceFieldMappings(&DataFieldMappingModel{
		DeviceName: types.StringValue("SERIAL_NUMBER"),
	})

	if out.DeviceNameMapping == nil || *out.DeviceNameMapping != "SERIAL_NUMBER" {
		t.Errorf("deviceNameMapping = %v", out.DeviceNameMapping)
	}
	if out.UserNameMapping != nil {
		t.Errorf("userNameMapping was sent as %q despite being unset", *out.UserNameMapping)
	}
}

// TestBuildDeviceFieldMappings_EmailDefaultsToEmailAddress pins that an email block
// with no source still sends a valid type — the field is required on the wire, so
// omitting it is not an option.
func TestBuildDeviceFieldMappings_EmailDefaultsToEmailAddress(t *testing.T) {
	out := buildDeviceFieldMappings(&DataFieldMappingModel{
		Email: &EmailMappingModel{Suffix: types.StringValue("example.test")},
	})

	if out.UserEmailMapping == nil {
		t.Fatal("userEmailMapping was omitted")
	}
	if out.UserEmailMapping.Type != defaultUserEmailMappingType {
		t.Errorf("type = %q, want %q", out.UserEmailMapping.Type, defaultUserEmailMappingType)
	}
	if out.UserEmailMapping.FieldSuffix == nil || *out.UserEmailMapping.FieldSuffix != "example.test" {
		t.Errorf("fieldSuffix = %v", out.UserEmailMapping.FieldSuffix)
	}
}

// TestBuildGroupSettings_EmptyListClearsMappings pins the difference between an
// absent list and an empty one: absent leaves the mappings out of the request,
// empty sends an empty array, which is how they are cleared.
func TestBuildGroupSettings_EmptyListClearsMappings(t *testing.T) {
	absent := buildGroupSettings(&GroupMappingModel{Enabled: types.BoolValue(true)})
	if absent.GroupMappings != nil {
		t.Errorf("an absent list should be omitted, got %+v", *absent.GroupMappings)
	}

	empty := buildGroupSettings(&GroupMappingModel{
		Enabled:  types.BoolValue(true),
		Mappings: []GroupMappingEntryModel{},
	})
	if empty.GroupMappings == nil {
		t.Fatal("an empty list should be sent, so the mappings are cleared")
	}
	if len(*empty.GroupMappings) != 0 {
		t.Errorf("expected an empty array, got %+v", *empty.GroupMappings)
	}
}

// TestBuildGroupSettings_PreservesOrder pins that mappings reach the server in the
// order the user wrote them, since membership is evaluated top to bottom.
func TestBuildGroupSettings_PreservesOrder(t *testing.T) {
	out := buildGroupSettings(&GroupMappingModel{
		Mappings: []GroupMappingEntryModel{
			{UEMGroupID: types.StringValue("computer_30"), SecurityCloudGroupID: types.StringValue("a")},
			{UEMGroupID: types.StringValue("computer_10"), SecurityCloudGroupID: types.StringValue("b")},
		},
	})

	if out.GroupMappings == nil {
		t.Fatal("mappings were omitted")
	}
	got := *out.GroupMappings
	if len(got) != 2 || got[0].EmmGroupID != "computer_30" || got[1].EmmGroupID != "computer_10" {
		t.Errorf("order was not preserved: %+v", got)
	}
}

// TestBuildGroupSettings_OmittedBlockSendsEmptyObject pins the reading of an absent
// Terraform block: unmanaged means default, so the group configuration is reset
// rather than left as whatever the tenant happens to hold.
func TestBuildGroupSettings_OmittedBlockSendsEmptyObject(t *testing.T) {
	out := buildGroupSettings(nil)
	if out == nil {
		t.Fatal("groupSettings must still be sent, so the server replaces rather than preserves")
	}
	if out.GroupMappingEnabled != nil || out.DefaultGroupID != nil || out.GroupMappings != nil {
		t.Errorf("expected every member unset, got %+v", out)
	}
}
