// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package self_service_branding_macos_test

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const (
	macosResourceAddress   = "jamfplatform_pro_self_service_branding_macos.test"
	macosDataSourceAddress = "data.jamfplatform_pro_self_service_branding_macos.test"
)

func macosClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

// captureAndClearMacos saves the tenant's existing macOS branding (if any) and
// deletes it so the test's create step (POST) succeeds — the endpoint caps the
// tenant at one configuration. The saved config is restored on cleanup so the
// shared tenant is left as found.
func captureAndClearMacos(t *testing.T) {
	t.Helper()
	c := macosClient(t)
	ctx := context.Background()

	existing, err := c.ListMacOSBrandingConfigurationsV1(ctx, nil)
	if err != nil {
		t.Fatalf("listing existing macOS branding: %v", err)
	}
	var saved *pro.MacOsBrandingConfiguration
	if len(existing) > 0 {
		saved = &existing[0]
		if saved.ID != nil {
			if err := c.DeleteMacOSBrandingConfigurationV1(ctx, *saved.ID); err != nil {
				t.Fatalf("clearing existing macOS branding: %v", err)
			}
		}
	}

	t.Cleanup(func() {
		// Remove whatever the test left, then restore the saved config.
		if cur, err := c.ListMacOSBrandingConfigurationsV1(ctx, nil); err == nil {
			for i := range cur {
				if cur[i].ID != nil {
					_ = c.DeleteMacOSBrandingConfigurationV1(ctx, *cur[i].ID)
				}
			}
		}
		if saved != nil {
			restore := *saved
			restore.ID = nil
			if _, err := c.CreateMacOSBrandingConfigurationV1(ctx, &restore); err != nil {
				t.Logf("warning: failed to restore original macOS branding after test: %v", err)
			}
		}
	})
}

// checkMacosAbsent is the CheckDestroy: after destroy the real DELETE must have
// run, leaving no macOS branding configuration on the tenant.
func checkMacosAbsent(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := macosClient(t).ListMacOSBrandingConfigurationsV1(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("listing macOS branding post-destroy: %w", err)
		}
		if len(got) != 0 {
			return fmt.Errorf("expected no macOS branding after destroy, got %d", len(got))
		}
		return nil
	}
}

func writeMacosPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			m.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating PNG: %v", err)
	}
	if err := png.Encode(f, m); err != nil {
		_ = f.Close()
		t.Fatalf("encoding PNG: %v", err)
	}
	// Close is checked rather than deferred: a dropped Close can hide a flush
	// failure that leaves the fixture truncated, which then fails somewhere far
	// less obvious than here.
	if err := f.Close(); err != nil {
		t.Fatalf("closing PNG: %v", err)
	}
}

func macosConfigCreate(iconPath, bannerPath string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_self_service_branding_image" "icon" {
  image_file_source = %q
}
resource "jamfplatform_pro_self_service_branding_image" "banner" {
  image_file_source = %q
}
resource "jamfplatform_pro_self_service_branding_macos" "test" {
  application_header = "TF Acc App"
  sidebar_heading    = "TF Sidebar"
  home_page_heading  = "TF Home"
  icon_id            = tonumber(jamfplatform_pro_self_service_branding_image.icon.id)
  banner_image_id    = tonumber(jamfplatform_pro_self_service_branding_image.banner.id)
}
`, iconPath, bannerPath)
}

func macosConfigUpdate(iconPath, bannerPath string) string {
	// sidebar_heading dropped (omit ⇒ cleared on the full-replace PUT);
	// home_page_subheading added.
	return fmt.Sprintf(`
resource "jamfplatform_pro_self_service_branding_image" "icon" {
  image_file_source = %q
}
resource "jamfplatform_pro_self_service_branding_image" "banner" {
  image_file_source = %q
}
resource "jamfplatform_pro_self_service_branding_macos" "test" {
  application_header   = "TF Acc App Updated"
  home_page_heading    = "TF Home"
  home_page_subheading = "TF Home Sub"
  icon_id              = tonumber(jamfplatform_pro_self_service_branding_image.icon.id)
  banner_image_id      = tonumber(jamfplatform_pro_self_service_branding_image.banner.id)
}
`, iconPath, bannerPath)
}

func macosConfigWithDataSource(iconPath, bannerPath string) string {
	return macosConfigUpdate(iconPath, bannerPath) + `
data "jamfplatform_pro_self_service_branding_macos" "test" {
  depends_on = [jamfplatform_pro_self_service_branding_macos.test]
}
`
}

func TestAccResource_ProSelfServiceBrandingMacos(t *testing.T) {
	captureAndClearMacos(t)

	dir := t.TempDir()
	icon := filepath.Join(dir, "icon.png")
	banner := filepath.Join(dir, "banner.png")
	writeMacosPNG(t, icon, 180, 180)
	writeMacosPNG(t, banner, 1500, 235)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkMacosAbsent(t),
		Steps: []resource.TestStep{
			{
				Config: macosConfigCreate(icon, banner),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macosResourceAddress, "id", "singleton"),
					resource.TestCheckResourceAttr(macosResourceAddress, "application_header", "TF Acc App"),
					resource.TestCheckResourceAttr(macosResourceAddress, "sidebar_heading", "TF Sidebar"),
					resource.TestCheckResourceAttrSet(macosResourceAddress, "icon_id"),
					resource.TestCheckResourceAttrSet(macosResourceAddress, "banner_image_id"),
				),
			},
			{
				Config: macosConfigUpdate(icon, banner),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macosResourceAddress, "application_header", "TF Acc App Updated"),
					resource.TestCheckResourceAttr(macosResourceAddress, "home_page_subheading", "TF Home Sub"),
					// sidebar_heading omitted ⇒ cleared (full-replace).
					resource.TestCheckNoResourceAttr(macosResourceAddress, "sidebar_heading"),
				),
			},
			{
				ResourceName:            macosResourceAddress,
				ImportState:             true,
				ImportStateId:           "singleton",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: macosConfigWithDataSource(icon, banner),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macosDataSourceAddress, "application_header", "TF Acc App Updated"),
					resource.TestCheckResourceAttr(macosDataSourceAddress, "home_page_subheading", "TF Home Sub"),
					resource.TestCheckResourceAttr(macosDataSourceAddress, "id", "singleton"),
				),
			},
		},
	})
}

// TestAccResource_ProSelfServiceBrandingMacos_RejectsBadImport pins that import
// with anything other than "singleton" is rejected.
func TestAccResource_ProSelfServiceBrandingMacos_RejectsBadImport(t *testing.T) {
	captureAndClearMacos(t)

	dir := t.TempDir()
	icon := filepath.Join(dir, "icon.png")
	banner := filepath.Join(dir, "banner.png")
	writeMacosPNG(t, icon, 180, 180)
	writeMacosPNG(t, banner, 1500, 235)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkMacosAbsent(t),
		Steps: []resource.TestStep{
			{
				Config: macosConfigCreate(icon, banner),
			},
			{
				ResourceName:  macosResourceAddress,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				// \s+ between words: Terraform wraps the error detail at ~80
				// cols, so the space after "must" may render as a newline.
				ExpectError: regexp.MustCompile(`must\s+be\s+imported\s+with\s+id`),
			},
		},
	})
}
