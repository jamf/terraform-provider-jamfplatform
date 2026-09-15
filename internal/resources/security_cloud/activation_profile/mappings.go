// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"slices"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// platformLabelByWire maps each platform value the Jamf API accepts to the
// spelling this provider uses. The map is keyed on the SDK's generated constants
// rather than string literals, so a value the SDK renames breaks the build here
// instead of failing at apply time.
//
// Both labels are the API spelling lowercased: the console's own platform list
// says "Mac" and "iOS", so there is no material divergence to translate, only
// case to normalise for HCL.
var platformLabelByWire = map[string]string{
	securitycloud.PublicApiCreateActivationProfileRequestPlatformsIOS: "ios",
	securitycloud.PublicApiCreateActivationProfileRequestPlatformsMac: "mac",
}

// platformLabels returns the accepted platform values in the order the API spec
// declares them, derived from the SDK's own generated value set rather than a
// restated list. A platform the SDK gains but this package has no label for is
// omitted here and caught by mappings_test.go.
func platformLabels() []string {
	values := securitycloud.PublicApiCreateActivationProfileRequestPlatformsValues()
	labels := make([]string, 0, len(values))
	for _, v := range values {
		if label, ok := platformLabelByWire[v]; ok {
			labels = append(labels, label)
		}
	}
	return labels
}

// platformWire converts a provider platform label back to the value the API
// expects, reporting whether the label is known.
func platformWire(label string) (string, bool) {
	for wire, l := range platformLabelByWire {
		if l == label {
			return wire, true
		}
	}
	return "", false
}

// sortedPlatformLabels returns labels in a stable order, for rendering a
// deterministic request body and a deterministic diagnostic.
func sortedPlatformLabels(labels []string) []string {
	out := slices.Clone(labels)
	slices.Sort(out)
	return out
}
