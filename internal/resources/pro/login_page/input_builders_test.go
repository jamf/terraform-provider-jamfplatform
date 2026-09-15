// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package login_page

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestBuildLoginPageSettingsInput verifies every plan field maps to the correct SDK field.
// Distinct values per field catch a swapped mapping. The three text fields are emitted as
// non-nil *string (always-emit — the server requires them on every write).
func TestBuildLoginPageSettingsInput(t *testing.T) {
	plan := LoginPageSettingsResourceModel{
		IncludeCustomDisclaimer: types.BoolValue(true),
		DisclaimerHeading:       types.StringValue("heading-v"),
		DisclaimerMainText:      types.StringValue("main-v"),
		ActionText:              types.StringValue("action-v"),
	}

	out := buildLoginPageSettingsInput(plan, nil)

	if !out.IncludeCustomDisclaimer {
		t.Errorf("IncludeCustomDisclaimer = false, want true")
	}
	if out.DisclaimerHeading == nil || *out.DisclaimerHeading != "heading-v" {
		t.Errorf("DisclaimerHeading = %v, want heading-v", out.DisclaimerHeading)
	}
	if out.DisclaimerMainText == nil || *out.DisclaimerMainText != "main-v" {
		t.Errorf("DisclaimerMainText = %v, want main-v", out.DisclaimerMainText)
	}
	if out.ActionText == nil || *out.ActionText != "action-v" {
		t.Errorf("ActionText = %v, want action-v", out.ActionText)
	}
}

// TestBuildLoginPageSettingsInput_OmittedAdoptsCurrent verifies the GET-on-create merge: a
// field omitted from the plan (Unknown/Null, as on first create) takes the value from the
// live settings `current` rather than being sent empty (which the server rejects). A
// declared field still wins over current.
func TestBuildLoginPageSettingsInput_OmittedAdoptsCurrent(t *testing.T) {
	current := &pro.LoginContent{
		IncludeCustomDisclaimer: true,           // omitted in plan -> adopt true
		DisclaimerHeading:       "live-heading", // omitted -> adopt
		DisclaimerMainText:      "live-main",    // declared in plan -> plan wins
		ActionText:              "live-action",  // omitted -> adopt
	}
	plan := LoginPageSettingsResourceModel{
		IncludeCustomDisclaimer: types.BoolUnknown(),
		DisclaimerHeading:       types.StringNull(),
		DisclaimerMainText:      types.StringValue("plan-main"), // declared
		ActionText:              types.StringUnknown(),
	}

	out := buildLoginPageSettingsInput(plan, current)

	if !out.IncludeCustomDisclaimer {
		t.Errorf("IncludeCustomDisclaimer: omitted should adopt current true")
	}
	if out.DisclaimerHeading == nil || *out.DisclaimerHeading != "live-heading" {
		t.Errorf("DisclaimerHeading = %v, want adopted live-heading", out.DisclaimerHeading)
	}
	if out.DisclaimerMainText == nil || *out.DisclaimerMainText != "plan-main" {
		t.Errorf("DisclaimerMainText = %v, want plan-main (plan wins)", out.DisclaimerMainText)
	}
	if out.ActionText == nil || *out.ActionText != "live-action" {
		t.Errorf("ActionText = %v, want adopted live-action", out.ActionText)
	}
}

// TestBuildLoginPageSettingsInput_NilCurrentOmittedEmpty verifies that with a nil merge base
// (update path) an omitted field falls back to empty string. In practice this never happens
// on update because UseStateForUnknown fills omitted fields with their prior known value
// before the plan reaches the builder.
func TestBuildLoginPageSettingsInput_NilCurrentOmittedEmpty(t *testing.T) {
	plan := LoginPageSettingsResourceModel{
		IncludeCustomDisclaimer: types.BoolUnknown(),
		DisclaimerHeading:       types.StringUnknown(),
		DisclaimerMainText:      types.StringValue("declared-main"),
		ActionText:              types.StringNull(),
	}

	out := buildLoginPageSettingsInput(plan, nil)

	if out.IncludeCustomDisclaimer {
		t.Errorf("omitted toggle with nil current should be false")
	}
	if out.DisclaimerHeading == nil || *out.DisclaimerHeading != "" {
		t.Errorf("omitted heading with nil current should be empty string, got %v", out.DisclaimerHeading)
	}
	if out.DisclaimerMainText == nil || *out.DisclaimerMainText != "declared-main" {
		t.Errorf("declared main should be declared-main, got %v", out.DisclaimerMainText)
	}
}
