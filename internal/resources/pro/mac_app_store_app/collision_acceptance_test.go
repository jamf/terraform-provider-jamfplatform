// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"strconv"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingMacAppStoreApp(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	bundle := "com.example.tfacc.macapp.existing"
	app := "https://apps.apple.com/app/id000000003"
	version := "1.0"
	created, err := c.CreateMacApplicationByID(ctx, "0", &proclassic.MacApplication{General: &proclassic.MacApplicationGeneral{Name: &name, BundleID: &bundle, URL: &app, Version: &version, Category: &proclassic.CategoryObject{ID: new(-1)}}})
	if err != nil || created.ID == nil {
		t.Fatalf("creating Mac App Store app %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteMacApplicationByID(ctx, id) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.macapp.collide"
				url             = "https://apps.apple.com/app/id000000002"
				deployment_type = "Make Available in Self Service"
			}
		}
	`, name)
}

func TestAccResource_ProMacAppStoreApp_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mac-app-store-app-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingMacAppStoreApp(t, name) },
				Config:      collisionConfig(strings.ToUpper(name) + " "),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*Mac\s+App\s+Store\s+app.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}
