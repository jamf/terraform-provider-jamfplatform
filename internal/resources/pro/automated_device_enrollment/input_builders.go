// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package automated_device_enrollment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildCreateTokenInput converts the Terraform plan + decoded base64 token
// bytes into the SDK upload payload. The plaintext token string lives on the
// config object (`WriteOnly`), so callers pre-decode it from the config and
// pass the resulting bytes in here. `TokenFileName` is sourced from the plan
// because it is plain Optional (non-WriteOnly).
func buildCreateTokenInput(plan AutomatedDeviceEnrollmentResourceModel, decodedToken []byte) *pro.DeviceEnrollmentToken {
	tokenBytes := decodedToken
	out := &pro.DeviceEnrollmentToken{
		EncodedToken: &tokenBytes,
	}
	if fileName := helpers.OptionalStringPointer(plan.TokenFileName); fileName != nil {
		out.TokenFileName = fileName
	}
	return out
}

// buildMetadataInput converts the Terraform plan into the SDK metadata payload
// used by the post-upload `UpdateDeviceEnrollmentV1` call (sets the
// user-visible name plus optional site / supervision-identity associations).
// All Apple-derived fields (org_*, server_*, admin_id, token_expiration_date)
// are server-owned and are intentionally omitted from this payload.
func buildMetadataInput(plan AutomatedDeviceEnrollmentResourceModel) *pro.DeviceEnrollmentInstance {
	out := &pro.DeviceEnrollmentInstance{
		Name: plan.Name.ValueString(),
	}
	if siteID := helpers.OptionalStringPointer(plan.SiteID); siteID != nil {
		out.SiteID = siteID
	}
	if supervisionID := helpers.OptionalStringPointer(plan.SupervisionIdentityID); supervisionID != nil {
		out.SupervisionIdentityID = supervisionID
	}
	return out
}
