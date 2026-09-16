// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestCollectLegacyPayloads_ValidPayload(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.applicationaccess",
			"settings": map[string]any{
				"allowSafariHistoryClearing": false,
				"allowSafariPrivateBrowsing": false,
			},
		},
	}
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "My Blueprint", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}
	if components[0].Identifier != "com.jamf.ddm-configuration-profile" {
		t.Errorf("expected identifier 'com.jamf.ddm-configuration-profile', got %q", components[0].Identifier)
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if config["payloadDisplayName"] != "My Blueprint" {
		t.Errorf("expected payloadDisplayName 'My Blueprint', got %v", config["payloadDisplayName"])
	}

	payloadContent, ok := config["payloadContent"].([]any)
	if !ok {
		t.Fatal("expected payloadContent to be an array")
	}
	if len(payloadContent) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloadContent))
	}

	payload := payloadContent[0].(map[string]any)
	if payload["payloadType"] != "com.apple.applicationaccess" {
		t.Errorf("expected payloadType 'com.apple.applicationaccess', got %v", payload["payloadType"])
	}
	if payload["allowSafariHistoryClearing"] != false {
		t.Errorf("expected allowSafariHistoryClearing false, got %v", payload["allowSafariHistoryClearing"])
	}
}

func TestCollectLegacyPayloads_NoSettings(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
		},
	}
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "Blueprint", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	payloadContent := config["payloadContent"].([]any)
	payload := payloadContent[0].(map[string]any)
	if payload["payloadType"] != "com.apple.wifi.managed" {
		t.Errorf("expected payloadType 'com.apple.wifi.managed', got %v", payload["payloadType"])
	}
}

func TestCollectLegacyPayloads_MixedTypeSettings(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.applicationaccess",
			"settings": map[string]any{
				"boolSetting":   true,
				"stringSetting": "hello",
				"numberSetting": float64(42),
			},
		},
	}
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "Test", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal configuration: %v", err)
	}
	payloadContent := config["payloadContent"].([]any)
	payload := payloadContent[0].(map[string]any)

	if payload["boolSetting"] != true {
		t.Errorf("expected boolSetting true, got %v", payload["boolSetting"])
	}
	if payload["stringSetting"] != "hello" {
		t.Errorf("expected stringSetting 'hello', got %v", payload["stringSetting"])
	}
	if payload["numberSetting"] != float64(42) {
		t.Errorf("expected numberSetting 42, got %v", payload["numberSetting"])
	}
}

func TestCollectLegacyPayloads_EmptyList(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	dynVal, _ := helpers.JSONToTerraformDynamic([]any{})

	r.collectLegacyPayloads(&components, &diags, dynVal, "Empty Blueprint", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal configuration: %v", err)
	}
	payloadContent, ok := config["payloadContent"].([]any)
	if !ok {
		t.Fatal("expected payloadContent to be an array")
	}
	if len(payloadContent) != 0 {
		t.Errorf("expected empty payload content, got %d items", len(payloadContent))
	}
}

func TestCollectLegacyPayloads_DuplicatePayloadType(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
			"settings": map[string]any{
				"networkName": "Office",
			},
		},
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
			"settings": map[string]any{
				"networkName": "Guest",
			},
		},
	}
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "Blueprint", nil)

	if !diags.HasError() {
		t.Fatal("expected error for duplicate payload_type, got none")
	}

	found := false
	for _, d := range diags.Errors() {
		if d.Summary() == "Duplicate payload_type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Duplicate payload_type' error, got: %v", diags)
	}
}

func TestCollectLegacyPayloads_NullDynamic(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	r.collectLegacyPayloads(&components, &diags, types.DynamicNull(), "Blueprint", nil)

	if !diags.HasError() {
		t.Error("expected error for null dynamic value")
	}
}
