// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	"strconv"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildMobileDeviceInvitationInput converts the Terraform plan model into the
// SDK MobileDeviceInvitationPost payload used for Create. The endpoint has no
// usable update operation, so this is only ever called from Create.
//
//   - `siteID` is supplied separately: it is the user's `enroll_into_site_id`
//     value (a Jamf Pro site ID). A non-empty string is parsed to an int and
//     sent as `<enroll_into_site><id>N</id></enroll_into_site>`. Empty means no
//     site — the block is omitted so the server defaults to `-1`/`NONE`.
//   - Write-name≠read-name asymmetry: the two boolean fields the UI labels
//     "multiple uses" and "require login" are WRITTEN under the names
//     `allow_multiple_uses` / `require_login` (Post.AllowMultipleUses /
//     Post.RequireLogin) and READ back under `multiple_uses_allowed` /
//     `login_required`. The Post type also carries `MultipleUsesAllowed` and
//     `LoginRequired` fields, but those are the read-side names and the server
//     ignores them on write, so they are left nil here.
//   - Server-derived fields (id, invitation, last_action, date_sent*,
//     expiration_date_epoch/_utc, and the read-only <site> element) are
//     intentionally omitted; they are populated on read only.
func buildMobileDeviceInvitationInput(plan MobileDeviceInvitationResourceModel, siteID string) *proclassic.MobileDeviceInvitationPost {
	in := &proclassic.MobileDeviceInvitationPost{
		InvitationType:             helpers.OptionalStringPointer(plan.InvitationType),
		ExpirationDate:             helpers.OptionalStringPointer(plan.ExpirationDate),
		KeepExistingSiteMembership: helpers.OptionalBoolPointer(plan.KeepExistingSiteMembership),
		// Write names (NOT the read names MultipleUsesAllowed / LoginRequired).
		AllowMultipleUses: helpers.OptionalBoolPointer(plan.MultipleUsesAllowed),
		RequireLogin:      helpers.OptionalBoolPointer(plan.RequireLogin),
		Subject:           helpers.OptionalStringPointer(plan.Subject),
		Message:           helpers.OptionalStringPointer(plan.Message),
		ReplyTo:           helpers.OptionalStringPointer(plan.ReplyTo),
		SentFrom:          helpers.OptionalStringPointer(plan.SentFrom),
		SentTo:            helpers.OptionalStringPointer(plan.SentTo),
		Username:          helpers.OptionalStringPointer(plan.Username),
		TargetIos:         helpers.OptionalStringPointer(plan.TargetIos),
	}

	if siteID != "" {
		if id, err := strconv.Atoi(siteID); err == nil {
			in.EnrollIntoSite = &proclassic.MobileDeviceInvitationPostEnrollIntoSite{ID: &id}
		}
	}

	return in
}
