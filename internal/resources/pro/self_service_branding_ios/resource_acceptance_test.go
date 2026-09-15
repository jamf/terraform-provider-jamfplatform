// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package self_service_branding_ios_test

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
	iosResourceAddress   = "jamfplatform_pro_self_service_branding_ios.test"
	iosDataSourceAddress = "data.jamfplatform_pro_self_service_branding_ios.test"
)

func iosClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

// captureAndClearIos saves the tenant's existing iOS branding (if any) and
// deletes it so the test's create step succeeds (the endpoint caps the tenant
// at one configuration). Restored on cleanup so the tenant is left as found.
func captureAndClearIos(t *testing.T) {
	t.Helper()
	c := iosClient(t)
	ctx := context.Background()

	existing, err := c.ListIOSBrandingConfigurationsV1(ctx, nil)
	if err != nil {
		t.Fatalf("listing existing iOS branding: %v", err)
	}
	var saved *pro.IosBrandingConfiguration
	if len(existing) > 0 {
		saved = &existing[0]
		if saved.ID != nil {
			if err := c.DeleteIOSBrandingConfigurationV1(ctx, *saved.ID); err != nil {
				t.Fatalf("clearing existing iOS branding: %v", err)
			}
		}
	}

	t.Cleanup(func() {
		if cur, err := c.ListIOSBrandingConfigurationsV1(ctx, nil); err == nil {
			for i := range cur {
				if cur[i].ID != nil {
					_ = c.DeleteIOSBrandingConfigurationV1(ctx, *cur[i].ID)
				}
			}
		}
		if saved != nil {
			restore := *saved
			restore.ID = nil
			if _, err := c.CreateIOSBrandingConfigurationV1(ctx, &restore); err != nil {
				t.Logf("warning: failed to restore original iOS branding after test: %v", err)
			}
		}
	})
}

func checkIosAbsent(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := iosClient(t).ListIOSBrandingConfigurationsV1(context.Background(), nil)
		if err != nil {
			return fmt.Errorf("listing iOS branding post-destroy: %w", err)
		}
		if len(got) != 0 {
			return fmt.Errorf("expected no iOS branding after destroy, got %d", len(got))
		}
		return nil
	}
}

func writeIosPNG(t *testing.T, path string) {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 180, 180))
	for y := range 180 {
		for x := range 180 {
			m.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
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

func iosConfigCreate(iconPath string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_self_service_branding_image" "icon" {
  image_file_source = %q
}
resource "jamfplatform_pro_self_service_branding_ios" "test" {
  main_header                  = "TF Acc Self Service"
  branding_name_color_code     = "000000"
  header_background_color_code = "FFFFFF"
  menu_icon_color_code         = "007AFF"
  status_bar_text_color        = "dark"
  icon_id                      = tonumber(jamfplatform_pro_self_service_branding_image.icon.id)
}
`, iconPath)
}

func iosConfigUpdate(iconPath string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_self_service_branding_image" "icon" {
  image_file_source = %q
}
resource "jamfplatform_pro_self_service_branding_ios" "test" {
  main_header                  = "TF Acc Updated"
  branding_name_color_code     = "112233"
  header_background_color_code = "445566"
  menu_icon_color_code         = "778899"
  status_bar_text_color        = "light"
}
`, iconPath)
}

func iosConfigWithDataSource(iconPath string) string {
	return iosConfigUpdate(iconPath) + `
data "jamfplatform_pro_self_service_branding_ios" "test" {
  depends_on = [jamfplatform_pro_self_service_branding_ios.test]
}
`
}

func TestAccResource_ProSelfServiceBrandingIos(t *testing.T) {
	captureAndClearIos(t)

	dir := t.TempDir()
	icon := filepath.Join(dir, "icon.png")
	writeIosPNG(t, icon)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIosAbsent(t),
		Steps: []resource.TestStep{
			{
				Config: iosConfigCreate(icon),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(iosResourceAddress, "id", "singleton"),
					resource.TestCheckResourceAttr(iosResourceAddress, "main_header", "TF Acc Self Service"),
					resource.TestCheckResourceAttr(iosResourceAddress, "header_background_color_code", "FFFFFF"),
					resource.TestCheckResourceAttr(iosResourceAddress, "status_bar_text_color", "dark"),
					resource.TestCheckResourceAttrSet(iosResourceAddress, "icon_id"),
				),
			},
			{
				Config: iosConfigUpdate(icon),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(iosResourceAddress, "main_header", "TF Acc Updated"),
					resource.TestCheckResourceAttr(iosResourceAddress, "menu_icon_color_code", "778899"),
					resource.TestCheckResourceAttr(iosResourceAddress, "status_bar_text_color", "light"),
					// icon_id omitted ⇒ cleared (full-replace).
					resource.TestCheckNoResourceAttr(iosResourceAddress, "icon_id"),
				),
			},
			{
				ResourceName:            iosResourceAddress,
				ImportState:             true,
				ImportStateId:           "singleton",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: iosConfigWithDataSource(icon),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(iosDataSourceAddress, "main_header", "TF Acc Updated"),
					resource.TestCheckResourceAttr(iosDataSourceAddress, "status_bar_text_color", "light"),
					resource.TestCheckResourceAttr(iosDataSourceAddress, "id", "singleton"),
				),
			},
		},
	})
}

// TestAccResource_ProSelfServiceBrandingIos_RejectsBadImport pins that import
// with anything other than "singleton" is rejected.
func TestAccResource_ProSelfServiceBrandingIos_RejectsBadImport(t *testing.T) {
	captureAndClearIos(t)

	dir := t.TempDir()
	icon := filepath.Join(dir, "icon.png")
	writeIosPNG(t, icon)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIosAbsent(t),
		Steps: []resource.TestStep{
			{
				Config: iosConfigCreate(icon),
			},
			{
				ResourceName:  iosResourceAddress,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				// \s+ between words: Terraform wraps the error detail at ~80
				// cols, so the space after "must" may render as a newline.
				ExpectError: regexp.MustCompile(`must\s+be\s+imported\s+with\s+id`),
			},
		},
	})
}
