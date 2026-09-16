// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

// --- Schema ---

func TestBlueprintResource_SchemaComponentBlocks(t *testing.T) {
	r := NewBlueprintResource()
	var resp resource.SchemaResponse
	r.(*BlueprintResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["component_blocks"]
	if !ok {
		t.Fatal("missing component_blocks attribute")
	}
	if !attr.IsOptional() {
		t.Error("component_blocks should be optional")
	}

	list, ok := attr.(resourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("component_blocks should be a ListNestedAttribute, got %T", attr)
	}

	nested := list.NestedObject.Attributes
	for _, name := range []string{"name", "activation_conditions", "raw_component", "passcode_policy", "software_update_settings", "legacy_payloads"} {
		if _, ok := nested[name]; !ok {
			t.Errorf("component_blocks block missing attribute %q", name)
		}
	}

	if nested["passcode_policy"].GetDeprecationMessage() != "" {
		t.Error("component attributes inside component_blocks must not be deprecated")
	}
}

func TestBlueprintResource_FlatComponentAttrsDeprecated(t *testing.T) {
	r := NewBlueprintResource()
	var resp resource.SchemaResponse
	r.(*BlueprintResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	deprecated := []string{
		"activation_conditions",
		"legacy_payloads",
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
	for _, name := range deprecated {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("missing flat attribute %q", name)
			continue
		}
		if attr.GetDeprecationMessage() == "" {
			t.Errorf("flat attribute %q should carry a DeprecationMessage", name)
		}
	}

	// Blueprint-level attributes must NOT be deprecated.
	for _, name := range []string{"name", "description", "deployed", "device_groups"} {
		if resp.Schema.Attributes[name].GetDeprecationMessage() != "" {
			t.Errorf("blueprint-level attribute %q must not be deprecated", name)
		}
	}
}

func TestBlueprintDataSource_SchemaComponentBlocks(t *testing.T) {
	d := NewBlueprintDataSource()
	var resp datasource.SchemaResponse
	d.(*BlueprintDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["component_blocks"]
	if !ok {
		t.Fatal("data source missing component_blocks attribute")
	}
	if !attr.IsComputed() {
		t.Error("data source component_blocks should be computed")
	}
}

// --- buildSteps (input) ---

func TestBuildSteps_BlockMode(t *testing.T) {
	r := &BlueprintResource{}
	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			{
				Name:                 types.StringValue("Block A"),
				ActivationConditions: types.StringValue("ANY @property(jamf.device.groups)"),
				Components: []ComponentModel{
					{Identifier: types.StringValue("com.jamf.ddm.passcode-settings"), Configuration: types.MapNull(types.StringType)},
				},
			},
			{
				Name: types.StringValue("Block B"),
				Components: []ComponentModel{
					{Identifier: types.StringValue("com.jamf.ddm.passcode-settings"), Configuration: types.MapNull(types.StringType)},
				},
			},
		},
	}

	steps, diags := r.buildSteps(context.Background(), data, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	if steps[0].Name == nil || *steps[0].Name != "Block A" {
		t.Errorf("expected step 0 name 'Block A', got %v", steps[0].Name)
	}
	if steps[0].ActivationPredicate == nil || *steps[0].ActivationPredicate != "ANY @property(jamf.device.groups)" {
		t.Errorf("expected step 0 activation predicate, got %v", steps[0].ActivationPredicate)
	}
	if steps[1].Name == nil || *steps[1].Name != "Block B" {
		t.Errorf("expected step 1 name 'Block B', got %v", steps[1].Name)
	}
	if steps[1].ActivationPredicate != nil {
		t.Errorf("expected step 1 activation predicate nil, got %v", *steps[1].ActivationPredicate)
	}

	// Same identifier appears in both blocks — the wire allows it and buildSteps must not merge.
	if len(steps[0].Components) != 1 || steps[0].Components[0].Identifier != "com.jamf.ddm.passcode-settings" {
		t.Errorf("expected step 0 to carry the passcode component, got %+v", steps[0].Components)
	}
	if len(steps[1].Components) != 1 || steps[1].Components[0].Identifier != "com.jamf.ddm.passcode-settings" {
		t.Errorf("expected step 1 to carry the passcode component, got %+v", steps[1].Components)
	}
}

func TestBuildSteps_FlatMode(t *testing.T) {
	r := &BlueprintResource{}
	data := &BlueprintResourceModel{
		Name:                 types.StringValue("BP"),
		ActivationConditions: types.StringValue("ANY x"),
		Components: []ComponentModel{
			{Identifier: types.StringValue("com.jamf.ddm.disk-management"), Configuration: types.MapNull(types.StringType)},
		},
	}

	steps, diags := r.buildSteps(context.Background(), data, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 flat step, got %d", len(steps))
	}
	if steps[0].Name == nil || *steps[0].Name != flatStepName {
		t.Errorf("expected flat step name %q, got %v", flatStepName, steps[0].Name)
	}
	if steps[0].ActivationPredicate == nil || *steps[0].ActivationPredicate != "ANY x" {
		t.Errorf("expected flat step activation predicate 'ANY x', got %v", steps[0].ActivationPredicate)
	}
}

// --- Dual-mode read (state) ---

func TestUpdateModelFromAPIResponse_BlockMode(t *testing.T) {
	ctx := context.Background()
	model := &BlueprintResourceModel{} // empty prior → block mode
	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
		Steps: []blueprints.BlueprintStep{
			{
				Name:       new("Step 1"),
				Components: []blueprints.Component{{Identifier: "com.jamf.custom.thing", Configuration: json.RawMessage(`{"a":"1"}`)}},
			},
			{
				Name:                new("Step 2"),
				ActivationPredicate: new("ANY @status(x) == 'y'"),
				Components:          []blueprints.Component{{Identifier: "com.jamf.custom.thing", Configuration: json.RawMessage(`{"a":"2"}`)}},
			},
			{
				Name: new("Step 3"),
				Components: []blueprints.Component{{
					Identifier:    "com.jamf.ai-governance",
					Configuration: json.RawMessage(`{"policies":[{"policyId":"11111111-2222-3333-4444-555555555555","versionNumber":3}]}`),
				}},
			},
		},
	}

	diags := updateModelFromAPIResponse(ctx, model, blueprint)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(model.ComponentBlocks) != 3 {
		t.Fatalf("expected 3 component blocks, got %d", len(model.ComponentBlocks))
	}
	if model.ComponentBlocks[0].Name.ValueString() != "Step 1" {
		t.Errorf("expected block 0 name 'Step 1', got %q", model.ComponentBlocks[0].Name.ValueString())
	}
	if !model.ComponentBlocks[0].ActivationConditions.IsNull() {
		t.Errorf("expected block 0 activation null, got %q", model.ComponentBlocks[0].ActivationConditions.ValueString())
	}
	if model.ComponentBlocks[1].ActivationConditions.ValueString() != "ANY @status(x) == 'y'" {
		t.Errorf("expected block 1 activation predicate, got %q", model.ComponentBlocks[1].ActivationConditions.ValueString())
	}
	// Same non-typed identifier lands in each block's raw_component independently.
	for i, block := range model.ComponentBlocks[:2] {
		if len(block.Components) != 1 || block.Components[0].Identifier.ValueString() != "com.jamf.custom.thing" {
			t.Errorf("expected block %d raw component 'com.jamf.custom.thing', got %+v", i, block.Components)
		}
	}

	aiGovernance := model.ComponentBlocks[2].AIGovernance
	if aiGovernance == nil || len(aiGovernance.Policies) != 1 {
		t.Fatalf("expected block 2 ai_governance to carry one policy, got %+v", aiGovernance)
	}
	if got := aiGovernance.Policies[0].PolicyID.ValueString(); got != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("expected the policy id from the wire, got %q", got)
	}
	if got := aiGovernance.Policies[0].Version.ValueInt64(); got != 3 {
		t.Errorf("expected version 3, got %d", got)
	}
	if len(model.ComponentBlocks[2].Components) != 0 {
		t.Errorf("com.jamf.ai-governance must not also land in raw_component, got %+v", model.ComponentBlocks[2].Components)
	}

	// Flat attributes must be cleared in block mode.
	if model.Components != nil {
		t.Error("expected flat raw_component nil in block mode")
	}
	if !model.ActivationConditions.IsNull() {
		t.Error("expected flat activation_conditions null in block mode")
	}
}

func TestUpdateModelFromAPIResponse_FlatModeMultiStepWarns(t *testing.T) {
	ctx := context.Background()
	model := &BlueprintResourceModel{
		Components: []ComponentModel{
			{Identifier: types.StringValue("com.jamf.ddm.disk-management"), Configuration: types.MapNull(types.StringType)},
		},
	}
	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
		Steps: []blueprints.BlueprintStep{
			{Components: []blueprints.Component{{Identifier: "com.jamf.ddm.disk-management", Configuration: json.RawMessage(`{"externalStorage":"deny"}`)}}},
			{Components: []blueprints.Component{{Identifier: "com.jamf.ddm.math-settings", Configuration: json.RawMessage(`{}`)}}},
		},
	}

	diags := updateModelFromAPIResponse(ctx, model, blueprint)
	if diags.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", diags)
	}
	if diags.WarningsCount() == 0 {
		t.Fatal("expected a migration warning for a multi-step blueprint in flat mode")
	}
	found := false
	for _, d := range diags.Warnings() {
		if d.Summary() == "Blueprint has multiple component blocks" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Blueprint has multiple component blocks' warning, got %v", diags)
	}
	if model.ComponentBlocks != nil {
		t.Error("flat mode must not populate component_blocks")
	}
}

func TestUpdateModelFromAPIResponse_EmptyPriorUsesBlockMode(t *testing.T) {
	ctx := context.Background()
	model := &BlueprintResourceModel{}
	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "NOT_DEPLOYED"},
		Steps: []blueprints.BlueprintStep{
			{Name: new("Only Block"), Components: []blueprints.Component{{Identifier: "com.jamf.custom.x", Configuration: json.RawMessage(`{"k":"v"}`)}}},
		},
	}

	updateModelFromAPIResponse(ctx, model, blueprint)

	if len(model.ComponentBlocks) != 1 {
		t.Fatalf("expected empty prior to read as one block, got %d blocks", len(model.ComponentBlocks))
	}
	if model.Components != nil {
		t.Error("expected flat raw_component nil when reading in block mode")
	}
}

// --- Typed component registration ---

// typedComponentCase pins one entry of stronglyTypedComponentIdentifiers to the component_blocks
// schema attribute that represents it, a wire configuration for it, and a check that the attribute
// was populated.
type typedComponentCase struct {
	identifier      string
	schemaAttribute string
	configuration   json.RawMessage
	populated       func(ComponentBlockModel) bool
}

// typedComponentCases enumerates every component identifier the provider maps onto a typed
// component_blocks attribute instead of raw_component. Registering one takes an entry in
// stronglyTypedComponentIdentifiers as well as the model field, schema attribute, read mapping and
// write collector, and a missing entry makes the component populate its typed attribute *and*
// raw_component — which fails an apply with "Provider produced inconsistent result after apply"
// rather than anything a compiler or a schema test can see. The table exists so that gap is a unit
// failure for all of them at once.
//
// Most rows carry `{}`: what each row guards is the identifier's routing, not the payload, and
// every typed component decodes an empty object into an all-null value. The two rows that carry a
// shape are the ones whose attribute stays empty without one — AI Governance, whose policies list
// is the component's whole content, and the legacy configuration profile, which is derived from
// payloadContent rather than from a typed struct.
func typedComponentCases() []typedComponentCase {
	empty := json.RawMessage(`{}`)
	return []typedComponentCase{
		{
			identifier:      "com.jamf.ai-governance",
			schemaAttribute: "ai_governance",
			configuration:   json.RawMessage(`{"policies":[{"policyId":"11111111-2222-3333-4444-555555555555","versionNumber":3}]}`),
			populated:       func(b ComponentBlockModel) bool { return b.AIGovernance != nil && len(b.AIGovernance.Policies) == 1 },
		},
		{
			// A declaration list is the component's whole content, so an empty object would leave
			// the attribute looking unpopulated the same way AI Governance would.
			identifier:      "com.jamf.ddm-strict",
			schemaAttribute: "apple_declarations",
			configuration:   json.RawMessage(`{"declarations":[{"channelType":"SYSTEM","kind":"CONFIGURATION","payloadKey":1,"type":"com.apple.configuration.siri.settings","payload":{"Enabled":true}}]}`),
			populated: func(b ComponentBlockModel) bool {
				return len(b.AppleDeclarations) == 1
			},
		},
		{
			identifier:      "com.jamf.ddm.audio-accessory-settings",
			schemaAttribute: "audio_accessory_settings",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.AudioAccessorySettings != nil },
		},
		{
			identifier:      "com.jamf.ddm.custom-declarations",
			schemaAttribute: "custom_declarations",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.CustomDeclarations != nil },
		},
		{
			identifier:      "com.jamf.ddm.disk-management",
			schemaAttribute: "disk_management_settings",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.DiskManagementSettings != nil },
		},
		{
			identifier:      "com.jamf.ddm.math-settings",
			schemaAttribute: "math_settings",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.MathSettings != nil },
		},
		{
			identifier:      "com.jamf.ddm.passcode-settings",
			schemaAttribute: "passcode_policy",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.PasscodePolicy != nil },
		},
		{
			identifier:      "com.jamf.ddm.safari-bookmarks",
			schemaAttribute: "safari_bookmarks",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.SafariBookmarks != nil },
		},
		{
			identifier:      "com.jamf.ddm.safari-extensions",
			schemaAttribute: "safari_extensions",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.SafariExtensions != nil },
		},
		{
			identifier:      "com.jamf.ddm.safari-settings",
			schemaAttribute: "safari_settings",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.SafariSettings != nil },
		},
		{
			identifier:      "com.jamf.ddm.service-background-tasks",
			schemaAttribute: "service_background_tasks",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.ServiceBackgroundTasks != nil },
		},
		{
			identifier:      "com.jamf.ddm.service-configuration-files",
			schemaAttribute: "service_configuration_files",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.ServiceConfigurationFiles != nil },
		},
		{
			identifier:      "com.jamf.ddm.sw-updates",
			schemaAttribute: "software_update",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.SoftwareUpdate != nil },
		},
		{
			identifier:      "com.jamf.ddm.software-update-settings",
			schemaAttribute: "software_update_settings",
			configuration:   empty,
			populated:       func(b ComponentBlockModel) bool { return b.SoftwareUpdateSettings != nil },
		},
		{
			identifier:      "com.jamf.ddm-configuration-profile",
			schemaAttribute: "legacy_payloads",
			configuration:   json.RawMessage(`{"payloadDisplayName":"BP","payloadContent":[{"payloadType":"com.apple.dock","payloadIdentifier":"jamf.dock","tilesize":50}]}`),
			populated:       func(b ComponentBlockModel) bool { return len(b.LegacyPayloads) == 1 },
		},
	}
}

func TestTypedComponentCases_CoverRegistrationAndSchema(t *testing.T) {
	attributes := componentBlockAttributes()
	covered := make(map[string]struct{}, len(stronglyTypedComponentIdentifiers))

	for _, tc := range typedComponentCases() {
		if _, registered := stronglyTypedComponentIdentifiers[tc.identifier]; !registered {
			t.Errorf("%q has a case but no stronglyTypedComponentIdentifiers entry, so it would also land in raw_component", tc.identifier)
		}
		if _, exists := attributes[tc.schemaAttribute]; !exists {
			t.Errorf("%q names component_blocks attribute %q, which does not exist", tc.identifier, tc.schemaAttribute)
		}
		if _, duplicate := covered[tc.identifier]; duplicate {
			t.Errorf("%q appears more than once in typedComponentCases", tc.identifier)
		}
		covered[tc.identifier] = struct{}{}
	}

	for identifier := range stronglyTypedComponentIdentifiers {
		if _, ok := covered[identifier]; !ok {
			t.Errorf("stronglyTypedComponentIdentifiers entry %q has no case in typedComponentCases — add one so its registration is guarded", identifier)
		}
	}
}

func TestTypedComponents_RoundTripWithoutLandingInRawComponent(t *testing.T) {
	ctx := context.Background()

	for _, tc := range typedComponentCases() {
		t.Run(tc.identifier, func(t *testing.T) {
			model := &BlueprintResourceModel{}
			blueprint := &blueprints.BlueprintDetail{
				DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
				Steps: []blueprints.BlueprintStep{{
					Name:       new("Block"),
					Components: []blueprints.Component{{Identifier: tc.identifier, Configuration: tc.configuration}},
				}},
			}

			diags := updateModelFromAPIResponse(ctx, model, blueprint)
			if diags.HasError() {
				t.Fatalf("unexpected read diagnostics: %v", diags)
			}
			if len(model.ComponentBlocks) != 1 {
				t.Fatalf("expected 1 component block, got %d", len(model.ComponentBlocks))
			}

			block := model.ComponentBlocks[0]
			if !tc.populated(block) {
				t.Errorf("expected %q to populate the %q attribute, got block %+v", tc.identifier, tc.schemaAttribute, block)
			}
			if len(block.Components) != 0 {
				t.Errorf("%q is strongly typed and must not also land in raw_component, got %+v", tc.identifier, block.Components)
			}

			r := &BlueprintResource{}
			steps, buildDiags := r.buildSteps(ctx, &BlueprintResourceModel{
				Name:            types.StringValue("BP"),
				ComponentBlocks: model.ComponentBlocks,
			}, nil)
			if buildDiags.HasError() {
				t.Fatalf("unexpected write diagnostics: %v", buildDiags)
			}
			if len(steps) != 1 {
				t.Fatalf("expected 1 step, got %d", len(steps))
			}
			if len(steps[0].Components) != 1 || steps[0].Components[0].Identifier != tc.identifier {
				t.Errorf("expected the step to carry exactly one %q component, got %+v", tc.identifier, steps[0].Components)
			}
		})
	}
}

func TestBuildTypedComponent_UndecodableConfigurationWarns(t *testing.T) {
	ctx := context.Background()
	model := &BlueprintResourceModel{}
	blueprint := &blueprints.BlueprintDetail{
		DeploymentState: &blueprints.DeploymentState{State: "DEPLOYED"},
		Steps: []blueprints.BlueprintStep{{
			Name: new("Block"),
			Components: []blueprints.Component{{
				Identifier:    "com.jamf.ai-governance",
				Configuration: json.RawMessage(`{"policies":"not-a-list"}`),
			}},
		}},
	}

	diags := updateModelFromAPIResponse(ctx, model, blueprint)
	if diags.HasError() {
		t.Fatalf("an undecodable component configuration must not fail the read: %v", diags)
	}
	if len(model.ComponentBlocks) != 1 || model.ComponentBlocks[0].AIGovernance != nil {
		t.Fatalf("expected the component to be absent from state, got %+v", model.ComponentBlocks)
	}

	found := false
	for _, d := range diags.Warnings() {
		if d.Summary() == "Blueprint component configuration could not be read" && strings.Contains(d.Detail(), "com.jamf.ai-governance") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning naming com.jamf.ai-governance, got %v", diags)
	}
}

// TestCollectBlockComponents_AppleDeclarationsAndRawComponentConflict pins the overlap the wire
// cannot represent: the platform stores two components sharing one identifier, and the read path
// keys them by identifier, so the block must be rejected rather than written.
func TestCollectBlockComponents_AppleDeclarationsAndRawComponentConflict(t *testing.T) {
	r := &BlueprintResource{}
	block := ComponentBlockModel{
		Name: types.StringValue("Block A"),
		Components: []ComponentModel{
			{Identifier: types.StringValue(appleDeclarationsIdentifier), Configuration: types.MapNull(types.StringType)},
		},
		AppleDeclarations: []AppleDeclarationModel{
			appleDeclaration("com.apple.configuration.passcode.settings", `{"RequireAlphanumericPasscode":true}`),
		},
	}

	components, diags := r.collectBlockComponents(context.Background(), block)
	if !diags.HasError() {
		t.Fatal("expected an error for a block setting both apple_declarations and a raw_component for that component")
	}
	if len(components) != 0 {
		t.Fatalf("expected no components to be built, got %+v", components)
	}

	summary := diags.Errors()[0].Summary()
	detail := diags.Errors()[0].Detail()
	if !strings.Contains(detail, "apple_declarations") || !strings.Contains(detail, "raw_component") {
		t.Errorf("error must name both attributes, got %q / %q", summary, detail)
	}
	if strings.Contains(summary, appleDeclarationsIdentifier) || strings.Contains(detail, appleDeclarationsIdentifier) {
		t.Errorf("user-facing text must not carry the wire identifier, got %q / %q", summary, detail)
	}
}

// TestBuildSteps_AppleDeclarationsAndRawComponentConflictEmitsNoStep proves the rejection blocks
// the write: the conflicting block contributes no step, so nothing reaches the wire.
func TestBuildSteps_AppleDeclarationsAndRawComponentConflictEmitsNoStep(t *testing.T) {
	r := &BlueprintResource{}
	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			{
				Name: types.StringValue("Block A"),
				Components: []ComponentModel{
					{Identifier: types.StringValue(appleDeclarationsIdentifier), Configuration: types.MapNull(types.StringType)},
				},
				AppleDeclarations: []AppleDeclarationModel{
					appleDeclaration("com.apple.configuration.passcode.settings", `{"RequireAlphanumericPasscode":true}`),
				},
			},
		},
	}

	steps, diags := r.buildSteps(context.Background(), data, nil)
	if !diags.HasError() {
		t.Fatal("expected an error from the conflicting block")
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %+v", steps)
	}
}

// blockWithEveryTypedAttribute returns a block that populates every strongly-typed component
// attribute. The typed values are zero-valued on purpose: the overlap guard runs before any
// component is built, so what a converter would emit is irrelevant to these tests.
func blockWithEveryTypedAttribute() ComponentBlockModel {
	return ComponentBlockModel{
		Name:                      types.StringValue("Every attribute"),
		AIGovernance:              &components.AIGovernanceComponent{},
		AppleDeclarations:         []AppleDeclarationModel{appleDeclaration("com.apple.configuration.passcode.settings", `{"RequireAlphanumericPasscode":true}`)},
		AudioAccessorySettings:    &components.AudioAccessorySettingsComponent{},
		CustomDeclarations:        &components.CustomDeclarationsComponent{},
		DiskManagementSettings:    &components.DiskManagementPolicyComponent{},
		MathSettings:              &components.MathSettingsComponent{},
		PasscodePolicy:            &components.PasscodePolicyComponent{},
		SafariBookmarks:           &components.SafariBookmarksComponent{},
		SafariExtensions:          &components.SafariExtensionsComponent{},
		SafariSettings:            &components.SafariSettingsComponent{},
		ServiceBackgroundTasks:    &components.ServiceBackgroundTasksComponent{},
		ServiceConfigurationFiles: &components.ServiceConfigurationFilesComponent{},
		SoftwareUpdate:            &components.SoftwareUpdateComponent{},
		SoftwareUpdateSettings:    &components.SoftwareUpdateSettingsComponent{},
		LegacyPayloads:            []BlockLegacyPayloadModel{{PayloadType: types.StringValue("com.apple.dock"), Settings: types.StringValue(`{}`)}},
	}
}

// TestPopulatedTypedComponentAttributes_CoversEveryTypedComponent pins the mapping the overlap guard
// is built on: every component the read path treats as strongly typed must be reachable from some
// Terraform attribute, or a new typed component ships with the overlap unguarded again.
func TestPopulatedTypedComponentAttributes_CoversEveryTypedComponent(t *testing.T) {
	mapped := make(map[string]string, len(stronglyTypedComponentIdentifiers))
	for _, attribute := range populatedTypedComponentAttributes(blockWithEveryTypedAttribute()) {
		if attribute.name == "" {
			t.Errorf("attribute for %q has no Terraform attribute name to report an overlap against", attribute.identifier)
		}
		if previous, duplicated := mapped[attribute.identifier]; duplicated {
			t.Errorf("identifier %q is claimed by both %s and %s", attribute.identifier, previous, attribute.name)
		}
		mapped[attribute.identifier] = attribute.name
	}

	for identifier := range stronglyTypedComponentIdentifiers {
		if _, ok := mapped[identifier]; !ok {
			t.Errorf("no strongly-typed attribute maps to %q, so a raw_component overlapping it cannot be caught", identifier)
		}
	}
	for identifier, name := range mapped {
		if _, ok := stronglyTypedComponentIdentifiers[identifier]; !ok {
			t.Errorf("%s writes %q, which stronglyTypedComponentIdentifiers does not list, so the read path would not treat it as typed", name, identifier)
		}
	}
}

// TestCollectBlockComponents_EveryTypedAttributeConflictsWithRawComponent generalises the Apple
// declarations case to every strongly-typed attribute: each one, set alongside a raw_component for
// the component it writes, is rejected before anything is built. Overlapping one identifier while
// every other typed attribute is set also proves the guard reports that attribute alone.
func TestCollectBlockComponents_EveryTypedAttributeConflictsWithRawComponent(t *testing.T) {
	r := &BlueprintResource{}

	for _, attribute := range populatedTypedComponentAttributes(blockWithEveryTypedAttribute()) {
		t.Run(attribute.name, func(t *testing.T) {
			block := blockWithEveryTypedAttribute()
			block.Components = []ComponentModel{
				{Identifier: types.StringValue(attribute.identifier), Configuration: types.MapNull(types.StringType)},
			}

			built, diags := r.collectBlockComponents(context.Background(), block)
			if !diags.HasError() {
				t.Fatalf("expected an error for %s set alongside a raw_component for the same component", attribute.name)
			}
			if len(built) != 0 {
				t.Fatalf("expected no components to be built, got %+v", built)
			}
			if len(diags.Errors()) != 1 {
				t.Fatalf("expected exactly one error, got %d: %v", len(diags.Errors()), diags.Errors())
			}

			summary := diags.Errors()[0].Summary()
			detail := diags.Errors()[0].Detail()
			if !strings.Contains(detail, attribute.name) || !strings.Contains(detail, "raw_component") {
				t.Errorf("error must name both attributes, got %q / %q", summary, detail)
			}
			if strings.Contains(summary, attribute.identifier) || strings.Contains(detail, attribute.identifier) {
				t.Errorf("user-facing text must not carry the wire identifier, got %q / %q", summary, detail)
			}
			if !strings.Contains(detail, block.Name.ValueString()) {
				t.Errorf("error must locate the block it is in, got %q", detail)
			}
		})
	}
}

// TestRawComponentOverlapDiags_UnnamedBlockOmitsTheLocator keeps the locator optional: a block with
// no name, and the deprecated flat style, have nothing to name, so the message reads as one sentence
// rather than trailing an empty quotation.
func TestRawComponentOverlapDiags_UnnamedBlockOmitsTheLocator(t *testing.T) {
	overlap := []typedComponentAttribute{{name: "passcode_policy", identifier: "com.jamf.ddm.passcode-settings"}}
	raw := []ComponentModel{
		{Identifier: types.StringValue("com.jamf.ddm.passcode-settings"), Configuration: types.MapNull(types.StringType)},
	}

	diags := rawComponentOverlapDiags("", raw, overlap)
	if len(diags.Errors()) != 1 {
		t.Fatalf("expected exactly one error, got %v", diags.Errors())
	}

	detail := diags.Errors()[0].Detail()
	if strings.Contains(detail, "component block") {
		t.Errorf("an unnamed block must not be located, got %q", detail)
	}
	if !strings.Contains(detail, "passcode_policy and a raw_component both manage the same component.") {
		t.Errorf("expected the unlocated sentence, got %q", detail)
	}
}

// TestBuildSteps_ConflictingBlockReportsOnlyTheOverlap proves a rejected block stops there: its
// legacy payloads are not collected afterwards, so an author is not also handed an error about
// payloads in a block that was already refused.
func TestBuildSteps_ConflictingBlockReportsOnlyTheOverlap(t *testing.T) {
	r := &BlueprintResource{}
	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			{
				Name:           types.StringValue("Block A"),
				PasscodePolicy: &components.PasscodePolicyComponent{},
				LegacyPayloads: []BlockLegacyPayloadModel{
					{PayloadType: types.StringValue("com.apple.dock"), Settings: types.StringValue(`{}`)},
					{PayloadType: types.StringValue("com.apple.dock"), Settings: types.StringValue(`{}`)},
				},
				Components: []ComponentModel{
					{Identifier: types.StringValue("com.jamf.ddm.passcode-settings"), Configuration: types.MapNull(types.StringType)},
				},
			},
		},
	}

	steps, diags := r.buildSteps(context.Background(), data, nil)
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %+v", steps)
	}
	if len(diags.Errors()) != 1 {
		t.Fatalf("expected only the overlap error, got %v", diags.Errors())
	}
	if summary := diags.Errors()[0].Summary(); summary != "Component configured twice" {
		t.Errorf("expected the overlap error, got %q", summary)
	}
}

// TestCollectBlockComponents_RawComponentForAnotherComponentIsAccepted keeps the guard from
// swallowing the supported case: a raw_component for a component no typed attribute in the block
// writes is not an overlap.
func TestCollectBlockComponents_RawComponentForAnotherComponentIsAccepted(t *testing.T) {
	r := &BlueprintResource{}
	block := ComponentBlockModel{
		Name:           types.StringValue("Block A"),
		PasscodePolicy: &components.PasscodePolicyComponent{},
		Components: []ComponentModel{
			{Identifier: types.StringValue("com.jamf.ddm.safari-settings"), Configuration: types.MapNull(types.StringType)},
		},
	}

	built, diags := r.collectBlockComponents(context.Background(), block)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags.Errors())
	}
	if len(built) != 2 {
		t.Fatalf("expected the raw component and the typed component, got %+v", built)
	}
}

// TestBuildSteps_TypedAndRawComponentConflictEmitsNoStep proves the rejection blocks the write for a
// typed attribute other than apple_declarations: the conflicting block contributes no step.
func TestBuildSteps_TypedAndRawComponentConflictEmitsNoStep(t *testing.T) {
	r := &BlueprintResource{}
	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			{
				Name:           types.StringValue("Block A"),
				PasscodePolicy: &components.PasscodePolicyComponent{},
				Components: []ComponentModel{
					{Identifier: types.StringValue("com.jamf.ddm.passcode-settings"), Configuration: types.MapNull(types.StringType)},
				},
			},
		},
	}

	steps, diags := r.buildSteps(context.Background(), data, nil)
	if !diags.HasError() {
		t.Fatal("expected an error from the conflicting block")
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %+v", steps)
	}
}

// TestBuildSteps_FlatLegacyPayloadsAndRawComponentConflictEmitsNoStep covers the one overlap the
// deprecated flat authoring style has to check for itself, because flatComponentsAsBlock cannot
// carry the dynamic legacy payload value.
func TestBuildSteps_FlatLegacyPayloadsAndRawComponentConflictEmitsNoStep(t *testing.T) {
	r := &BlueprintResource{}
	data := &BlueprintResourceModel{
		Name:           types.StringValue("BP"),
		LegacyPayloads: flatLegacyPayloadsValue(t),
		Components: []ComponentModel{
			{Identifier: types.StringValue(legacyConfigProfileIdentifier), Configuration: types.MapNull(types.StringType)},
		},
	}

	steps, diags := r.buildSteps(context.Background(), data, nil)
	if !diags.HasError() {
		t.Fatal("expected an error for legacy_payloads set alongside a raw_component for the same component")
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %+v", steps)
	}

	detail := diags.Errors()[0].Detail()
	if !strings.Contains(detail, "legacy_payloads") || !strings.Contains(detail, "raw_component") {
		t.Errorf("error must name both attributes, got %q", detail)
	}
	if strings.Contains(detail, legacyConfigProfileIdentifier) {
		t.Errorf("user-facing text must not carry the wire identifier, got %q", detail)
	}
}

// flatLegacyPayloadsValue builds a valid deprecated top-level legacy_payloads value: one payload
// whose settings arrive as an object rather than the JSON string a block uses.
func flatLegacyPayloadsValue(t *testing.T) types.Dynamic {
	t.Helper()

	settings, diags := types.ObjectValue(
		map[string]attr.Type{"tilesize": types.NumberType},
		map[string]attr.Value{"tilesize": types.NumberValue(big.NewFloat(32))},
	)
	if diags.HasError() {
		t.Fatalf("building settings: %v", diags)
	}

	payload, diags := types.ObjectValue(
		map[string]attr.Type{"payload_type": types.StringType, "settings": settings.Type(context.Background())},
		map[string]attr.Value{"payload_type": types.StringValue("com.apple.dock"), "settings": settings},
	)
	if diags.HasError() {
		t.Fatalf("building payload: %v", diags)
	}

	payloads, diags := types.TupleValue([]attr.Type{payload.Type(context.Background())}, []attr.Value{payload})
	if diags.HasError() {
		t.Fatalf("building payload list: %v", diags)
	}

	return types.DynamicValue(payloads)
}
