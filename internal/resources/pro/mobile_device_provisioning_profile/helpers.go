// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// bigIntStringOrNull converts a nil-safe *proclassic.BigInt to a TF String,
// mapping nil to null. The classic generator types expiration_date_epoch as
// BigInt (epoch millis can exceed int64 for far-future dates); we surface it as
// a decimal string to avoid any precision loss.
func bigIntStringOrNull(b *proclassic.BigInt) types.String {
	if b == nil {
		return types.StringNull()
	}
	return types.StringValue(b.String())
}

// intMillisStringOrNull converts a nil-safe *int epoch-millis value to a TF
// String, mapping nil and zero to null. creation_date_epoch is typed *int by
// the SDK; we mirror it as a string for symmetry with expiration_date_epoch.
func intMillisStringOrNull(p *int) types.String {
	if p == nil || *p == 0 {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(*p))
}
