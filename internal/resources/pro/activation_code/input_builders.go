// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// buildActivationCodeInput converts the Terraform plan into a ProClassic payload. BOTH
// fields are always sent: the activation code is a license secret, and a partial PUT
// (organization name only, or code only) risks wiping the license. The code is
// whitespace-trimmed defensively — license keys are whitespace-sensitive and a stray
// trailing newline would be rejected by the API.
func buildActivationCodeInput(plan ActivationCodeResourceModel) *proclassic.ActivationCode {
	org := plan.OrganizationName.ValueString()
	code := strings.TrimSpace(plan.Code.ValueString())
	return &proclassic.ActivationCode{
		OrganizationName: &org,
		Code:             &code,
	}
}
