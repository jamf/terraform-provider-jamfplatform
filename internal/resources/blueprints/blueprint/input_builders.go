// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"maps"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/appledeclarations"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

// flatStepName is the single step name used when a blueprint is authored with the deprecated flat
// top-level component attributes (no component_blocks).
const flatStepName = "Declaration group"

// buildSteps converts the model into the ordered SDK steps for a create/update request. In block
// mode (component_blocks set) it emits one step per block, preserving order, per-block name, and
// per-block activation condition. In the deprecated flat mode it emits the single "Declaration
// group" step carrying every top-level component and the top-level activation condition.
//
// A block's legacy payloads are checked for a raw_component overlap inside collectBlockComponents,
// which carries them; the deprecated flat value is not carried there, because the flat (dynamic) and
// block (JSON-string) shapes differ, so flat mode checks that one overlap itself.
//
// stored carries the legacy payload identifiers the service has already assigned, so an update
// writes them back rather than letting the service mint replacements; it is nil on create. See
// storedLegacyPayloadIdentifiers.
func (r *BlueprintResource) buildSteps(ctx context.Context, data *BlueprintResourceModel, stored *storedLegacyPayloadIdentifiers) ([]blueprints.BlueprintStep, diag.Diagnostics) {
	var diags diag.Diagnostics
	blueprintName := data.Name.ValueString()

	if len(data.ComponentBlocks) > 0 {
		blockNames := make([]types.String, len(data.ComponentBlocks))
		for i, block := range data.ComponentBlocks {
			blockNames[i] = block.Name
		}
		storedByBlock := stored.resolve(blockNames)

		steps := make([]blueprints.BlueprintStep, 0, len(data.ComponentBlocks))
		for i, block := range data.ComponentBlocks {
			components, blockDiags := r.collectBlockComponents(ctx, block)
			if !blockDiags.HasError() {
				r.collectBlockLegacyPayloads(&components, &blockDiags, block.LegacyPayloads, blueprintName, storedByBlock[i])
			}
			diags.Append(blockDiags...)
			if blockDiags.HasError() {
				continue
			}
			steps = append(steps, blueprints.BlueprintStep{
				Name:                block.Name.ValueStringPointer(),
				ActivationPredicate: block.ActivationConditions.ValueStringPointer(),
				Components:          components,
			})
		}
		return steps, diags
	}

	flatBlock := data.flatComponentsAsBlock()
	components, flatDiags := r.collectBlockComponents(ctx, flatBlock)
	if !data.LegacyPayloads.IsNull() && !data.LegacyPayloads.IsUnknown() {
		flatDiags.Append(rawComponentOverlapDiags("", flatBlock.Components, []typedComponentAttribute{
			{name: "legacy_payloads", identifier: legacyConfigProfileIdentifier},
		})...)
		if !flatDiags.HasError() {
			r.collectLegacyPayloads(&components, &flatDiags, data.LegacyPayloads, blueprintName, stored.resolve([]types.String{types.StringValue(flatStepName)})[0])
		}
	}
	diags.Append(flatDiags...)

	if flatDiags.HasError() {
		return nil, diags
	}

	stepName := flatStepName
	steps := []blueprints.BlueprintStep{
		{
			Name:                &stepName,
			Components:          components,
			ActivationPredicate: data.ActivationConditions.ValueStringPointer(),
		},
	}
	return steps, diags
}

// collectBlockComponents gathers the raw and strongly-typed components of one block into SDK
// component values. Legacy payloads are collected separately by the caller because the flat
// (dynamic) and block (JSON-string) shapes differ. The flat top-level authoring style reuses this
// by passing data.flatComponentsAsBlock(); each entry in component_blocks passes its own carrier.
//
// A block authoring a strongly-typed component alongside a raw_component for the same component is
// rejected before anything is built, so the block emits nothing. The platform stores both: a step
// carrying two components with one identifier is accepted and echoed back in full — wire-probed on
// the EU gateway 2026-09-12 for a passcode settings pair and a legacy configuration profile pair,
// and earlier for a declarations pair — and the read path keys components by identifier, so one of
// the two becomes unrepresentable in state. Each attribute's read-side guard is keyed off the prior
// raw set and so cannot catch the first apply that creates the overlap.
func (r *BlueprintResource) collectBlockComponents(ctx context.Context, block ComponentBlockModel) ([]blueprints.Component, diag.Diagnostics) {
	var allComponents []blueprints.Component
	var diags diag.Diagnostics

	typedAttributes := populatedTypedComponentAttributes(block)
	if overlaps := rawComponentOverlapDiags(block.Name.ValueString(), block.Components, typedAttributes); overlaps.HasError() {
		return nil, overlaps
	}

	for _, comp := range block.Components {
		component := blueprints.Component{
			Identifier: comp.Identifier.ValueString(),
		}

		if helpers.IsConfiguredValue(comp.Configuration) {
			configMap := make(map[string]string)
			configDiags := comp.Configuration.ElementsAs(ctx, &configMap, false)
			if configDiags.HasError() {
				diags.Append(configDiags...)
				continue
			}

			jsonObj := make(map[string]any)
			for key, value := range configMap {
				setNestedValue(jsonObj, key, value)
			}

			jsonBytes, err := json.Marshal(jsonObj)
			if err != nil {
				diags.AddError(
					"Error encoding component configuration",
					"Could not encode component configuration to JSON: "+helpers.APIErrorDetail(err),
				)
				continue
			}

			component.Configuration = json.RawMessage(jsonBytes)
		}
		allComponents = append(allComponents, component)
	}

	r.collectStronglyTypedComponents(&allComponents, &diags, typedAttributes)
	r.appendAppleDeclarations(&allComponents, &diags, block.AppleDeclarations)

	return allComponents, diags
}

// appendAppleDeclarations assembles the Apple declarations component from a block's declaration
// list and appends it. An empty list writes no component, the same as omitting the attribute: there
// is nothing a component holding no declarations would express.
//
// kind and payloadKey are derived rather than authored — see AppleDeclarationModel.
func (r *BlueprintResource) appendAppleDeclarations(allComponents *[]blueprints.Component, diags *diag.Diagnostics, declarations []AppleDeclarationModel) {
	if len(declarations) == 0 {
		return
	}

	wire := make([]blueprints.CustomDeclaration, 0, len(declarations))
	for idx, declaration := range declarations {
		var payload map[string]any
		if err := json.Unmarshal([]byte(declaration.Payload.ValueString()), &payload); err != nil {
			diags.AddError(
				"Invalid Apple declaration payload",
				"payload for declaration type "+declaration.Type.ValueString()+
					" must be a JSON object string (write it with jsonencode or read it from a .json file): "+helpers.APIErrorDetail(err),
			)
			return
		}

		declarationType := declaration.Type.ValueString()
		wire = append(wire, blueprints.CustomDeclaration{
			ChannelType: declaration.ChannelType.ValueString(),
			Kind:        appledeclarations.KindForType(declarationType),
			Payload:     payload,
			PayloadKey:  idx + 1,
			Type:        declarationType,
		})
	}

	configJSON, err := json.Marshal(blueprints.CustomDeclarationsConfiguration{Declarations: wire})
	if err != nil {
		diags.AddError("Error encoding Apple declarations configuration", "Could not encode configuration to JSON: "+helpers.APIErrorDetail(err))
		return
	}

	*allComponents = append(*allComponents, blueprints.Component{
		Identifier:    appleDeclarationsIdentifier,
		Configuration: json.RawMessage(configJSON),
	})
}

// typedComponentAttribute is one strongly-typed component attribute a block populates, paired with
// the wire identifier it writes. The Terraform attribute name travels with it so an overlap with
// raw_component is reported against the attribute an author wrote rather than the identifier it
// writes: an identifier is API plumbing and must not reach user-facing text (see STYLE_GUIDE
// §Attribute names mirror the Jamf Pro admin UI).
//
// component is nil for the two attributes assembled in this package rather than in components/ —
// apple_declarations and legacy_payloads — which the collector therefore skips.
type typedComponentAttribute struct {
	name       string
	label      string
	identifier string
	component  components.ComponentConverter
}

// populatedTypedComponentAttributes reports the strongly-typed component attributes the block sets,
// in the order their components are written. It is the single place that knows which attribute
// writes which component, and two callers need that mapping: the collector builds each one, and the
// raw_component overlap guard names the attribute it collides with. A converter's own GetIdentifier
// is the identifier's source, so no converter's identifier is restated here.
//
// Attribute order is the order the platform receives the components in, so a new entry goes where
// its component should sit rather than alphabetically.
func populatedTypedComponentAttributes(block ComponentBlockModel) []typedComponentAttribute {
	candidates := []struct {
		name      string
		label     string
		set       bool
		component components.ComponentConverter
	}{
		{"ai_governance", "AI governance", block.AIGovernance != nil, block.AIGovernance},
		{"audio_accessory_settings", "audio accessory settings", block.AudioAccessorySettings != nil, block.AudioAccessorySettings},
		{"custom_declarations", "custom declarations", block.CustomDeclarations != nil, block.CustomDeclarations},
		{"disk_management_settings", "disk management settings", block.DiskManagementSettings != nil, block.DiskManagementSettings},
		{"math_settings", "math settings", block.MathSettings != nil, block.MathSettings},
		{"passcode_policy", "passcode policy", block.PasscodePolicy != nil, block.PasscodePolicy},
		{"safari_bookmarks", "safari bookmarks", block.SafariBookmarks != nil, block.SafariBookmarks},
		{"safari_extensions", "safari extensions", block.SafariExtensions != nil, block.SafariExtensions},
		{"safari_settings", "safari settings", block.SafariSettings != nil, block.SafariSettings},
		{"service_background_tasks", "service background tasks", block.ServiceBackgroundTasks != nil, block.ServiceBackgroundTasks},
		{"service_configuration_files", "service configuration files", block.ServiceConfigurationFiles != nil, block.ServiceConfigurationFiles},
		{"software_update", "software update", block.SoftwareUpdate != nil, block.SoftwareUpdate},
		{"software_update_settings", "software update settings", block.SoftwareUpdateSettings != nil, block.SoftwareUpdateSettings},
	}

	attributes := make([]typedComponentAttribute, 0, len(candidates)+2)
	for _, candidate := range candidates {
		if !candidate.set {
			continue
		}
		attributes = append(attributes, typedComponentAttribute{
			name:       candidate.name,
			label:      candidate.label,
			identifier: candidate.component.GetIdentifier(),
			component:  candidate.component,
		})
	}

	if len(block.AppleDeclarations) > 0 {
		attributes = append(attributes, typedComponentAttribute{name: "apple_declarations", identifier: appleDeclarationsIdentifier})
	}
	if len(block.LegacyPayloads) > 0 {
		attributes = append(attributes, typedComponentAttribute{name: "legacy_payloads", identifier: legacyConfigProfileIdentifier})
	}

	return attributes
}

// rawComponentOverlapDiags reports every populated strongly-typed attribute that manages the same
// component as one of the block's raw_component entries, one error per overlap. See
// collectBlockComponents for why an overlap cannot be written.
//
// blockName locates the overlap for an author who has many blocks, and is empty for the deprecated
// flat style, which has only the one implicit step to look at.
func rawComponentOverlapDiags(blockName string, rawComponents []ComponentModel, attributes []typedComponentAttribute) diag.Diagnostics {
	var diags diag.Diagnostics
	rawIdentifiers := rawIdentifierSet(rawComponents)

	in := ""
	if blockName != "" {
		in = " in component block " + strconv.Quote(blockName)
	}

	for _, attribute := range attributes {
		if _, handledAsRaw := rawIdentifiers[attribute.identifier]; !handledAsRaw {
			continue
		}
		diags.AddError(
			"Component configured twice",
			attribute.name+" and a raw_component both manage the same component"+in+". Keep one of the two: the "+
				"platform stores both copies and the provider can hold only one in state, so the other would "+
				"reach devices without appearing in a plan.",
		)
	}

	return diags
}

// collectStronglyTypedComponents builds each strongly-typed component the block populates, in
// attribute order. apple_declarations and legacy_payloads carry no converter and are assembled by
// their own appenders.
func (r *BlueprintResource) collectStronglyTypedComponents(allComponents *[]blueprints.Component, diags *diag.Diagnostics, attributes []typedComponentAttribute) {
	for _, attribute := range attributes {
		if attribute.component == nil {
			continue
		}
		r.collectSingleComponent(allComponents, diags, attribute.component, attribute.label)
	}
}

// collectSingleComponent is a helper function that can collect any type of strongly-typed component.
func (r *BlueprintResource) collectSingleComponent(allComponents *[]blueprints.Component, diags *diag.Diagnostics, comp components.ComponentConverter, componentName string) {
	clientComp, err := comp.ToClientComponent()
	if err != nil {
		diags.AddError("Failed to build "+componentName+" component", helpers.APIErrorDetail(err))
		return
	}
	*allComponents = append(*allComponents, *clientComp)
}

// legacyPayloadEntry is one legacy payload flattened to its payload type and settings map, the
// common shape both the flat (dynamic) and block (JSON-string) legacy collectors reduce to.
type legacyPayloadEntry struct {
	PayloadType string
	Settings    map[string]any
}

// collectLegacyPayloads builds the legacy configuration profile component from the deprecated
// dynamic top-level legacy_payloads value.
func (r *BlueprintResource) collectLegacyPayloads(allComponents *[]blueprints.Component, diags *diag.Diagnostics, legacyPayloads types.Dynamic, blueprintName string, storedIdentifiers map[string]string) {
	raw, err := helpers.TerraformDynamicToJSON(legacyPayloads)
	if err != nil {
		diags.AddError("Error reading legacy payloads", "Could not convert legacy payloads to JSON: "+helpers.APIErrorDetail(err))
		return
	}

	items, ok := raw.([]any)
	if !ok {
		diags.AddError("Invalid legacy_payloads", "Expected a list of objects, got a non-list value.")
		return
	}

	entries := make([]legacyPayloadEntry, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			diags.AddError("Invalid legacy_payloads entry", "Each legacy payload must be an object.")
			return
		}

		payloadType, _ := obj["payload_type"].(string)
		entry := legacyPayloadEntry{PayloadType: payloadType}
		if settings, exists := obj["settings"]; exists {
			if settingsMap, ok := settings.(map[string]any); ok {
				entry.Settings = settingsMap
			}
		}
		entries = append(entries, entry)
	}

	r.appendLegacyConfigProfile(allComponents, diags, entries, blueprintName, storedIdentifiers)
}

// collectBlockLegacyPayloads builds the legacy configuration profile component from a block's
// legacy_payloads list, whose settings arrive as JSON object strings.
func (r *BlueprintResource) collectBlockLegacyPayloads(allComponents *[]blueprints.Component, diags *diag.Diagnostics, payloads []BlockLegacyPayloadModel, blueprintName string, storedIdentifiers map[string]string) {
	if len(payloads) == 0 {
		return
	}

	entries := make([]legacyPayloadEntry, 0, len(payloads))
	for _, payload := range payloads {
		entry := legacyPayloadEntry{PayloadType: payload.PayloadType.ValueString()}
		if helpers.IsConfiguredValue(payload.Settings) && payload.Settings.ValueString() != "" {
			var settingsMap map[string]any
			if err := json.Unmarshal([]byte(payload.Settings.ValueString()), &settingsMap); err != nil {
				diags.AddError(
					"Invalid legacy payload settings",
					"settings for payload_type "+entry.PayloadType+" must be a JSON object string (use jsonencode): "+helpers.APIErrorDetail(err),
				)
				return
			}
			entry.Settings = settingsMap
		}
		entries = append(entries, entry)
	}

	r.appendLegacyConfigProfile(allComponents, diags, entries, blueprintName, storedIdentifiers)
}

// appendLegacyConfigProfile assembles the shared com.jamf.ddm-configuration-profile component from
// the flattened legacy payload entries and appends it. It rejects a missing payload type or a
// duplicate payload type.
//
// `payloadIdentifier` is the service's to own, so an authored one is dropped and the stored one
// written back where there is one; a payload the service has not yet stamped is sent without the
// key, for the service to mint. This matches the configuration profile resources, which mask the
// field from the diff unconditionally and overwrite an authored value with the stored one before
// every write. Dropping it after the authored settings are merged in is deliberate: settings is a
// free-form object, so the key can reach here from configuration, and the service honours whatever
// arrives.
func (r *BlueprintResource) appendLegacyConfigProfile(allComponents *[]blueprints.Component, diags *diag.Diagnostics, entries []legacyPayloadEntry, blueprintName string, storedIdentifiers map[string]string) {
	seenPayloadTypes := make(map[string]bool, len(entries))
	payloadArray := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.PayloadType == "" {
			diags.AddError("Missing payload_type", "Each legacy payload must include a payload_type key.")
			return
		}

		if seenPayloadTypes[entry.PayloadType] {
			diags.AddError(
				"Duplicate payload_type",
				"Legacy payloads must not contain duplicate payload types. Found duplicate: "+entry.PayloadType,
			)
			return
		}
		seenPayloadTypes[entry.PayloadType] = true

		payload := map[string]any{"payloadType": entry.PayloadType}
		maps.Copy(payload, entry.Settings)
		delete(payload, "payloadIdentifier")
		if identifier, stored := storedIdentifiers[entry.PayloadType]; stored {
			payload["payloadIdentifier"] = identifier
		}
		payloadArray = append(payloadArray, payload)
	}

	config := map[string]any{
		"payloadDisplayName": blueprintName,
		"payloadContent":     payloadArray,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		diags.AddError("Error encoding legacy payloads configuration", "Could not encode configuration to JSON: "+helpers.APIErrorDetail(err))
		return
	}

	*allComponents = append(*allComponents, blueprints.Component{
		Identifier:    legacyConfigProfileIdentifier,
		Configuration: json.RawMessage(configJSON),
	})
}
