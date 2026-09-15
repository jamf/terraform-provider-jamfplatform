// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"strconv"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// extractJSONWebTokenConfigurationID returns the assigned ID as a string from a
// Create/GET response. The classic endpoint echoes the integer ID at the top
// level (<json_web_token_configuration><id>).
func extractJSONWebTokenConfigurationID(c *proclassic.JsonWebTokenConfiguration) string {
	if c == nil || c.ID == nil {
		return ""
	}
	return strconv.Itoa(*c.ID)
}
