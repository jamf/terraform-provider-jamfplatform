// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

// updateModelFromAPIResponse updates the Terraform model with data from the API response. It
// selects between the deprecated flat top-level attributes and ordered component_blocks based on
// which the prior model used (a fresh/empty model — import, list resource — defaults to blocks).
func updateModelFromAPIResponse(ctx context.Context, model *BlueprintResourceModel, blueprint *blueprints.BlueprintDetail) diag.Diagnostics {
	var diags diag.Diagnostics

	blockMode := len(model.ComponentBlocks) > 0 || !model.hasFlatComponents()

	model.ID = types.StringValue(blueprint.ID)
	model.Name = types.StringValue(blueprint.Name)
	model.Description = helpers.ReconcileOptionalStringPointer(blueprint.Description, model.Description)
	model.Created = types.StringValue(blueprint.Created.Format(time.RFC3339))
	model.Updated = types.StringValue(blueprint.Updated.Format(time.RFC3339))
	if blueprint.DeploymentState != nil {
		model.DeploymentState = types.StringValue(blueprint.DeploymentState.State)
		model.Deployed = types.BoolValue(strings.EqualFold(blueprint.DeploymentState.State, blueprintDeploymentStateDeployed))
	} else {
		model.DeploymentState = types.StringValue("")
		model.Deployed = types.BoolValue(false)
	}

	deviceGroupsSet, _ := types.SetValueFrom(context.Background(), types.StringType, scopeDeviceGroups(blueprint.Scope))
	model.DeviceGroups = deviceGroupsSet

	if blockMode {
		updateComponentBlocksFromAPI(ctx, &diags, model, blueprint)
	} else {
		updateFlatComponentsFromAPI(ctx, &diags, model, blueprint)
	}

	model.Timeouts = helpers.EnsureResourceTimeouts(model.Timeouts, blueprintTimeoutAttributeTypes)
	return diags
}

// updateComponentBlocksFromAPI populates model.ComponentBlocks from every wire step (block mode)
// and clears the deprecated flat attributes so state carries a single representation.
func updateComponentBlocksFromAPI(ctx context.Context, diags *diag.Diagnostics, model *BlueprintResourceModel, blueprint *blueprints.BlueprintDetail) {
	prior := model.ComponentBlocks
	blocks := make([]ComponentBlockModel, 0, len(blueprint.Steps))

	for i, step := range blueprint.Steps {
		var priorRaw map[string]struct{}
		var priorLegacy []BlockLegacyPayloadModel
		var priorDeclarations []AppleDeclarationModel
		priorName := types.StringNull()
		priorActivation := types.StringNull()
		if i < len(prior) {
			priorRaw = rawIdentifierSet(prior[i].Components)
			priorLegacy = prior[i].LegacyPayloads
			priorDeclarations = prior[i].AppleDeclarations
			priorName = prior[i].Name
			priorActivation = prior[i].ActivationConditions
		}

		block, apiComponentsByID := mapStepComponents(ctx, diags, step, priorRaw, false)
		block.Name = helpers.ReconcileOptionalStringPointer(step.Name, priorName)
		block.ActivationConditions = helpers.ReconcileOptionalStringPointer(step.ActivationPredicate, priorActivation)
		block.LegacyPayloads = flattenBlockLegacyPayloads(priorLegacy, apiComponentsByID, priorRaw)
		block.AppleDeclarations = flattenAppleDeclarations(diags, priorDeclarations, apiComponentsByID, priorRaw)
		blocks = append(blocks, block)
	}

	if len(blocks) > 0 {
		model.ComponentBlocks = blocks
	} else {
		model.ComponentBlocks = nil
	}

	model.applyFlatComponentsFromBlock(ComponentBlockModel{})
	model.LegacyPayloads = types.DynamicNull()
	model.ActivationConditions = types.StringNull()
}

// updateFlatComponentsFromAPI populates the deprecated flat top-level attributes from the first
// step (flat mode). When the blueprint has more than one step it also emits a migration warning:
// the flat attributes cannot represent the extra blocks, so applying would collapse them.
//
// It asks mapStepComponents to keep a blockOnlyComponentIdentifiers component in raw_component,
// because the flat attributes have no field for one and dropping it from state would delete it from
// the blueprint on the next apply without a plan diff.
func updateFlatComponentsFromAPI(ctx context.Context, diags *diag.Diagnostics, model *BlueprintResourceModel, blueprint *blueprints.BlueprintDetail) {
	priorRaw := rawIdentifierSet(model.Components)

	var step blueprints.BlueprintStep
	if len(blueprint.Steps) > 0 {
		step = blueprint.Steps[0]
	}

	block, apiComponentsByID := mapStepComponents(ctx, diags, step, priorRaw, true)
	model.applyFlatComponentsFromBlock(block)
	model.LegacyPayloads = flattenFlatLegacyPayloads(model.LegacyPayloads, apiComponentsByID, priorRaw)
	model.ActivationConditions = helpers.ReconcileOptionalStringPointer(step.ActivationPredicate, model.ActivationConditions)

	if len(blueprint.Steps) > 1 {
		diags.AddWarning(
			"Blueprint has multiple component blocks",
			"This blueprint has multiple component blocks, but the configuration uses the deprecated top-level component attributes, which can only represent the first block. "+
				"Only the first block is reflected in state, and applying this configuration would remove the others. Migrate to `component_blocks` to manage every block.",
		)
	}
}

// rawIdentifierSet collects the raw_component identifiers a prior model authored, so components
// that also have a strongly-typed representation stay in raw_component when the user chose it.
func rawIdentifierSet(components []ComponentModel) map[string]struct{} {
	identifiers := make(map[string]struct{}, len(components))
	for _, comp := range components {
		if identifier := comp.Identifier.ValueString(); identifier != "" {
			identifiers[identifier] = struct{}{}
		}
	}
	return identifiers
}

// mapStepComponents converts one wire step's raw and strongly-typed components into a
// ComponentBlockModel carrier, and returns the step's components keyed by identifier so the caller
// can flatten the components assembled here rather than in the components package. It leaves Name,
// ActivationConditions, LegacyPayloads and AppleDeclarations unset — the caller reconciles those.
//
// retainBlockOnlyAsRaw keeps a blockOnlyComponentIdentifiers component in raw_component instead of
// skipping it as strongly typed, and only the flat-mode caller sets it: block mode has a typed
// attribute for every such component, so retaining it there would populate the typed attribute and
// raw_component at once. The mode is threaded in rather than the flat caller appending the fallback
// afterwards, so a retained component keeps its position in the platform's component order.
func mapStepComponents(ctx context.Context, diags *diag.Diagnostics, step blueprints.BlueprintStep, priorRawIdentifiers map[string]struct{}, retainBlockOnlyAsRaw bool) (ComponentBlockModel, map[string]blueprints.Component) {
	var block ComponentBlockModel

	apiComponentsByID := make(map[string]blueprints.Component)
	var rawComponents []ComponentModel

	for _, comp := range step.Components {
		apiComponentsByID[comp.Identifier] = comp

		_, handledAsRaw := priorRawIdentifiers[comp.Identifier]
		_, isTyped := stronglyTypedComponentIdentifiers[comp.Identifier]
		_, blockOnly := blockOnlyComponentIdentifiers[comp.Identifier]
		if isTyped && !handledAsRaw && (!retainBlockOnlyAsRaw || !blockOnly) {
			continue
		}

		configMap := make(map[string]string)
		if comp.Configuration != nil {
			var jsonObj map[string]any
			if err := json.Unmarshal(comp.Configuration, &jsonObj); err == nil {
				flattenJSON(jsonObj, "", configMap)
			}
		}
		configMapValue, _ := types.MapValueFrom(ctx, types.StringType, configMap)

		rawComponents = append(rawComponents, ComponentModel{
			Identifier:    types.StringValue(comp.Identifier),
			Configuration: configMapValue,
		})
	}

	if len(rawComponents) > 0 {
		block.Components = rawComponents
	}

	updateStronglyTypedComponentsFromAPI(diags, &block, apiComponentsByID, priorRawIdentifiers)
	return block, apiComponentsByID
}

// updateStronglyTypedComponentsFromAPI updates all strongly-typed components of a block from the
// API response.
func updateStronglyTypedComponentsFromAPI(diags *diag.Diagnostics, block *ComponentBlockModel, apiComponentsByID map[string]blueprints.Component, rawIdentifiers map[string]struct{}) {
	block.AudioAccessorySettings = buildTypedComponent[components.AudioAccessorySettingsComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.audio-accessory-settings", func(raw json.RawMessage, target *components.AudioAccessorySettingsComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.AIGovernance = buildTypedComponent[components.AIGovernanceComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ai-governance", func(raw json.RawMessage, target *components.AIGovernanceComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.CustomDeclarations = buildTypedComponent[components.CustomDeclarationsComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.custom-declarations", func(raw json.RawMessage, target *components.CustomDeclarationsComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.DiskManagementSettings = buildTypedComponent[components.DiskManagementPolicyComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.disk-management", func(raw json.RawMessage, target *components.DiskManagementPolicyComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.MathSettings = buildTypedComponent[components.MathSettingsComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.math-settings", func(raw json.RawMessage, target *components.MathSettingsComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.PasscodePolicy = buildTypedComponent[components.PasscodePolicyComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.passcode-settings", func(raw json.RawMessage, target *components.PasscodePolicyComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.SafariBookmarks = buildTypedComponent[components.SafariBookmarksComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.safari-bookmarks", func(raw json.RawMessage, target *components.SafariBookmarksComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.SafariExtensions = buildTypedComponent[components.SafariExtensionsComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.safari-extensions", func(raw json.RawMessage, target *components.SafariExtensionsComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.SafariSettings = buildTypedComponent[components.SafariSettingsComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.safari-settings", func(raw json.RawMessage, target *components.SafariSettingsComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.ServiceBackgroundTasks = buildTypedComponent[components.ServiceBackgroundTasksComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.service-background-tasks", func(raw json.RawMessage, target *components.ServiceBackgroundTasksComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.ServiceConfigurationFiles = buildTypedComponent[components.ServiceConfigurationFilesComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.service-configuration-files", func(raw json.RawMessage, target *components.ServiceConfigurationFilesComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.SoftwareUpdate = buildTypedComponent[components.SoftwareUpdateComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.sw-updates", func(raw json.RawMessage, target *components.SoftwareUpdateComponent) error {
		return target.FromRawConfiguration(raw)
	})

	block.SoftwareUpdateSettings = buildTypedComponent[components.SoftwareUpdateSettingsComponent](diags, apiComponentsByID, rawIdentifiers, "com.jamf.ddm.software-update-settings", func(raw json.RawMessage, target *components.SoftwareUpdateSettingsComponent) error {
		return target.FromRawConfiguration(raw)
	})
}

// buildTypedComponent is a generic helper to build a strongly-typed singleton component pointer.
//
// A configuration the typed shape cannot decode leaves the component absent, and every typed
// component attribute is Optional without being Computed, so a configuration that declares the
// block would then show a permanent diff proposing to add it. That is indistinguishable from a
// component Jamf never returned, so the decode failure is reported as a warning naming the
// identifier rather than swallowed. It stays a warning, not an error: the rest of the blueprint
// reads correctly, and failing the read would make an otherwise-manageable blueprint unusable.
func buildTypedComponent[T any](diags *diag.Diagnostics, apiComponentsByID map[string]blueprints.Component, rawIdentifiers map[string]struct{}, identifier string, populate func(json.RawMessage, *T) error) *T {
	if _, handledAsRaw := rawIdentifiers[identifier]; handledAsRaw {
		return nil
	}

	config, ok := parseComponentConfiguration(apiComponentsByID, identifier)
	if !ok {
		return nil
	}

	var component T
	if err := populate(config, &component); err != nil {
		appendUndecodableComponentWarning(diags, identifier, err)
		return nil
	}

	return &component
}

// appendUndecodableComponentWarning reports a component configuration this provider cannot read,
// naming the identifier so an operator can find it in the Jamf Pro editor. See buildTypedComponent
// for why this is a warning rather than an error.
func appendUndecodableComponentWarning(diags *diag.Diagnostics, identifier string, err error) {
	diags.AddWarning(
		"Blueprint component configuration could not be read",
		"Jamf returned a configuration for the "+identifier+" component that this provider could not decode, so "+
			"the component is absent from state. A configuration declaring it will keep proposing to add it until "+
			"the configuration Jamf holds can be read. Reported while decoding: "+err.Error(),
	)
}

// parseComponentConfiguration returns the raw JSON configuration of a component by its identifier.
func parseComponentConfiguration(apiComponentsByID map[string]blueprints.Component, identifier string) (json.RawMessage, bool) {
	apiComp, exists := apiComponentsByID[identifier]
	if !exists || apiComp.Configuration == nil {
		return nil, false
	}
	return apiComp.Configuration, true
}

// legacyPayloadItems extracts the legacy configuration profile's payloads from the wire as a list
// of `{payload_type, settings}` maps (settings present only when non-empty), or nil when the
// component is absent or malformed. The caller decides how to render them (dynamic or JSON string).
//
// The blueprints service stamps its own metadata onto every payload it stores (see
// serverStampedPayloadKeys), so a payload's settings are masked against what the author declared:
// a stamped key the author did not write is dropped, and one the author did write is kept, since
// the service echoes an authored value back verbatim. priorSettingsByType carries the author's
// settings keyed by payload type, and may be nil (import — nothing authored to mask against).
func legacyPayloadItems(apiComponentsByID map[string]blueprints.Component, priorSettingsByType map[string]map[string]any) []any {
	rawJSON, ok := parseComponentConfiguration(apiComponentsByID, "com.jamf.ddm-configuration-profile")
	if !ok {
		return nil
	}

	var config map[string]any
	if err := json.Unmarshal(rawJSON, &config); err != nil {
		return nil
	}

	payloadArray, ok := config["payloadContent"].([]any)
	if !ok {
		return nil
	}

	var items []any
	for _, item := range payloadArray {
		payloadMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		payloadType, _ := payloadMap["payloadType"].(string)

		settingsMap := make(map[string]any, len(payloadMap))
		for k, v := range payloadMap {
			if k == "payloadType" || k == "payloadIdentifier" {
				continue
			}
			settingsMap[k] = v
		}
		maskServerStampedPayloadKeys(settingsMap, priorSettingsByType[payloadType])
		if restored, ok := restoreRedactedValues(settingsMap, priorSettingsByType[payloadType]).(map[string]any); ok {
			settingsMap = restored
		}

		entry := map[string]any{"payload_type": payloadType}
		if len(settingsMap) > 0 {
			entry["settings"] = settingsMap
		}
		items = append(items, entry)
	}

	if len(items) == 0 {
		return nil
	}
	return items
}

// serverStampedPayloadKeys are the per-payload metadata keys the blueprints service writes onto
// every legacy payload it stores, whether or not the author supplied them. `payloadType` and
// `payloadIdentifier` are not listed: legacyPayloadItems already lifts those out of settings, the
// first into `payload_type` and the second because the provider derives it from the payload type
// (see generatePayloadIdentifier).
var serverStampedPayloadKeys = [...]string{
	"payloadDisplayName",
	"payloadOrganization",
	"payloadUUID",
	"payloadVersion",
}

// maskServerStampedPayloadKeys deletes from a payload's server-derived settings every metadata key
// the service stamps on that the author did not declare, so the stamp never reads as a settings
// change. A stamped key the author did declare is left in place, because the service preserves an
// authored value and state must keep reflecting it.
func maskServerStampedPayloadKeys(settings map[string]any, priorSettings map[string]any) {
	for _, key := range serverStampedPayloadKeys {
		if _, authored := priorSettings[key]; authored {
			continue
		}
		delete(settings, key)
	}
}

// redactionSentinel is what Jamf returns in place of a payload value it treats as a credential: a
// run of asterisks, never the value that was written. Wire probing found it on the Wi-Fi payload's
// Password and on EAPClientConfiguration's OuterIdentity, UserName and UserPassword, while
// com.apple.loginwindow's AutologinPassword came back in the clear — so which keys are redacted is
// Jamf's decision, not something Apple's schema describes. The provider therefore recognises the
// value rather than a list of keys.
const redactionSentinel = '*'

// minRedactionLength is the shortest run of asterisks treated as a redaction, so a genuine short
// value made of asterisks is not mistaken for one. Jamf's own sentinel is ten characters.
const minRedactionLength = 4

// isRedacted reports whether a wire value is Jamf's redaction sentinel rather than a real value.
func isRedacted(value any) bool {
	text, ok := value.(string)
	if !ok || len(text) < minRedactionLength {
		return false
	}
	for _, r := range text {
		if r != redactionSentinel {
			return false
		}
	}
	return true
}

// restoreRedactedValues walks a payload's server-derived settings alongside what the author wrote and
// puts the authored value back wherever Jamf returned a redaction. Without this, a payload carrying a
// credential can never settle: the wire value differs from configuration on every read, so the plan
// shows a change that applying cannot resolve.
//
// Only a redacted leaf is substituted, and only where the author actually wrote something at the same
// path. On import there is nothing to restore from, so the sentinel stays in state — the value is not
// recoverable from the service, and inventing one would be worse than showing what it returned.
func restoreRedactedValues(wire, authored any) any {
	switch typed := wire.(type) {
	case map[string]any:
		authoredMap, ok := authored.(map[string]any)
		if !ok {
			return wire
		}
		for key, value := range typed {
			authoredValue, present := authoredMap[key]
			if !present {
				continue
			}
			typed[key] = restoreRedactedValues(value, authoredValue)
		}
		return typed
	case []any:
		authoredSlice, ok := authored.([]any)
		if !ok {
			return wire
		}
		for i, value := range typed {
			if i >= len(authoredSlice) {
				break
			}
			typed[i] = restoreRedactedValues(value, authoredSlice[i])
		}
		return typed
	default:
		if isRedacted(wire) {
			if _, ok := authored.(string); ok {
				return authored
			}
		}
		return wire
	}
}

// pruneJSONNulls returns value with every null-valued object key removed, recursively through
// objects and arrays. A legacy configuration profile payload's null-valued key is discarded rather
// than stored, so a settings object authored with explicit nulls (com.apple.notificationsettings is
// the common case) never comes back key-for-key; a declaration payload's null is stored instead, so
// there the pruning runs on both sides and cancels out (see jsonStringMatchesObject). Pruning
// before comparing lets an author keep their nulls in configuration without manufacturing a diff.
// A key the author gave a
// non-null value keeps its place, so a genuine discard — a key the service does not recognise, which
// it drops silently — still reads as a mismatch instead of being masked away.
func pruneJSONNulls(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		pruned := make(map[string]any, len(typed))
		for k, v := range typed {
			if v == nil {
				continue
			}
			pruned[k] = pruneJSONNulls(v)
		}
		return pruned
	case []any:
		pruned := make([]any, 0, len(typed))
		for _, v := range typed {
			pruned = append(pruned, pruneJSONNulls(v))
		}
		return pruned
	default:
		return value
	}
}

// flattenFlatLegacyPayloads renders the wire legacy payloads into the deprecated top-level dynamic
// value. When the user manages the configuration profile as a raw_component it is left untouched;
// when the server value is semantically identical to the prior value that prior value is preserved
// (the dynamic null-typing reconcile — see dynamicPayloadsMatchJSON).
func flattenFlatLegacyPayloads(prior types.Dynamic, apiComponentsByID map[string]blueprints.Component, rawIdentifiers map[string]struct{}) types.Dynamic {
	if _, handledAsRaw := rawIdentifiers["com.jamf.ddm-configuration-profile"]; handledAsRaw {
		return prior
	}

	items := legacyPayloadItems(apiComponentsByID, priorSettingsFromDynamic(prior))
	if len(items) == 0 {
		return types.DynamicNull()
	}

	if dynamicPayloadsMatchJSON(prior, items) {
		return prior
	}

	dynVal, err := helpers.JSONToTerraformDynamic(items)
	if err != nil {
		return types.DynamicNull()
	}
	return dynVal
}

// priorSettingsFromDynamic reads the author's per-payload settings out of the deprecated top-level
// dynamic value, keyed by payload type, so the wire payloads can be masked against what was
// actually written. It returns nil when the prior value carries nothing usable (import, or a first
// create with no prior state).
func priorSettingsFromDynamic(prior types.Dynamic) map[string]map[string]any {
	if prior.IsNull() || prior.IsUnknown() {
		return nil
	}

	raw, err := helpers.TerraformDynamicToJSON(prior)
	if err != nil {
		return nil
	}

	items, ok := raw.([]any)
	if !ok {
		return nil
	}

	settingsByType := make(map[string]map[string]any, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		payloadType, _ := obj["payload_type"].(string)
		if settings, ok := obj["settings"].(map[string]any); ok {
			settingsByType[payloadType] = settings
		}
	}
	return settingsByType
}

// priorSettingsFromBlockPayloads reads the author's per-payload settings out of a block's typed
// legacy payload list, keyed by payload type, decoding each settings JSON string. It returns nil
// when nothing was authored.
func priorSettingsFromBlockPayloads(prior []BlockLegacyPayloadModel) map[string]map[string]any {
	if len(prior) == 0 {
		return nil
	}

	settingsByType := make(map[string]map[string]any, len(prior))
	for _, entry := range prior {
		if !helpers.IsConfiguredValue(entry.Settings) {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(entry.Settings.ValueString()), &settings); err != nil {
			continue
		}
		settingsByType[entry.PayloadType.ValueString()] = settings
	}
	return settingsByType
}

// flattenAppleDeclarations renders the wire Apple declarations into a block's declaration list.
// When the author manages the component as a raw_component the prior value is left untouched.
//
// Each prior payload string is kept when it is semantically identical to the server value, so a
// payload read from a file, or a jsonencode() the platform re-serialised, stays as the author wrote
// it. That is what makes file() a usable way to author one, and it is the same reconciliation a
// legacy payload's settings gets. Position is the identity — payloadKey is derived from it — so the
// prior declaration at index i is the one the server's index i came from; two declarations of one
// type in a block are legal, so matching by type would reconcile the wrong one.
//
// No declarations reads as absent rather than as an empty list: an empty list writes no component,
// so a non-null empty list in state could only ever diff.
func flattenAppleDeclarations(diags *diag.Diagnostics, prior []AppleDeclarationModel, apiComponentsByID map[string]blueprints.Component, rawIdentifiers map[string]struct{}) []AppleDeclarationModel {
	if _, handledAsRaw := rawIdentifiers[appleDeclarationsIdentifier]; handledAsRaw {
		return prior
	}

	rawJSON, ok := parseComponentConfiguration(apiComponentsByID, appleDeclarationsIdentifier)
	if !ok {
		return nil
	}

	var config blueprints.CustomDeclarationsConfiguration
	if err := json.Unmarshal(rawJSON, &config); err != nil {
		appendUndecodableComponentWarning(diags, appleDeclarationsIdentifier, err)
		return nil
	}
	if len(config.Declarations) == 0 {
		return nil
	}

	declarations := make([]AppleDeclarationModel, 0, len(config.Declarations))
	for i, declaration := range config.Declarations {
		entry := AppleDeclarationModel{
			ChannelType: types.StringValue(declaration.ChannelType),
			Type:        types.StringValue(declaration.Type),
		}

		switch {
		case i < len(prior) && jsonStringMatchesObject(prior[i].Payload, declaration.Payload):
			entry.Payload = prior[i].Payload
		default:
			encoded, err := json.Marshal(declaration.Payload)
			if err != nil {
				appendUndecodableComponentWarning(diags, appleDeclarationsIdentifier, err)
				return nil
			}
			entry.Payload = types.StringValue(string(encoded))
		}

		declarations = append(declarations, entry)
	}
	return declarations
}

// flattenBlockLegacyPayloads renders the wire legacy payloads into a block's typed list, with each
// payload's settings as a canonical JSON string. When the user manages the configuration profile as
// a raw_component the prior value is left untouched. For each payload, the prior settings string is
// preserved when it is semantically identical to the server value, keeping diffs stable.
func flattenBlockLegacyPayloads(prior []BlockLegacyPayloadModel, apiComponentsByID map[string]blueprints.Component, rawIdentifiers map[string]struct{}) []BlockLegacyPayloadModel {
	if _, handledAsRaw := rawIdentifiers["com.jamf.ddm-configuration-profile"]; handledAsRaw {
		return prior
	}

	items := legacyPayloadItems(apiComponentsByID, priorSettingsFromBlockPayloads(prior))
	if len(items) == 0 {
		return nil
	}

	priorByType := make(map[string]types.String, len(prior))
	for _, entry := range prior {
		priorByType[entry.PayloadType.ValueString()] = entry.Settings
	}

	result := make([]BlockLegacyPayloadModel, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		payloadType, _ := obj["payload_type"].(string)
		entry := BlockLegacyPayloadModel{PayloadType: types.StringValue(payloadType)}

		settings, hasSettings := obj["settings"].(map[string]any)
		switch {
		case !hasSettings:
			entry.Settings = types.StringNull()
		case jsonStringMatchesObject(priorByType[payloadType], settings):
			entry.Settings = priorByType[payloadType]
		default:
			if encoded, err := json.Marshal(settings); err == nil {
				entry.Settings = types.StringValue(string(encoded))
			} else {
				entry.Settings = types.StringNull()
			}
		}
		result = append(result, entry)
	}
	return result
}

// jsonStringMatchesObject reports whether a JSON object string the author wrote is semantically
// identical to the object the server returned, comparing canonical JSON encodings (sorted keys,
// float64 numbers) with the explicit nulls pruned from both sides (see pruneJSONNulls). It is what
// keeps an authored JSON string stable when the server echoes an equivalent value.
//
// Both JSON-string attributes in this resource use it — a legacy payload's settings and an Apple
// declaration's payload — because both services accept an object, store it their own way and
// re-serialise it on the way out. The two differ on what becomes of a key whose value is null, and
// pruning both sides is what lets one helper serve both. A legacy configuration profile payload's
// null key is dropped, so the server value has no null to prune and pruning it is a no-op. An Apple
// declaration payload's null key is stored and echoed back verbatim: on the EU gateway, 2026-09-12,
// a POST /blueprints/v1/blueprints carrying payload {"Enabled":true,"ForceProfanityFilter":null} was
// read back with the null intact. Pruning only the authored side would then mismatch, state would
// take the canonical encoding rather than the authored bytes, and because payload is Required the
// framework would reject the apply as an inconsistent result.
func jsonStringMatchesObject(prior types.String, settings map[string]any) bool {
	if prior.IsNull() || prior.IsUnknown() {
		return false
	}

	var priorObj any
	if err := json.Unmarshal([]byte(prior.ValueString()), &priorObj); err != nil {
		return false
	}

	priorBytes, err := json.Marshal(pruneJSONNulls(priorObj))
	if err != nil {
		return false
	}

	settingsBytes, err := json.Marshal(pruneJSONNulls(settings))
	if err != nil {
		return false
	}

	return bytes.Equal(priorBytes, settingsBytes)
}

// dynamicPayloadsMatchJSON reports whether the prior dynamic value is
// semantically identical to the server-derived payload items, comparing their
// canonical JSON encodings with the authored side's explicit nulls pruned (see
// pruneJSONNulls). Numbers normalise to float64 on both sides and
// json.Marshal sorts object keys, so the comparison is order-independent for
// object keys and insensitive to the dynamic null-typing that otherwise causes
// a perpetual diff.
func dynamicPayloadsMatchJSON(prior types.Dynamic, apiItems []any) bool {
	if prior.IsNull() || prior.IsUnknown() {
		return false
	}

	priorJSON, err := helpers.TerraformDynamicToJSON(prior)
	if err != nil {
		return false
	}

	priorBytes, err := json.Marshal(pruneJSONNulls(priorJSON))
	if err != nil {
		return false
	}

	apiBytes, err := json.Marshal(apiItems)
	if err != nil {
		return false
	}

	return bytes.Equal(priorBytes, apiBytes)
}

// checkLegacyPayloadDiscards warns when the blueprints service did not store a legacy payload as it
// was written. The service validates each payload against Apple's profile-specific payload schema
// for that payload type (the same key vocabulary as apple/device-management `mdm/profiles`) and
// silently discards any key the schema does not define, so a mistyped or unsupported key never
// reaches a device and never appears in state — which Terraform then reports only as the opaque
// "provider produced inconsistent result after apply". Key lookup is case-insensitive and the stored
// key is canonicalised to Apple's spelling, so a miscased key is reported here too: it is discarded
// under the spelling the author used. This runs on the write paths, against the planned model before
// state is rebuilt from the response, and names what was dropped so the cause is legible.
//
// A key the author set to null is not reported: the service drops nulls by design and the flatteners
// already tolerate that (see pruneJSONNulls). A key whose value has the wrong type is not reported
// either — the service rejects the whole write with a validation failure, which surfaces as an
// error from the create or update call instead.
func checkLegacyPayloadDiscards(planned *BlueprintResourceModel, blueprint *blueprints.BlueprintDetail) diag.Diagnostics {
	var diags diag.Diagnostics

	if len(planned.ComponentBlocks) > 0 {
		for i, block := range planned.ComponentBlocks {
			var step blueprints.BlueprintStep
			if i < len(blueprint.Steps) {
				step = blueprint.Steps[i]
			}
			appendLegacyPayloadDiscardWarnings(&diags, priorSettingsFromBlockPayloads(block.LegacyPayloads), step, describeBlockPosition(i, block.Name))
		}
		return diags
	}

	var step blueprints.BlueprintStep
	if len(blueprint.Steps) > 0 {
		step = blueprint.Steps[0]
	}
	appendLegacyPayloadDiscardWarnings(&diags, priorSettingsFromDynamic(planned.LegacyPayloads), step, "legacy_payloads")

	return diags
}

// describeBlockPosition names a component block for a diagnostic, by name where it has one and by
// its index either way, so a blueprint with several blocks points at the right one.
func describeBlockPosition(index int, name types.String) string {
	if helpers.IsConfiguredValue(name) && name.ValueString() != "" {
		return fmt.Sprintf("component_blocks[%d] (%s)", index, name.ValueString())
	}
	return fmt.Sprintf("component_blocks[%d]", index)
}

// appendLegacyPayloadDiscardWarnings compares each payload's authored settings against what the
// step actually stored and adds one warning per payload that lost keys.
func appendLegacyPayloadDiscardWarnings(diags *diag.Diagnostics, authoredByType map[string]map[string]any, step blueprints.BlueprintStep, location string) {
	if len(authoredByType) == 0 {
		return
	}

	apiComponentsByID := make(map[string]blueprints.Component, len(step.Components))
	for _, comp := range step.Components {
		apiComponentsByID[comp.Identifier] = comp
	}

	storedByType := make(map[string]map[string]any)
	for _, item := range legacyPayloadItems(apiComponentsByID, authoredByType) {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		payloadType, _ := obj["payload_type"].(string)
		settings, _ := obj["settings"].(map[string]any)
		storedByType[payloadType] = settings
	}

	for _, payloadType := range slices.Sorted(maps.Keys(authoredByType)) {
		discarded := discardedSettingsPaths(authoredByType[payloadType], storedByType[payloadType], "")
		if len(discarded) == 0 {
			continue
		}
		diags.AddWarning(
			"Legacy payload settings were not stored",
			fmt.Sprintf(
				"Jamf did not store %d setting(s) written for the %s payload in %s: %s. "+
					"The platform validates each legacy payload against Apple's payload keys for that payload type and silently drops any key it does not define, "+
					"so these settings will not reach any device and Terraform will report a difference on every plan. "+
					"Check each key against Apple's documentation for this payload type, including its exact capitalisation.",
				len(discarded), payloadType, location, strings.Join(discarded, ", "),
			),
		)
	}
}

// discardedSettingsPaths returns the dotted paths of every non-null value present in authored but
// missing from stored, descending through nested objects and positional array entries so a dropped
// key inside a nested array (com.apple.notificationsettings is the common shape) is named in full.
func discardedSettingsPaths(authored, stored any, path string) []string {
	switch authoredTyped := authored.(type) {
	case map[string]any:
		storedTyped, ok := stored.(map[string]any)
		if !ok {
			storedTyped = nil
		}
		var paths []string
		for _, key := range slices.Sorted(maps.Keys(authoredTyped)) {
			value := authoredTyped[key]
			if value == nil {
				continue
			}
			child := key
			if path != "" {
				child = path + "." + key
			}
			storedValue, present := storedTyped[key]
			if !present {
				paths = append(paths, child)
				continue
			}
			paths = append(paths, discardedSettingsPaths(value, storedValue, child)...)
		}
		return paths
	case []any:
		storedTyped, ok := stored.([]any)
		if !ok {
			return nil
		}
		var paths []string
		for i, value := range authoredTyped {
			if i >= len(storedTyped) {
				break
			}
			paths = append(paths, discardedSettingsPaths(value, storedTyped[i], fmt.Sprintf("%s[%d]", path, i))...)
		}
		return paths
	default:
		return nil
	}
}
