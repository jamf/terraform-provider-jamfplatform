// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
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

func TestFlattenFlatLegacyPayloads_WithPayloads(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier:    "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"Test","payloadContent":[{"payloadType":"com.apple.wifi.managed","payloadIdentifier":"test-uuid","SSID_STR":"TestNetwork"}]}`),
		},
	}

	got := flattenFlatLegacyPayloads(types.DynamicNull(), apiComponents, map[string]struct{}{})

	if got.IsNull() {
		t.Fatal("expected non-null legacy payloads")
	}

	raw, err := helpers.TerraformDynamicToJSON(got)
	if err != nil {
		t.Fatalf("failed to convert dynamic to JSON: %v", err)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected list, got %T", raw)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(items))
	}

	payload, ok := items[0].(map[string]any)
	if !ok {
		t.Fatal("expected payload to be a map")
	}
	if payload["payload_type"] != "com.apple.wifi.managed" {
		t.Errorf("expected payload_type 'com.apple.wifi.managed', got %v", payload["payload_type"])
	}

	settings, ok := payload["settings"].(map[string]any)
	if !ok {
		t.Fatal("expected settings to be a map")
	}
	if settings["SSID_STR"] != "TestNetwork" {
		t.Errorf("expected SSID_STR 'TestNetwork', got %v", settings["SSID_STR"])
	}
	if _, exists := settings["payloadType"]; exists {
		t.Error("payloadType should not be in settings")
	}
}

func TestFlattenFlatLegacyPayloads_NoComponent(t *testing.T) {
	existingDyn, _ := helpers.JSONToTerraformDynamic([]any{map[string]any{"payload_type": "should be cleared"}})
	got := flattenFlatLegacyPayloads(existingDyn, map[string]blueprints.Component{}, map[string]struct{}{})

	if !got.IsNull() {
		t.Error("expected null legacy payloads when component is absent")
	}
}

func TestFlattenFlatLegacyPayloads_HandledAsRaw(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier:    "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadContent":[{"payloadType":"com.apple.wifi.managed"}]}`),
		},
	}
	rawIdentifiers := map[string]struct{}{
		"com.jamf.ddm-configuration-profile": {},
	}

	existingDyn, _ := helpers.JSONToTerraformDynamic([]any{map[string]any{"payload_type": "existing"}})
	got := flattenFlatLegacyPayloads(existingDyn, apiComponents, rawIdentifiers)

	raw, err := helpers.TerraformDynamicToJSON(got)
	if err != nil {
		t.Fatalf("failed to convert: %v", err)
	}
	items := raw.([]any)
	if len(items) != 1 {
		t.Error("expected legacy payloads to remain unchanged when handled as raw")
	}
}

func TestFlattenFlatLegacyPayloads_NoPayloadContent(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier:    "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"Test"}`),
		},
	}

	got := flattenFlatLegacyPayloads(types.DynamicNull(), apiComponents, map[string]struct{}{})

	if !got.IsNull() {
		t.Error("expected null legacy payloads when payloadContent is absent")
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
		Components: []ComponentModel{
			{Identifier: types.StringValue("com.jamf.ddm.disk-management")},
		},
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

	if len(model.Components) != 1 {
		t.Fatalf("expected 1 raw component, got %d", len(model.Components))
	}
	if model.Components[0].Identifier.ValueString() != "com.jamf.ddm.disk-management" {
		t.Errorf("expected identifier 'com.jamf.ddm.disk-management', got %q", model.Components[0].Identifier.ValueString())
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

	if model.Components != nil {
		t.Error("expected nil components when no steps")
	}
}

// TestUpdateLegacyPayloadsFromAPI_PreservesConfigShapeOnMatch pins the issue
// #282 fix: when the server response is semantically identical to the incoming
// (configuration-shaped) value, the reader keeps that value verbatim so the
// dynamic null-typing does not manufacture a diff.
func TestFlattenFlatLegacyPayloads_PreservesConfigShapeOnMatch(t *testing.T) {
	prior, err := helpers.JSONToTerraformDynamic([]any{
		map[string]any{
			"payload_type": "com.apple.notificationsettings",
			"settings": map[string]any{
				"NotificationSettings": []any{
					map[string]any{
						"BundleIdentifier":     "com.apple.tips",
						"AlertType":            float64(0),
						"BadgesEnabled":        nil,
						"NotificationsEnabled": false,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to build prior value: %v", err)
	}

	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier:    "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadContent":[{"payloadType":"com.apple.notificationsettings","payloadIdentifier":"generated-uuid","NotificationSettings":[{"BundleIdentifier":"com.apple.tips","AlertType":0,"BadgesEnabled":null,"NotificationsEnabled":false}]}]}`),
		},
	}

	got := flattenFlatLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if !got.Equal(prior) {
		t.Errorf("expected config-shaped prior value to be preserved on semantic match, got %#v", got)
	}
}

// TestUpdateLegacyPayloadsFromAPI_OverwritesOnMismatch verifies that a genuine
// server-side difference is still surfaced rather than masked by the reconcile.
func TestFlattenFlatLegacyPayloads_OverwritesOnMismatch(t *testing.T) {
	prior, err := helpers.JSONToTerraformDynamic([]any{
		map[string]any{
			"payload_type": "com.apple.notificationsettings",
			"settings":     map[string]any{"BundleIdentifier": "com.apple.tips.old"},
		},
	})
	if err != nil {
		t.Fatalf("failed to build prior value: %v", err)
	}

	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier:    "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadContent":[{"payloadType":"com.apple.notificationsettings","BundleIdentifier":"com.apple.tips.new"}]}`),
		},
	}

	got := flattenFlatLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if got.Equal(prior) {
		t.Fatal("expected differing server value to overwrite the prior value")
	}

	raw, err := helpers.TerraformDynamicToJSON(got)
	if err != nil {
		t.Fatalf("failed to convert result: %v", err)
	}
	settings := raw.([]any)[0].(map[string]any)["settings"].(map[string]any)
	if settings["BundleIdentifier"] != "com.apple.tips.new" {
		t.Errorf("expected overwritten value 'com.apple.tips.new', got %v", settings["BundleIdentifier"])
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

func TestFlattenFlatLegacyPayloads_MasksServerStampedMetadata(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}
	prior, err := helpers.JSONToTerraformDynamic([]any{map[string]any{
		"payload_type": "com.apple.applicationaccess",
		"settings":     map[string]any{"allowSafariPrivateBrowsing": false},
	}})
	if err != nil {
		t.Fatalf("failed to build prior dynamic: %v", err)
	}

	got := flattenFlatLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if !got.Equal(prior) {
		t.Errorf("expected the prior dynamic preserved once the stamped metadata is masked, got %v", got)
	}
}

func TestFlattenFlatLegacyPayloads_MasksStampedMetadataOnImport(t *testing.T) {
	// Import has no prior value to mask against, so the stamped metadata must still be dropped —
	// otherwise the imported state carries keys no configuration would ever declare.
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"pi","payloadUUID":"pi",` +
				`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}

	got := flattenFlatLegacyPayloads(types.DynamicNull(), apiComponents, map[string]struct{}{})

	raw, err := helpers.TerraformDynamicToJSON(got)
	if err != nil {
		t.Fatalf("failed to convert dynamic to JSON: %v", err)
	}
	settings := raw.([]any)[0].(map[string]any)["settings"].(map[string]any)
	for _, key := range serverStampedPayloadKeys {
		if _, exists := settings[key]; exists {
			t.Errorf("expected %s masked out of settings on import, got %v", key, settings)
		}
	}
	if _, exists := settings["allowSafariPrivateBrowsing"]; !exists {
		t.Errorf("expected the real payload key kept, got %v", settings)
	}
}

// TestFlattenFlatLegacyPayloads_KeepsAuthoredServiceOwnedMetadata is the deprecated top-level
// attribute's half of TestFlattenBlockLegacyPayloads_KeepsAuthoredServiceOwnedMetadata. The
// attribute is no more Computed than the block one, so a one-sided mask fails an apply there too.
func TestFlattenFlatLegacyPayloads_KeepsAuthoredServiceOwnedMetadata(t *testing.T) {
	apiComponents := map[string]blueprints.Component{
		"com.jamf.ddm-configuration-profile": {
			Identifier: "com.jamf.ddm-configuration-profile",
			Configuration: json.RawMessage(`{"payloadDisplayName":"bp","payloadContent":[{` +
				`"payloadType":"com.apple.applicationaccess","payloadIdentifier":"SERVER-IDENT","payloadUUID":"SERVER-IDENT",` +
				`"payloadVersion":1,"payloadDisplayName":"Restrictions","payloadOrganization":"JAMF Software",` +
				`"allowSafariPrivateBrowsing":false}]}`),
		},
	}
	prior, err := helpers.JSONToTerraformDynamic([]any{map[string]any{
		"payload_type": "com.apple.applicationaccess",
		"settings": map[string]any{
			"allowSafariPrivateBrowsing": false,
			"payloadIdentifier":          "AUTHORED-IDENT",
			"payloadUUID":                "AUTHORED-UUID",
		},
	}})
	if err != nil {
		t.Fatalf("failed to build prior dynamic: %v", err)
	}

	got := flattenFlatLegacyPayloads(prior, apiComponents, map[string]struct{}{})

	if !got.Equal(prior) {
		t.Errorf("expected the prior dynamic preserved once the service-owned metadata is masked, got %v", got)
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

// --- Block-only components in flat mode ---

// TestUpdateFlatComponentsFromAPI_KeepsBlockOnlyComponentInRawComponent pins the fallback that
// stops a flat-mode read losing a component the flat attributes cannot represent. Dropping it is
// silent and destructive: a flat-mode apply rewrites every step from state, so a component absent
// from state is deleted from the blueprint with nothing in the plan to show it.
func TestUpdateFlatComponentsFromAPI_KeepsBlockOnlyComponentInRawComponent(t *testing.T) {
	ctx := context.Background()

	for identifier := range blockOnlyComponentIdentifiers {
		t.Run(identifier, func(t *testing.T) {
			model := &BlueprintResourceModel{
				Components: []ComponentModel{
					{Identifier: types.StringValue("com.jamf.custom.x"), Configuration: types.MapNull(types.StringType)},
				},
			}
			blueprint := &blueprints.BlueprintDetail{
				DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
				Steps: []blueprints.BlueprintStep{{
					Components: []blueprints.Component{
						{Identifier: identifier, Configuration: json.RawMessage(`{}`)},
						{Identifier: "com.jamf.custom.x", Configuration: json.RawMessage(`{"k":"v"}`)},
					},
				}},
			}

			diags := updateModelFromAPIResponse(ctx, model, blueprint)
			if diags.HasError() {
				t.Fatalf("unexpected error diagnostics: %v", diags)
			}
			if model.ComponentBlocks != nil {
				t.Fatal("expected a flat-mode read, got component_blocks populated")
			}

			var identifiers []string
			for _, comp := range model.Components {
				identifiers = append(identifiers, comp.Identifier.ValueString())
			}
			if len(identifiers) != 2 || identifiers[0] != identifier || identifiers[1] != "com.jamf.custom.x" {
				t.Fatalf("expected %q retained in raw_component in the platform's component order, got %v", identifier, identifiers)
			}
		})
	}
}

// TestUpdateComponentBlocksFromAPI_BlockOnlyComponentStaysTypedInBlockMode is the other half: the
// fallback is flat-mode only, and a block-mode read that also wrote raw_component would populate the
// typed attribute and raw_component at once, which fails an apply with "Provider produced
// inconsistent result after apply".
func TestUpdateComponentBlocksFromAPI_BlockOnlyComponentStaysTypedInBlockMode(t *testing.T) {
	ctx := context.Background()

	for identifier := range blockOnlyComponentIdentifiers {
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

// TestBlockOnlyComponentIdentifiers_CoversEveryTypedComponentWithoutFlatField is the durable guard
// behind the fallback above. A typed component identifier with no BlueprintResourceModel field is
// unrepresentable in flat mode, so it must be registered in blockOnlyComponentIdentifiers or a
// flat-mode read drops it. The check reads the model's tfsdk tags rather than a hand-kept list, so
// the next block-only component fails here instead of silently reintroducing the drop.
func TestBlockOnlyComponentIdentifiers_CoversEveryTypedComponentWithoutFlatField(t *testing.T) {
	flatFields := make(map[string]struct{})
	modelType := reflect.TypeFor[BlueprintResourceModel]()
	for field := range modelType.Fields() {
		if tag, ok := field.Tag.Lookup("tfsdk"); ok {
			flatFields[tag] = struct{}{}
		}
	}

	attributeFor := make(map[string]string, len(stronglyTypedComponentIdentifiers))
	for _, tc := range typedComponentCases() {
		attributeFor[tc.identifier] = tc.schemaAttribute
	}

	for identifier := range stronglyTypedComponentIdentifiers {
		attribute, known := attributeFor[identifier]
		if !known {
			t.Errorf("stronglyTypedComponentIdentifiers entry %q has no typedComponentCases row, so its flat representation cannot be checked", identifier)
			continue
		}

		_, hasFlatField := flatFields[attribute]
		_, blockOnly := blockOnlyComponentIdentifiers[identifier]

		switch {
		case hasFlatField && blockOnly:
			t.Errorf("%q has the BlueprintResourceModel field %q, so it must not be in blockOnlyComponentIdentifiers — it would land in raw_component as well as its flat attribute", identifier, attribute)
		case !hasFlatField && !blockOnly:
			t.Errorf("%q maps to component_blocks attribute %q, which BlueprintResourceModel has no field for, so a flat-mode read has nowhere to put it — register it in blockOnlyComponentIdentifiers or a flat-mode read drops it and the next apply deletes it", identifier, attribute)
		}
	}

	for identifier := range blockOnlyComponentIdentifiers {
		if _, typed := stronglyTypedComponentIdentifiers[identifier]; !typed {
			t.Errorf("blockOnlyComponentIdentifiers entry %q is not strongly typed, so it already stays in raw_component and the entry is inert", identifier)
		}
	}
}
