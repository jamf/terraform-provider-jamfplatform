// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	"strconv"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildComputerInvitationInput converts the Terraform plan model into the SDK
// ComputerInvitation payload used for Create. The endpoint has no update
// operation, so this is only ever called from Create.
//
//   - `sshPassword` is supplied separately because it is a WriteOnly attribute
//     (null in the plan model) and must be read from req.Config by the caller.
//   - `siteID` is supplied separately: it is the user's `enroll_into_site_id`
//     value (a Jamf Pro site ID). A non-empty string is parsed to an int and
//     sent as `<enroll_into_site><id>N</id></enroll_into_site>`. Empty means no
//     site — the block is omitted so the server defaults to `-1`/`NONE`.
//   - Server-derived fields (id, invitation, invitation_status, times_used,
//     invited_user_uuid, expiration_date_epoch/_utc, and the read-only <site>
//     element) are intentionally omitted; they are populated on read only.
func buildComputerInvitationInput(plan ComputerInvitationResourceModel, sshPassword *string, siteID string) *proclassic.ComputerInvitation {
	in := &proclassic.ComputerInvitation{
		InvitationType:              helpers.OptionalStringPointer(plan.InvitationType),
		ExpirationDate:              helpers.OptionalStringPointer(plan.ExpirationDate),
		KeepExistingSiteMembership:  helpers.OptionalBoolPointer(plan.KeepExistingSiteMembership),
		MultipleUsesAllowed:         helpers.OptionalBoolPointer(plan.MultipleUsesAllowed),
		CreateAccountIfDoesNotExist: helpers.OptionalBoolPointer(plan.CreateAccountIfDoesNotExist),
		HideAccount:                 helpers.OptionalBoolPointer(plan.HideAccount),
		LockDownSsh:                 helpers.OptionalBoolPointer(plan.LockDownSSH),
		SshUsername:                 helpers.OptionalStringPointer(plan.SSHUsername),
		SshPassword:                 sshPassword,
	}

	if siteID != "" {
		if id, err := strconv.Atoi(siteID); err == nil {
			in.EnrollIntoSite = &proclassic.ComputerInvitationEnrollIntoSite{ID: &id}
		}
	}

	return in
}
