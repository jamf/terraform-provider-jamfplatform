// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// smartMobileDeviceGroupTimeoutAttributeTypes is the attribute type map for the
// resource's timeouts object, used to build a null value on the import and list
// paths where no configuration supplied one.
var smartMobileDeviceGroupTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}
