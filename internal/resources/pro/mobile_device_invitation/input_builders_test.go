// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_invitation

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildMobileDeviceInvitationInput_FullPayload(t *testing.T) {
	plan := MobileDeviceInvitationResourceModel{
		InvitationType:             types.StringValue("USER_INITIATED_EMAIL"),
		ExpirationDate:             types.StringValue("2026-12-31 23:59:00"),
		KeepExistingSiteMembership: types.BoolValue(true),
		MultipleUsesAllowed:        types.BoolValue(true),
		RequireLogin:               types.BoolValue(true),
		Subject:                    types.StringValue("Enroll your device"),
		Message:                    types.StringValue("Tap the link"),
		ReplyTo:                    types.StringValue("noreply@example.com"),
		SentFrom:                   types.StringValue("it@example.com"),
		SentTo:                     types.StringValue("user@example.com"),
		Username:                   types.StringValue("jdoe"),
		TargetIos:                  types.StringValue("iOS 17"),
	}

	got := buildMobileDeviceInvitationInput(plan, "7")

	if got.InvitationType == nil || *got.InvitationType != "USER_INITIATED_EMAIL" {
		t.Errorf("invitation_type not propagated: %v", got.InvitationType)
	}
	if got.ExpirationDate == nil || *got.ExpirationDate != "2026-12-31 23:59:00" {
		t.Errorf("expiration_date not propagated: %v", got.ExpirationDate)
	}
	if got.Subject == nil || *got.Subject != "Enroll your device" {
		t.Errorf("subject not propagated: %v", got.Subject)
	}
	if got.SentTo == nil || *got.SentTo != "user@example.com" {
		t.Errorf("sent_to not propagated: %v", got.SentTo)
	}
	if got.TargetIos != nil {
		t.Errorf("target_ios must never be sent, Jamf Pro ignores it: %v", *got.TargetIos)
	}
	if got.EnrollIntoSite == nil || got.EnrollIntoSite.ID == nil || *got.EnrollIntoSite.ID != 7 {
		t.Fatalf("enroll_into_site ID not set from enroll_into_site_id, got %+v", got.EnrollIntoSite)
	}
}

// TestBuildMobileDeviceInvitationInput_WriteNameAsymmetry pins the load-bearing
// write-name≠read-name contract: on WRITE, "multiple uses" and "require login"
// go out under the names allow_multiple_uses (Post.AllowMultipleUses) and
// require_login (Post.RequireLogin). The Post type also carries the read-side
// names MultipleUsesAllowed / LoginRequired, which the server ignores on write
// and which must therefore be left nil. This is the only guard protecting the
// mapping through a future SDK regen.
func TestBuildMobileDeviceInvitationInput_WriteNameAsymmetry(t *testing.T) {
	plan := MobileDeviceInvitationResourceModel{
		InvitationType:      types.StringValue("USER_INITIATED_URL"),
		MultipleUsesAllowed: types.BoolValue(true),
		RequireLogin:        types.BoolValue(true),
	}
	got := buildMobileDeviceInvitationInput(plan, "")

	if got.AllowMultipleUses == nil || !*got.AllowMultipleUses {
		t.Errorf("multiple_uses_allowed must be written via AllowMultipleUses (allow_multiple_uses), got %v", got.AllowMultipleUses)
	}
	if got.RequireLogin == nil || !*got.RequireLogin {
		t.Errorf("require_login must be written via RequireLogin (require_login), got %v", got.RequireLogin)
	}
	// The read-side names must NOT be set on write (server ignores them).
	if got.MultipleUsesAllowed != nil {
		t.Errorf("read-name MultipleUsesAllowed must be nil on write, got %v", got.MultipleUsesAllowed)
	}
	if got.LoginRequired != nil {
		t.Errorf("read-name LoginRequired must be nil on write, got %v", got.LoginRequired)
	}
}

func TestBuildMobileDeviceInvitationInput_OmitsSiteWhenEmpty(t *testing.T) {
	plan := MobileDeviceInvitationResourceModel{
		InvitationType: types.StringValue("USER_INITIATED_EMAIL"),
	}
	got := buildMobileDeviceInvitationInput(plan, "")
	if got.EnrollIntoSite != nil {
		t.Errorf("expected no enroll_into_site block when site id empty, got %+v", got.EnrollIntoSite)
	}
}

func TestBuildMobileDeviceInvitationInput_OmitsNullBools(t *testing.T) {
	plan := MobileDeviceInvitationResourceModel{
		InvitationType:      types.StringValue("USER_INITIATED_URL"),
		MultipleUsesAllowed: types.BoolNull(),
		RequireLogin:        types.BoolUnknown(),
	}
	got := buildMobileDeviceInvitationInput(plan, "")
	if got.AllowMultipleUses != nil {
		t.Errorf("null bool must serialise as nil (omitted), got %v", got.AllowMultipleUses)
	}
	if got.RequireLogin != nil {
		t.Errorf("unknown bool must serialise as nil (omitted), got %v", got.RequireLogin)
	}
}
