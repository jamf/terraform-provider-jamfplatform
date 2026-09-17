// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// staticComputerGroupTimeoutAttributeTypes types the timeouts object, for the
// two paths that build one without a prior value to copy: an identity-only
// import and a list result hydrated for config generation.
var staticComputerGroupTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
