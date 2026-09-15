// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_macos_settings

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// default_landing_page validates against explicit SDK constants rather than
// SelfServiceInteractionSettingsDefaultLandingPageValues(), so an SDK bump cannot
// widen validation while the attribute description still lists four pages in
// prose. This is the tripwire for a page Jamf adds.
func TestDefaultLandingPageEnum_HasNotGrown(t *testing.T) {
	want := map[string]bool{
		pro.SelfServiceInteractionSettingsDefaultLandingPageHome:          true,
		pro.SelfServiceInteractionSettingsDefaultLandingPageBrowse:        true,
		pro.SelfServiceInteractionSettingsDefaultLandingPageHistory:       true,
		pro.SelfServiceInteractionSettingsDefaultLandingPageNotifications: true,
	}
	got := pro.SelfServiceInteractionSettingsDefaultLandingPageValues()
	for _, v := range got {
		if !want[v] {
			t.Errorf("SelfServiceInteractionSettingsDefaultLandingPage gained value %q: update the default_landing_page validator and its description", v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("SelfServiceInteractionSettingsDefaultLandingPage has %d values, schema validates %d", len(got), len(want))
	}
}

// The description enumerates the accepted pages in prose. Since the validator is
// an explicit list, the two can drift independently — assert they agree, so a
// future edit to one is forced to touch the other.
func TestDefaultLandingPageDescription_ListsEveryAcceptedValue(t *testing.T) {
	var resp resource.SchemaResponse
	NewSelfServiceMacosSettingsResource().(*SelfServiceMacosSettingsResource).
		Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	attr, ok := resp.Schema.Attributes["default_landing_page"]
	if !ok {
		t.Fatal("default_landing_page attribute missing from schema")
	}
	desc := attr.GetMarkdownDescription()

	for v := range map[string]bool{
		pro.SelfServiceInteractionSettingsDefaultLandingPageHome:          true,
		pro.SelfServiceInteractionSettingsDefaultLandingPageBrowse:        true,
		pro.SelfServiceInteractionSettingsDefaultLandingPageHistory:       true,
		pro.SelfServiceInteractionSettingsDefaultLandingPageNotifications: true,
	} {
		if !strings.Contains(desc, v) {
			t.Errorf("default_landing_page description does not mention accepted value %q", v)
		}
	}
}
