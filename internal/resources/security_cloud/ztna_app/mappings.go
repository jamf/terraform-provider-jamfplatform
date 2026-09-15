// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"slices"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Three enumerated attributes on this resource are fixed dropdowns in the admin UI
// whose labels differ from the values the API stores, so each is exposed as the UI
// label and translated at the boundary — the convention in
// STYLE_GUIDE §Translating UI labels/presets to wire values, and the one
// ztna_grouped_gateway already follows for its routing strategy.
//
// Every table below is keyed by the SDK's generated value constants rather than
// string literals, so a value the SDK renames breaks the build, and the
// accepted-label slices are derived from its `*Values()` helpers, so a value it
// gains or loses fails TestLabelCoverage rather than silently vanishing from the
// schema.
//
// Label provenance — every label here was read off the admin UI in the screenshots
// this was built from, on 2026-08-30:
//
//   - routing mode (`routing.traffic_routing`): the "Application traffic routing" dropdown on
//     the Routing tab. Both options observed: "Encrypt and route via ZTNA:" for
//     CUSTOM and "Direct device routing" for DIRECT. The trailing colon on the
//     first is UI punctuation introducing the gateway picker below it, not part of
//     the name, so it is dropped.
//   - DNS IP resolution (`routing.routing_mode`): the "Routing mode" section
//     directly below, whose two options are "Standard routing (recommended)" for
//     IPv6 and "Legacy routing" for IPv4. "(recommended)" is guidance rather than a
//     name, and "routing" repeats the attribute, so the labels reduce to "Standard"
//     and "Legacy". The pairing is the one thing here worth stating plainly, because
//     nothing in either name suggests the other: the choice is literally an IP
//     version, and the UI's note — "Only select legacy routing if your users'
//     devices or applications are known to be incompatible with IPv6" — is what
//     makes it legible.
//   - risk level (`security.device_risk.deny_at_risk_level`): the "Deny access to
//     devices starting at the following risk level" dropdown on the Security tab.
//     All three options observed as "Low", "Medium" and "High".
//
// `BLOCK` is a third RoutingType the SDK deliberately does not generate — it is not
// exposed in the public API — so it needs no label and cannot reach the schema.
//
// Nothing here changes what goes on the wire, so a wrong label is a documentation
// and ergonomics bug rather than a data-integrity one.
var (
	// routingModeLabels maps each stored routing type to its admin-UI label.
	routingModeLabels = map[string]string{
		securitycloud.RoutingTypeCustom: "Encrypt and route via ZTNA",
		securitycloud.RoutingTypeDirect: "Direct device routing",
	}

	// dnsResolutionLabels maps each stored DNS IP resolution type to its admin-UI
	// label.
	dnsResolutionLabels = map[string]string{
		securitycloud.RoutingDnsIpResolutionTypeIPv6: "Standard",
		securitycloud.RoutingDnsIpResolutionTypeIPv4: "Legacy",
	}

	// riskLevelLabels maps each stored risk threshold to its admin-UI label.
	riskLevelLabels = map[string]string{
		securitycloud.RiskControlsLevelThresholdLow:    "Low",
		securitycloud.RiskControlsLevelThresholdMedium: "Medium",
		securitycloud.RiskControlsLevelThresholdHigh:   "High",
	}
)

// App type labels. These are not an API vocabulary at all: the wire has no app-type
// field, and the form is inferred from whether `predefinedAppId` is set. The two
// spellings are the values the admin UI's "App type" column shows, reproduced so a
// configuration can branch on the form without null-checking a UUID.
const (
	appTypePredefined = "Predefined"
	appTypeCustom     = "Custom"
)

// routingModeValues returns the accepted routing-mode labels, in the order the SDK
// declares the underlying values.
func routingModeValues() []string {
	return labelsFor(securitycloud.RoutingTypeValues(), routingModeLabels)
}

// dnsResolutionValues returns the accepted routing-mode (DNS IP resolution) labels.
// The SDK declares IPv4 first; the UI lists Standard first, and Standard is the
// server's own default, so the order is reversed to match.
func dnsResolutionValues() []string {
	values := securitycloud.RoutingDnsIpResolutionTypeValues()
	reversed := make([]string, 0, len(values))
	for _, value := range slices.Backward(values) {
		reversed = append(reversed, value)
	}
	return labelsFor(reversed, dnsResolutionLabels)
}

// riskLevelValues returns the accepted risk-level labels, lowest first, which is the
// order both the SDK and the UI dropdown use.
func riskLevelValues() []string {
	return labelsFor(securitycloud.RiskControlsLevelThresholdValues(), riskLevelLabels)
}

// appTypeValues returns the two app-type labels, predefined first, matching the
// order the UI list sorts them in.
func appTypeValues() []string {
	return []string{appTypePredefined, appTypeCustom}
}

// labelsFor renders a set of stored values as their admin-UI labels, preserving the
// order of the input. A value with no label falls back to itself so an SDK addition
// surfaces as an odd-looking but usable schema value rather than an empty string;
// TestLabelCoverage is what turns that into a test failure.
func labelsFor(values []string, labels map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if label, ok := labels[v]; ok {
			out = append(out, label)
			continue
		}
		out = append(out, v)
	}
	return out
}

// wireValueFor translates an admin-UI label back to the value the API stores. An
// unrecognised label passes through unchanged: the schema's OneOf validator has
// already rejected anything not in the table, so reaching here with an unknown label
// means the two disagree, and sending it lets the server produce the error rather
// than the provider sending an empty string.
func wireValueFor(label string, labels map[string]string) string {
	for wire, candidate := range labels {
		if candidate == label {
			return wire
		}
	}
	return label
}

// labelFor translates a stored value into its admin-UI label, falling back to the
// value itself. Read paths use this, so a value the table does not know still
// round-trips through state instead of blanking the attribute.
func labelFor(wire string, labels map[string]string) string {
	if label, ok := labels[wire]; ok {
		return label
	}
	return wire
}

// appTypeFor returns the app-type label implied by a predefined app ID.
func appTypeFor(predefinedAppID *string) string {
	if predefinedAppID != nil && *predefinedAppID != "" {
		return appTypePredefined
	}
	return appTypeCustom
}

// markdownList renders a value set as a comma-separated list of backticked
// literals, so a description and its validator are generated from one slice.
func markdownList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, "`"+v+"`")
	}
	return strings.Join(quoted, ", ")
}
