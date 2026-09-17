// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// smartComputerGroupTimeoutAttributeTypes is the timeout attribute type map used
// to build a null timeouts value on the import and list-hydration paths, where
// no configuration supplied one.
var smartComputerGroupTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
