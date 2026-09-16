// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

const payloadIdentifierNamespaceKey = "legacy_payload_identifier_namespace"

type privateStateReader interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}

type privateStateWriter interface {
	SetKey(context.Context, string, []byte) diag.Diagnostics
}

// ensurePayloadIdentifierNamespace returns the resource's stable private namespace, creating it
// when a resource predates namespaced legacy payload identifiers or has just been imported.
func ensurePayloadIdentifierNamespace(ctx context.Context, reader privateStateReader, writer privateStateWriter) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if reader != nil {
		raw, readDiags := reader.GetKey(ctx, payloadIdentifierNamespaceKey)
		diags.Append(readDiags...)
		if diags.HasError() {
			return "", diags
		}
		if len(raw) > 0 {
			var namespace string
			if err := json.Unmarshal(raw, &namespace); err != nil || namespace == "" {
				diags.AddError("Invalid legacy payload identifier namespace", "The Blueprint's private state contains an invalid legacy payload identifier namespace.")
				return "", diags
			}
			return namespace, diags
		}
	}

	namespace, err := uuid.GenerateUUID()
	if err != nil {
		diags.AddError("Could not generate legacy payload identifier namespace", helpers.APIErrorDetail(err))
		return "", diags
	}
	if writer == nil {
		diags.AddError("Missing private state", "The provider could not persist the legacy payload identifier namespace.")
		return "", diags
	}
	raw, err := json.Marshal(namespace)
	if err != nil {
		diags.AddError("Could not encode legacy payload identifier namespace", helpers.APIErrorDetail(err))
		return "", diags
	}
	diags.Append(writer.SetKey(ctx, payloadIdentifierNamespaceKey, raw)...)
	return namespace, diags
}

// generatePayloadIdentifier produces a deterministic UUID-formatted identifier scoped to one
// payload position in a Blueprint resource.
func generatePayloadIdentifier(namespace string, stepIndex int, payloadType string) string {
	hash := sha256.Sum256([]byte(namespace + "\x00" + strconv.Itoa(stepIndex) + "\x00" + payloadType))
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
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
