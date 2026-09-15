// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_connect

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildJamfConnectInput converts the planned deployment settings into the SDK
// update payload. Only the two writable fields are sent — the server ignores
// the read-only fields it echoes back, and the PUT is a full replace, so
// version is always emitted when a non-NONE deployment type requires it.
//
// When auto_deployment_type is NONE, version is omitted: Jamf Connect ignores
// (and clears) the version in that mode, and the config validator forbids the
// user from setting one.
func buildJamfConnectInput(plan JamfConnectResourceModel) *pro.LinkedConnectProfile {
	deploymentType := plan.AutoDeploymentType.ValueString()
	input := &pro.LinkedConnectProfile{
		AutoDeploymentType: &deploymentType,
	}
	if deploymentType != autoDeploymentNone {
		version := plan.Version.ValueString()
		input.Version = &version
	}
	return input
}
