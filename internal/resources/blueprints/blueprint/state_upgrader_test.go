// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

func TestSliceToPointer_Empty(t *testing.T) {
	var s []string
	result := sliceToPointer(s)
	if result != nil {
		t.Error("expected nil for empty slice")
	}
}

func TestSliceToPointer_Nil(t *testing.T) {
	result := sliceToPointer[int](nil)
	if result != nil {
		t.Error("expected nil for nil slice")
	}
}

func TestSliceToPointer_SingleElement(t *testing.T) {
	s := []string{"hello"}
	result := sliceToPointer(s)
	if result == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *result != "hello" {
		t.Errorf("expected 'hello', got %q", *result)
	}
}

func TestSliceToPointer_MultipleElements(t *testing.T) {
	s := []int{1, 2, 3}
	result := sliceToPointer(s)
	if result == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *result != 1 {
		t.Errorf("expected 1, got %d", *result)
	}
}

func TestBlueprintSchemaV0_HasComponentBlocks(t *testing.T) {
	s := blueprintSchemaV0()
	if s == nil {
		t.Fatal("v0 schema is nil")
	}

	expectedBlocks := []string{
		"raw_component",
		"audio_accessory_settings",
		"custom_declarations",
		"disk_management_settings",
		"math_settings",
		"passcode_policy",
		"safari_bookmarks",
		"safari_extensions",
		"safari_settings",
		"service_background_tasks",
		"service_configuration_files",
		"software_update",
		"software_update_settings",
	}

	for _, name := range expectedBlocks {
		if _, ok := s.Blocks[name]; !ok {
			t.Errorf("v0 schema missing block %q", name)
		}
	}
}

func TestBlueprintSchemaV0_BlocksAreListNested(t *testing.T) {
	s := blueprintSchemaV0()

	for name, block := range s.Blocks {
		if _, ok := block.(schema.ListNestedBlock); !ok {
			t.Errorf("block %q should be ListNestedBlock, got %T", name, block)
		}
	}
}

func TestBlueprintSchemaV0_ScalarAttributes(t *testing.T) {
	s := blueprintSchemaV0()

	expectedAttrs := []string{"id", "name", "description", "deployed", "device_groups", "legacy_payloads", "created", "updated", "deployment_state", "timeouts"}
	for _, name := range expectedAttrs {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("v0 schema missing attribute %q", name)
		}
	}
}

func TestBlueprintSchemaV0_ComponentsNotInAttributes(t *testing.T) {
	s := blueprintSchemaV0()

	componentNames := []string{
		"audio_accessory_settings",
		"safari_settings",
		"software_update",
		"raw_component",
	}

	for _, name := range componentNames {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("v0 schema should NOT have %q as an attribute (it should be a block)", name)
		}
	}
}

func TestBlueprintSchemaV0_SafariBookmarksNestedBlocks(t *testing.T) {
	s := blueprintSchemaV0()

	block, ok := s.Blocks["safari_bookmarks"]
	if !ok {
		t.Fatal("missing safari_bookmarks block")
	}

	listBlock := block.(schema.ListNestedBlock)
	managedBookmarks, ok := listBlock.NestedObject.Blocks["managed_bookmarks"]
	if !ok {
		t.Fatal("missing managed_bookmarks inner block")
	}

	mbListBlock := managedBookmarks.(schema.ListNestedBlock)
	bookmarks, ok := mbListBlock.NestedObject.Blocks["bookmarks"]
	if !ok {
		t.Fatal("missing bookmarks inner block")
	}

	bListBlock := bookmarks.(schema.ListNestedBlock)
	folder, ok := bListBlock.NestedObject.Blocks["folder"]
	if !ok {
		t.Fatal("missing folder inner block")
	}

	folderListBlock := folder.(schema.ListNestedBlock)
	if _, ok := folderListBlock.NestedObject.Attributes["title"]; !ok {
		t.Error("folder block missing 'title' attribute")
	}
	if _, ok := folderListBlock.NestedObject.Attributes["url"]; !ok {
		t.Error("folder block missing 'url' attribute")
	}
}

func TestBlueprintSchemaV0_ServiceBackgroundTasksNestedBlocks(t *testing.T) {
	s := blueprintSchemaV0()

	block := s.Blocks["service_background_tasks"].(schema.ListNestedBlock)
	bgTasks := block.NestedObject.Blocks["background_tasks"].(schema.ListNestedBlock)

	if _, ok := bgTasks.NestedObject.Attributes["task_type"]; !ok {
		t.Error("background_tasks missing 'task_type' attribute")
	}

	execRef, ok := bgTasks.NestedObject.Blocks["executable_asset_reference"]
	if !ok {
		t.Fatal("missing executable_asset_reference block")
	}
	if _, ok := execRef.(schema.SingleNestedBlock); !ok {
		t.Errorf("executable_asset_reference should be SingleNestedBlock, got %T", execRef)
	}

	launchdConfigs, ok := bgTasks.NestedObject.Blocks["launchd_configurations"]
	if !ok {
		t.Fatal("missing launchd_configurations block")
	}
	launchdList := launchdConfigs.(schema.ListNestedBlock)
	fileRef, ok := launchdList.NestedObject.Blocks["file_asset_reference"]
	if !ok {
		t.Fatal("missing file_asset_reference block")
	}
	if _, ok := fileRef.(schema.SingleNestedBlock); !ok {
		t.Errorf("file_asset_reference should be SingleNestedBlock, got %T", fileRef)
	}
}

func TestBlueprintSchemaV1_LegacyPayloadsIsString(t *testing.T) {
	s := blueprintSchemaV1()
	if s == nil {
		t.Fatal("v1 schema is nil")
	}

	attr, ok := s.Attributes["legacy_payloads"]
	if !ok {
		t.Fatal("v1 schema missing legacy_payloads attribute")
	}
	if _, ok := attr.(schema.StringAttribute); !ok {
		t.Errorf("v1 legacy_payloads should be StringAttribute, got %T", attr)
	}
}

func TestBlueprintSchemaV1_ComponentsAreAttributes(t *testing.T) {
	s := blueprintSchemaV1()

	componentAttrs := []string{
		"audio_accessory_settings",
		"custom_declarations",
		"disk_management_settings",
		"math_settings",
		"passcode_policy",
		"safari_bookmarks",
		"safari_extensions",
		"safari_settings",
		"service_background_tasks",
		"service_configuration_files",
		"software_update",
		"software_update_settings",
		"raw_component",
	}

	for _, name := range componentAttrs {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("v1 schema missing attribute %q", name)
		}
	}
}

func TestUpgradeLegacyPayloadsFromString_ValidJSON(t *testing.T) {
	input := types.StringValue(`[{"payloadType":"com.apple.applicationaccess","payloadIdentifier":"test-uuid","allowSafariHistoryClearing":false}]`)

	result := upgradeLegacyPayloadsFromString(input)

	if result.IsNull() {
		t.Fatal("expected non-null result")
	}

	raw, err := helpers.TerraformDynamicToJSON(result)
	if err != nil {
		t.Fatalf("failed to convert: %v", err)
	}
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected list, got %T", raw)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(items))
	}
	payload := items[0].(map[string]any)
	if payload["payload_type"] != "com.apple.applicationaccess" {
		t.Errorf("expected payload_type 'com.apple.applicationaccess', got %v", payload["payload_type"])
	}
	if _, ok := payload["settings"]; !ok {
		t.Error("expected settings key")
	}
}

func TestUpgradeLegacyPayloadsFromString_NullString(t *testing.T) {
	result := upgradeLegacyPayloadsFromString(types.StringNull())
	if !result.IsNull() {
		t.Error("expected null for null string")
	}
}

func TestUpgradeLegacyPayloadsFromString_EmptyArray(t *testing.T) {
	result := upgradeLegacyPayloadsFromString(types.StringValue("[]"))
	if !result.IsNull() {
		t.Error("expected null for empty array")
	}
}

func TestUpgradeLegacyPayloadsFromString_InvalidJSON(t *testing.T) {
	result := upgradeLegacyPayloadsFromString(types.StringValue("not-json"))
	if !result.IsNull() {
		t.Error("expected null for invalid JSON")
	}
}

func TestUpgradeLegacyPayloadsFromString_NoSettings(t *testing.T) {
	input := types.StringValue(`[{"payloadType":"com.apple.wifi.managed","payloadIdentifier":"uuid"}]`)

	result := upgradeLegacyPayloadsFromString(input)

	if result.IsNull() {
		t.Fatal("expected non-null result")
	}

	raw, err := helpers.TerraformDynamicToJSON(result)
	if err != nil {
		t.Fatalf("failed to convert: %v", err)
	}
	items := raw.([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(items))
	}
	payload := items[0].(map[string]any)
	if _, ok := payload["settings"]; ok {
		t.Error("expected no settings key when no extra keys")
	}
}

// TestBlueprintResourceModelV3Upgrade covers the v3 to v4 upgrade: apple_declarations stops being
// an object wrapping a `declaration` list and becomes the list.
//
// Without this upgrader, state written by v0.33.0 cannot be decoded against the new schema at all,
// so `terraform plan` fails before it reaches the configuration. It is unit-tested because an
// acceptance test cannot write state from an older provider build.
func TestBlueprintResourceModelV3Upgrade(t *testing.T) {
	declaration := AppleDeclarationModel{
		ChannelType: types.StringValue("SYSTEM"),
		Payload:     types.StringValue(`{"Enabled":true}`),
		Type:        types.StringValue("com.apple.configuration.siri.settings"),
	}

	prior := blueprintResourceModelV3{
		ID:   types.StringValue("6fdc7ca2-b1cb-4053-b0c1-b2316aabf1dc"),
		Name: types.StringValue("Baseline"),
		ComponentBlocks: []componentBlockModelV3{
			{
				Name:              types.StringValue("Declarations"),
				AppleDeclarations: &appleDeclarationsComponentV3{Declarations: []AppleDeclarationModel{declaration}},
				LegacyPayloads: []BlockLegacyPayloadModel{
					{PayloadType: types.StringValue("com.apple.dock"), Settings: types.StringValue(`{"tilesize":48}`)},
				},
			},
			{
				Name:           types.StringValue("Passcode only"),
				PasscodePolicy: &components.PasscodePolicyComponent{},
			},
			{
				Name:              types.StringValue("Empty declarations"),
				AppleDeclarations: &appleDeclarationsComponentV3{},
			},
		},
	}

	upgraded := prior.upgrade()

	if upgraded.ID != prior.ID || upgraded.Name != prior.Name {
		t.Errorf("scalars did not carry across: %+v", upgraded)
	}
	if len(upgraded.ComponentBlocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(upgraded.ComponentBlocks))
	}

	first := upgraded.ComponentBlocks[0]
	if len(first.AppleDeclarations) != 1 {
		t.Fatalf("expected the declaration list to be unwrapped, got %+v", first.AppleDeclarations)
	}
	if first.AppleDeclarations[0] != declaration {
		t.Errorf("declaration changed during the upgrade: %+v", first.AppleDeclarations[0])
	}
	// Everything else in the block has to survive, or the upgrade silently deletes components from
	// the blueprint on the next apply.
	if len(first.LegacyPayloads) != 1 {
		t.Errorf("legacy payloads were dropped: %+v", first.LegacyPayloads)
	}
	if upgraded.ComponentBlocks[1].PasscodePolicy == nil {
		t.Error("a block with no declarations lost its other components")
	}
	if upgraded.ComponentBlocks[1].AppleDeclarations != nil {
		t.Errorf("an absent component became a list: %+v", upgraded.ComponentBlocks[1].AppleDeclarations)
	}
	// An empty list writes no component, so state must not hold one either.
	if upgraded.ComponentBlocks[2].AppleDeclarations != nil {
		t.Errorf("an empty declaration list must upgrade to absent, got %+v", upgraded.ComponentBlocks[2].AppleDeclarations)
	}
}

// TestBlueprintSchemaV3MatchesTheStateItDecodes pins the two things the derived prior schema has to
// get right: apple_declarations in its old object shape, and the version it claims.
func TestBlueprintSchemaV3MatchesTheStateItDecodes(t *testing.T) {
	prior := blueprintSchemaV3(context.Background())

	if prior.Version != 3 {
		t.Errorf("Version = %d, want 3", prior.Version)
	}

	blocks, ok := prior.Attributes["component_blocks"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("component_blocks is missing or not a ListNestedAttribute")
	}
	declarations, ok := blocks.NestedObject.Attributes["apple_declarations"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("apple_declarations must be a SingleNestedAttribute in the v3 schema")
	}
	if _, ok := declarations.Attributes["declaration"].(schema.ListNestedAttribute); !ok {
		t.Error("apple_declarations.declaration must be a ListNestedAttribute in the v3 schema")
	}

	// Deriving the prior schema must not leave the live one mutated, or every later plan would see
	// the old shape.
	var resp resource.SchemaResponse
	NewBlueprintResource().(*BlueprintResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	current, ok := resp.Schema.Attributes["component_blocks"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("current component_blocks is missing or not a ListNestedAttribute")
	}
	if _, ok := current.NestedObject.Attributes["apple_declarations"].(schema.ListNestedAttribute); !ok {
		t.Error("the live apple_declarations is no longer a ListNestedAttribute")
	}
	if resp.Schema.Version != 6 {
		t.Errorf("current Version = %d, want 6", resp.Schema.Version)
	}
}

// blueprintStateV3 is a verbatim schema-version-3 state document, frozen. It records what schema
// version 3 actually wrote — most importantly a component block carrying apple_declarations in its
// old single-object-wrapping-a-list shape — and must never be regenerated from the live schema:
// regenerating it would make it agree with whatever the schema has since become, which is the one
// thing TestBlueprintSchemaV3DecodesFrozenV3State exists to detect. It populates a first block's
// apple_declarations, legacy_payloads and passcode_policy, leaves a second block's components
// absent, and carries every remaining attribute as an explicit JSON null, the way Terraform writes
// state.
const blueprintStateV3 = `{
  "id": "3f6c1d8e-4b2a-4c77-9e51-0a2b6d9f1c34",
  "name": "Frozen v3 blueprint",
  "description": "A blueprint written by schema version 3.",
  "deployed": true,
  "device_groups": ["3a1f0c72-5d88-4e19-b6a2-9c7e4f10d5bb"],
  "activation_conditions": "@status(os.version) >= \"15.0\"",
  "created": "2026-01-14T09:12:33Z",
  "updated": "2026-02-02T17:45:01Z",
  "deployment_state": "SUCCEEDED",
  "timeouts": {"create": "10m", "read": null, "update": "10m", "delete": null},
  "legacy_payloads": null,
  "raw_component": null,
  "audio_accessory_settings": null,
  "custom_declarations": null,
  "disk_management_settings": null,
  "math_settings": null,
  "passcode_policy": null,
  "safari_bookmarks": null,
  "safari_extensions": null,
  "safari_settings": null,
  "service_background_tasks": null,
  "service_configuration_files": null,
  "software_update": null,
  "software_update_settings": null,
  "component_blocks": [
    {
      "name": "Baseline",
      "activation_conditions": "@status(os.version) >= \"15.0\"",
      "apple_declarations": {
        "declaration": [
          {
            "channel": "DEVICE",
            "payload": "{\"Enabled\":true}",
            "type": "com.apple.configuration.softwareupdate.enforcement.specific"
          },
          {
            "channel": "USER",
            "payload": "{\"PayloadContentIdentifiers\":[]}",
            "type": "com.apple.configuration.services.configuration-files"
          }
        ]
      },
      "legacy_payloads": [
        {
          "payload_type": "com.apple.applicationaccess",
          "settings": "{\"allowCamera\":false}"
        }
      ],
      "passcode_policy": {
        "change_at_next_auth": null,
        "custom_regex_description": null,
        "custom_regex_pattern": null,
        "failed_attempts_reset_in_minutes": null,
        "maximum_failed_attempts": 6,
        "maximum_grace_period_in_minutes": null,
        "maximum_inactivity_in_minutes": null,
        "maximum_passcode_age_in_days": null,
        "minimum_complex_characters": null,
        "minimum_length": 8,
        "passcode_reuse_limit": null,
        "require_alphanumeric_passcode": false,
        "require_complex_passcode": null,
        "require_passcode": true
      },
      "ai_governance": null,
      "raw_component": null,
      "audio_accessory_settings": null,
      "custom_declarations": null,
      "disk_management_settings": null,
      "math_settings": null,
      "safari_bookmarks": null,
      "safari_extensions": null,
      "safari_settings": null,
      "service_background_tasks": null,
      "service_configuration_files": null,
      "software_update": null,
      "software_update_settings": null
    },
    {
      "name": "Empty",
      "activation_conditions": null,
      "apple_declarations": null,
      "legacy_payloads": null,
      "passcode_policy": null,
      "ai_governance": null,
      "raw_component": null,
      "audio_accessory_settings": null,
      "custom_declarations": null,
      "disk_management_settings": null,
      "math_settings": null,
      "safari_bookmarks": null,
      "safari_extensions": null,
      "safari_settings": null,
      "service_background_tasks": null,
      "service_configuration_files": null,
      "software_update": null,
      "software_update_settings": null
    }
  ]
}`

// TestBlueprintSchemaV3DecodesFrozenV3State proves the derived prior schema can still read real
// state. blueprintSchemaV3 builds itself from the live attribute set, so an attribute whose type
// changes — exactly what this change did to apple_declarations — silently reshapes the v3 schema
// out from under state a user has on disk, and a shape assertion against the derived schema cannot
// see it. Running a frozen document through the whole 3 → 4 upgrade can: decoding it with the same
// options the framework uses, then reading it into the frozen blueprintResourceModelV3, so the next
// reshape of any component_blocks attribute fails here instead of inside a user's upgrade.
func TestBlueprintSchemaV3DecodesFrozenV3State(t *testing.T) {
	ctx := context.Background()
	r := NewBlueprintResource().(*BlueprintResource)

	upgrader, ok := r.UpgradeState(ctx)[3]
	if !ok {
		t.Fatal("no state upgrader is registered for schema version 3")
	}

	priorSchema := upgrader.PriorSchema
	if priorSchema == nil {
		t.Fatal("the version 3 upgrader declares no prior schema")
	}

	raw := tfprotov6.RawState{JSON: []byte(blueprintStateV3)}
	priorValue, err := raw.UnmarshalWithOpts(priorSchema.Type().TerraformType(ctx), tfprotov6.UnmarshalOpts{
		ValueFromJSONOpts: tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true},
	})
	if err != nil {
		t.Fatalf("the derived v3 schema no longer decodes real v3 state: %v", err)
	}

	var currentSchema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &currentSchema)

	req := resource.UpgradeStateRequest{
		State:    &tfsdk.State{Raw: priorValue, Schema: *priorSchema},
		RawState: &raw,
	}
	resp := resource.UpgradeStateResponse{
		State: tfsdk.State{
			Raw:    tftypes.NewValue(currentSchema.Schema.Type().TerraformType(ctx), nil),
			Schema: currentSchema.Schema,
		},
	}
	upgrader.StateUpgrader(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrading frozen v3 state failed: %v", resp.Diagnostics.Errors())
	}

	var upgraded BlueprintResourceModel
	if diags := resp.State.Get(ctx, &upgraded); diags.HasError() {
		t.Fatalf("reading the upgraded state failed: %v", diags.Errors())
	}

	if len(upgraded.ComponentBlocks) != 2 {
		t.Fatalf("ComponentBlocks length = %d, want 2", len(upgraded.ComponentBlocks))
	}
	if got := len(upgraded.ComponentBlocks[0].AppleDeclarations); got != 2 {
		t.Errorf("the first block carries %d declarations, want 2", got)
	}
	if upgraded.ComponentBlocks[1].AppleDeclarations != nil {
		t.Errorf("the second block gained declarations: %+v", upgraded.ComponentBlocks[1].AppleDeclarations)
	}
	if upgraded.ComponentBlocks[0].PasscodePolicy == nil {
		t.Error("the first block lost its passcode_policy")
	}
	if got := len(upgraded.ComponentBlocks[0].LegacyPayloads); got != 1 {
		t.Errorf("the first block carries %d legacy payloads, want 1", got)
	}
}

// TestBlueprintResourceModelV4Upgrade_TopLevelAttributesBecomeOneBlock covers the 4 to 5 upgrade. v4
// held the first component block in the top-level component attributes, which v5 removed, so the
// upgrade moves them into component_blocks[0]. Dropping them instead would leave the platform
// holding components that state no longer lists, and the next apply would delete them without a
// plan diff.
func TestBlueprintResourceModelV4Upgrade_TopLevelAttributesBecomeOneBlock(t *testing.T) {
	t.Parallel()

	prior := blueprintResourceModelV4{
		ID:                   types.StringValue("6fdc7ca2-b1cb-4053-b0c1-b2316aabf1dc"),
		Name:                 types.StringValue("Baseline"),
		Description:          types.StringValue("kept"),
		Deployed:             types.BoolValue(true),
		DeviceGroups:         types.SetNull(types.StringType),
		ActivationConditions: types.StringValue("ANY @property(jamf.device.osVersion) >= '15.0'"),
		Components: []ComponentModel{
			{Identifier: types.StringValue("com.jamf.custom.thing"), Configuration: types.MapNull(types.StringType)},
		},
		PasscodePolicy:         &components.PasscodePolicyComponent{},
		SoftwareUpdateSettings: &components.SoftwareUpdateSettingsComponent{},
		LegacyPayloads:         flatLegacyPayloadsValue(t),
		DeploymentState:        types.StringValue("SUCCEEDED"),
	}

	upgraded, diags := prior.upgrade()
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if upgraded.ID != prior.ID || upgraded.Name != prior.Name || upgraded.Description != prior.Description || upgraded.DeploymentState != prior.DeploymentState {
		t.Errorf("scalars did not carry across: %+v", upgraded)
	}
	if len(upgraded.ComponentBlocks) != 1 {
		t.Fatalf("expected the top-level attributes to become 1 block, got %d", len(upgraded.ComponentBlocks))
	}

	block := upgraded.ComponentBlocks[0]
	if got := block.Name.ValueString(); got != flatStepNameV4 {
		t.Errorf("block name = %q, want %q, the step name v4 stored, so the first refresh finds nothing to change", got, flatStepNameV4)
	}
	if got := block.ActivationConditions.ValueString(); got != "ANY @property(jamf.device.osVersion) >= '15.0'" {
		t.Errorf("the top-level activation condition did not move to the block: %q", got)
	}
	if len(block.Components) != 1 || block.Components[0].Identifier.ValueString() != "com.jamf.custom.thing" {
		t.Errorf("raw_component was dropped: %+v", block.Components)
	}
	if block.PasscodePolicy == nil || block.SoftwareUpdateSettings == nil {
		t.Errorf("typed components were dropped: %+v", block)
	}
	if len(block.LegacyPayloads) != 1 {
		t.Fatalf("legacy payloads were dropped: %+v", block.LegacyPayloads)
	}
	if got := block.LegacyPayloads[0].PayloadType.ValueString(); got != "com.apple.dock" {
		t.Errorf("payload_type = %q", got)
	}
	if got := block.LegacyPayloads[0].Settings.ValueString(); got != `{"tilesize":32}` {
		t.Errorf("settings must become the JSON string a block carries, got %q", got)
	}
}

// TestBlueprintResourceModelV4Upgrade_BlocksWinAndEmptyStaysEmpty covers the two states that must
// not gain a block: one already in component_blocks, whose top-level attributes v4 cleared, and one
// that never managed a component.
func TestBlueprintResourceModelV4Upgrade_BlocksWinAndEmptyStaysEmpty(t *testing.T) {
	t.Parallel()

	t.Run("state that already uses component_blocks keeps exactly those blocks", func(t *testing.T) {
		t.Parallel()

		prior := blueprintResourceModelV4{
			Name:                 types.StringValue("Blocks"),
			ActivationConditions: types.StringValue("left over"),
			ComponentBlocks: []ComponentBlockModel{
				{Name: types.StringValue("one"), PasscodePolicy: &components.PasscodePolicyComponent{}},
				{Name: types.StringValue("two")},
			},
		}

		upgraded, diags := prior.upgrade()
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if len(upgraded.ComponentBlocks) != 2 || upgraded.ComponentBlocks[0].Name.ValueString() != "one" || upgraded.ComponentBlocks[1].Name.ValueString() != "two" {
			t.Errorf("blocks changed: %+v", upgraded.ComponentBlocks)
		}
	})

	t.Run("state that managed no component gains no block", func(t *testing.T) {
		t.Parallel()

		prior := blueprintResourceModelV4{
			Name:           types.StringValue("Empty"),
			LegacyPayloads: types.DynamicNull(),
		}

		upgraded, diags := prior.upgrade()
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if upgraded.ComponentBlocks != nil {
			t.Errorf("an empty blueprint gained a block: %+v", upgraded.ComponentBlocks)
		}
	})
}

// TestBlockLegacyPayloadsFromDynamic covers the one conversion the upgrade performs rather than
// carries across: the removed dynamic legacy_payloads value, whose settings were objects, becomes
// the list a block holds, whose settings are JSON strings.
func TestBlockLegacyPayloadsFromDynamic(t *testing.T) {
	t.Parallel()

	t.Run("settings keep every value and come out with sorted keys", func(t *testing.T) {
		t.Parallel()

		value, err := helpers.JSONToTerraformDynamic([]any{
			map[string]any{
				"payload_type": "com.apple.applicationaccess",
				"settings":     map[string]any{"b": true, "a": "text", "c": []any{"x", "y"}, "d": float64(42)},
			},
			map[string]any{"payload_type": "com.apple.wifi.managed"},
		})
		if err != nil {
			t.Fatalf("building the dynamic value: %v", err)
		}

		got, err := blockLegacyPayloadsFromDynamic(value)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 payloads, got %+v", got)
		}
		if got[0].Settings.ValueString() != `{"a":"text","b":true,"c":["x","y"],"d":42}` {
			t.Errorf("settings = %q", got[0].Settings.ValueString())
		}
		if !got[1].Settings.IsNull() || got[1].PayloadType.ValueString() != "com.apple.wifi.managed" {
			t.Errorf("a payload without settings must keep null settings, got %+v", got[1])
		}

		var roundTripped map[string]any
		if err := json.Unmarshal([]byte(got[0].Settings.ValueString()), &roundTripped); err != nil {
			t.Fatalf("settings are not valid JSON: %v", err)
		}
	})

	t.Run("null and unknown values yield nothing", func(t *testing.T) {
		t.Parallel()

		for _, value := range []types.Dynamic{types.DynamicNull(), types.DynamicUnknown()} {
			got, err := blockLegacyPayloadsFromDynamic(value)
			if err != nil || got != nil {
				t.Errorf("got %+v, %v for %s", got, err, value)
			}
		}
	})

	t.Run("a value that is not a list of objects is an error", func(t *testing.T) {
		t.Parallel()

		if _, err := blockLegacyPayloadsFromDynamic(types.DynamicValue(types.StringValue("x"))); err == nil {
			t.Error("a string was accepted as a payload list")
		}
	})
}

// TestBlueprintSchemaV4HasTheRemovedAttributes pins what the derived prior schema has to carry: the
// top-level attributes v5 removed, at the version it claims, without mutating the live schema.
func TestBlueprintSchemaV4HasTheRemovedAttributes(t *testing.T) {
	prior := blueprintSchemaV4(context.Background())

	if prior.Version != 4 {
		t.Errorf("Version = %d, want 4", prior.Version)
	}
	for _, name := range []string{"activation_conditions", "legacy_payloads", "raw_component", "passcode_policy", "software_update_settings", "component_blocks"} {
		if _, ok := prior.Attributes[name]; !ok {
			t.Errorf("the v4 schema is missing %q", name)
		}
	}

	var resp resource.SchemaResponse
	NewBlueprintResource().(*BlueprintResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if _, ok := resp.Schema.Attributes["passcode_policy"]; ok {
		t.Error("deriving the v4 schema leaked a removed attribute into the live schema")
	}
}

// blueprintStateV4Flat is a verbatim schema-version-4 state document, frozen, for a blueprint
// authored with the top-level component attributes. Like blueprintStateV3 it must never be
// regenerated from the live schema, because it exists to catch the live schema drifting away from
// what version 4 actually wrote. legacy_payloads uses the encoding Terraform gives a dynamic value:
// its type beside its value.
const blueprintStateV4Flat = `{
  "id": "5c0e2c1a-8d3b-4f61-a7d2-6b9e1f4a0c55",
  "name": "Frozen v4 flat blueprint",
  "description": "A blueprint written by schema version 4.",
  "deployed": true,
  "device_groups": ["3a1f0c72-5d88-4e19-b6a2-9c7e4f10d5bb"],
  "activation_conditions": "@status(os.version) >= \"15.0\"",
  "created": "2026-03-14T09:12:33Z",
  "updated": "2026-03-15T17:45:01Z",
  "deployment_state": "SUCCEEDED",
  "timeouts": {"create": null, "read": null, "update": null, "delete": null},
  "component_blocks": null,
  "legacy_payloads": {
    "type": ["tuple", [["object", {"payload_type": "string", "settings": ["object", {"tilesize": "number"}]}]]],
    "value": [{"payload_type": "com.apple.dock", "settings": {"tilesize": 48}}]
  },
  "raw_component": [{"identifier": "com.jamf.custom.thing", "configuration": {"a": "1"}}],
  "audio_accessory_settings": null,
  "custom_declarations": null,
  "disk_management_settings": null,
  "math_settings": null,
  "passcode_policy": {
    "change_at_next_auth": null,
    "custom_regex_description": null,
    "custom_regex_pattern": null,
    "failed_attempts_reset_in_minutes": null,
    "maximum_failed_attempts": 6,
    "maximum_grace_period_in_minutes": null,
    "maximum_inactivity_in_minutes": null,
    "maximum_passcode_age_in_days": null,
    "minimum_complex_characters": null,
    "minimum_length": 8,
    "passcode_reuse_limit": null,
    "require_alphanumeric_passcode": false,
    "require_complex_passcode": null,
    "require_passcode": true
  },
  "safari_bookmarks": null,
  "safari_extensions": null,
  "safari_settings": null,
  "service_background_tasks": null,
  "service_configuration_files": null,
  "software_update": null,
  "software_update_settings": null
}`

// TestBlueprintSchemaV4DecodesFrozenV4State runs a frozen v4 flat-mode document through the whole 4
// to 5 upgrade with the same decoding options the framework uses. The derived prior schema builds
// itself from the live attribute set, so a change to any component schema reshapes the v4 schema
// under state a user has on disk, and only a real document can see that.
func TestBlueprintSchemaV4DecodesFrozenV4State(t *testing.T) {
	ctx := context.Background()
	r := NewBlueprintResource().(*BlueprintResource)

	upgrader, ok := r.UpgradeState(ctx)[4]
	if !ok {
		t.Fatal("no state upgrader is registered for schema version 4")
	}
	priorSchema := upgrader.PriorSchema
	if priorSchema == nil {
		t.Fatal("the version 4 upgrader declares no prior schema")
	}

	raw := tfprotov6.RawState{JSON: []byte(blueprintStateV4Flat)}
	priorValue, err := raw.UnmarshalWithOpts(priorSchema.Type().TerraformType(ctx), tfprotov6.UnmarshalOpts{
		ValueFromJSONOpts: tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true},
	})
	if err != nil {
		t.Fatalf("the derived v4 schema no longer decodes real v4 state: %v", err)
	}

	var currentSchema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &currentSchema)

	req := resource.UpgradeStateRequest{
		State:    &tfsdk.State{Raw: priorValue, Schema: *priorSchema},
		RawState: &raw,
	}
	resp := resource.UpgradeStateResponse{
		State: tfsdk.State{
			Raw:    tftypes.NewValue(currentSchema.Schema.Type().TerraformType(ctx), nil),
			Schema: currentSchema.Schema,
		},
	}
	upgrader.StateUpgrader(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrading frozen v4 state failed: %v", resp.Diagnostics.Errors())
	}

	var upgraded BlueprintResourceModel
	if diags := resp.State.Get(ctx, &upgraded); diags.HasError() {
		t.Fatalf("reading the upgraded state failed: %v", diags.Errors())
	}

	if upgraded.Name.ValueString() != "Frozen v4 flat blueprint" || upgraded.DeploymentState.ValueString() != "SUCCEEDED" {
		t.Errorf("scalars did not carry across: %+v", upgraded)
	}
	if len(upgraded.ComponentBlocks) != 1 {
		t.Fatalf("ComponentBlocks length = %d, want 1", len(upgraded.ComponentBlocks))
	}

	block := upgraded.ComponentBlocks[0]
	if block.Name.ValueString() != flatStepNameV4 {
		t.Errorf("block name = %q", block.Name.ValueString())
	}
	if block.ActivationConditions.ValueString() != `@status(os.version) >= "15.0"` {
		t.Errorf("activation condition = %q", block.ActivationConditions.ValueString())
	}
	if block.PasscodePolicy == nil {
		t.Error("the block lost the passcode_policy")
	}
	if len(block.Components) != 1 || block.Components[0].Identifier.ValueString() != "com.jamf.custom.thing" {
		t.Errorf("the block lost its raw_component: %+v", block.Components)
	}
	if len(block.LegacyPayloads) != 1 || block.LegacyPayloads[0].Settings.ValueString() != `{"tilesize":48}` {
		t.Errorf("the block lost its legacy payload: %+v", block.LegacyPayloads)
	}
}

// TestUpgradeStateRegistersEveryPriorVersion fails when a schema version has no upgrader, which
// leaves state written at that version undecodable and breaks the plan before it reaches the
// configuration.
func TestUpgradeStateRegistersEveryPriorVersion(t *testing.T) {
	ctx := context.Background()
	r := NewBlueprintResource().(*BlueprintResource)

	var current resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &current)

	upgraders := r.UpgradeState(ctx)
	for version := range current.Schema.Version {
		upgrader, ok := upgraders[version]
		if !ok {
			t.Errorf("no state upgrader for schema version %d", version)
			continue
		}
		if upgrader.PriorSchema == nil {
			t.Errorf("the version %d upgrader declares no prior schema", version)
		}
	}
}
