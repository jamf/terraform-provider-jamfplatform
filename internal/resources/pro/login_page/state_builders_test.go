// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package login_page

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// fullResponse returns a LoginContent with distinct values per field so a swapped mapping
// is caught.
func fullResponse() *pro.LoginContent {
	return &pro.LoginContent{
		IncludeCustomDisclaimer: true,
		DisclaimerHeading:       "heading-r",
		DisclaimerMainText:      "main-r",
		ActionText:              "action-r",
	}
}

func TestAssignLoginPageSettingsResourceModel_AllFields(t *testing.T) {
	var state LoginPageSettingsResourceModel
	assignLoginPageSettingsResourceModel(&state, fullResponse())

	if state.IncludeCustomDisclaimer.IsNull() || !state.IncludeCustomDisclaimer.ValueBool() {
		t.Errorf("include_custom_disclaimer = %v, want true", state.IncludeCustomDisclaimer)
	}
	if state.DisclaimerHeading.ValueString() != "heading-r" {
		t.Errorf("disclaimer_heading = %q, want heading-r", state.DisclaimerHeading.ValueString())
	}
	if state.DisclaimerMainText.ValueString() != "main-r" {
		t.Errorf("disclaimer_main_text = %q, want main-r", state.DisclaimerMainText.ValueString())
	}
	if state.ActionText.ValueString() != "action-r" {
		t.Errorf("action_text = %q, want action-r", state.ActionText.ValueString())
	}
}

func TestAssignLoginPageSettingsDataSourceModel_AllFields(t *testing.T) {
	var state LoginPageSettingsDataSourceModel
	assignLoginPageSettingsDataSourceModel(&state, fullResponse())

	if !state.IncludeCustomDisclaimer.ValueBool() {
		t.Errorf("include_custom_disclaimer = %v, want true", state.IncludeCustomDisclaimer)
	}
	if state.DisclaimerHeading.ValueString() != "heading-r" {
		t.Errorf("disclaimer_heading = %q, want heading-r", state.DisclaimerHeading.ValueString())
	}
	if state.ActionText.ValueString() != "action-r" {
		t.Errorf("action_text = %q, want action-r", state.ActionText.ValueString())
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched.
func TestAssign_DoesNotClobberID(t *testing.T) {
	state := LoginPageSettingsResourceModel{ID: types.StringValue("singleton")}
	assignLoginPageSettingsResourceModel(&state, fullResponse())
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := LoginPageSettingsDataSourceModel{ID: types.StringValue("singleton")}
	assignLoginPageSettingsDataSourceModel(&dsState, fullResponse())
	if dsState.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered on data source: got %q, want %q", dsState.ID.ValueString(), "singleton")
	}
}

// TestSingletonIDConstant pins the import identifier.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
