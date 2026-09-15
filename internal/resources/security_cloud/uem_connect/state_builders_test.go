// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// jamfProConnector returns a response shaped like the one a live Jamf Pro
// connector returns, with the defaults the server applies on create
// (wire-verified 2026-08-28).
func jamfProConnector() *securitycloud.ConnectorConfig {
	return &securitycloud.ConnectorConfig{
		ID:                       "6a91b958619ef153a5a63d72",
		Vendor:                   "JAMF_PRO",
		URL:                      "https://example.jamfcloud.com",
		Connected:                true,
		Enabled:                  true,
		Scheduled:                true,
		RefreshRateMinutes:       1440,
		DeviceUnmanagedThreshold: 3,
		ConcurrentSyncEnabled:    true,
		DeviceRiskTagging:        false,
		UemVersion:               new("11.31.1"),
		SyncConfig: &securitycloud.SyncConfig{
			AutoDeviceDeletion:     securitycloud.SyncConfigAutoDeviceDeletionDeletedOrRetired,
			DisableSyncOnAuthError: true,
		},
		DeviceFieldMappings: &securitycloud.DeviceFieldMappings{
			DeviceNameMapping:  new(defaultDeviceNameMapping),
			UserNameMapping:    new(defaultUserNameMapping),
			UserIDMapping:      new(defaultUserIDMapping),
			PhoneNumberMapping: new(defaultPhoneNumberMapping),
			UserEmailMapping: &securitycloud.EmailMapping{
				Type:                  defaultUserEmailMappingType,
				UseOnlyIfEmailMissing: new(false),
			},
		},
		GroupSettings: &securitycloud.GroupSettings{
			GroupMappingEnabled: new(true),
			GroupMappings:       &[]securitycloud.GroupMapping{},
		},
	}
}

// TestAssignResourceModel_PlatformTenantForm pins the discriminator the whole
// import path rests on: tenantId present means the integration was set up by
// naming a tenant, so `platform_tenant` is populated and `oauth` is left nil —
// even though the response also carries the clientId Jamf Security Cloud
// provisioned for itself.
func TestAssignResourceModel_PlatformTenantForm(t *testing.T) {
	config := jamfProConnector()
	config.TenantID = new("ff584e5b-d9f8-4c1c-8752-449d8c5e45d5")
	config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: "256c303d-28dc-497a-aa1c-4548282c1666"}

	var state UEMConnectResourceModel
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.PlatformTenant == nil {
		t.Fatal("platform_tenant was not populated")
	}
	if got := state.PlatformTenant.TenantID.ValueString(); got != *config.TenantID {
		t.Errorf("tenant_id = %q, want %q", got, *config.TenantID)
	}
	if state.OAuth != nil {
		t.Errorf("oauth was populated for a platform-tenant integration: %+v", state.OAuth)
	}
}

// TestAssignResourceModel_OAuthForm is the other half: a null tenantId means
// credentials were supplied, so `oauth` carries the client ID and the secret stays
// null because it is never returned.
func TestAssignResourceModel_OAuthForm(t *testing.T) {
	config := jamfProConnector()
	config.TenantID = nil
	config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: "d3bcff8b-670a-48a3-b6a6-17e7694536e0"}

	var state UEMConnectResourceModel
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.PlatformTenant != nil {
		t.Errorf("platform_tenant was populated for an OAuth integration: %+v", state.PlatformTenant)
	}
	if state.OAuth == nil {
		t.Fatal("oauth was not populated")
	}
	if got := state.OAuth.ClientID.ValueString(); got != "d3bcff8b-670a-48a3-b6a6-17e7694536e0" {
		t.Errorf("client_id = %q", got)
	}
	if !state.OAuth.ClientSecret.IsNull() {
		t.Errorf("client_secret must stay null; got %q", state.OAuth.ClientSecret.ValueString())
	}
}

// TestAssignResourceModel_EmptyTenantIDReadsAsOAuth covers the nullable field
// arriving as an empty string rather than as null. Either way the integration has
// no tenant, and treating "" as a tenant would populate platform_tenant with
// nothing in it.
func TestAssignResourceModel_EmptyTenantIDReadsAsOAuth(t *testing.T) {
	config := jamfProConnector()
	config.TenantID = new("")

	var state UEMConnectResourceModel
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.PlatformTenant != nil {
		t.Errorf("platform_tenant was populated from an empty tenant ID: %+v", state.PlatformTenant)
	}
}

// TestAssignResourceModel_EmptyClientIDReadsAsNull covers the response-side client
// ID arriving empty rather than absent. The field used to be a pointer, so absence
// was a nil deviceSyncAuth *or* a nil clientId; it is a plain string now, so the
// only way the response can say "no client ID" is an empty one on a present
// credentials object. Committing "" would present as a configured client ID that is
// the empty string, so it has to read as null.
func TestAssignResourceModel_EmptyClientIDReadsAsNull(t *testing.T) {
	config := jamfProConnector()
	config.TenantID = nil
	config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: ""}

	var state UEMConnectResourceModel
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.OAuth == nil {
		t.Fatal("oauth was not populated")
	}
	if !state.OAuth.ClientID.IsNull() {
		t.Errorf("client_id = %q, want null", state.OAuth.ClientID.ValueString())
	}
}

// TestAssignDataSourceModel_EmptyClientIDReadsAsNull is the data source half of the
// same case: it reads the client ID off the same field, through the same helper.
func TestAssignDataSourceModel_EmptyClientIDReadsAsNull(t *testing.T) {
	config := jamfProConnector()
	config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: ""}

	var state UEMConnectDataSourceModel
	if diags := assignUEMConnectDataSourceModel(context.Background(), &state, config); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.ClientID.IsNull() {
		t.Errorf("client_id = %q, want null", state.ClientID.ValueString())
	}
}

// TestAssignResourceModel_PreservesWriteOnlyRotationCounter pins that a refresh
// does not wipe the user's rotation counter. The server has never seen it, so
// nothing in the response can restore it — only preserving it works.
func TestAssignResourceModel_PreservesWriteOnlyRotationCounter(t *testing.T) {
	config := jamfProConnector()
	config.TenantID = nil
	config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: "client"}

	state := UEMConnectResourceModel{
		OAuth: &OAuthModel{
			ClientID:              types.StringValue("client"),
			ClientSecret:          types.StringValue("should-not-survive"),
			ClientSecretWOVersion: types.Int64Value(3),
		},
	}
	if diags := assignUEMConnectResourceModel(&state, config, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := state.OAuth.ClientSecretWOVersion.ValueInt64(); got != 3 {
		t.Errorf("client_secret_wo_version = %d, want 3", got)
	}
	if !state.OAuth.ClientSecret.IsNull() {
		t.Errorf("client_secret survived the refresh: %q", state.OAuth.ClientSecret.ValueString())
	}
}

// TestAssignResourceModel_TranslatesAutoDeleteBehaviour pins that the value
// reaching state is this resource's vocabulary and not the wire's.
func TestAssignResourceModel_TranslatesAutoDeleteBehaviour(t *testing.T) {
	cases := map[string]string{
		securitycloud.SyncConfigAutoDeviceDeletionDisabled:         "keep_deleted_or_retired",
		securitycloud.SyncConfigAutoDeviceDeletionDeletedOrRetired: "remove_deleted_or_retired",
		securitycloud.SyncConfigAutoDeviceDeletionUnmanaged:        "remove_deleted_or_unmanaged",
	}

	for wire, want := range cases {
		t.Run(wire, func(t *testing.T) {
			config := jamfProConnector()
			config.SyncConfig.AutoDeviceDeletion = wire

			var state UEMConnectResourceModel
			if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got := state.UEMAutoDeleteBehaviour.ValueString(); got != want {
				t.Errorf("uem_auto_delete_behavior = %q, want %q", got, want)
			}
		})
	}
}

// TestAssignResourceModel_UnknownAutoDeleteBehaviourErrors pins that an
// unrecognised value fails loudly. Committing null for it would present as "the
// setting is unset" and a subsequent apply would quietly rewrite the tenant's
// choice.
func TestAssignResourceModel_UnknownAutoDeleteBehaviourErrors(t *testing.T) {
	config := jamfProConnector()
	config.SyncConfig.AutoDeviceDeletion = "SOMETHING_NEW"

	var state UEMConnectResourceModel
	diags := assignUEMConnectResourceModel(&state, config, true)
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic for an unrecognised auto-delete behavior")
	}
}

// TestAssignResourceModel_PreservesGroupMappingOrder pins that group mappings keep
// the order they are stored in. Membership is evaluated top to bottom, so a
// reordering silently changes which group a device lands in.
func TestAssignResourceModel_PreservesGroupMappingOrder(t *testing.T) {
	config := jamfProConnector()
	config.GroupSettings.GroupMappings = &[]securitycloud.GroupMapping{
		{EmmGroupID: "computer_30", WanderaGroupID: "group-a"},
		{EmmGroupID: "computer_10", WanderaGroupID: "group-b"},
		{EmmGroupID: "mobile_20", WanderaGroupID: "group-c"},
	}

	var state UEMConnectResourceModel
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	want := []string{"computer_30", "computer_10", "mobile_20"}
	if len(state.GroupMembershipMapping.Mappings) != len(want) {
		t.Fatalf("got %d mappings, want %d", len(state.GroupMembershipMapping.Mappings), len(want))
	}
	for i, w := range want {
		if got := state.GroupMembershipMapping.Mappings[i].UEMGroupID.ValueString(); got != w {
			t.Errorf("mappings[%d].uem_group_id = %q, want %q", i, got, w)
		}
	}
}

// TestAssignResourceModel_NilNestedResponses covers a response missing the optional
// nested objects entirely, which must not panic.
func TestAssignResourceModel_NilNestedResponses(t *testing.T) {
	config := jamfProConnector()
	config.DeviceFieldMappings = nil
	config.GroupSettings = nil

	var state UEMConnectResourceModel
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.UserDataFieldMapping != nil {
		t.Error("user_data_field_mapping should be nil when the response omits it")
	}
	if state.GroupMembershipMapping != nil {
		t.Error("group_membership_mapping should be nil when the response omits it")
	}
}

// TestAssignResourceModel_NilSyncConfig pins that an absent syncConfig is reported
// rather than absorbed.
//
// Both attributes it carries are Optional+Computed with a schema default, so the
// plan always holds a known value for them. Returning null would trip the
// framework's consistency check with a message naming no cause; the diagnostic names
// one.
func TestAssignResourceModel_NilSyncConfig(t *testing.T) {
	config := jamfProConnector()
	config.SyncConfig = nil

	var state UEMConnectResourceModel
	diags := assignUEMConnectResourceModel(&state, config, true)
	if !diags.HasError() {
		t.Fatal("a response with no syncConfig should be an error, got none")
	}
}

// TestAssignDataSourceModel pins the fields the data source adds over the
// resource — the ones that move on their own and so are read-only by design.
func TestAssignDataSourceModel(t *testing.T) {
	started := time.Date(2026, 8, 28, 16, 32, 7, 0, time.UTC)
	config := jamfProConnector()
	config.TenantID = new("ff584e5b-d9f8-4c1c-8752-449d8c5e45d5")
	config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: "client"}
	config.LatestSync = &securitycloud.LatestSync{
		TransactionID: "71e2e32b-6650-4f2a-b9d8-9a82b3989888",
		Status:        securitycloud.LatestSyncStatusCompleted,
		RefreshType:   new(securitycloud.LatestSyncRefreshTypeManual),
		Started:       &started,
		ErrorDetails:  &securitycloud.SyncErrorDetails{},
	}

	var state UEMConnectDataSourceModel
	if diags := assignUEMConnectDataSourceModel(context.Background(), &state, config); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := state.JamfProVersion.ValueString(); got != "11.31.1" {
		t.Errorf("jamf_pro_version = %q, want 11.31.1", got)
	}
	if !state.Connected.ValueBool() {
		t.Error("connected should be true")
	}
	if got := state.PlatformTenantID.ValueString(); got != *config.TenantID {
		t.Errorf("platform_tenant_id = %q", got)
	}
	if state.LatestSync.IsNull() {
		t.Fatal("latest_sync should be populated")
	}
	attrs := state.LatestSync.Attributes()
	if got := attrs["started"].(types.String).ValueString(); got != "2026-08-28T16:32:07Z" {
		t.Errorf("latest_sync.started = %q", got)
	}
	if !attrs["finished"].(types.String).IsNull() {
		t.Error("latest_sync.finished should be null for a sync with no finish time")
	}
	if !attrs["error_reason"].(types.String).IsNull() {
		t.Error("latest_sync.error_reason should be null when the sync did not fail")
	}
}

// TestAssignDataSourceModel_NoLatestSync covers a connector that has never synced,
// where the whole object is absent rather than empty.
func TestAssignDataSourceModel_NoLatestSync(t *testing.T) {
	config := jamfProConnector()
	config.LatestSync = nil

	var state UEMConnectDataSourceModel
	if diags := assignUEMConnectDataSourceModel(context.Background(), &state, config); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.LatestSync.IsNull() {
		t.Error("latest_sync should be null before the first sync")
	}
}

// TestAssignResourceModel_UnmanagedBlocksStayNull is the regression test for the
// failure this resource shipped with first: Jamf Security Cloud returns both
// optional blocks populated on every read, so populating them unconditionally
// breaks the framework's consistency contract wherever the plan said null.
//
// It failed as "Provider produced inconsistent result after apply: .group_membership_mapping:
// was null, but now cty.ObjectVal(...)", and only under an acceptance apply — unit
// tests, lint and make generate never exercise the plan/apply comparison.
func TestAssignResourceModel_UnmanagedBlocksStayNull(t *testing.T) {
	config := jamfProConnector()
	config.TenantID = new("ff584e5b")

	// A plan that declares neither block. Both must stay nil, whatever the
	// response carries.
	state := UEMConnectResourceModel{}
	if diags := assignUEMConnectResourceModel(&state, config, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.UserDataFieldMapping != nil {
		t.Errorf("user_data_field_mapping was populated for an unmanaged block: %+v", state.UserDataFieldMapping)
	}
	if state.GroupMembershipMapping != nil {
		t.Errorf("group_membership_mapping was populated for an unmanaged block: %+v", state.GroupMembershipMapping)
	}
}

// TestAssignResourceModel_RemovingAManagedBlockLeavesItNull covers the Update case
// the STYLE_GUIDE singles out as easy to miss. The target model is the *new* plan,
// so a block the user has just deleted is nil there even though the prior state and
// the server both still hold it.
func TestAssignResourceModel_RemovingAManagedBlockLeavesItNull(t *testing.T) {
	config := jamfProConnector()
	config.GroupSettings.GroupMappings = &[]securitycloud.GroupMapping{
		{EmmGroupID: "computer_30", WanderaGroupID: "group-a"},
	}

	// The new plan: group_membership_mapping removed, user_data_field_mapping still managed.
	state := UEMConnectResourceModel{
		UserDataFieldMapping: &DataFieldMappingModel{},
	}
	if diags := assignUEMConnectResourceModel(&state, config, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.GroupMembershipMapping != nil {
		t.Errorf("group_membership_mapping was repopulated from the response after being removed: %+v", state.GroupMembershipMapping)
	}
	if state.UserDataFieldMapping == nil {
		t.Fatal("user_data_field_mapping was managed and should have been populated")
	}
	if got := state.UserDataFieldMapping.DeviceName.ValueString(); got != defaultDeviceNameMapping {
		t.Errorf("device_name = %q, want the server value %q", got, defaultDeviceNameMapping)
	}
}

// TestAssignResourceModel_NestedBlockGatesIndependently pins that the gate applies
// one level down too. `email` and `mappings` are themselves Optional-only, so a
// managed parent with an unmanaged child must leave the child null.
func TestAssignResourceModel_NestedBlockGatesIndependently(t *testing.T) {
	config := jamfProConnector()
	config.GroupSettings.GroupMappings = &[]securitycloud.GroupMapping{
		{EmmGroupID: "computer_30", WanderaGroupID: "group-a"},
	}

	state := UEMConnectResourceModel{
		// The parent blocks are declared; the children are not.
		UserDataFieldMapping:   &DataFieldMappingModel{},
		GroupMembershipMapping: &GroupMappingModel{},
	}
	if diags := assignUEMConnectResourceModel(&state, config, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.UserDataFieldMapping.Email != nil {
		t.Errorf("email was populated for an unmanaged nested block: %+v", state.UserDataFieldMapping.Email)
	}
	if state.GroupMembershipMapping.Mappings != nil {
		t.Errorf("mappings was populated for an unmanaged nested collection: %+v", state.GroupMembershipMapping.Mappings)
	}
	// The parent's own scalars are Optional+Computed, so those do come from the
	// server — that is the difference between a scalar and a nested block here.
	if state.GroupMembershipMapping.Enabled.IsNull() {
		t.Error("group_membership_mapping.enabled is Optional+Computed and should carry the server value")
	}
}

// TestAssignResourceModel_ImportPopulatesEverything pins the one case where the gate
// is lifted. An import starts from no state, so nothing but the response can
// populate the blocks, and refusing to would leave an imported resource looking
// unconfigured.
func TestAssignResourceModel_ImportPopulatesEverything(t *testing.T) {
	config := jamfProConnector()
	config.GroupSettings.GroupMappings = &[]securitycloud.GroupMapping{
		{EmmGroupID: "computer_30", WanderaGroupID: "group-a"},
	}

	state := UEMConnectResourceModel{}
	if diags := assignUEMConnectResourceModel(&state, config, true); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.UserDataFieldMapping == nil || state.UserDataFieldMapping.Email == nil {
		t.Error("an import must populate user_data_field_mapping and its email block")
	}
	if state.GroupMembershipMapping == nil || len(state.GroupMembershipMapping.Mappings) != 1 {
		t.Errorf("an import must populate group_membership_mapping and its mappings: %+v", state.GroupMembershipMapping)
	}
}

// TestAssignResourceModel_ServerAddressFollowsTheForm pins the attribute the
// resource refuses to be told and the response reports anyway.
//
// Jamf Security Cloud returns the resolved address for both forms, and on the
// platform_tenant form `uem_server_url` conflicts with `platform_tenant`, so no
// configuration can hold it. Committing the resolved value made
// `terraform plan -generate-config-out` emit a file the resource's own validators
// refuse — the pair reported as "cannot be configured together" and, with the
// address in and no `oauth`, "must be configured together" as well (issue #379).
// Create already drops it; this is the read side agreeing.
//
// Both forms are covered on a refresh and on an import, because the import path
// lifts the block gates and a fix applied to only one of the two would leave an
// imported integration carrying an address it cannot be configured with.
func TestAssignResourceModel_ServerAddressFollowsTheForm(t *testing.T) {
	for name, tc := range map[string]struct {
		tenantID *string
		wantURL  bool
	}{
		"platform tenant": {new("ff584e5b-d9f8-4c1c-8752-449d8c5e45d5"), false},
		"oauth":           {nil, true},
	} {
		for form, isImport := range map[string]bool{"refresh": false, "import": true} {
			t.Run(name+"/"+form, func(t *testing.T) {
				config := jamfProConnector()
				config.TenantID = tc.tenantID
				config.DeviceSyncAuth = &securitycloud.DeviceSyncAuth{ClientID: "256c303d-28dc-497a-aa1c-4548282c1666"}

				var state UEMConnectResourceModel
				if diags := assignUEMConnectResourceModel(&state, config, isImport); diags.HasError() {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}

				if !tc.wantURL {
					if !state.UEMServerURL.IsNull() {
						t.Errorf("uem_server_url = %q, want nothing: it conflicts with platform_tenant, so no configuration can hold it",
							state.UEMServerURL.ValueString())
					}
					if state.PlatformTenant == nil {
						t.Error("platform_tenant must still be populated")
					}
					if state.OAuth != nil {
						t.Errorf("oauth was populated for a platform-tenant integration: %+v", state.OAuth)
					}
					return
				}
				if got := state.UEMServerURL.ValueString(); got != config.URL {
					t.Errorf("uem_server_url = %q, want the configured address %q", got, config.URL)
				}
				if state.OAuth == nil {
					t.Error("oauth must be populated on the form that supplies credentials")
				}
			})
		}
	}
}

// TestAssignDataSourceModel_ReportsTheServerAddressOnEitherForm is the other half
// of the asymmetry. A data source owns no configuration to contradict, and the
// address Jamf Security Cloud resolved from the tenant is the thing worth reading,
// so it is reported whichever form set the integration up.
func TestAssignDataSourceModel_ReportsTheServerAddressOnEitherForm(t *testing.T) {
	for name, tenantID := range map[string]*string{
		"platform tenant": new("ff584e5b-d9f8-4c1c-8752-449d8c5e45d5"),
		"oauth":           nil,
	} {
		t.Run(name, func(t *testing.T) {
			config := jamfProConnector()
			config.TenantID = tenantID

			var state UEMConnectDataSourceModel
			if diags := assignUEMConnectDataSourceModel(context.Background(), &state, config); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if got := state.UEMServerURL.ValueString(); got != config.URL {
				t.Errorf("uem_server_url = %q, want the resolved address %q", got, config.URL)
			}
		})
	}
}
