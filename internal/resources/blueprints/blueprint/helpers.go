// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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
// payload sent without one is accepted and stamped with a fresh UUID **on every write**.
// `payloadContent` is an array and a merge-patch replaces an array wholesale, so a body omitting the
// key re-identifies every payload in the blueprint whenever any part of it changed.
//
// Jamf's own web app avoids that by reading the blueprint, mutating one field and writing the stored
// identifiers back, which a UI save confirms: editing one payload's setting in the web app left the
// identifier and the per-payload display name, organization and version untouched, wire-verified
// 2026-09-16. The web app also mints its own identifiers client-side, lowercase where the service
// mints uppercase, which is why it must be sending them back. This type is that read half, so the
// provider keeps what the web app would have kept. It mirrors the configuration profile resources,
// where payloadhelpers.InjectTopLevelIdentifierValues carries the stored identifier onto the
// outgoing payload for the same reason.
//
// A rotated identifier does **not** reinstall the payload on a device. That reads as the obvious
// reason to preserve one, and it is wrong: wire-verified on macOS 26.6, 2026-09-16, a blueprint
// write followed by a deploy reinstalls the profile whether the identifiers moved or not, and a
// write rotating all four replaced the payload in place with no orphan and no duplicate. Parity with
// the web app and a stable stored blueprint are the reasons here, not device churn.
//
// Nor does a duplicated identifier collide. Two blueprints delivering the same payload type under
// one identifier install as two profiles, each keeping its own settings, wire-verified the same day:
// the device keys a payload on its **profile's** top-level identifier, which Jamf assigns per
// blueprint, rather than on the nested one. Managed preferences composite the same way: two
// blueprints setting disjoint keys of one payload type land every key in the single
// /Library/Managed Preferences/<domain>.plist, and they do so whether the two share an identifier or
// carry different ones, so the merge is keyed on the preference domain and the identifier plays no
// part either way. The plist's own PayloadUUID records whichever payload wrote last.
//
// So the derivation every released version used, sha256 of the payload type and therefore identical
// across every blueprint sharing a type, needs no migration: there is nothing for a re-mint to
// repair.
//
// A payload is located by step and by legacyPayloadIdentity, the same pairing
// checkLegacyPayloadDiscards uses. That is the payload type for most payloads, and the type plus the
// preference domain for a managed preferences payload, of which a block may carry one per domain;
// appendLegacyConfigProfile rejects a repeat of either, so the identity is unique within a block.
// Steps are matched by name ahead of position so that inserting or reordering a block keeps every
// other block's identifiers: a positional match alone would shift them onto neighbouring payloads.
// Position remains the fallback, since a block name is optional and need not be unique.
type storedLegacyPayloadIdentifiers struct {
	stepIndexByName map[string]int
	byStepIndex     []map[string]string
	// ambiguous holds identifiers dropped because more than one payload in a single step carried
	// them, so the service reissues a distinct one for each. macOS refuses a whole profile
	// whose payload identifiers are not unique — "the PayloadIdentifier is used more than once in
	// the profile", ConfigProfilePluginDomain:-107, wire-verified on macOS 26.6 on 2026-09-16,
	// where neither payload installed and the device retried indefinitely while the blueprint
	// reported DEPLOYED and SUCCEEDED. Writing a stored duplicate back would reproduce that profile
	// on every update, so a duplicate names nothing, the same way a name two steps share does.
	//
	// The scope is one step, and that is measured rather than assumed. A step's legacy payloads fold
	// into one component and install as one profile, so a duplicate inside a step is what the device
	// rejects. The same identifier on payloads in two different steps of one blueprint installed
	// cleanly, as did the same identifier in two separate blueprints, because each is a profile of
	// its own and the device scopes the uniqueness rule to a profile. Widening this to the whole
	// blueprint would re-identify payloads a device is content with.
	ambiguous []string
}

// newStoredLegacyPayloadIdentifiers reads the stored identifiers out of a blueprint as fetched from
// the service. A nil blueprint yields nil, which callers treat as "nothing stored yet" — the create
// path, where every identifier is the service's to mint.
//
// A name two steps share cannot say which of them a block means, so it names neither and both fall
// back to position. resolve applies the same reasoning to a name two blocks share.
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
		identifiers, ambiguous := legacyPayloadIdentifiersInStep(step)
		stored.byStepIndex = append(stored.byStepIndex, identifiers)
		stored.ambiguous = append(stored.ambiguous, ambiguous...)

		if step.Name == nil || *step.Name == "" {
			continue
		}
		if _, seen := stored.stepIndexByName[*step.Name]; seen {
			duplicateNames[*step.Name] = true
			continue
		}
		stored.stepIndexByName[*step.Name] = i
	}

	for name := range duplicateNames {
		delete(stored.stepIndexByName, name)
	}

	return stored
}

// mcxPreferenceDomains returns the preference domains a managed preferences payload declares,
// sorted, and reports whether the payload carries the dictionary they sit under at all. A payload of
// any other type yields nothing, so a caller need not test the type first.
//
// `settings` may be either an authored settings object or a whole stored payload dictionary: the
// only key read is the one the domains sit under, which both carry in the same place.
//
// The key is matched without regard to case, because Jamf stores a miscased key under Apple's
// spelling, so a payload authored `payloadcontent` reaches a device as `PayloadContent` and its
// domains are as real as any other payload's. Apple's spelling wins where a payload somehow carries
// both, so the answer does not depend on map iteration order. The miscasing itself is reported
// separately by appleprofiles.Validate.
//
// A null-valued key is not a domain. The platform discards one before it stores the payload (see
// pruneJSONNulls, and legacyPayloadSettingsBehaviour, which promises an author their nulls can stay
// in configuration), so a null domain reaches no device and must be counted as absent — wire-probed
// on the EU environment, 2026-09-17, where `PayloadContent` sent as
// {"com.example.kept": {...}, "com.example.nulled": null} read back carrying only the kept domain,
// and one sent as {"com.example.onlynull": null} read back with no PayloadContent at all. Counting
// it would break the rule in both directions: a payload storing one domain beside a null one would
// be refused for setting two, naming a domain that is never stored, and a payload whose only domain
// is null would pass as one and then fail the apply with an inconsistent result, which is the very
// outcome appendMCXDomainProblems' empty case exists to refuse at plan.
func mcxPreferenceDomains(payloadType string, settings map[string]any) ([]string, bool) {
	if payloadType != mcxPayloadType {
		return nil, false
	}

	content, present := settings[mcxPreferenceDomainsKey]
	if !present {
		for _, key := range slices.Sorted(maps.Keys(settings)) {
			if strings.EqualFold(key, mcxPreferenceDomainsKey) {
				content, present = settings[key], true
				break
			}
		}
	}
	if !present {
		return nil, false
	}

	stored, ok := content.(map[string]any)
	if !ok {
		return nil, false
	}

	domains := make([]string, 0, len(stored))
	for domain, value := range stored {
		if value == nil {
			continue
		}
		domains = append(domains, domain)
	}
	slices.Sort(domains)
	return domains, true
}

// legacyPayloadIdentity returns the key one legacy payload is located by within a component block.
// It is the payload type for every payload but one, and the payload type followed by the preference
// domain in brackets for a managed preferences payload, of which a block may carry several.
//
// One key per payload is what the read-merge-write of stored identifiers depends on. A block's
// payloads fold into a single profile, and macOS refuses a profile whose payload identifiers are not
// unique — refusing every payload in it rather than the duplicated ones ("the PayloadIdentifier is
// used more than once in the profile", ConfigProfilePluginDomain:-107, wire-verified on macOS 26.6
// on 2026-09-16). Keying several managed preferences payloads on their shared type collapses them to
// one entry, so each would be written with the same stored identifier and none of them would
// install.
//
// A payload type never contains " (", so a composite key cannot collide with a bare one, and the
// composite doubles as the label a diagnostic names the payload by.
//
// A managed preferences payload with no single domain falls back to its bare type, because there is
// no domain to key it on. Neither shape that reaches the fallback is authorable: appendMCXDomainProblems
// refuses both more than one domain and none at all during plan. The fallback still runs on the read
// path, where the payloads come from the wire rather than from configuration and a blueprint edited
// outside Terraform can carry either shape, so it keys on the type. That also leaves
// appendLegacyConfigProfile's duplicate check able to see two domainless payloads in one block and
// reject the second, which is the invariant its unit test pins.
func legacyPayloadIdentity(payloadType string, settings map[string]any) string {
	domains, _ := mcxPreferenceDomains(payloadType, settings)
	if len(domains) != 1 {
		return payloadType
	}
	return payloadType + " (" + domains[0] + ")"
}

// blockLegacyPayloadIdentity returns the legacyPayloadIdentity of one entry in a block's typed
// legacy payload list, decoding its settings JSON string. Unparseable or absent settings yield the
// bare payload type, which is what the entry would key on anyway: collectBlockLegacyPayloads reports
// settings it cannot decode, and a payload without any carries no preference domain.
func blockLegacyPayloadIdentity(entry BlockLegacyPayloadModel) string {
	payloadType := entry.PayloadType.ValueString()
	if !helpers.IsConfiguredValue(entry.Settings) {
		return payloadType
	}

	var settings map[string]any
	if err := json.Unmarshal([]byte(entry.Settings.ValueString()), &settings); err != nil {
		return payloadType
	}
	return legacyPayloadIdentity(payloadType, settings)
}

// legacyPayloadIdentifiersInStep maps each legacy payload of a step's configuration profile
// component to the identifier the service stored for it, keyed by legacyPayloadIdentity. A payload
// the service has not stamped is omitted rather than mapped to the empty string, so a caller cannot
// mistake it for a stored value.
//
// The identity comes from the stored payload's own dictionary rather than from configuration,
// because this reads the wire: a stored managed preferences payload carries its preference domain
// there, and a blueprint edited outside Terraform may carry a payload the configuration has never
// described.
func legacyPayloadIdentifiersInStep(step blueprints.BlueprintStep) (map[string]string, []string) {
	identifiers := make(map[string]string)
	for _, component := range step.Components {
		if component.Identifier != legacyConfigProfileIdentifier {
			continue
		}

		var configuration struct {
			PayloadContent []map[string]any `json:"payloadContent"`
		}
		if err := json.Unmarshal(component.Configuration, &configuration); err != nil {
			continue
		}
		for _, payload := range configuration.PayloadContent {
			payloadType, _ := payload["payloadType"].(string)
			identifier, _ := payload["payloadIdentifier"].(string)
			if payloadType == "" || identifier == "" {
				continue
			}
			identifiers[legacyPayloadIdentity(payloadType, payload)] = identifier
		}
	}

	return identifiers, dropDuplicateIdentifiers(identifiers)
}

// dropDuplicateIdentifiers removes from one step's map every identifier more than one payload
// carries, and returns those identifiers sorted. Dropping rather than keeping one of them is what
// makes the outcome safe: the service then mints a distinct value for each, and a rotated identifier
// costs nothing (see storedLegacyPayloadIdentifiers), where writing the duplicate back costs the
// whole profile.
func dropDuplicateIdentifiers(identifiers map[string]string) []string {
	identitiesByIdentifier := make(map[string][]string, len(identifiers))
	for identity, identifier := range identifiers {
		identitiesByIdentifier[identifier] = append(identitiesByIdentifier[identifier], identity)
	}

	var duplicates []string
	for identifier, identities := range identitiesByIdentifier {
		if len(identities) < 2 {
			continue
		}
		duplicates = append(duplicates, identifier)
		for _, identity := range identities {
			delete(identifiers, identity)
		}
	}
	slices.Sort(duplicates)
	return duplicates
}

// resolve pairs each component block with the stored identifiers of the step it continues, one entry
// per block and in block order. A block with no stored counterpart gets nil, so every one of its
// payloads is the service's to mint.
//
// Matching runs in two passes, and the order matters. The first claims every step whose name a block
// names, so inserting or reordering a block keeps the other blocks' identifiers. The second walks
// the steps no name claimed, in stored order, and hands the next one to each block the first pass
// left over — which is all an unnamed block has, and is what lets a block renamed and moved in the
// same apply go on continuing the step it came from. Pairing a leftover block with the step at its
// own index instead would strand a step whenever a name match landed out of position, so a rename
// combined with a move would mint fresh identifiers for a block whose step was sitting unclaimed.
//
// A name more than one block carries claims nothing, for the reason newStoredLegacyPayloadIdentifiers
// drops a name two steps share: it cannot say which block means which step, and the first block to
// ask would otherwise take the step and leave its namesake — whose payloads are a different profile
// — to mint. Both such blocks fall through to the positional pass. The schema permits this, since
// component_blocks[].name is optional and carries no uniqueness validator.
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

	blockNameCounts := make(map[string]int, len(blockNames))
	for _, name := range blockNames {
		if helpers.IsConfiguredValue(name) && name.ValueString() != "" {
			blockNameCounts[name.ValueString()]++
		}
	}

	for i, name := range blockNames {
		if !helpers.IsConfiguredValue(name) || name.ValueString() == "" {
			continue
		}
		if blockNameCounts[name.ValueString()] > 1 {
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

	next := 0
	for i := range blockNames {
		if matchedByName[i] {
			continue
		}
		for next < len(s.byStepIndex) && claimed[next] {
			next++
		}
		if next >= len(s.byStepIndex) {
			break
		}
		resolved[i] = s.byStepIndex[next]
		claimed[next] = true
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
// A failed read is an error rather than a fallback to minting. Proceeding would re-identify every
// legacy payload in the blueprint, and it would do so silently, since the field is masked out of
// state. A failed apply the operator can retry is the better outcome.
func (r *BlueprintResource) readStoredLegacyPayloadIdentifiers(ctx context.Context, blueprintID string) (*storedLegacyPayloadIdentifiers, diag.Diagnostics) {
	var diags diag.Diagnostics

	blueprint, err := r.client.GetBlueprint(ctx, blueprintID)
	if err != nil {
		diags.AddError(
			"Error reading the blueprint before updating it",
			"This read failed, so nothing reached Jamf Pro and the blueprint is unchanged. Retry the apply. "+
				"Reported while reading: "+helpers.APIErrorDetail(err),
		)
		return nil, diags
	}

	stored := newStoredLegacyPayloadIdentifiers(blueprint)
	if len(stored.ambiguous) > 0 {
		diags.AddWarning(
			"Legacy payload identifiers were reissued",
			"Jamf Pro held one identifier on more than one legacy payload of this blueprint: "+
				strings.Join(stored.ambiguous, ", ")+". A device refuses a configuration profile whose payload "+
				"identifiers are not unique, and refuses all of its payloads rather than the duplicated ones, so "+
				"the provider left those out of this update for Jamf Pro to assign fresh ones. Your settings are "+
				"unaffected. A blueprint reaches this state by being edited outside Terraform.",
		)
	}

	return stored, diags
}
