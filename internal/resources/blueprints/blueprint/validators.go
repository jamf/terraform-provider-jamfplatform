// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/appleprofiles"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// legacyPayloadSchemaValidator checks each legacy payload against Apple's declared payload keys
// during plan, so the three ways Jamf refuses or rewrites a payload are caught before an apply:
// a value of the wrong type or a missing required key fails the write outright, and a miscased key
// is stored under Apple's spelling, leaving configuration and state permanently apart.
//
// Every finding is an error. Jamf accepts a payload carrying an unrecognised key, reports success
// and discards the key, so a warning would leave the operator with a payload that silently never
// applies — the same reasoning as internal/common/appledeclarations. The escape hatch is the same
// too, but it is per block rather than per payload: appendLegacyConfigProfile folds every payload in
// a block into one com.jamf.ddm-configuration-profile component, so a partial move is rejected by
// rawComponentOverlapDiags rather than written as two copies of that component. The embedded tables
// are refreshed daily so that erroring on an unrecognised name cannot block a working configuration
// for long, and a finding that the snapshot could explain says so and names it. See
// internal/common/appleprofiles.
//
// It also carries the one rule Apple's schemas cannot express: a custom settings payload sets
// exactly one preference domain. A Mac applies one of several and drops the others, and Jamf
// discards an empty dictionary. See appendMCXDomainProblems.
type legacyPayloadSchemaValidator struct{}

// blockLegacyPayloadSchemaValidator validates a component block's typed legacy payload list, whose
// settings arrive as JSON object strings.
func blockLegacyPayloadSchemaValidator() validator.List {
	return legacyPayloadSchemaValidator{}
}

// flatLegacyPayloadSchemaValidator validates the deprecated top-level dynamic legacy payload value,
// whose settings arrive as objects rather than strings.
func flatLegacyPayloadSchemaValidator() validator.Dynamic {
	return legacyPayloadSchemaValidator{}
}

func (v legacyPayloadSchemaValidator) Description(ctx context.Context) string {
	return v.MarkdownDescription(ctx)
}

func (v legacyPayloadSchemaValidator) MarkdownDescription(context.Context) string {
	_, release := appleprofiles.Provenance()
	return fmt.Sprintf(
		"each payload's settings must match Apple's declared keys for its payload type (schemas from %s), and a custom settings payload must set exactly one preference domain",
		release,
	)
}

// ValidateList checks a block's legacy payload list.
func (v legacyPayloadSchemaValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if !helpers.IsConfiguredValue(req.ConfigValue) {
		return
	}

	var payloads []BlockLegacyPayloadModel
	diags := req.ConfigValue.ElementsAs(ctx, &payloads, false)
	if diags.HasError() {
		// An element still unknown at plan time cannot be validated; the wire will decide.
		return
	}

	for i, payload := range payloads {
		entry := req.Path.AtListIndex(i)
		if !helpers.IsConfiguredValue(payload.PayloadType) {
			continue
		}

		var settings map[string]any
		if helpers.IsConfiguredValue(payload.Settings) && payload.Settings.ValueString() != "" {
			if err := json.Unmarshal([]byte(payload.Settings.ValueString()), &settings); err != nil {
				// collectBlockLegacyPayloads reports unparseable settings; nothing to add here.
				continue
			}
		}

		appendPayloadProblems(&resp.Diagnostics, payload.PayloadType.ValueString(), settings, entry, entry.AtName("payload_type"), entry.AtName("settings"))
		appendMCXDomainProblems(&resp.Diagnostics, payload.PayloadType.ValueString(), settings, entry.AtName("settings"), "")
	}
}

// ValidateDynamic checks the deprecated top-level dynamic legacy payload value.
func (v legacyPayloadSchemaValidator) ValidateDynamic(_ context.Context, req validator.DynamicRequest, resp *validator.DynamicResponse) {
	if !helpers.IsConfiguredValue(req.ConfigValue) {
		return
	}

	raw, err := helpers.TerraformDynamicToJSON(req.ConfigValue)
	if err != nil {
		return
	}
	items, ok := raw.([]any)
	if !ok {
		// collectLegacyPayloads reports a non-list value; nothing to add here.
		return
	}

	for i, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		payloadType, _ := obj["payload_type"].(string)
		if payloadType == "" {
			continue
		}
		settings, _ := obj["settings"].(map[string]any)

		// A dynamic value carries no traversable schema path for its elements, so every finding is
		// reported against the attribute itself and located by the path in its detail.
		appendPayloadProblems(&resp.Diagnostics, payloadType, settings, req.Path, req.Path, req.Path)
		appendMCXDomainProblems(&resp.Diagnostics, payloadType, settings, req.Path, fmt.Sprintf("payload %d (%s)", i+1, payloadType))
	}
}

// appendMCXDomainProblems reports a custom settings payload that sets any number of preference
// domains other than one, which Apple's schemas cannot catch and Jamf does not report.
//
// The dictionary the domains sit under is a declared free-form (wildcard) dictionary, so
// appleprofiles.Validate stops descending at it and everything below is passthrough — correctly,
// because Jamf stores whatever arrives there and a device honours it. What neither side checks is
// how many domains arrived, and each count off one has its own wire evidence.
//
// More than one is lost on the device. Wire probing on macOS 26.6 on 2026-09-17 found a payload
// storing several applies exactly one and discards the rest: the deploy reports SUCCEEDED,
// com.apple.ManagedClient logs nothing, and which domain survived was not predictable across two
// probes. Splitting the same domains one per payload applied all of them, which is what the Jamf Pro
// profile editor produces. Nothing else would catch it: a discarded domain is discarded by the
// device rather than by the service, so the stored blueprint round-trips, every plan settles, and
// appendLegacyPayloadDiscardWarnings sees nothing missing. That is why it is an error rather than a
// warning — a green apply delivering two of three domains is the outcome the check exists to
// prevent.
//
// None at all is lost on the wire. Jamf discards an empty dictionary: wire probing on the EU
// environment on 2026-09-17 sent a payload carrying `PayloadContent: {}` and read it back with the
// stamped metadata keys and no PayloadContent at all. The read then finds no settings,
// flattenBlockLegacyPayloads writes a null, and Terraform fails the apply with an inconsistent
// result. The shape cannot round-trip whatever the provider stores for it, so it has to be refused
// before the write. It carries its own summary so an operator reads the count that applies rather
// than the other one.
//
// An absent dictionary is appleprofiles.Validate's to report as a missing required key, and this
// check stays out of its way: mcxPreferenceDomains reports whether the dictionary exists at all,
// which is what separates an absent one from an empty one. A null-valued domain is not one either,
// for the reason mcxPreferenceDomains gives.
//
// The rule is not a macOS workaround, and the distinction matters because the device evidence above
// is macOS 26.6 alone and says nothing about iOS. One domain per payload is the shape the Jamf Pro
// profile editor produces on every platform, so it is the shape the platform's own tooling, its
// stored blueprints and any operator reading either will agree on. The macOS observation is why a
// multi-domain payload is actively harmful rather than merely unidiomatic; editor parity is why the
// rule holds regardless of what a device does with it.
//
// payloadLabel names the payload for an operator when the attribute path cannot. The block carrier
// passes an empty string, because its path already points at the element. The deprecated flat
// carrier passes a position and type, since a dynamic value has no traversable path for its
// elements and every finding lands on the attribute itself.
func appendMCXDomainProblems(diags *diag.Diagnostics, payloadType string, settings map[string]any, settingsPath path.Path, payloadLabel string) {
	domains, present := mcxPreferenceDomains(payloadType, settings)
	if !present || len(domains) == 1 {
		return
	}

	prefix := ""
	if payloadLabel != "" {
		prefix = payloadLabel + ": "
	}

	if len(domains) == 0 {
		diags.AddAttributeError(
			settingsPath,
			"Custom settings payload sets no preference domain",
			prefix+"this payload's PayloadContent dictionary sets no preference domain. The platform discards an "+
				"empty dictionary, so the payload cannot be stored and the apply fails. Set one preference domain "+
				"under PayloadContent, or remove the payload. A domain set to null counts as none, because the "+
				"platform discards it before storing the payload.",
		)
		return
	}

	diags.AddAttributeError(
		settingsPath,
		"Custom settings payload sets more than one preference domain",
		fmt.Sprintf(
			"%sthis payload sets %d preference domains: %s. A Mac applies one and discards the rest without "+
				"reporting it, and you cannot predict which one it keeps. Write one payload per preference domain, "+
				"as the Jamf Pro profile editor does: a profile covering three domains carries three custom settings "+
				"payloads. A component block may carry as many custom settings payloads as it has domains.",
			prefix, len(domains), strings.Join(domains, ", "),
		),
	)
}

// appendPayloadProblems runs one payload through the schema table and turns each problem into a
// diagnostic. Every problem is an error, because Jamf was observed to refuse or silently rewrite
// each of these writes; a problem the embedded snapshot could explain
// (appleprofiles.Problem.StaleTableSuspect) additionally names the snapshot and the escape hatch,
// which is what distinguishes the two classes rather than the severity. validators_test.go pins
// that split, since nothing else in the package would fail if a finding became advisory again.
func appendPayloadProblems(diags *diag.Diagnostics, payloadType string, settings map[string]any, payloadPath, typePath, settingsPath path.Path) {
	problems := appleprofiles.Validate(payloadType, settings)
	if len(problems) == 0 {
		return
	}

	for _, problem := range problems {
		target := settingsPath
		summary := "Legacy payload setting does not match Apple's schema"
		switch problem.Kind {
		case appleprofiles.UnknownPayloadType, appleprofiles.MiscasedPayloadType:
			target = typePath
			summary = "Unrecognised legacy payload type"
		case appleprofiles.MissingRequiredKey:
			target = payloadPath
			summary = "Legacy payload is missing a required setting"
		}

		detail := problem.Detail
		if problem.Path != "" {
			detail = problem.Path + ": " + detail
		}
		if problem.StaleTableSuspect() {
			detail += fmt.Sprintf(
				" The provider's schemas come from apple/device-management %s. If Apple has published this since,"+
					" upgrade the provider; to deliver these payloads without these checks, move every legacy payload"+
					" in the same block to a single raw_component with identifier com.jamf.ddm-configuration-profile."+
					" The platform stores a block's legacy payloads as one component, so move them all or leave"+
					" them all here.",
				appleprofiles.ProvenanceSummary(),
			)
		}
		diags.AddAttributeError(target, summary, detail)
	}
}
