// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildRemovableMacAddressInput converts the Terraform plan model into an SDK
// RemovableMacAddress payload. MacAddress (the wire `name`) is required by the schema
// so we always send it as a non-nil pointer. ID is omitted on write — Create uses path
// id="0" and Update derives it from state.
func buildRemovableMacAddressInput(plan RemovableMacAddressResourceModel) *proclassic.RemovableMacAddress {
	return &proclassic.RemovableMacAddress{
		Name: helpers.OptionalStringPointer(plan.MacAddress),
	}
}
