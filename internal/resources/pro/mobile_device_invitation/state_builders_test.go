// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func bigInt(s string) *proclassic.BigInt {
	var b proclassic.BigInt
	b.SetString(s)
	return &b
}

func TestSiteValues_EchoVerbatim(t *testing.T) {
	// The `-1` id marker echoes verbatim, but the derived name nulls on the
	// sentinel (the classic GET nondeterministically echoes/omits "NONE").
	noneID := siteIDValue(&proclassic.MobileDeviceInvitationEnrollIntoSite{ID: new(-1), Name: new("NONE")})
	if noneID.ValueString() != "-1" {
		t.Errorf("no-site id must echo -1, got %q", noneID.ValueString())
	}
	noneName := siteNameValue(&proclassic.MobileDeviceInvitationEnrollIntoSite{ID: new(-1), Name: new("NONE")})
	if !noneName.IsNull() {
		t.Errorf("no-site name must be null on the sentinel, got %q", noneName.ValueString())
	}

	if !siteIDValue(nil).IsNull() {
		t.Errorf("nil site id must be null")
	}
	if !siteNameValue(nil).IsNull() {
		t.Errorf("nil site name must be null")
	}

	named := &proclassic.MobileDeviceInvitationEnrollIntoSite{ID: new(7), Name: new("HQ")}
	if got := siteIDValue(named).ValueString(); got != "7" {
		t.Errorf("named site id must echo 7, got %q", got)
	}
	if got := siteNameValue(named).ValueString(); got != "HQ" {
		t.Errorf("named site name must echo HQ, got %q", got)
	}
}

// TestAssignMobileDeviceInvitationResourceModel_ReadNameAsymmetry pins the
// read-name side of the write-name≠read-name contract: the schema's
// `require_login` is populated from the wire `login_required` field
// (c.LoginRequired), and `multiple_uses_allowed` from c.MultipleUsesAllowed.
// Wiring require_login from the wrong source field is an easy regression, so it
// is asserted explicitly.
func TestAssignMobileDeviceInvitationResourceModel_ReadNameAsymmetry(t *testing.T) {
	state := MobileDeviceInvitationResourceModel{}
	src := &proclassic.MobileDeviceInvitation{
		ID:                  new(260),
		InvitationType:      new("USER_INITIATED_EMAIL"),
		MultipleUsesAllowed: new(true),
		LoginRequired:       new(true),
	}
	assignMobileDeviceInvitationResourceModel(&state, src)

	if !state.MultipleUsesAllowed.ValueBool() {
		t.Errorf("multiple_uses_allowed must be read from c.MultipleUsesAllowed")
	}
	if !state.RequireLogin.ValueBool() {
		t.Errorf("require_login must be read from the wire login_required field (c.LoginRequired)")
	}
}

// TestAssignMobileDeviceInvitationResourceModel_EmptyEmailCollapsesToNull
// verifies that an omitted email field the server echoes as "" lands as null in
// state (StringPointerValueOrNull collapses ""→null), so an Optional-only field
// the user omitted does not surface a post-apply inconsistency ("" vs null).
func TestAssignMobileDeviceInvitationResourceModel_EmptyEmailCollapsesToNull(t *testing.T) {
	state := MobileDeviceInvitationResourceModel{}
	src := &proclassic.MobileDeviceInvitation{
		ID:      new(260),
		Subject: new(""),
		Message: new(""),
		ReplyTo: new(""),
	}
	assignMobileDeviceInvitationResourceModel(&state, src)

	for name, v := range map[string]types.String{"subject": state.Subject, "message": state.Message, "reply_to": state.ReplyTo} {
		if !v.IsNull() {
			t.Errorf("empty-echo %s must collapse to null, got %q", name, v.ValueString())
		}
	}
}

func TestAssignMobileDeviceInvitationResourceModel_DriftAndSentinels(t *testing.T) {
	state := MobileDeviceInvitationResourceModel{
		// User-authored value that the server will echo back drifted.
		ExpirationDate: types.StringValue("2026-12-31 23:59:00"),
	}
	src := &proclassic.MobileDeviceInvitation{
		ID:                  new(260),
		Invitation:          bigInt("308000000000000000000000000000000000001"),
		InvitationType:      new("USER_INITIATED_EMAIL"),
		ExpirationDate:      new("2026-12-31 23:58:59.918"),
		ExpirationDateEpoch: bigInt("1798761539918"),
		ExpirationDateUtc:   new("2026-12-31T23:58:59.918+0000"),
		EnrollIntoSite:      &proclassic.MobileDeviceInvitationEnrollIntoSite{ID: new(-1), Name: new("NONE")},
		MultipleUsesAllowed: new(true),
		LoginRequired:       new(false),
		TargetIos:           new("iOS 4"),
		LastAction:          new("SENT"),
		DateSent:            new("2026-06-04 12:00:00"),
		DateSentEpoch:       bigInt("1780000000000"),
	}

	assignMobileDeviceInvitationResourceModel(&state, src)

	if state.ID.ValueString() != "260" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.ExpirationDate.ValueString() != "2026-12-31 23:59:00" {
		t.Errorf("expiration_date drift not reconciled: got %q", state.ExpirationDate.ValueString())
	}
	if state.EnrollIntoSiteID.ValueString() != "-1" {
		t.Errorf("no-site id must echo -1, got %q", state.EnrollIntoSiteID.ValueString())
	}
	if !state.EnrollIntoSiteName.IsNull() {
		t.Errorf("no-site name must be null on the sentinel, got %q", state.EnrollIntoSiteName.ValueString())
	}
	if state.Invitation.ValueString() != "308000000000000000000000000000000000001" {
		t.Errorf("invitation precision lost: %q", state.Invitation.ValueString())
	}
	if state.DateSentEpoch.ValueString() != "1780000000000" {
		t.Errorf("date_sent_epoch lost: %q", state.DateSentEpoch.ValueString())
	}
	if state.TargetIos.ValueString() != "iOS 4" {
		t.Errorf("target_ios not mapped: %q", state.TargetIos.ValueString())
	}
}
