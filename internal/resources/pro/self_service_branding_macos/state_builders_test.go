// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_macos

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestIntPtrValueOrNull(t *testing.T) {
	if v := intPtrValueOrNull(nil); !v.IsNull() {
		t.Errorf("intPtrValueOrNull(nil) = %v, want null", v)
	}
	if v := intPtrValueOrNull(new(7)); v.ValueInt64() != 7 {
		t.Errorf("intPtrValueOrNull(7) = %v, want 7", v)
	}
}

func TestAssignSelfServiceBrandingMacosResourceModel_Nulls(t *testing.T) {
	var state SelfServiceBrandingMacosResourceModel
	state.ID = types.StringValue("preexisting")
	assignSelfServiceBrandingMacosResourceModel(&state, &pro.MacOsBrandingConfiguration{})

	if !state.ApplicationHeader.IsNull() || !state.SidebarHeading.IsNull() ||
		!state.SidebarSubheading.IsNull() || !state.HomePageHeading.IsNull() ||
		!state.HomePageSubheading.IsNull() || !state.IconID.IsNull() || !state.BannerImageID.IsNull() {
		t.Errorf("expected all-null state for empty config, got %+v", state)
	}
	// Assigner must not clobber ID.
	if state.ID.ValueString() != "preexisting" {
		t.Errorf("assigner clobbered ID: %q", state.ID.ValueString())
	}
}

func TestAssignSelfServiceBrandingMacosResourceModel_Values(t *testing.T) {
	var state SelfServiceBrandingMacosResourceModel
	cfg := &pro.MacOsBrandingConfiguration{
		ApplicationName:       new("App"),
		BrandingName:          new("Side"),
		BrandingNameSecondary: new("Sub"),
		HomeHeading:           new("Home"),
		HomeSubheading:        new("HomeSub"),
		IconID:                new(81),
		BrandingHeaderImageID: new(82),
	}
	assignSelfServiceBrandingMacosResourceModel(&state, cfg)

	if state.ApplicationHeader.ValueString() != "App" {
		t.Errorf("ApplicationHeader = %q", state.ApplicationHeader.ValueString())
	}
	if state.SidebarSubheading.ValueString() != "Sub" {
		t.Errorf("SidebarSubheading = %q", state.SidebarSubheading.ValueString())
	}
	if state.IconID.ValueInt64() != 81 {
		t.Errorf("IconID = %d", state.IconID.ValueInt64())
	}
	if state.BannerImageID.ValueInt64() != 82 {
		t.Errorf("BannerImageID = %d", state.BannerImageID.ValueInt64())
	}
}
