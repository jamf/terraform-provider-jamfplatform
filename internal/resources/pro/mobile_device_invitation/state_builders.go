// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/invitationcommon"
)

// siteIDValue / siteNameValue map a read-back MobileDeviceInvitationEnrollIntoSite
// into the enroll_into_site_id / enroll_into_site_name attributes. The id preserves
// the `-1` "no site" marker (Optional+Computed, mirroring the site_id convention);
// the derived name is nulled on the sentinel via helpers.DerivedRefName rather than
// echoed, since the classic GET nondeterministically echoes or omits "NONE" there.
// A nil block (absent on the wire) maps both to null.
func siteIDValue(s *proclassic.MobileDeviceInvitationEnrollIntoSite) types.String {
	if s == nil {
		return types.StringNull()
	}
	return helpers.StringValueFromIntPtr(s.ID)
}

func siteNameValue(s *proclassic.MobileDeviceInvitationEnrollIntoSite) types.String {
	if s == nil {
		return types.StringNull()
	}
	return helpers.DerivedRefName(s.ID, s.Name)
}

// assignMobileDeviceInvitationResourceModel populates a resource model from a
// MobileDeviceInvitation response.
//
//   - state.ID is only overwritten when the API ID is non-nil so a transient
//     GET that drops the ID does not clobber the value persisted from Create.
//   - expiration_date is reconciled at minute-ish (time-delta) granularity
//     against the existing state value to absorb the server's ~1s drift.
//   - enroll_into_site_id / enroll_into_site_name echo the read-back site
//     verbatim (including `-1`/`NONE` when no site is assigned).
//   - Write-name≠read-name asymmetry: the schema's `multiple_uses_allowed` is
//     read from the wire `multiple_uses_allowed` field (c.MultipleUsesAllowed),
//     and the schema's `require_login` is read from the wire `login_required`
//     field (c.LoginRequired).
//   - The read-only <site> element is intentionally not surfaced.
func assignMobileDeviceInvitationResourceModel(state *MobileDeviceInvitationResourceModel, m *proclassic.MobileDeviceInvitation) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	state.Invitation = invitationcommon.BigIntStringOrNull(m.Invitation)
	state.InvitationType = helpers.StringPointerValueOrNull(m.InvitationType)
	state.ExpirationDate = invitationcommon.ReconcileExpirationDate(m.ExpirationDate, state.ExpirationDate)
	state.ExpirationDateEpoch = invitationcommon.BigIntStringOrNull(m.ExpirationDateEpoch)
	state.ExpirationDateUtc = helpers.StringPointerValueOrNull(m.ExpirationDateUtc)
	state.EnrollIntoSiteID = siteIDValue(m.EnrollIntoSite)
	state.EnrollIntoSiteName = siteNameValue(m.EnrollIntoSite)
	state.KeepExistingSiteMembership = helpers.BoolPointerValueOrNull(m.KeepExistingSiteMembership)
	state.MultipleUsesAllowed = helpers.BoolPointerValueOrNull(m.MultipleUsesAllowed)
	state.RequireLogin = helpers.BoolPointerValueOrNull(m.LoginRequired)
	state.Subject = helpers.StringPointerValueOrNull(m.Subject)
	state.Message = helpers.StringPointerValueOrNull(m.Message)
	state.ReplyTo = helpers.StringPointerValueOrNull(m.ReplyTo)
	state.SentFrom = helpers.StringPointerValueOrNull(m.SentFrom)
	state.SentTo = helpers.StringPointerValueOrNull(m.SentTo)
	state.Username = helpers.StringPointerValueOrNull(m.Username)
	state.TargetIos = helpers.StringPointerValueOrNull(m.TargetIos)
	state.LastAction = helpers.StringPointerValueOrNull(m.LastAction)
	state.DateSent = helpers.StringPointerValueOrNull(m.DateSent)
	state.DateSentUtc = helpers.StringPointerValueOrNull(m.DateSentUtc)
	state.DateSentEpoch = invitationcommon.BigIntStringOrNull(m.DateSentEpoch)
}

// assignMobileDeviceInvitationDataSourceModel populates a data source model from
// a MobileDeviceInvitation response. Symmetric with the resource builder. The
// data source is read-only, so the expiration_date is surfaced verbatim (no
// drift reconciliation — there is no user-authored config value to preserve).
func assignMobileDeviceInvitationDataSourceModel(state *MobileDeviceInvitationDataSourceModel, m *proclassic.MobileDeviceInvitation) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	state.Invitation = invitationcommon.BigIntStringOrNull(m.Invitation)
	state.InvitationType = helpers.StringPointerValueOrNull(m.InvitationType)
	state.ExpirationDate = helpers.StringPointerValueOrNull(m.ExpirationDate)
	state.ExpirationDateEpoch = invitationcommon.BigIntStringOrNull(m.ExpirationDateEpoch)
	state.ExpirationDateUtc = helpers.StringPointerValueOrNull(m.ExpirationDateUtc)
	state.EnrollIntoSiteID = siteIDValue(m.EnrollIntoSite)
	state.EnrollIntoSiteName = siteNameValue(m.EnrollIntoSite)
	state.KeepExistingSiteMembership = helpers.BoolPointerValueOrNull(m.KeepExistingSiteMembership)
	state.MultipleUsesAllowed = helpers.BoolPointerValueOrNull(m.MultipleUsesAllowed)
	state.RequireLogin = helpers.BoolPointerValueOrNull(m.LoginRequired)
	state.Subject = helpers.StringPointerValueOrNull(m.Subject)
	state.Message = helpers.StringPointerValueOrNull(m.Message)
	state.ReplyTo = helpers.StringPointerValueOrNull(m.ReplyTo)
	state.SentFrom = helpers.StringPointerValueOrNull(m.SentFrom)
	state.SentTo = helpers.StringPointerValueOrNull(m.SentTo)
	state.Username = helpers.StringPointerValueOrNull(m.Username)
	state.TargetIos = helpers.StringPointerValueOrNull(m.TargetIos)
	state.LastAction = helpers.StringPointerValueOrNull(m.LastAction)
	state.DateSent = helpers.StringPointerValueOrNull(m.DateSent)
	state.DateSentUtc = helpers.StringPointerValueOrNull(m.DateSentUtc)
	state.DateSentEpoch = invitationcommon.BigIntStringOrNull(m.DateSentEpoch)
}
