// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"sort"
	"strconv"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Both enumerated attributes on this resource are fixed dropdowns in the admin UI
// whose labels differ from the values the API stores, so each is exposed as the UI
// label and translated at the boundary — the convention in
// STYLE_GUIDE §Translating UI labels/presets to wire values.
//
// Label provenance — observed means read off the admin UI in the screenshots this
// was built from:
//
//   - routing strategy: all three observed ("Nearest", "Random", "First
//     available"). The pairing of "First available" with `ACTIVE_STANDBY` is the
//     one inference here, and it is a strong one: the UI describes it as "The
//     first available gateway on the grouped gateways list will be used" and the
//     API describes `ACTIVE_STANDBY` as traffic going to the primary with
//     failover to the secondary. Nothing else in the set could pair with it.
//   - required gateway stability: "30 minutes" observed. The other four labels
//     follow the same "<n> <unit>" form over the durations the server names in its
//     own rejection message — "300 (5 min), 1800 (30 min), 3600 (1 h), 10800
//     (3 h), 28800 (8 h)" — but the exact wording of those four was not seen on a
//     screen. Unverified.
//
// A wrong label here is a documentation and ergonomics bug rather than a
// data-integrity one, since the stored value is what reaches the wire either way.

// routingStrategyLabels maps each stored routing strategy to its admin-UI label,
// keyed by the SDK's generated constants so a renamed value breaks the build.
var routingStrategyLabels = map[string]string{
	securitycloud.RoutingStrategyNearest:       "Nearest",
	securitycloud.RoutingStrategyRandom:        "Random",
	securitycloud.RoutingStrategyActiveStandby: "First available",
}

// gatewayStabilityLabels maps each accepted stability duration to its admin-UI
// label. The SDK documents the set but generates no helper for it, so unlike the
// routing strategies this table is the only source for both halves — which is why
// TestGatewayStabilityDurations pins the numbers against the server's own message.
var gatewayStabilityLabels = map[int64]string{
	300:   "5 minutes",
	1800:  "30 minutes",
	3600:  "1 hour",
	10800: "3 hours",
	28800: "8 hours",
}

// gatewayStabilityOrder lists the durations shortest-first, which is the order the
// admin UI's dropdown presents them in and the order the documented value list
// should read.
var gatewayStabilityOrder = []int64{300, 1800, 3600, 10800, 28800}

// routingStrategyValues returns the accepted routing-strategy labels, in the order
// the SDK declares the underlying values.
func routingStrategyValues() []string {
	values := securitycloud.RoutingStrategyValues()
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, labelForStrategy(v))
	}
	sort.Strings(out)
	return out
}

// gatewayStabilityValues returns the accepted stability labels, shortest first.
func gatewayStabilityValues() []string {
	out := make([]string, 0, len(gatewayStabilityOrder))
	for _, seconds := range gatewayStabilityOrder {
		out = append(out, labelForStability(seconds))
	}
	return out
}

// labelForStrategy translates a stored routing strategy into its admin-UI label,
// falling back to the stored value so an SDK addition still round-trips through
// state rather than blanking the attribute.
func labelForStrategy(wire string) string {
	if label, ok := routingStrategyLabels[wire]; ok {
		return label
	}
	return wire
}

// wireStrategyFor translates an admin-UI label back to the stored routing
// strategy. An unrecognised label passes through: the schema's OneOf validator has
// already rejected anything not in the table, so reaching here with an unknown
// label means the two disagree, and sending it lets the server produce the error.
func wireStrategyFor(label string) string {
	for wire, candidate := range routingStrategyLabels {
		if candidate == label {
			return wire
		}
	}
	return label
}

// labelForStability translates a stored duration into its admin-UI label. A
// duration with no label renders as a bare second count, which is what a grouped
// gateway created before the current constraint can hold — the SDK notes those may
// return a legacy value, and showing it beats blanking the attribute.
func labelForStability(seconds int64) string {
	if label, ok := gatewayStabilityLabels[seconds]; ok {
		return label
	}
	return strconv.FormatInt(seconds, 10) + " seconds"
}

// wireStabilityFor translates an admin-UI label back to the stored duration. Zero
// means unrecognised, which the schema's OneOf validator has already ruled out.
func wireStabilityFor(label string) int64 {
	for seconds, candidate := range gatewayStabilityLabels {
		if candidate == label {
			return seconds
		}
	}
	return 0
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
