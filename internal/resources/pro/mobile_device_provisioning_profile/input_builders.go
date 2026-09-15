// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildProvisioningProfileInput converts the Terraform plan model into an SDK
// payload. Only `name` and the optional profile blob are emitted; uuid/dates are
// read-only and display_name is server-derived. The profile blob is nested under
// general.profile.data and is only sent when supplied (the empty-shell case
// creates a profile with no blob).
//
// display_name is intentionally NOT sent: Jamf Pro forces display_name == name,
// and worse, the wire is order-sensitive — the SDK marshals <display_name> before
// <name>, so the server collapses both to whichever element comes last (name).
// Sending it would only invite an "inconsistent result after apply" mismatch.
//
// ID is omitted on write — Create uses path id="0". This payload is only ever
// used on Create: a blob-bearing profile rejects every PUT, so Update issues no
// SDK write.
func buildProvisioningProfileInput(plan ProvisioningProfileResourceModel) *proclassic.MobileDeviceProvisioningProfile {
	general := &proclassic.MobileDeviceProvisioningProfileGeneral{
		Name: helpers.OptionalStringPointer(plan.Name),
	}

	if data := helpers.OptionalStringPointer(plan.ProfileData); data != nil {
		general.Profile = &proclassic.MobileDeviceProvisioningProfileGeneralProfile{
			Data: data,
		}
	}

	return &proclassic.MobileDeviceProvisioningProfile{General: general}
}
