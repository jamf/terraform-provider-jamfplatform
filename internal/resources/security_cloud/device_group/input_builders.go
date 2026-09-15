// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// buildGroupCreateInput converts the Terraform plan model into the create payload.
//
// The wire body is `{"name": …}` and nothing more. A create carrying additional
// fields is accepted with them discarded, so there is nothing to be gained by
// sending anything else.
func buildGroupCreateInput(plan DeviceGroupResourceModel) *securitycloud.CreateGroupRequest {
	return &securitycloud.CreateGroupRequest{Name: plan.Name.ValueString()}
}

// buildGroupUpdateInput converts the Terraform plan model into the update payload.
//
// Update is a full PUT of the one writable field, so the merge-versus-replace
// question that governs most resources here has no content: there is no field an
// omission could preserve or clear. A PUT with an empty body is refused with the
// same blank-name error as a create.
func buildGroupUpdateInput(plan DeviceGroupResourceModel) *securitycloud.UpdateGroupRequest {
	return &securitycloud.UpdateGroupRequest{Name: plan.Name.ValueString()}
}
