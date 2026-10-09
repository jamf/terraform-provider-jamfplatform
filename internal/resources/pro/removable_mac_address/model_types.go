// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// RemovableMacAddressResourceModel represents the Terraform resource model for a
// Jamf Pro removable MAC address. MacAddress maps to the wire `name` field.
type RemovableMacAddressResourceModel struct {
	ID         types.String           `tfsdk:"id"`
	MacAddress types.String           `tfsdk:"mac_address"`
	Timeouts   resourceTimeouts.Value `tfsdk:"timeouts"`
}

// RemovableMacAddressDataSourceModel represents the Terraform data source model for a
// Jamf Pro removable MAC address. Either id or mac_address must be supplied (enforced
// by ExactlyOneOf at config validation).
type RemovableMacAddressDataSourceModel struct {
	ID         types.String             `tfsdk:"id"`
	MacAddress types.String             `tfsdk:"mac_address"`
	Timeouts   datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// removableMacAddressIdentityModel represents the identity object for removable MAC
// address resources and list results.
type removableMacAddressIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// RemovableMacAddressListResourceModel represents the config model for removable MAC
// address list queries. Classic has no RSQL — the filter shape is the shared
// client-side substring block.
type RemovableMacAddressListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
