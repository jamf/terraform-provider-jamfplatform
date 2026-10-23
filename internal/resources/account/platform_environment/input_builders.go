// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// buildCreateRequest converts the Terraform plan model into the create payload.
// A null region is left out of the request, which Jamf Account reads as a global
// environment.
func buildCreateRequest(plan PlatformEnvironmentResourceModel) *account.CreateEnvironmentRequest {
	request := &account.CreateEnvironmentRequest{
		Name:  plan.Name.ValueString(),
		Label: plan.Type.ValueString(),
	}
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() {
		region := plan.Region.ValueString()
		request.MetaRegion = &region
	}
	return request
}

// buildUpdateRequest converts the Terraform plan model into the update payload.
// The update is a full replacement of the name and type. The region cannot be
// sent, because Jamf Account fixes it when the environment is created.
func buildUpdateRequest(plan PlatformEnvironmentResourceModel) *account.UpdateEnvironmentRequest {
	return &account.UpdateEnvironmentRequest{
		Name:  plan.Name.ValueString(),
		Label: plan.Type.ValueString(),
	}
}
