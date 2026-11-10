// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computertarget

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// Permissions is the privilege an action needs when it targets a computer by
// management_id: the id is mapped to a Jamf Pro computer through the inventory,
// and a failed lookup fails the invocation. A serial number or UDID resolves
// without it. Every action importing this package renders it through
// permissions.SectionWith; internal/conformance enforces that.
var Permissions = permissions.Helper{
	When:     "the action targets a computer by `management_id`",
	Registry: pro.Privileges,
	Methods:  []string{"ListComputersInventoryV4"},
}
