// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"strconv"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// extractRestrictedSoftwareID returns the assigned ID as a string from a
// Create/GET response. Create returns the ID at the top level
// (<restricted_software><id>); GET omits the top-level element and echoes the
// ID inside <general>. Prefer the top-level reading, fall back to general.
func extractRestrictedSoftwareID(rs *proclassic.RestrictedSoftware) string {
	if rs == nil {
		return ""
	}
	if rs.ID != nil {
		return strconv.Itoa(*rs.ID)
	}
	if rs.General != nil && rs.General.ID != nil {
		return strconv.Itoa(*rs.General.ID)
	}
	return ""
}
