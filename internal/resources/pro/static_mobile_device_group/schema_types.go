// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// staticMobileDeviceGroupTimeoutAttributeTypes types the timeouts object so a
// list result and an identity-only refresh can seed a null one.
var staticMobileDeviceGroupTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
