// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// storedLegacyPayloadIdentifiers holds the `payloadIdentifier` the blueprints service has already
// assigned to each stored legacy payload, so an update can send the value back rather than let the
// service mint a replacement.
//
// The service owns this field. It is absent from the Blueprints API specification — which requires
// only `payloadType` in a `payloadContent` entry — and wire probing on 2026-09-16 established that a
// payload sent without one is accepted and stamped with a fresh UUID **on every write**. Because the
// update is a whole-body merge-patch, letting it mint would re-identify every payload in the
// blueprint whenever any part of it changed, and Apple keys an installed payload on its identifier,
// so each unrelated edit would reinstall every profile on every scoped device. Jamf's own web app
// avoids that by reading the blueprint, mutating one field and writing the stored identifiers back;
// this type is that read half. It mirrors the configuration profile resources, where
// payloadhelpers.InjectTopLevelIdentifierValues carries the stored identifier onto the outgoing
// payload for the same reason.
//
// A payload is located by step and by `payloadType`, the same pairing checkLegacyPayloadDiscards
// uses, because a type is unique within a block (appendLegacyConfigProfile rejects a duplicate).
// Steps are matched by name ahead of position so that inserting or reordering a block keeps every
// other block's identifiers: a positional match alone would shift them onto neighbouring payloads.
// Position remains the fallback, since a block name is optional and need not be unique.
type storedLegacyPayloadIdentifiers struct {
	stepIndexByName map[string]int
	byStepIndex     []map[string]string
}

// newStoredLegacyPayloadIdentifiers reads the stored identifiers out of a blueprint as fetched from
// the service. A nil blueprint yields nil, which callers treat as "nothing stored yet" — the create
// path, where every identifier is the service's to mint.
func newStoredLegacyPayloadIdentifiers(blueprint *blueprints.BlueprintDetail) *storedLegacyPayloadIdentifiers {
	if blueprint == nil {
		return nil
	}

	stored := &storedLegacyPayloadIdentifiers{
		stepIndexByName: make(map[string]int, len(blueprint.Steps)),
		byStepIndex:     make([]map[string]string, 0, len(blueprint.Steps)),
	}
	duplicateNames := make(map[string]bool, len(blueprint.Steps))

	for i, step := range blueprint.Steps {
		stored.byStepIndex = append(stored.byStepIndex, legacyPayloadIdentifiersInStep(step))

		if step.Name == nil || *step.Name == "" {
			continue
		}
		if _, seen := stored.stepIndexByName[*step.Name]; seen {
			duplicateNames[*step.Name] = true
			continue
		}
		stored.stepIndexByName[*step.Name] = i
	}

	// A name two steps share cannot say which of them a block means, so it names neither and both
	// fall back to position.
	for name := range duplicateNames {
		delete(stored.stepIndexByName, name)
	}

	return stored
}

// legacyPayloadIdentifiersInStep maps payload type to stored identifier for a step's legacy
// configuration profile component. A payload the service has not stamped is omitted rather than
// mapped to the empty string, so a caller cannot mistake it for a stored value.
func legacyPayloadIdentifiersInStep(step blueprints.BlueprintStep) map[string]string {
	identifiers := make(map[string]string)
	for _, component := range step.Components {
		if component.Identifier != legacyConfigProfileIdentifier {
			continue
		}

		var configuration struct {
			PayloadContent []struct {
				PayloadType       string `json:"payloadType"`
				PayloadIdentifier string `json:"payloadIdentifier"`
			} `json:"payloadContent"`
		}
		if err := json.Unmarshal(component.Configuration, &configuration); err != nil {
			continue
		}
		for _, payload := range configuration.PayloadContent {
			if payload.PayloadType == "" || payload.PayloadIdentifier == "" {
				continue
			}
			identifiers[payload.PayloadType] = payload.PayloadIdentifier
		}
	}
	return identifiers
}

// resolve pairs each component block with the stored identifiers of the step it continues, one entry
// per block and in block order. A block with no stored counterpart gets nil, so every one of its
// payloads is the service's to mint.
//
// Matching runs in two passes, and the order matters. The first claims every step whose name a block
// names, so inserting or reordering a block keeps the other blocks' identifiers. The second fills
// the blocks left over from the unclaimed steps by position, which is all an unnamed block has.
//
// A step is claimed at most once across both passes. Without that, a positional match could take a
// step that a later block goes on to claim by name, and the same identifier would be written into
// two profiles — which is worse than minting a new one, because Apple keys an installed payload on
// its identifier and two profiles would then be fighting over the same payload on the device.
func (s *storedLegacyPayloadIdentifiers) resolve(blockNames []types.String) []map[string]string {
	resolved := make([]map[string]string, len(blockNames))
	if s == nil {
		return resolved
	}

	claimed := make([]bool, len(s.byStepIndex))
	matchedByName := make([]bool, len(blockNames))

	for i, name := range blockNames {
		if !helpers.IsConfiguredValue(name) || name.ValueString() == "" {
			continue
		}
		index, ok := s.stepIndexByName[name.ValueString()]
		if !ok || claimed[index] {
			continue
		}
		resolved[i] = s.byStepIndex[index]
		claimed[index] = true
		matchedByName[i] = true
	}

	for i := range blockNames {
		if matchedByName[i] || i >= len(s.byStepIndex) || claimed[i] {
			continue
		}
		resolved[i] = s.byStepIndex[i]
		claimed[i] = true
	}

	return resolved
}

// describeBlueprintBlocks renders one numbered line per component block, naming the block and the
// components it carries, for the diagnostic that lists what a flat-mode configuration cannot hold
// (see checkFlatModeBlockLoss). An unnamed block is labelled so its position stays readable.
func describeBlueprintBlocks(steps []blueprints.BlueprintStep) string {
	var b strings.Builder
	for i, step := range steps {
		name := "(unnamed)"
		if step.Name != nil && *step.Name != "" {
			name = *step.Name
		}

		identifiers := make([]string, 0, len(step.Components))
		for _, comp := range step.Components {
			identifiers = append(identifiers, comp.Identifier)
		}

		components := "no components"
		if len(identifiers) > 0 {
			components = strings.Join(identifiers, ", ")
		}

		fmt.Fprintf(&b, "  %d. %s — %s\n", i+1, name, components)
	}
	return b.String()
}

// setNestedValue sets a value in a nested map structure using underscore notation.
func setNestedValue(obj map[string]any, key string, value string) {
	parts := strings.Split(key, "_")
	current := obj

	for i := range len(parts) - 1 {
		if current[parts[i]] == nil {
			current[parts[i]] = make(map[string]any)
		}
		if nested, ok := current[parts[i]].(map[string]any); ok {
			current = nested
		} else {
			current[parts[i]] = make(map[string]any)
			current = current[parts[i]].(map[string]any)
		}
	}

	finalKey := parts[len(parts)-1]
	if value == "" {
		current[finalKey] = nil
	} else if value == "true" {
		current[finalKey] = true
	} else if value == "false" {
		current[finalKey] = false
	} else if num, err := strconv.Atoi(value); err == nil {
		current[finalKey] = num
	} else {
		if strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{") {
			var jsonValue any
			if err := json.Unmarshal([]byte(value), &jsonValue); err == nil {
				current[finalKey] = jsonValue
				return
			}
		}
		current[finalKey] = value
	}
}

// flattenJSON flattens a nested JSON object into a flat map with underscore notation keys.
func flattenJSON(obj map[string]any, prefix string, result map[string]string) {
	for key, value := range obj {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "_" + key
		}

		switch v := value.(type) {
		case map[string]any:
			flattenJSON(v, fullKey, result)
		case nil:
			result[fullKey] = ""
		case bool:
			result[fullKey] = strconv.FormatBool(v)
		case float64:
			result[fullKey] = strconv.FormatFloat(v, 'f', -1, 64)
		case int:
			result[fullKey] = strconv.Itoa(v)
		case string:
			result[fullKey] = v
		default:
			if jsonBytes, err := json.Marshal(v); err == nil {
				result[fullKey] = string(jsonBytes)
			} else {
				result[fullKey] = ""
			}
		}
	}
}

// desiredDeployedValue returns the desired deployed state based on the provided types.Bool value.
func desiredDeployedValue(v types.Bool) bool {
	if !helpers.IsConfiguredValue(v) {
		return true
	}
	return v.ValueBool()
}

// reconcileBlueprintDeployment ensures the blueprint's deployment state matches the desired state.
func (r *BlueprintResource) reconcileBlueprintDeployment(ctx context.Context, blueprintID string, desiredDeployed bool) (*blueprints.BlueprintDetail, error) {
	blueprint, err := r.client.GetBlueprint(ctx, blueprintID)
	if err != nil {
		return nil, err
	}

	deployedState := ""
	if blueprint.DeploymentState != nil {
		deployedState = blueprint.DeploymentState.State
	}

	if desiredDeployed {
		if !strings.EqualFold(deployedState, blueprintDeploymentStateDeployed) {
			if err := r.client.DeployBlueprint(ctx, blueprintID); err != nil {
				return blueprint, err
			}
			return r.client.GetBlueprint(ctx, blueprintID)
		}
		return blueprint, nil
	}

	if strings.EqualFold(deployedState, blueprintDeploymentStateNotDeployed) {
		return blueprint, nil
	}

	if err := r.client.UndeployBlueprint(ctx, blueprintID); err != nil {
		return blueprint, err
	}

	return r.client.GetBlueprint(ctx, blueprintID)
}

// scopeDeviceGroups safely extracts device group IDs from an optional blueprint scope.
func scopeDeviceGroups(scope *blueprints.BlueprintScope) []string {
	if scope == nil {
		return []string{}
	}
	return scope.DeviceGroups
}

// isDeleteMaybeComplete reports whether a failed DELETE could still have
// removed the blueprint, which is the only case where Delete is entitled to
// warn instead of erroring and let the framework drop the resource from state.
//
// A Jamf-produced 500 qualifies: the service has been observed completing the
// delete and then failing the response. An edge error page does not, however it
// is statused — a CDN, WAF or gateway 500 means the request never arrived, so
// the blueprint is still live and still deployed, and dropping it from state
// would have a later apply recreate it alongside itself.
func isDeleteMaybeComplete(err error) bool {
	return helpers.IsServerError(err) && !helpers.IsEdgeBlocked(err)
}

// readStoredLegacyPayloadIdentifiers fetches the blueprint so an update can write its stored legacy
// payload identifiers back rather than let the service mint replacements. This is the read half of
// the read-merge-write the Jamf web app performs, and the reason it cannot be served from Terraform
// state: the identifiers are masked out of state on read, because the service owns them and an
// authored value is discarded.
//
// A failed read is an error rather than a fallback to minting. Proceeding would silently re-identify
// every legacy payload in the blueprint, reinstalling the profiles on every scoped device, which is
// worse than a failed apply the operator can retry.
func (r *BlueprintResource) readStoredLegacyPayloadIdentifiers(ctx context.Context, blueprintID string) (*storedLegacyPayloadIdentifiers, diag.Diagnostics) {
	var diags diag.Diagnostics

	blueprint, err := r.client.GetBlueprint(ctx, blueprintID)
	if err != nil {
		diags.AddError(
			"Error reading blueprint before update",
			"Could not read the blueprint's stored legacy payload identifiers, which an update must preserve: "+helpers.APIErrorDetail(err),
		)
		return nil, diags
	}

	return newStoredLegacyPayloadIdentifiers(blueprint), diags
}
