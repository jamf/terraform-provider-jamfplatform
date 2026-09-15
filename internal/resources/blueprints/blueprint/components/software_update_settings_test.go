// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSoftwareUpdateSettings_GetIdentifier(t *testing.T) {
	c := &SoftwareUpdateSettingsComponent{}
	if c.GetIdentifier() != "com.jamf.ddm.software-update-settings" {
		t.Errorf("expected 'com.jamf.ddm.software-update-settings', got %q", c.GetIdentifier())
	}
}

func TestSoftwareUpdateSettings_ToRawConfiguration_AllFields(t *testing.T) {
	c := &SoftwareUpdateSettingsComponent{
		AllowStandardUserOSUpdates:     types.BoolValue(true),
		AutomaticDownload:              types.StringValue("AlwaysOn"),
		AutomaticInstallOSUpdates:      types.StringValue("AlwaysOff"),
		AutomaticInstallSecurityUpdate: types.StringValue("Allowed"),
		BetaProgramEnrollment:          types.StringValue("AlwaysOn"),
		BetaRequireProgramToken:        types.StringValue("beta-token"),
		BetaRequireProgramDescription:  types.StringValue("Beta Description"),
		BetaOfferPrograms: []BetaProgramModel{
			{
				Token:       types.StringValue("offer-token-1"),
				Description: types.StringValue("Offer 1"),
			},
		},
		DeferralCombinedPeriod:               types.Int64Value(30),
		DeferralMajorPeriod:                  types.Int64Value(60),
		DeferralMinorPeriod:                  types.Int64Value(14),
		DeferralSystemPeriod:                 types.Int64Value(7),
		NotificationsEnabled:                 types.BoolValue(true),
		RapidSecurityResponseEnabled:         types.BoolValue(true),
		RapidSecurityResponseRollbackEnabled: types.BoolValue(false),
		RecommendedCadence:                   types.StringValue("Newest"),
	}

	rawCfg, err := c.ToRawConfiguration()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(rawCfg, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	allowUpdates, ok := config["AllowStandardUserOSUpdates"].(map[string]any)
	if !ok {
		t.Fatal("expected AllowStandardUserOSUpdates to be a map")
	}
	if allowUpdates["Enabled"] != true {
		t.Errorf("expected AllowStandardUserOSUpdates Enabled true, got %v", allowUpdates["Enabled"])
	}
	if allowUpdates["Included"] != true {
		t.Errorf("expected AllowStandardUserOSUpdates Included true, got %v", allowUpdates["Included"])
	}

	automaticActions, ok := config["AutomaticActions"].(map[string]any)
	if !ok {
		t.Fatal("expected AutomaticActions to be a map")
	}
	download := automaticActions["Download"].(map[string]any)
	if download["Value"] != "AlwaysOn" {
		t.Errorf("expected Download Value 'AlwaysOn', got %v", download["Value"])
	}
	if download["Included"] != true {
		t.Errorf("expected Download Included true, got %v", download["Included"])
	}

	beta, ok := config["Beta"].(map[string]any)
	if !ok {
		t.Fatal("expected Beta to be a map")
	}
	if beta["Included"] != true {
		t.Errorf("expected Beta Included true, got %v", beta["Included"])
	}
	betaValue := beta["Value"].(map[string]any)
	if betaValue["ProgramEnrollment"] != "AlwaysOn" {
		t.Errorf("expected ProgramEnrollment 'AlwaysOn', got %v", betaValue["ProgramEnrollment"])
	}

	offerPrograms := betaValue["OfferPrograms"].([]any)
	if len(offerPrograms) != 1 {
		t.Fatalf("expected 1 offer program, got %d", len(offerPrograms))
	}
	offerProg := offerPrograms[0].(map[string]any)
	if offerProg["Token"] != "offer-token-1" {
		t.Errorf("expected Token 'offer-token-1', got %v", offerProg["Token"])
	}

	requireProgram := betaValue["RequireProgram"].(map[string]any)
	if requireProgram["Token"] != "beta-token" {
		t.Errorf("expected RequireProgram Token 'beta-token', got %v", requireProgram["Token"])
	}
	if requireProgram["Description"] != "Beta Description" {
		t.Errorf("expected RequireProgram Description 'Beta Description', got %v", requireProgram["Description"])
	}

	deferrals, ok := config["Deferrals"].(map[string]any)
	if !ok {
		t.Fatal("expected Deferrals to be a map")
	}
	combined := deferrals["CombinedPeriodInDays"].(map[string]any)
	if combined["Value"] != float64(30) {
		t.Errorf("expected CombinedPeriodInDays Value 30, got %v", combined["Value"])
	}

	notifications := config["Notifications"].(map[string]any)
	if notifications["Enabled"] != true {
		t.Errorf("expected Notifications Enabled true, got %v", notifications["Enabled"])
	}

	rsr := config["RapidSecurityResponse"].(map[string]any)
	enable := rsr["Enable"].(map[string]any)
	if enable["Enabled"] != true {
		t.Errorf("expected RapidSecurityResponse Enable Enabled true, got %v", enable["Enabled"])
	}
	rollback := rsr["EnableRollback"].(map[string]any)
	if rollback["Enabled"] != false {
		t.Errorf("expected RapidSecurityResponse EnableRollback Enabled false, got %v", rollback["Enabled"])
	}

	cadence := config["RecommendedCadence"].(map[string]any)
	if cadence["Value"] != "Newest" {
		t.Errorf("expected RecommendedCadence Value 'Newest', got %v", cadence["Value"])
	}
}

func TestSoftwareUpdateSettings_ToRawConfiguration_NullDefaults(t *testing.T) {
	c := &SoftwareUpdateSettingsComponent{}

	rawCfg, err := c.ToRawConfiguration()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(rawCfg, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	allowUpdates := config["AllowStandardUserOSUpdates"].(map[string]any)
	if allowUpdates["Included"] != false {
		t.Errorf("expected AllowStandardUserOSUpdates Included false for null, got %v", allowUpdates["Included"])
	}

	automaticActions := config["AutomaticActions"].(map[string]any)
	download := automaticActions["Download"].(map[string]any)
	if download["Value"] != "Allowed" {
		t.Errorf("expected default Download Value 'Allowed', got %v", download["Value"])
	}
	if download["Included"] != false {
		t.Errorf("expected Download Included false for null, got %v", download["Included"])
	}

	if _, exists := config["Beta"]; exists {
		t.Error("expected no Beta key for null beta settings")
	}

	cadence := config["RecommendedCadence"].(map[string]any)
	if cadence["Value"] != "All" {
		t.Errorf("expected default RecommendedCadence Value 'All', got %v", cadence["Value"])
	}
}

func TestSoftwareUpdateSettings_FromRawConfiguration(t *testing.T) {
	rawMap := map[string]any{
		"AllowStandardUserOSUpdates": map[string]any{
			"Enabled":  true,
			"Included": true,
		},
		"AutomaticActions": map[string]any{
			"Download": map[string]any{
				"Value":    "AlwaysOn",
				"Included": true,
			},
			"InstallOSUpdates": map[string]any{
				"Value":    "AlwaysOff",
				"Included": true,
			},
			"InstallSecurityUpdate": map[string]any{
				"Value":    "Allowed",
				"Included": true,
			},
		},
		"Beta": map[string]any{
			"Included": true,
			"Value": map[string]any{
				"ProgramEnrollment": "AlwaysOn",
				"OfferPrograms": []any{
					map[string]any{
						"Token":       "token-1",
						"Description": "Program 1",
					},
				},
				"RequireProgram": map[string]any{
					"Token":       "req-token",
					"Description": "Req Desc",
				},
			},
		},
		"Deferrals": map[string]any{
			"CombinedPeriodInDays": map[string]any{
				"Value":    float64(30),
				"Included": true,
			},
			"MajorPeriodInDays": map[string]any{
				"Value":    float64(60),
				"Included": true,
			},
			"MinorPeriodInDays": map[string]any{
				"Value":    float64(14),
				"Included": true,
			},
			"SystemPeriodInDays": map[string]any{
				"Value":    float64(7),
				"Included": true,
			},
		},
		"Notifications": map[string]any{
			"Enabled":  true,
			"Included": true,
		},
		"RapidSecurityResponse": map[string]any{
			"Enable": map[string]any{
				"Enabled":  true,
				"Included": true,
			},
			"EnableRollback": map[string]any{
				"Enabled":  false,
				"Included": true,
			},
		},
		"RecommendedCadence": map[string]any{
			"Value":    "Newest",
			"Included": true,
		},
	}
	raw, _ := json.Marshal(rawMap)

	c := &SoftwareUpdateSettingsComponent{}
	if err := c.FromRawConfiguration(raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.AllowStandardUserOSUpdates.ValueBool() != true {
		t.Errorf("expected AllowStandardUserOSUpdates true, got %v", c.AllowStandardUserOSUpdates.ValueBool())
	}
	if c.AutomaticDownload.ValueString() != "AlwaysOn" {
		t.Errorf("expected AutomaticDownload 'AlwaysOn', got %q", c.AutomaticDownload.ValueString())
	}
	if c.AutomaticInstallOSUpdates.ValueString() != "AlwaysOff" {
		t.Errorf("expected AutomaticInstallOSUpdates 'AlwaysOff', got %q", c.AutomaticInstallOSUpdates.ValueString())
	}
	if c.AutomaticInstallSecurityUpdate.ValueString() != "Allowed" {
		t.Errorf("expected AutomaticInstallSecurityUpdate 'Allowed', got %q", c.AutomaticInstallSecurityUpdate.ValueString())
	}
	if c.BetaProgramEnrollment.ValueString() != "AlwaysOn" {
		t.Errorf("expected BetaProgramEnrollment 'AlwaysOn', got %q", c.BetaProgramEnrollment.ValueString())
	}
	if len(c.BetaOfferPrograms) != 1 {
		t.Fatalf("expected 1 beta offer program, got %d", len(c.BetaOfferPrograms))
	}
	if c.BetaOfferPrograms[0].Token.ValueString() != "token-1" {
		t.Errorf("expected offer Token 'token-1', got %q", c.BetaOfferPrograms[0].Token.ValueString())
	}
	if c.BetaOfferPrograms[0].Description.ValueString() != "Program 1" {
		t.Errorf("expected offer Description 'Program 1', got %q", c.BetaOfferPrograms[0].Description.ValueString())
	}
	if c.BetaRequireProgramToken.ValueString() != "req-token" {
		t.Errorf("expected BetaRequireProgramToken 'req-token', got %q", c.BetaRequireProgramToken.ValueString())
	}
	if c.BetaRequireProgramDescription.ValueString() != "Req Desc" {
		t.Errorf("expected BetaRequireProgramDescription 'Req Desc', got %q", c.BetaRequireProgramDescription.ValueString())
	}
	if c.DeferralCombinedPeriod.ValueInt64() != 30 {
		t.Errorf("expected DeferralCombinedPeriod 30, got %d", c.DeferralCombinedPeriod.ValueInt64())
	}
	if c.DeferralMajorPeriod.ValueInt64() != 60 {
		t.Errorf("expected DeferralMajorPeriod 60, got %d", c.DeferralMajorPeriod.ValueInt64())
	}
	if c.DeferralMinorPeriod.ValueInt64() != 14 {
		t.Errorf("expected DeferralMinorPeriod 14, got %d", c.DeferralMinorPeriod.ValueInt64())
	}
	if c.DeferralSystemPeriod.ValueInt64() != 7 {
		t.Errorf("expected DeferralSystemPeriod 7, got %d", c.DeferralSystemPeriod.ValueInt64())
	}
	if c.NotificationsEnabled.ValueBool() != true {
		t.Errorf("expected NotificationsEnabled true, got %v", c.NotificationsEnabled.ValueBool())
	}
	if c.RapidSecurityResponseEnabled.ValueBool() != true {
		t.Errorf("expected RapidSecurityResponseEnabled true, got %v", c.RapidSecurityResponseEnabled.ValueBool())
	}
	if c.RapidSecurityResponseRollbackEnabled.ValueBool() != false {
		t.Errorf("expected RapidSecurityResponseRollbackEnabled false, got %v", c.RapidSecurityResponseRollbackEnabled.ValueBool())
	}
	if c.RecommendedCadence.ValueString() != "Newest" {
		t.Errorf("expected RecommendedCadence 'Newest', got %q", c.RecommendedCadence.ValueString())
	}
}

func TestSoftwareUpdateSettings_FromRawConfiguration_NotIncluded(t *testing.T) {
	rawMap := map[string]any{
		"AllowStandardUserOSUpdates": map[string]any{
			"Enabled":  false,
			"Included": false,
		},
		"AutomaticActions": map[string]any{
			"Download": map[string]any{
				"Value":    "Allowed",
				"Included": false,
			},
			"InstallOSUpdates": map[string]any{
				"Value":    "Allowed",
				"Included": false,
			},
			"InstallSecurityUpdate": map[string]any{
				"Value":    "Allowed",
				"Included": false,
			},
		},
		"Deferrals": map[string]any{
			"CombinedPeriodInDays": map[string]any{
				"Value":    float64(0),
				"Included": false,
			},
			"MajorPeriodInDays": map[string]any{
				"Value":    float64(0),
				"Included": false,
			},
			"MinorPeriodInDays": map[string]any{
				"Value":    float64(0),
				"Included": false,
			},
			"SystemPeriodInDays": map[string]any{
				"Value":    float64(0),
				"Included": false,
			},
		},
		"Notifications": map[string]any{
			"Enabled":  false,
			"Included": false,
		},
		"RapidSecurityResponse": map[string]any{
			"Enable": map[string]any{
				"Enabled":  false,
				"Included": false,
			},
			"EnableRollback": map[string]any{
				"Enabled":  false,
				"Included": false,
			},
		},
		"RecommendedCadence": map[string]any{
			"Value":    "All",
			"Included": false,
		},
	}
	raw, _ := json.Marshal(rawMap)

	c := &SoftwareUpdateSettingsComponent{}
	if err := c.FromRawConfiguration(raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !c.AllowStandardUserOSUpdates.IsNull() {
		t.Error("expected null AllowStandardUserOSUpdates when not included")
	}
	if !c.AutomaticDownload.IsNull() {
		t.Error("expected null AutomaticDownload when not included")
	}
	if !c.DeferralCombinedPeriod.IsNull() {
		t.Error("expected null DeferralCombinedPeriod when not included")
	}
	if !c.DeferralMajorPeriod.IsNull() {
		t.Error("expected null DeferralMajorPeriod when not included")
	}
	if !c.DeferralMinorPeriod.IsNull() {
		t.Error("expected null DeferralMinorPeriod when not included")
	}
	if !c.DeferralSystemPeriod.IsNull() {
		t.Error("expected null DeferralSystemPeriod when not included")
	}
	if !c.NotificationsEnabled.IsNull() {
		t.Error("expected null NotificationsEnabled when not included")
	}
	if !c.RecommendedCadence.IsNull() {
		t.Error("expected null RecommendedCadence when not included")
	}
}

func TestSoftwareUpdateSettings_Roundtrip(t *testing.T) {
	original := &SoftwareUpdateSettingsComponent{
		AllowStandardUserOSUpdates:           types.BoolValue(true),
		AutomaticDownload:                    types.StringValue("AlwaysOn"),
		AutomaticInstallOSUpdates:            types.StringValue("AlwaysOff"),
		AutomaticInstallSecurityUpdate:       types.StringValue("Allowed"),
		DeferralCombinedPeriod:               types.Int64Value(15),
		DeferralMajorPeriod:                  types.Int64Value(30),
		DeferralMinorPeriod:                  types.Int64Value(7),
		DeferralSystemPeriod:                 types.Int64Value(14),
		NotificationsEnabled:                 types.BoolValue(true),
		RapidSecurityResponseEnabled:         types.BoolValue(true),
		RapidSecurityResponseRollbackEnabled: types.BoolValue(false),
		RecommendedCadence:                   types.StringValue("Oldest"),
	}

	rawCfg, err := original.ToRawConfiguration()
	if err != nil {
		t.Fatalf("ToRawConfiguration error: %v", err)
	}

	restored := &SoftwareUpdateSettingsComponent{}
	if err := restored.FromRawConfiguration(rawCfg); err != nil {
		t.Fatalf("FromRawConfiguration error: %v", err)
	}

	if restored.AllowStandardUserOSUpdates.ValueBool() != true {
		t.Errorf("roundtrip: expected AllowStandardUserOSUpdates true, got %v", restored.AllowStandardUserOSUpdates.ValueBool())
	}
	if restored.AutomaticDownload.ValueString() != "AlwaysOn" {
		t.Errorf("roundtrip: expected AutomaticDownload 'AlwaysOn', got %q", restored.AutomaticDownload.ValueString())
	}
	if restored.AutomaticInstallOSUpdates.ValueString() != "AlwaysOff" {
		t.Errorf("roundtrip: expected AutomaticInstallOSUpdates 'AlwaysOff', got %q", restored.AutomaticInstallOSUpdates.ValueString())
	}
	if restored.DeferralCombinedPeriod.ValueInt64() != 15 {
		t.Errorf("roundtrip: expected DeferralCombinedPeriod 15, got %d", restored.DeferralCombinedPeriod.ValueInt64())
	}
	if restored.NotificationsEnabled.ValueBool() != true {
		t.Errorf("roundtrip: expected NotificationsEnabled true, got %v", restored.NotificationsEnabled.ValueBool())
	}
	if restored.RapidSecurityResponseEnabled.ValueBool() != true {
		t.Errorf("roundtrip: expected RapidSecurityResponseEnabled true, got %v", restored.RapidSecurityResponseEnabled.ValueBool())
	}
	if restored.RecommendedCadence.ValueString() != "Oldest" {
		t.Errorf("roundtrip: expected RecommendedCadence 'Oldest', got %q", restored.RecommendedCadence.ValueString())
	}
}

func TestSoftwareUpdateSettings_ToClientComponent(t *testing.T) {
	c := &SoftwareUpdateSettingsComponent{
		AllowStandardUserOSUpdates: types.BoolValue(true),
		NotificationsEnabled:       types.BoolValue(false),
	}

	comp, err := c.ToClientComponent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if comp.Identifier != "com.jamf.ddm.software-update-settings" {
		t.Errorf("expected identifier 'com.jamf.ddm.software-update-settings', got %q", comp.Identifier)
	}
	if comp.Configuration == nil {
		t.Fatal("expected non-nil configuration")
	}
}

// TestSoftwareUpdateSettings_FromRawConfiguration_UIWrittenQuotedScalars is the
// regression test for issue #431.
//
// The blueprints service validates a component configuration on write and then serves
// it back byte-for-byte: it coerces "5" to 5 for validation but never re-serialises
// from its own models, so whatever JSON encoding the writer used is what every later
// read returns. The Jamf Pro web UI writes these scalars as JSON strings, so a
// blueprint built there answers {"Value": "5"} where the spec declares an integer.
//
// Before jamfplatform-go-sdk v1.1.0 that stopped Go's decoder at the first quoted
// number and the whole component was dropped from state, so
// `terraform plan -generate-config-out` emitted a resource missing a component the
// blueprint has. The SDK now carries tolerant UnmarshalJSON methods on these types.
// This test pins the behaviour from the provider's side of the boundary, because the
// consumer is what decodes a configuration — Component.Configuration is a
// json.RawMessage and no SDK method decodes it.
//
// The payload is a raw literal rather than a marshalled map on purpose: marshalling a
// Go map emits bare numbers and booleans, which is the encoding that always worked.
func TestSoftwareUpdateSettings_FromRawConfiguration_UIWrittenQuotedScalars(t *testing.T) {
	raw := []byte(`{
		"Deferrals": {
			"CombinedPeriodInDays": {"Value": "30", "Included": "true"},
			"MajorPeriodInDays":    {"Value": "5",  "Included": true},
			"MinorPeriodInDays":    {"Value": 14,   "Included": true},
			"SystemPeriodInDays":   {"Value": "7",  "Included": "true"}
		},
		"Notifications": {"Enabled": "true", "Included": "true"},
		"RecommendedCadence": {"Value": "Newest", "Included": true}
	}`)

	c := &SoftwareUpdateSettingsComponent{}
	if err := c.FromRawConfiguration(raw); err != nil {
		t.Fatalf("a UI-written configuration must decode, got: %v", err)
	}

	for _, tc := range []struct {
		field string
		got   types.Int64
		want  int64
	}{
		{"DeferralCombinedPeriod", c.DeferralCombinedPeriod, 30},
		{"DeferralMajorPeriod", c.DeferralMajorPeriod, 5},
		{"DeferralMinorPeriod", c.DeferralMinorPeriod, 14},
		{"DeferralSystemPeriod", c.DeferralSystemPeriod, 7},
	} {
		if tc.got.IsNull() {
			t.Errorf("%s is null, so the component would be missing a value the blueprint holds", tc.field)
			continue
		}
		if tc.got.ValueInt64() != tc.want {
			t.Errorf("%s = %d, want %d", tc.field, tc.got.ValueInt64(), tc.want)
		}
	}

	if c.NotificationsEnabled.IsNull() || !c.NotificationsEnabled.ValueBool() {
		t.Errorf("NotificationsEnabled = %v, want true — a quoted boolean must decode too", c.NotificationsEnabled)
	}
	if c.RecommendedCadence.ValueString() != "Newest" {
		t.Errorf("RecommendedCadence = %q, want %q", c.RecommendedCadence.ValueString(), "Newest")
	}
}

// TestSoftwareUpdateSettings_FromRawConfiguration_UncoercibleScalarStillFails guards the
// other side of the tolerance added for issue #431: only a JSON string holding a valid
// scalar of the declared kind is rewritten. Text that is not a number must still fail,
// rather than decode as zero and silently report a deferral period the tenant does not
// have.
func TestSoftwareUpdateSettings_FromRawConfiguration_UncoercibleScalarStillFails(t *testing.T) {
	raw := []byte(`{"Deferrals": {"MajorPeriodInDays": {"Value": "none", "Included": true}}}`)

	c := &SoftwareUpdateSettingsComponent{}
	err := c.FromRawConfiguration(raw)
	if err == nil {
		t.Fatalf("a non-numeric deferral period must fail to decode, got MajorPeriodInDays=%v", c.DeferralMajorPeriod)
	}
	if !strings.Contains(err.Error(), "MajorPeriodInDays") {
		t.Errorf("the error must name the field a caller can look up, got: %v", err)
	}
}
