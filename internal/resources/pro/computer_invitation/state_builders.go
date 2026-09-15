// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_invitation

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/invitationcommon"
)

// siteIDValue / siteNameValue echo a read-back ComputerInvitationEnrollIntoSite
// verbatim into the enroll_into_site_id / enroll_into_site_name attributes,
// preserving the `-1` "no site" marker on the id (Optional+Computed, mirroring the
// site_id convention). The derived name is nulled on the sentinel via
// helpers.DerivedRefName rather than echoed, since the classic GET
// nondeterministically echoes or omits "NONE" there. A nil block (absent on the
// wire) maps both to null.
func siteIDValue(s *proclassic.ComputerInvitationEnrollIntoSite) types.String {
	if s == nil {
		return types.StringNull()
	}
	return helpers.StringValueFromIntPtr(s.ID)
}

func siteNameValue(s *proclassic.ComputerInvitationEnrollIntoSite) types.String {
	if s == nil {
		return types.StringNull()
	}
	return helpers.DerivedRefName(s.ID, s.Name)
}

// assignComputerInvitationResourceModel populates a resource model from a
// ComputerInvitation response.
//
//   - state.ID is only overwritten when the API ID is non-nil so a transient
//     GET that drops the ID does not clobber the value persisted from Create.
//   - expiration_date is reconciled at minute-ish (time-delta) granularity
//     against the existing state value to absorb the server's ~1s drift.
//   - enroll_into_site_id / enroll_into_site_name echo the read-back site
//     verbatim (including `-1`/`NONE` when no site is assigned).
//   - ssh_password is WriteOnly (excluded from state by the framework) and the
//     classic GET never echoes the plaintext, so it is never touched here.
//   - The read-only <site> element is intentionally not surfaced.
func assignComputerInvitationResourceModel(state *ComputerInvitationResourceModel, c *proclassic.ComputerInvitation) {
	if c == nil {
		return
	}
	if c.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(c.ID)
	}
	state.Invitation = invitationcommon.BigIntStringOrNull(c.Invitation)
	state.InvitationType = helpers.StringPointerValueOrNull(c.InvitationType)
	state.ExpirationDate = invitationcommon.ReconcileExpirationDate(c.ExpirationDate, state.ExpirationDate)
	state.ExpirationDateEpoch = invitationcommon.BigIntStringOrNull(c.ExpirationDateEpoch)
	state.ExpirationDateUtc = helpers.StringPointerValueOrNull(c.ExpirationDateUtc)
	state.EnrollIntoSiteID = siteIDValue(c.EnrollIntoSite)
	state.EnrollIntoSiteName = siteNameValue(c.EnrollIntoSite)
	state.KeepExistingSiteMembership = helpers.BoolPointerValueOrNull(c.KeepExistingSiteMembership)
	state.MultipleUsesAllowed = helpers.BoolPointerValueOrNull(c.MultipleUsesAllowed)
	state.CreateAccountIfDoesNotExist = helpers.BoolPointerValueOrNull(c.CreateAccountIfDoesNotExist)
	state.HideAccount = helpers.BoolPointerValueOrNull(c.HideAccount)
	state.LockDownSSH = helpers.BoolPointerValueOrNull(c.LockDownSsh)
	state.SSHUsername = helpers.StringPointerValueOrEmpty(c.SshUsername)
	state.InvitationStatus = helpers.StringPointerValueOrNull(c.InvitationStatus)
	state.TimesUsed = invitationcommon.Int64ValueFromIntPtr(c.TimesUsed)
	state.InvitedUserUUID = helpers.StringPointerValueOrNull(c.InvitedUserUUID)
}

// assignComputerInvitationDataSourceModel populates a data source model from a
// ComputerInvitation response. Symmetric with the resource builder, minus the
// WriteOnly secret + rotation companion. The data source is read-only, so the
// expiration_date is surfaced verbatim (no drift reconciliation — there is no
// user-authored config value to preserve) and the site id/name echo the
// read-back values verbatim (including `-1`/`NONE`).
func assignComputerInvitationDataSourceModel(state *ComputerInvitationDataSourceModel, c *proclassic.ComputerInvitation) {
	if c == nil {
		return
	}
	if c.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(c.ID)
	}
	state.Invitation = invitationcommon.BigIntStringOrNull(c.Invitation)
	state.InvitationType = helpers.StringPointerValueOrNull(c.InvitationType)
	state.ExpirationDate = helpers.StringPointerValueOrNull(c.ExpirationDate)
	state.ExpirationDateEpoch = invitationcommon.BigIntStringOrNull(c.ExpirationDateEpoch)
	state.ExpirationDateUtc = helpers.StringPointerValueOrNull(c.ExpirationDateUtc)
	state.EnrollIntoSiteID = siteIDValue(c.EnrollIntoSite)
	state.EnrollIntoSiteName = siteNameValue(c.EnrollIntoSite)
	state.KeepExistingSiteMembership = helpers.BoolPointerValueOrNull(c.KeepExistingSiteMembership)
	state.MultipleUsesAllowed = helpers.BoolPointerValueOrNull(c.MultipleUsesAllowed)
	state.CreateAccountIfDoesNotExist = helpers.BoolPointerValueOrNull(c.CreateAccountIfDoesNotExist)
	state.HideAccount = helpers.BoolPointerValueOrNull(c.HideAccount)
	state.LockDownSSH = helpers.BoolPointerValueOrNull(c.LockDownSsh)
	state.SSHUsername = helpers.StringPointerValueOrEmpty(c.SshUsername)
	state.InvitationStatus = helpers.StringPointerValueOrNull(c.InvitationStatus)
	state.TimesUsed = invitationcommon.Int64ValueFromIntPtr(c.TimesUsed)
	state.InvitedUserUUID = helpers.StringPointerValueOrNull(c.InvitedUserUUID)
}
