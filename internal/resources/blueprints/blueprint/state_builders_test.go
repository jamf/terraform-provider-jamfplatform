// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

func TestParseComponentConfiguration_Found(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm.disk-management": {
			Identifier:    "com.jamf.ddm.disk-management",
			Configuration: json.RawMessage(`{"externalStorage":"deny","networkStorage":"deny"}`),
		},
	}

	rawConfig, ok := parseComponentConfiguration(apiComponents, "com.jamf.ddm.disk-management")
	if !ok {
		t.Fatal("expected to find configuration")
	}
	var config map[string]any
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		t.Fatalf("failed to unmarshal configuration: %v", err)
	}
	if config["externalStorage"] != "deny" {
		t.Errorf("expected externalStorage 'deny', got %v", config["externalStorage"])
	}
}

func TestParseComponentConfiguration_NotFound(t *testing.T) {
	apiComponents := map[string]blueprints.Component{}

	_, ok := parseComponentConfiguration(apiComponents, "com.jamf.ddm.disk-management")
	if ok {
		t.Error("expected not found for missing component")
	}
}

func TestParseComponentConfiguration_NilConfiguration(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm.disk-management": {
			Identifier:    "com.jamf.ddm.disk-management",
			Configuration: nil,
		},
	}

	_, ok := parseComponentConfiguration(apiComponents, "com.jamf.ddm.disk-management")
	if ok {
		t.Error("expected not found for nil configuration")
	}
}

func TestParseComponentConfiguration_InvalidJSON(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm.disk-management": {
			Identifier:    "com.jamf.ddm.disk-management",
			Configuration: json.RawMessage(`{invalid`),
		},
	}

	rawConfig, ok := parseComponentConfiguration(apiComponents, "com.jamf.ddm.disk-management")
	if !ok {
		t.Error("expected to find raw bytes even for invalid JSON — validation is caller's responsibility")
	}
	if string(rawConfig) != "{invalid" {
		t.Errorf("expected raw bytes returned as-is, got %q", string(rawConfig))
	}
}

func TestUpdateModelFromAPIResponse_BasicFields(t *testing.T) {
	ctx := t.Context()
	model := &BlueprintResourceModel{}
	desc := "A test blueprint"
	created, _ := time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
	updated, _ := time.Parse(time.RFC3339, "2025-01-02T00:00:00Z")
	blueprint := &blueprints.BlueprintDetail{
		ID:          "bp-123",
		Name:        "Test Blueprint",
		Description: &desc,
		Created:     created,
		Updated:     updated,
		DeploymentState: &blueprints.DeploymentState{
			State: "DEPLOYED",
		},
		Scope: &blueprints.BlueprintScope{
			DeviceGroups: []string{"group-1", "group-2"},
		},
		Steps: []blueprints.BlueprintStep{},
	}

	updateModelFromAPIResponse(ctx, model, blueprint)

	if model.ID.ValueString() != "bp-123" {
		t.Errorf("expected ID 'bp-123', got %q", model.ID.ValueString())
	}
	if model.Name.ValueString() != "Test Blueprint" {
		t.Errorf("expected Name 'Test Blueprint', got %q", model.Name.ValueString())
	}
	if model.Description.ValueString() != "A test blueprint" {
		t.Errorf("expected Description 'A test blueprint', got %q", model.Description.ValueString())
	}
	if model.DeploymentState.ValueString() != "DEPLOYED" {
		t.Errorf("expected DeploymentState 'DEPLOYED', got %q", model.DeploymentState.ValueString())
	}
	if !model.Deployed.ValueBool() {
		t.Error("expected Deployed to be true for DEPLOYED state")
	}
	if model.Created.ValueString() != "2025-01-01T00:00:00Z" {
		t.Errorf("expected Created timestamp, got %q", model.Created.ValueString())
	}
}

func TestUpdateModelFromAPIResponse_EmptyDescription(t *testing.T) {
	ctx := t.Context()
	model := &BlueprintResourceModel{Description: types.StringNull()}
	blueprint := &blueprints.BlueprintDetail{
		Description:     nil,
		DeploymentState: &blueprints.DeploymentState{State: "NOT_DEPLOYED"},
		Steps:           []blueprints.BlueprintStep{},
	}

	updateModelFromAPIResponse(ctx, model, blueprint)

	if !model.Description.IsNull() {
		t.Error("expected Description to remain null when API returns empty and model is null")
	}
}

func TestUpdateModelFromAPIResponse_NotDeployed(t *testing.T) {
	ctx := t.Context()
	model := &BlueprintResourceModel{}
	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "NOT_DEPLOYED"},
		Steps:           []blueprints.BlueprintStep{},
	}

	updateModelFromAPIResponse(ctx, model, blueprint)

	if model.Deployed.ValueBool() {
		t.Error("expected Deployed to be false for NOT_DEPLOYED state")
	}
}

func TestUpdateModelFromAPIResponse_RawComponents(t *testing.T) {
	ctx := t.Context()
	model := &BlueprintResourceModel{
		ComponentBlocks: []ComponentBlockModel{{
			Components: []ComponentModel{
				{Identifier: types.StringValue("com.jamf.ddm.disk-management")},
			},
		}},
	}

	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
		Steps: []blueprints.BlueprintStep{
			{
				Components: []blueprints.Component{
					{
						Identifier:    "com.jamf.ddm.disk-management",
						Configuration: json.RawMessage(`{"externalStorage":"deny"}`),
					},
				},
			},
		},
	}

	updateModelFromAPIResponse(ctx, model, blueprint)

	if len(model.ComponentBlocks) != 1 || len(model.ComponentBlocks[0].Components) != 1 {
		t.Fatalf("expected 1 block with 1 raw component, got %+v", model.ComponentBlocks)
	}
	if got := model.ComponentBlocks[0].Components[0].Identifier.ValueString(); got != "com.jamf.ddm.disk-management" {
		t.Errorf("expected identifier 'com.jamf.ddm.disk-management', got %q", got)
	}
}

func TestUpdateModelFromAPIResponse_NoSteps(t *testing.T) {
	ctx := t.Context()
	model := &BlueprintResourceModel{}
	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
		Steps:           []blueprints.BlueprintStep{},
	}

	updateModelFromAPIResponse(ctx, model, blueprint)

	if model.ComponentBlocks != nil {
		t.Error("expected nil component_blocks when no steps")
	}
}

// The three wire behaviours the blueprints service applies to a stored legacy payload, pinned as
// unit tests because each one manufactured a perpetual diff (and a post-apply inconsistent-result
// error) when the flatteners echoed the wire back verbatim:
//
//   - it stamps Apple's common payload metadata onto every payload (payloadDisplayName,
//     payloadOrganization, payloadUUID, payloadVersion);
//   - it discards a null-valued key rather than storing it;
//   - it discards a key Apple's schema for that payload type does not define.
//
// The first two are absorbed (masked / null-tolerant compare); the third is surfaced, as a genuine
// mismatch plus a warning naming the keys.

func TestFlattenBlockLegacyPayloads_MasksServerStampedMetadata(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}
	authored := `{"allowSafariPrivateBrowsing":false}`
	prior := []BlockLegacyPayloadModel{{
		PayloadType: types.StringValue("com.apple.applicationaccess"),
		Settings:    types.StringValue(authored),
	}}

	got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	if got[0].Settings.ValueString() != authored {
		t.Errorf("expected the authored settings preserved verbatim, got %q", got[0].Settings.ValueString())
	}
}

func TestFlattenBlockLegacyPayloads_KeepsAuthoredMetadata(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"My Own Name","payloadOrganization":"Acme",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}
	// The author declared two of the stamped keys, and the service echoed both back, so both must
	// survive the mask — dropping them would flip the diff around and delete the author's values.
	authored := `{"allowSafariPrivateBrowsing":false,"payloadDisplayName":"My Own Name","payloadOrganization":"Acme"}`
	prior := []BlockLegacyPayloadModel{{
		PayloadType: types.StringValue("com.apple.applicationaccess"),
		Settings:    types.StringValue(authored),
	}}

	got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	if got[0].Settings.ValueString() != authored {
		t.Errorf("expected the authored settings preserved verbatim, got %q", got[0].Settings.ValueString())
	}
}

// TestFlattenBlockLegacyPayloads_KeepsAuthoredServiceOwnedMetadata covers the two metadata keys the
// service assigns itself (see providerMaskedPayloadKeys). The provider strips both from every write
// and from the server-derived settings, so it has to strip them from the authored string before
// comparing too: settings is Optional rather than Computed, so the planned value is what the author
// wrote, and state taking the stripped encoding instead would fail the apply with "Provider produced
// inconsistent result after apply" and repeat on every plan after it.
//
// The Apple spelling is covered alongside the Jamf one because an author is sent to it:
// appleprofiles.Validate reports `payloadUUID` as a miscased key and names `PayloadUUID` as the
// spelling to use, and for the seven payload types that declare a top-level wildcard neither
// spelling is reported at all.
func TestFlattenBlockLegacyPayloads_KeepsAuthoredServiceOwnedMetadata(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"SERVER-IDENT","payloadUUID":"SERVER-IDENT",` +
				`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}

	tests := []struct {
		name     string
		authored string
	}{
		{"jamf spelling payloadUUID", `{"allowSafariPrivateBrowsing":false,"payloadUUID":"AUTHORED-UUID"}`},
		{"jamf spelling payloadIdentifier", `{"allowSafariPrivateBrowsing":false,"payloadIdentifier":"AUTHORED-IDENT"}`},
		{"apple spelling PayloadUUID", `{"PayloadUUID":"AUTHORED-UUID","allowSafariPrivateBrowsing":false}`},
		{"apple spelling PayloadIdentifier", `{"PayloadIdentifier":"AUTHORED-IDENT","allowSafariPrivateBrowsing":false}`},
		{"both keys at once", `{"allowSafariPrivateBrowsing":false,"payloadIdentifier":"AUTHORED-IDENT","payloadUUID":"AUTHORED-UUID"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prior := []BlockLegacyPayloadModel{{
				PayloadType: types.StringValue("com.apple.applicationaccess"),
				Settings:    types.StringValue(tc.authored),
			}}

			got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

			if len(got) != 1 {
				t.Fatalf("expected 1 payload, got %d", len(got))
			}
			if got[0].Settings.ValueString() != tc.authored {
				t.Errorf("expected the authored settings preserved verbatim, got %q", got[0].Settings.ValueString())
			}
		})
	}
}

// TestFlattenBlockLegacyPayloads_ServiceOwnedMetadataNeverReachesState is the other half of the
// mask: an author who wrote neither key gets neither back, so the service's own identifier and UUID
// stay out of state.
func TestFlattenBlockLegacyPayloads_ServiceOwnedMetadataNeverReachesState(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"SERVER-IDENT","payloadUUID":"SERVER-IDENT",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}

	got := flattenBlockLegacyPayloads(nil, apiComponents, map[string]struct{}{})

	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	for _, key := range providerMaskedPayloadKeys {
		if strings.Contains(got[0].Settings.ValueString(), key) {
			t.Errorf("expected %s masked out of state, got %q", key, got[0].Settings.ValueString())
		}
	}
}

func TestFlattenBlockLegacyPayloads_ToleratesDiscardedNulls(t *testing.T) {
	// The service stored the payload without PreviewType or GroupingType, because the author gave
	// them null. The authored string must survive so the nulls stay in configuration shape.
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.notificationsettings","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"Notifications","payloadOrganization":"JAMF Software",` +
				`"NotificationSettings":[{"AlertType":0,"BundleIdentifier":"com.example.app","NotificationsEnabled":false}]}]}`),
		},
	}
	authored := `{"NotificationSettings":[{"AlertType":0,"BundleIdentifier":"com.example.app","GroupingType":null,"NotificationsEnabled":false,"PreviewType":null}]}`
	prior := []BlockLegacyPayloadModel{{
		PayloadType: types.StringValue("com.apple.notificationsettings"),
		Settings:    types.StringValue(authored),
	}}

	got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	if got[0].Settings.ValueString() != authored {
		t.Errorf("expected the authored settings preserved verbatim, got %q", got[0].Settings.ValueString())
	}
}

func TestFlattenBlockLegacyPayloads_SurfacesDiscardedNonNullKey(t *testing.T) {
	// allowCamera was authored with a real value and is absent from the wire, so this is a genuine
	// mismatch — the service refused it — and state must move to the wire truth.
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}
	prior := []BlockLegacyPayloadModel{{
		PayloadType: types.StringValue("com.apple.applicationaccess"),
		Settings:    types.StringValue(`{"allowCamera":true,"allowSafariPrivateBrowsing":false}`),
	}}

	got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	if got[0].Settings.ValueString() != `{"allowSafariPrivateBrowsing":false}` {
		t.Errorf("expected state to move to the wire truth, got %q", got[0].Settings.ValueString())
	}
}

func TestPruneJSONNulls(t *testing.T) {
	var value any
	if err := json.Unmarshal([]byte(`{"a":null,"b":1,"c":{"d":null,"e":"x"},"f":[{"g":null,"h":2}],"i":[]}`), &value); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	got, err := json.Marshal(pruneJSONNulls(value))
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	want := `{"b":1,"c":{"e":"x"},"f":[{"h":2}],"i":[]}`
	if string(got) != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestDiscardedSettingsPaths(t *testing.T) {
	tests := []struct {
		name     string
		authored string
		stored   string
		want     []string
	}{
		{
			name:     "nothing discarded",
			authored: `{"allowCamera":true}`,
			stored:   `{"allowCamera":true}`,
		},
		{
			name:     "null keys are not discards",
			authored: `{"allowCamera":true,"allowScreenShot":null}`,
			stored:   `{"allowCamera":true}`,
		},
		{
			name:     "top-level key dropped",
			authored: `{"allowCamera":true,"bogusKey":"x"}`,
			stored:   `{"allowCamera":true}`,
			want:     []string{"bogusKey"},
		},
		{
			name:     "key dropped inside a nested array entry",
			authored: `{"NotificationSettings":[{"BundleIdentifier":"a","Bogus":1}]}`,
			stored:   `{"NotificationSettings":[{"BundleIdentifier":"a"}]}`,
			want:     []string{"NotificationSettings[0].Bogus"},
		},
		{
			name:     "whole payload rejected",
			authored: `{"allowCamera":true,"bogusKey":"x"}`,
			stored:   ``,
			want:     []string{"allowCamera", "bogusKey"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var authored, stored any
			if err := json.Unmarshal([]byte(tt.authored), &authored); err != nil {
				t.Fatalf("failed to unmarshal authored: %v", err)
			}
			if tt.stored != "" {
				if err := json.Unmarshal([]byte(tt.stored), &stored); err != nil {
					t.Fatalf("failed to unmarshal stored: %v", err)
				}
			}

			got := discardedSettingsPaths(authored, stored, "")
			if len(got) != len(tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("expected %v, got %v", tt.want, got)
					break
				}
			}
		})
	}
}

func TestCheckLegacyPayloadDiscards_WarnsOnDiscardedKey(t *testing.T) {
	stepName := "Restrictions"
	blueprint := &blueprints.BlueprintDetail{
		Steps: []blueprints.BlueprintStep{{
			Name: &stepName,
			Components: []blueprints.Component{{
				Identifier: "com.jamf.ddm-configuration-profile",
				Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
					`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi",` +
					`"allowSafariPrivateBrowsing":false}]}`),
			}},
		}},
	}
	planned := &BlueprintResourceModel{
		ComponentBlocks: []ComponentBlockModel{{
			Name: types.StringValue("Restrictions"),
			LegacyPayloads: []BlockLegacyPayloadModel{{
				PayloadType: types.StringValue("com.apple.applicationaccess"),
				Settings:    types.StringValue(`{"allowSafariPrivateBrowsing":false,"AllowCamera":true,"bogusKey":"x"}`),
			}},
		}},
	}

	diags := checkLegacyPayloadDiscards(planned, blueprint)

	if diags.WarningsCount() != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", diags.WarningsCount(), diags)
	}
	if diags.ErrorsCount() != 0 {
		t.Fatalf("expected no errors, got %v", diags.Errors())
	}
	detail := diags.Warnings()[0].Detail()
	for _, want := range []string{"AllowCamera", "bogusKey", "com.apple.applicationaccess", "component_blocks[0] (Restrictions)"} {
		if !strings.Contains(detail, want) {
			t.Errorf("expected the warning to name %q, got %q", want, detail)
		}
	}
	if strings.Contains(detail, "allowSafariPrivateBrowsing") {
		t.Errorf("expected the stored key not reported as discarded, got %q", detail)
	}
}

func TestCheckLegacyPayloadDiscards_SilentWhenNothingDiscarded(t *testing.T) {
	blueprint := &blueprints.BlueprintDetail{
		Steps: []blueprints.BlueprintStep{{
			Components: []blueprints.Component{{
				Identifier: "com.jamf.ddm-configuration-profile",
				Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
					`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi","payloadUUID":"pi",` +
					`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
					`"allowSafariPrivateBrowsing":false}]}`),
			}},
		}},
	}
	planned := &BlueprintResourceModel{
		ComponentBlocks: []ComponentBlockModel{{
			LegacyPayloads: []BlockLegacyPayloadModel{{
				PayloadType: types.StringValue("com.apple.applicationaccess"),
				Settings:    types.StringValue(`{"allowSafariPrivateBrowsing":false,"allowCamera":null}`),
			}},
		}},
	}

	if diags := checkLegacyPayloadDiscards(planned, blueprint); len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %v", diags)
	}
}

// TestCheckLegacyPayloadDiscards_SilentForServiceOwnedMetadata pins that a key the provider itself
// removes is never reported as one the platform dropped. The warning tells an operator to check the
// key against Apple's documentation and its capitalisation, which for these two keys is a dead end:
// Apple defines both, and the reason they are absent is the provider, not the platform.
func TestCheckLegacyPayloadDiscards_SilentForServiceOwnedMetadata(t *testing.T) {
	blueprint := &blueprints.BlueprintDetail{
		Steps: []blueprints.BlueprintStep{{
			Components: []blueprints.Component{{
				Identifier: "com.jamf.ddm-configuration-profile",
				Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
					`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"SERVER-IDENT","payloadUUID":"SERVER-IDENT",` +
					`"allowSafariPrivateBrowsing":false}]}`),
			}},
		}},
	}
	planned := &BlueprintResourceModel{
		ComponentBlocks: []ComponentBlockModel{{
			LegacyPayloads: []BlockLegacyPayloadModel{{
				PayloadType: types.StringValue("com.apple.applicationaccess"),
				Settings: types.StringValue(
					`{"PayloadUUID":"APPLE-SPELLING","allowSafariPrivateBrowsing":false,` +
						`"payloadIdentifier":"AUTHORED-IDENT","payloadUUID":"AUTHORED-UUID"}`),
			}},
		}},
	}

	if diags := checkLegacyPayloadDiscards(planned, blueprint); len(diags) != 0 {
		t.Errorf("expected no diagnostics, got %v", diags)
	}
}

func TestIsRedacted(t *testing.T) {
	tests := []struct {
		value any
		want  bool
	}{
		{"**********", true},
		{"****", true},
		{"***", false}, // shorter than the sentinel the provider will trust
		{"abc", false},
		{"*abc*", false},
		{"", false},
		{42, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := isRedacted(tt.value); got != tt.want {
			t.Errorf("isRedacted(%#v) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestFlattenBlockLegacyPayloads_RestoresRedactedCredential(t *testing.T) {
	// Jamf returns a Wi-Fi credential as a run of asterisks, nested inside EAPClientConfiguration.
	// Without restoring the authored value the payload can never settle: the wire value differs from
	// configuration on every read and applying cannot resolve it.
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.wifi.managed","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"Wi-Fi","payloadOrganization":"JAMF Software",` +
				`"SSID_STR":"EXAMPLE-CORP","Password":"**********",` +
				`"EAPClientConfiguration":{"AcceptEAPTypes":[25],"UserName":"**********","UserPassword":"**********"}}]}`),
		},
	}
	authored := `{"EAPClientConfiguration":{"AcceptEAPTypes":[25],"UserName":"svc-user","UserPassword":"s3cret"},"Password":"psk-secret","SSID_STR":"EXAMPLE-CORP"}`
	prior := []BlockLegacyPayloadModel{{
		PayloadType: types.StringValue("com.apple.wifi.managed"),
		Settings:    types.StringValue(authored),
	}}

	got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	if got[0].Settings.ValueString() != authored {
		t.Errorf("expected the authored settings preserved verbatim, got %q", got[0].Settings.ValueString())
	}
}

func TestRestoreRedactedValues_LeavesUnauthoredRedactionAlone(t *testing.T) {
	// On import there is no authored value to restore from, so the sentinel has to stay: it is not
	// recoverable from the service, and inventing a value would be worse than showing what it sent.
	var wire, authored any
	if err := json.Unmarshal([]byte(`{"Password":"**********","SSID_STR":"X"}`), &wire); err != nil {
		t.Fatalf("failed to decode wire: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"SSID_STR":"X"}`), &authored); err != nil {
		t.Fatalf("failed to decode authored: %v", err)
	}

	got, err := json.Marshal(restoreRedactedValues(wire, authored))
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if string(got) != `{"Password":"**********","SSID_STR":"X"}` {
		t.Errorf("expected the sentinel left in place, got %s", got)
	}
}

func TestRestoreRedactedValues_DoesNotSubstituteNonRedactedValue(t *testing.T) {
	// A genuine out-of-band change must still surface, so only a redaction is substituted.
	var wire, authored any
	if err := json.Unmarshal([]byte(`{"Password":"changed-elsewhere"}`), &wire); err != nil {
		t.Fatalf("failed to decode wire: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"Password":"psk-secret"}`), &authored); err != nil {
		t.Fatalf("failed to decode authored: %v", err)
	}

	got, err := json.Marshal(restoreRedactedValues(wire, authored))
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if string(got) != `{"Password":"changed-elsewhere"}` {
		t.Errorf("expected the wire value kept, got %s", got)
	}
}

// --- Typed components in blocks ---

// TestUpdateComponentBlocksFromAPI_TypedComponentStaysOutOfRawComponent pins that a component with a
// typed component_blocks attribute is read into that attribute only. Writing raw_component as well
// would populate both at once, which fails an apply with "Provider produced inconsistent result
// after apply".
func TestUpdateComponentBlocksFromAPI_TypedComponentStaysOutOfRawComponent(t *testing.T) {
	ctx := context.Background()

	for _, identifier := range []string{"com.jamf.ai-governance", appleDeclarationsIdentifier} {
		t.Run(identifier, func(t *testing.T) {
			model := &BlueprintResourceModel{}
			blueprint := &blueprints.BlueprintDetail{
				DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
				Steps: []blueprints.BlueprintStep{{
					Name:       new("Block"),
					Components: []blueprints.Component{{Identifier: identifier, Configuration: json.RawMessage(`{}`)}},
				}},
			}

			diags := updateModelFromAPIResponse(ctx, model, blueprint)
			if diags.HasError() {
				t.Fatalf("unexpected error diagnostics: %v", diags)
			}
			if len(model.ComponentBlocks) != 1 {
				t.Fatalf("expected 1 component block, got %d", len(model.ComponentBlocks))
			}
			if got := model.ComponentBlocks[0].Components; len(got) != 0 {
				t.Errorf("%q has a typed component_blocks attribute, so a block-mode read must not also write raw_component, got %+v", identifier, got)
			}
		})
	}
}

// TestFlattenBlockLegacyPayloads_EachPreferenceDomainKeepsItsOwnAuthoredString covers the read side
// of one block carrying a custom settings payload per preference domain. Each payload's authored
// JSON string must be paired with the wire payload for its own domain: pairing by payload type
// leaves only one of them in the lookup, so the others are re-encoded and every plan reports a
// difference in a string that says the same thing.
func TestFlattenBlockLegacyPayloads_EachPreferenceDomainKeepsItsOwnAuthoredString(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[` +
				`{"payloadType":"com.apple.ManagedClient.preferences","payloadIdentifier":"one","payloadUUID":"one",` +
				`"PayloadContent":{"com.example.first":{"Forced":[{"mcx_preference_settings":{"key":1}}]}}},` +
				`{"payloadType":"com.apple.ManagedClient.preferences","payloadIdentifier":"two","payloadUUID":"two",` +
				`"PayloadContent":{"com.example.second":{"Forced":[{"mcx_preference_settings":{"key":2}}]}}}]}`),
		},
	}

	authoredFirst := `{ "PayloadContent": { "com.example.first": { "Forced": [ { "mcx_preference_settings": { "key": 1 } } ] } } }`
	authoredSecond := `{ "PayloadContent": { "com.example.second": { "Forced": [ { "mcx_preference_settings": { "key": 2 } } ] } } }`
	prior := []BlockLegacyPayloadModel{
		{PayloadType: types.StringValue("com.apple.ManagedClient.preferences"), Settings: types.StringValue(authoredFirst)},
		{PayloadType: types.StringValue("com.apple.ManagedClient.preferences"), Settings: types.StringValue(authoredSecond)},
	}

	got := flattenBlockLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if len(got) != 2 {
		t.Fatalf("expected 2 payloads, got %d: %v", len(got), got)
	}
	if got[0].Settings.ValueString() != authoredFirst {
		t.Errorf("the first domain did not keep its authored string, got %q", got[0].Settings.ValueString())
	}
	if got[1].Settings.ValueString() != authoredSecond {
		t.Errorf("the second domain did not keep its authored string, got %q", got[1].Settings.ValueString())
	}
}

// TestCheckLegacyPayloadDiscards_SeveralPreferenceDomainsCompareIndependently pins the keying
// appendLegacyPayloadDiscardWarnings depends on. Both sides key on legacyPayloadIdentity, so each
// custom settings payload is compared against what was stored for its own preference domain. Keying
// by payload type collapses the two to one entry and compares one payload against the other's
// settings, which reports every key of both as discarded.
//
// The payloads are stored in the reverse of the authored order deliberately: with the orders
// matching, a type-keyed comparison happens to pick the right settings and the regression is
// invisible. The array order is the service's, so nothing guarantees it matches configuration.
func TestCheckLegacyPayloadDiscards_SeveralPreferenceDomainsCompareIndependently(t *testing.T) {
	const (
		first  = "com.example.first"
		second = "com.example.second"
	)
	mcxSettings := func(domain, key string) string {
		return `{"PayloadContent":{"` + domain + `":{"Forced":[{"mcx_preference_settings":{"` + key + `":true}}]}}}`
	}
	storedPayload := func(domain, key string) string {
		return `{"payloadType":"com.apple.ManagedClient.preferences","payloadIdentifier":"pi-` + domain + `",` +
			`"PayloadContent":{"` + domain + `":{"Forced":[{"mcx_preference_settings":{"` + key + `":true}}]}}}`
	}

	stepName := "Preferences"
	blueprint := &blueprints.BlueprintDetail{
		Steps: []blueprints.BlueprintStep{{
			Name: &stepName,
			Components: []blueprints.Component{{
				Identifier: "com.jamf.ddm-configuration-profile",
				Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[` +
					storedPayload(second, "SecondKey") + `,` + storedPayload(first, "FirstKey") + `]}`),
			}},
		}},
	}
	planned := &BlueprintResourceModel{
		ComponentBlocks: []ComponentBlockModel{{
			Name: types.StringValue(stepName),
			LegacyPayloads: []BlockLegacyPayloadModel{
				{
					PayloadType: types.StringValue("com.apple.ManagedClient.preferences"),
					Settings:    types.StringValue(mcxSettings(first, "FirstKey")),
				},
				{
					PayloadType: types.StringValue("com.apple.ManagedClient.preferences"),
					Settings:    types.StringValue(mcxSettings(second, "SecondKey")),
				},
			},
		}},
	}

	diags := checkLegacyPayloadDiscards(planned, blueprint)

	if diags.WarningsCount() != 0 {
		t.Errorf("two faithfully stored preference domains produced %d warning(s): %v", diags.WarningsCount(), diags.Warnings())
	}
	if diags.ErrorsCount() != 0 {
		t.Errorf("expected no errors, got %v", diags.Errors())
	}
}
