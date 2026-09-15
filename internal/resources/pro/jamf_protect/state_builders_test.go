// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_protect

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func protectSettingsFixture() *pro.ProtectSettingsResponse {
	return &pro.ProtectSettingsResponse{
		ApiClientID:      "protect-client-id",
		ApiClientName:    "terraform-provider-jamfplatform",
		AutoInstall:      true,
		ID:               "1",
		LastSyncTime:     "2026-06-10T12:00:00Z",
		PlatformPlanSync: false,
		ProtectURL:       "https://example.protect.jamfcloud.com/graphql",
		RegistrationID:   "reg-abc-123",
		SyncStatus:       "COMPLETED",
	}
}

func TestAssignJamfProtectResourceModel_RoundTrip(t *testing.T) {
	var state JamfProtectResourceModel
	assignJamfProtectResourceModel(&state, protectSettingsFixture())

	if got := state.APIURL.ValueString(); got != "https://example.protect.jamfcloud.com/graphql" {
		t.Errorf("APIURL = %q", got)
	}
	// apiClientId echo maps back to the user-authored client_id.
	if got := state.ClientID.ValueString(); got != "protect-client-id" {
		t.Errorf("ClientID = %q, want apiClientId echo", got)
	}
	if !state.AutoInstall.ValueBool() {
		t.Errorf("AutoInstall must be true")
	}
	if got := state.RegistrationID.ValueString(); got != "reg-abc-123" {
		t.Errorf("RegistrationID = %q", got)
	}
	if got := state.APIClientName.ValueString(); got != "terraform-provider-jamfplatform" {
		t.Errorf("APIClientName = %q", got)
	}
	if state.PlatformPlanSync.ValueBool() {
		t.Errorf("PlatformPlanSync must be false")
	}
	if got := state.LastSyncTime.ValueString(); got != "2026-06-10T12:00:00Z" {
		t.Errorf("LastSyncTime = %q", got)
	}
	if got := state.SyncStatus.ValueString(); got != "COMPLETED" {
		t.Errorf("SyncStatus = %q", got)
	}
}

// TestAssignJamfProtectResourceModel_FreshRegisterShape covers the response
// right after a fresh register: syncStatus UNKNOWN, lastSyncTime null
// (decoded to ""), autoInstall false.
func TestAssignJamfProtectResourceModel_FreshRegisterShape(t *testing.T) {
	resp := protectSettingsFixture()
	resp.AutoInstall = false
	resp.LastSyncTime = ""
	resp.SyncStatus = "UNKNOWN"

	var state JamfProtectResourceModel
	assignJamfProtectResourceModel(&state, resp)

	if state.AutoInstall.IsNull() || state.AutoInstall.ValueBool() {
		t.Errorf("AutoInstall = %v, want false", state.AutoInstall)
	}
	if state.LastSyncTime.IsNull() || state.LastSyncTime.ValueString() != "" {
		t.Errorf("LastSyncTime = %v, want empty string value", state.LastSyncTime)
	}
	if got := state.SyncStatus.ValueString(); got != "UNKNOWN" {
		t.Errorf("SyncStatus = %q, want UNKNOWN", got)
	}
}

// TestAssignJamfProtectResourceModel_DoesNotClobber pins that the assigner
// leaves the CRUD-handler-owned and round-trip-from-state fields untouched:
// ID (stamped by the handler), the WriteOnly password (framework-stripped),
// password_wo_version (never echoed by the wire), and timeouts.
func TestAssignJamfProtectResourceModel_DoesNotClobber(t *testing.T) {
	state := JamfProtectResourceModel{
		ID:                types.StringValue("pre-existing"),
		Password:          types.StringValue("plaintext-should-not-move"),
		PasswordWoVersion: types.Int64Value(7),
	}
	assignJamfProtectResourceModel(&state, protectSettingsFixture())

	if got := state.ID.ValueString(); got != "pre-existing" {
		t.Errorf("assigner clobbered ID: %q", got)
	}
	if got := state.Password.ValueString(); got != "plaintext-should-not-move" {
		t.Errorf("assigner touched Password: %q", got)
	}
	if got := state.PasswordWoVersion.ValueInt64(); got != 7 {
		t.Errorf("assigner touched PasswordWoVersion: %d", got)
	}
}

// TestSingletonIDConstant catches accidental drift in the shared constant the
// import path and acceptance tests depend on.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID = %q, want \"singleton\"", helpers.SingletonID)
	}
}

func TestAssignJamfProtectPlanModel(t *testing.T) {
	plan := pro.JamfProtectPlan{
		Description:      "Monitors all the things",
		ID:               "2",
		Name:             "Default Plan",
		ProfileID:        42,
		ProfileName:      "Jamf Protect Configuration - Default Plan",
		ProfileVersion:   3,
		ScopeDescription: "All Computers",
		SiteID:           "-1",
		UUID:             "11111111-2222-3333-4444-555555555555",
	}

	m := assignJamfProtectPlanModel(&plan)

	if m.ID.ValueString() != "2" || m.Name.ValueString() != "Default Plan" {
		t.Errorf("plan row mapped wrong: %+v", m)
	}
	if m.UUID.ValueString() != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("UUID = %q", m.UUID.ValueString())
	}
	if m.ProfileID.ValueInt64() != 42 || m.ProfileVersion.ValueInt64() != 3 {
		t.Errorf("profile ints mapped wrong: id=%d version=%d", m.ProfileID.ValueInt64(), m.ProfileVersion.ValueInt64())
	}
	if m.ScopeDescription.ValueString() != "All Computers" || m.SiteID.ValueString() != "-1" {
		t.Errorf("scope/site mapped wrong: %+v", m)
	}
	if m.Description.ValueString() != "Monitors all the things" {
		t.Errorf("Description = %q", m.Description.ValueString())
	}
	if m.ProfileName.ValueString() != "Jamf Protect Configuration - Default Plan" {
		t.Errorf("ProfileName = %q", m.ProfileName.ValueString())
	}
}

// TestMapJamfProtectPlans_EmptyIsNonNil pins that an empty catalog serialises
// as an empty list (the unregistered-tenant / never-synced case is not an
// error for the data source).
func TestMapJamfProtectPlans_EmptyIsNonNil(t *testing.T) {
	out := mapJamfProtectPlans(nil)
	if out == nil {
		t.Fatalf("mapJamfProtectPlans(nil) must return a non-nil empty slice")
	}
	if len(out) != 0 {
		t.Errorf("expected empty slice, got %d rows", len(out))
	}
}

func TestMapJamfProtectPlans_Multiple(t *testing.T) {
	out := mapJamfProtectPlans([]pro.JamfProtectPlan{
		{ID: "1", Name: "Plan A"},
		{ID: "2", Name: "Plan B"},
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
	if out[0].Name.ValueString() != "Plan A" || out[1].Name.ValueString() != "Plan B" {
		t.Errorf("rows out of order or mismapped: %+v", out)
	}
}
