// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// valuesDocs renders a vocabulary as a comma-separated list of code-formatted
// values for an attribute description.
//
// Both vocabularies the resource writes, the platform environment type and the
// region, are taken from the SDK's own *Values() helpers, so the documented list
// and the OneOf validator cannot drift from each other or from what Jamf Account
// accepts. Values keep Jamf's spelling: the Jamf Account console shows the type
// as "Production" or "Sandbox", which differs only in case, so translating would
// add a mapping for no gain.
func valuesDocs(values []string) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = "`" + v + "`"
	}
	return strings.Join(parts, ", ")
}

// typeValues returns the platform environment types Jamf Account accepts.
func typeValues() []string {
	return account.EnvironmentLabelValues()
}

// regionValues returns the regions a platform environment can be pinned to.
func regionValues() []string {
	return account.MetaRegionValues()
}
