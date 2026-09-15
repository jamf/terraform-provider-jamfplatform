// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_ios

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestIntPtrValueOrNull(t *testing.T) {
	if v := intPtrValueOrNull(nil); !v.IsNull() {
		t.Errorf("intPtrValueOrNull(nil) = %v, want null", v)
	}
	if v := intPtrValueOrNull(new(5)); v.ValueInt64() != 5 {
		t.Errorf("intPtrValueOrNull(5) = %v, want 5", v)
	}
}

func TestBuildSelfServiceBrandingIosInput(t *testing.T) {
	plan := SelfServiceBrandingIosResourceModel{
		MainHeader:                types.StringValue("Self Service"),
		BrandingNameColorCode:     types.StringValue("000000"),
		HeaderBackgroundColorCode: types.StringValue("FFFFFF"),
		MenuIconColorCode:         types.StringValue("007AFF"),
		StatusBarTextColor:        types.StringValue("dark"),
		IconID:                    types.Int64Null(),
	}
	got := buildSelfServiceBrandingIosInput(plan)
	if got.BrandingName != "Self Service" {
		t.Errorf("BrandingName = %q", got.BrandingName)
	}
	if got.HeaderBackgroundColorCode != "FFFFFF" || got.MenuIconColorCode != "007AFF" || got.StatusBarTextColor != "dark" {
		t.Errorf("colour fields mismatch: %+v", got)
	}
	if got.IconID != nil {
		t.Errorf("IconID = %v, want nil (omitted when null)", got.IconID)
	}

	plan.IconID = types.Int64Value(81)
	if got := buildSelfServiceBrandingIosInput(plan); got.IconID == nil || *got.IconID != 81 {
		t.Errorf("IconID = %v, want 81", got.IconID)
	}
}

func TestAssignSelfServiceBrandingIosResourceModel(t *testing.T) {
	var state SelfServiceBrandingIosResourceModel
	state.ID = types.StringValue("preexisting")
	cfg := &pro.IosBrandingConfiguration{
		BrandingName:              "Self Service",
		BrandingNameColorCode:     "000000",
		HeaderBackgroundColorCode: "FFFFFF",
		MenuIconColorCode:         "007AFF",
		StatusBarTextColor:        "dark",
		IconID:                    new(81),
	}
	assignSelfServiceBrandingIosResourceModel(&state, cfg)

	if state.MainHeader.ValueString() != "Self Service" {
		t.Errorf("MainHeader = %q", state.MainHeader.ValueString())
	}
	if state.StatusBarTextColor.ValueString() != "dark" {
		t.Errorf("StatusBarTextColor = %q", state.StatusBarTextColor.ValueString())
	}
	if state.IconID.ValueInt64() != 81 {
		t.Errorf("IconID = %d", state.IconID.ValueInt64())
	}
	// Assigner must not clobber ID.
	if state.ID.ValueString() != "preexisting" {
		t.Errorf("assigner clobbered ID: %q", state.ID.ValueString())
	}

	// nil iconId ⇒ null.
	var state2 SelfServiceBrandingIosResourceModel
	assignSelfServiceBrandingIosResourceModel(&state2, &pro.IosBrandingConfiguration{BrandingName: "x", StatusBarTextColor: "light"})
	if !state2.IconID.IsNull() {
		t.Errorf("IconID = %v, want null", state2.IconID)
	}
}
