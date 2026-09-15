// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /dockitems endpoint. Classic
// has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any other classic acceptance work in
// this package.

package dock_item_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckDockItemDestroy verifies dock items created during the test
// were destroyed.
func testAccCheckDockItemDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_dock_item" {
				continue
			}
			_, err := c.GetDockItemByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro dock item %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro dock item %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func dockItemConfig(name, dockType, path string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_dock_item" "test" {
			name = %q
			type = %q
			path = %q
		}
	`, name, dockType, path)
}

// TestAccResource_ProDockItem_App exercises App-type CRUD: create with concrete
// name/type/path, in-place rename + path change (verifies server recomputes
// PLIST contents on update — see Phase A audit option 2a, no plan modifier on
// contents), and import. The rename step also covers the GET-after-Update
// path (classic Update returns 201 + empty body).
func TestAccResource_ProDockItem_App(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-dock-item-app-" + suffix
	renamed := "tf-acc-dock-item-app-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDockItemDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: dockItemConfig(original, "App", "/Applications/Calculator.app"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_dock_item.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "name", original),
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "type", "App"),
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "path", "/Applications/Calculator.app"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_dock_item.test", "contents"),
				),
			},
			{
				// Rename + change path. Server must recompute the PLIST
				// contents to reflect the new file-label + _CFURLString.
				// Assert contents reflects the new path — proves the 2a
				// "no plan modifier on contents" choice surfaces drift on
				// every update without retaining stale state.
				Config: dockItemConfig(renamed, "App", "/Applications/TextEdit.app"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "name", renamed),
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "path", "/Applications/TextEdit.app"),
					resource.TestMatchResourceAttr("jamfplatform_pro_dock_item.test", "contents", regexp.MustCompile("TextEdit")),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_dock_item.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProDockItem_File exercises File-type CRUD with a URI-style
// path (the Jamf UI hints at `file://localhost/...`).
func TestAccResource_ProDockItem_File(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-dock-item-file-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDockItemDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: dockItemConfig(name, "File", "file://localhost/Library/Documentation/README.txt"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "type", "File"),
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "path", "file://localhost/Library/Documentation/README.txt"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_dock_item.test", "contents"),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_dock_item.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProDockItem_Folder exercises Folder-type CRUD. Folders emit
// tile-type=directory-tile in the PLIST (vs file-tile for App/File).
func TestAccResource_ProDockItem_Folder(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-dock-item-folder-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDockItemDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: dockItemConfig(name, "Folder", "~/Downloads"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "type", "Folder"),
					resource.TestCheckResourceAttr("jamfplatform_pro_dock_item.test", "path", "~/Downloads"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_dock_item.test", "contents"),
				),
			},
		},
	})
}
