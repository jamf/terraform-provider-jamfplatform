// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package service_discovery_enrollment

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// serviceDiscoveryEnrollmentTimeoutAttributeTypes defines the timeout attribute
// types for the resource operations.
var serviceDiscoveryEnrollmentTimeoutAttributeTypes = map[string]attr.Type{
	"create": types.StringType,
	"read":   types.StringType,
	"update": types.StringType,
	"delete": types.StringType,
}

// enrollmentType wire enum values, aliased from pro.ServiceDiscoveryVersion.
//
// The SDK did once expose ServiceDiscoveryVersion as a bare string alias with no
// constants, which is why these were literals; it now generates all three, so
// they alias.
const (
	enrollmentTypeNone    = pro.ServiceDiscoveryVersionNone
	enrollmentTypeMDMBYOD = pro.ServiceDiscoveryVersionMDMByod
	enrollmentTypeMDMADDE = pro.ServiceDiscoveryVersionMDMAdde
)

// validEnrollmentTypes is the accepted enrollment_type vocabulary (OneOf).
var validEnrollmentTypes = []string{
	enrollmentTypeNone,
	enrollmentTypeMDMBYOD,
	enrollmentTypeMDMADDE,
}

// wellKnownSettingAttrTypes is the attr.Type map matching wellKnownSettingModel and
// the nested-object schema. Used to build a types.List of well-known settings rows.
func wellKnownSettingAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"server_uuid":     types.StringType,
		"enrollment_type": types.StringType,
		"org_name":        types.StringType,
	}
}

// wellKnownSettingObjectType is the types.ObjectType element type for a types.List of
// well-known settings rows.
func wellKnownSettingObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: wellKnownSettingAttrTypes()}
}
