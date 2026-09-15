// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro v3 patch-software-title-configurations
// endpoints, plus the one classic /patchsoftwaretitles call that mints a title's
// id (see crud.go). That classic create has known concurrency issues when
// multiple writes hit the same resource type — keep these tests serial with any
// future classic acceptance work in this package.
//
// The fixtures use the real "8x8 Work" patch catalog entry: name_id "285",
// source_id 1, with versions "8.33.2.2" and "8.32.2.10". If the catalog entry or
// either version is removed from the test tenant's patch source, these tests
// will need updating.

package patch_software_title_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const (
	accTitleNameID     = "285" // 8x8 Work
	accTitleSourceID   = 1
	accTitleVersion    = "8.33.2.2"
	accTitleVersionAlt = "8.32.2.10"
	patchSoftwareType  = "jamfplatform_pro_patch_software_title"
)

// testAccCheckPatchSoftwareTitleDestroy verifies titles created during the test
// were destroyed.
//
// The read goes through the same v3 configurations endpoint the resource itself
// deletes through. Wire-probed 2026-09-02: the v3 DELETE removes the classic
// title with it, so a 404 here means the object is gone from both surfaces.
func testAccCheckPatchSoftwareTitleDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != patchSoftwareType {
				continue
			}
			_, err := c.GetPatchSoftwareTitleConfigurationV3(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro patch software title %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro patch software title %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProPatchSoftwareTitle_Basic exercises create, then a
// multi-attribute in-place update mutating every non-RequiresReplace attribute:
// category_id, site_id, both notification bools, and version_packages (add a key
// in step 2, then change which package it points at AND remove it in step 3 to
// exercise both the assign and the empty-package clear/unassign path). Finally
// import with justified ignores.
//
// Only timeouts is ignored on that import, and every other attribute is
// verified. version_packages included: an import declares nothing and so
// hydrates no assignments (#403), and step 4 left the title with none, so both
// sides are null either way — a step whose configuration declares assignments
// has to ignore it, because imported state holds null while applied state holds
// the declared keys. available_versions is server-derived and already matches by
// this step, and source_id is resolved from the patch source name on import, so
// it round-trips.
func TestAccResource_ProPatchSoftwareTitle_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	cat := "tf-acc-pst-cat-" + suffix
	site := "tf-acc-pst-site-" + suffix
	pkgA := "tf-acc-pst-pkgA-" + suffix
	pkgB := "tf-acc-pst-pkgB-" + suffix

	// Step 1: mint the title with defaults; assign nothing.
	stepCreate := fmt.Sprintf(`
		resource "jamfplatform_pro_package" "pkg_a" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_patch_software_title" "test" {
			name      = "tf-acc 8x8 Work %[3]s"
			name_id   = %q
			source_id = %d
		}
	`, pkgA, pkgA, suffix, accTitleNameID, accTitleSourceID)

	// Step 2: add category + site + flip notifications + assign package A to a
	// real version.
	stepAssign := fmt.Sprintf(`
		resource "jamfplatform_pro_category" "cat" {
			name     = %q
			priority = 9
		}

		resource "jamfplatform_pro_site" "site" {
			name = %q
		}

		resource "jamfplatform_pro_package" "pkg_a" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_patch_software_title" "test" {
			name               = "tf-acc 8x8 Work %[5]s"
			name_id            = %q
			source_id          = %d
			category_id        = jamfplatform_pro_category.cat.id
			site_id            = jamfplatform_pro_site.site.id
			web_notification   = false
			email_notification = false

			version_packages = {
				%q = jamfplatform_pro_package.pkg_a.id
			}
		}
	`, cat, site, pkgA, pkgA, suffix, accTitleNameID, accTitleSourceID, accTitleVersion)

	// Step 3: point the same version at a different package, exercising the
	// re-assign path through the v3 packages array.
	stepClear := fmt.Sprintf(`
		resource "jamfplatform_pro_category" "cat" {
			name     = %q
			priority = 9
		}

		resource "jamfplatform_pro_site" "site" {
			name = %q
		}

		resource "jamfplatform_pro_package" "pkg_a" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_package" "pkg_b" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_patch_software_title" "test" {
			name               = "tf-acc 8x8 Work %[7]s"
			name_id            = %q
			source_id          = %d
			category_id        = "-1"
			site_id            = "-1"
			web_notification   = true
			email_notification = true

			# Re-point version A at package B to prove the assign path mutates in
			# place rather than accumulating.
			version_packages = {
				%q = jamfplatform_pro_package.pkg_b.id
			}
		}
	`, cat, site, pkgA, pkgA, pkgB, pkgB, suffix, accTitleNameID, accTitleSourceID, accTitleVersion)

	// Step 4: remove the version_packages key entirely → exercises the unassign
	// path, where the key drops out of the replacement array Update sends.
	stepUnassign := fmt.Sprintf(`
		resource "jamfplatform_pro_package" "pkg_a" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_package" "pkg_b" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_patch_software_title" "test" {
			name      = "tf-acc 8x8 Work %[5]s"
			name_id   = %q
			source_id = %d
		}
	`, pkgA, pkgA, pkgB, pkgB, suffix, accTitleNameID, accTitleSourceID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchSoftwareTitleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: stepCreate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(patchSoftwareType+".test", "id"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "name_id", accTitleNameID),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "source_id", "1"),
					// Server defaults. v3 reports an unassigned category or site as
					// the literal "-1", the only non-positive id it accepts on a
					// write, so both are always known.
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "category_id", "-1"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "site_id", "-1"),
					// Catalog versions populated by the server.
					resource.TestCheckResourceAttrSet(patchSoftwareType+".test", "available_versions.#"),
				),
			},
			{
				Config: stepAssign,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(patchSoftwareType+".test", "category_id", "jamfplatform_pro_category.cat", "id"),
					resource.TestCheckResourceAttrPair(patchSoftwareType+".test", "site_id", "jamfplatform_pro_site.site", "id"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "web_notification", "false"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "email_notification", "false"),
					resource.TestCheckResourceAttrPair(patchSoftwareType+".test", "version_packages."+accTitleVersion, "jamfplatform_pro_package.pkg_a", "id"),
				),
			},
			{
				Config: stepClear,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "category_id", "-1"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "site_id", "-1"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "web_notification", "true"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "email_notification", "true"),
					// Version A now points at package B.
					resource.TestCheckResourceAttrPair(patchSoftwareType+".test", "version_packages."+accTitleVersion, "jamfplatform_pro_package.pkg_b", "id"),
				),
			},
			{
				Config: stepUnassign,
				Check: resource.ComposeAggregateTestCheckFunc(
					// version_packages fully cleared (unassign path).
					resource.TestCheckNoResourceAttr(patchSoftwareType+".test", "version_packages.%"),
				),
			},
			{
				ResourceName:            patchSoftwareType + ".test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProPatchSoftwareTitle_ImportSettleKeepsAssignments pins #403:
// importing a title that already has packages assigned and then applying the
// settle plan without declaring version_packages must leave every assignment in
// place. It regressed once already — hydration adopted the server's whole
// assignment set into state (#391 via #400), Update read those keys as the
// managed subset, and the fold unassigned every one the configuration did not
// mention. A patch policy on that title was left with no package to deploy.
//
// It earns an acceptance test because both halves are invisible to a unit test:
// the state an import produces comes from the framework, and the erasure lands
// on the server rather than in state. The title is created out of band so the
// import is a real one — an address Terraform already manages cannot be
// imported over.
//
// Step 2 renames the title. Without a change there is nothing to apply, and the
// fold that does the damage never runs.
//
// ImportStateVerify is off because the configuration declares no
// version_packages, so imported state and applied state can never match on it.
// An import step reads neither Check nor ConfigStateChecks, so what the import
// committed is asserted through ImportStateCheck, and what survives on the
// server through the step-2 Check.
func TestAccResource_ProPatchSoftwareTitle_ImportSettleKeepsAssignments(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc 8x8 Work settle " + suffix

	pkgID := testAccCreatePackageOutOfBand(t, "tf-acc-pst-settle-pkg-"+suffix)
	titleID := testAccCreatePatchTitleOutOfBand(t, name, pkgID)

	config := func(n string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_patch_software_title" "test" {
			name      = %q
			name_id   = %q
			source_id = %d
		}
	`, n, accTitleNameID, accTitleSourceID)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchSoftwareTitleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:             config(name),
				ResourceName:       patchSoftwareType + ".test",
				ImportState:        true,
				ImportStateId:      titleID,
				ImportStatePersist: true,
				ImportStateVerify:  false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					if got, ok := states[0].Attributes["version_packages.%"]; ok {
						return fmt.Errorf("import hydrated %s undeclared version_packages entries; it must hydrate none (#403)", got)
					}
					return nil
				},
			},
			{
				Config: config(name + " renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "name", name+" renamed"),
					resource.TestCheckNoResourceAttr(patchSoftwareType+".test", "version_packages.%"),
					testAccCheckPatchTitleAssignment(t, &titleID, accTitleVersion, &pkgID),
				),
			},
		},
	})
}

// testAccCreatePackageOutOfBand creates a metadata-only package (no upload) and
// returns its id. Assigning a package to a title needs nothing more than an id
// the server recognises.
//
// Priority carries the admin UI's own default of 10 because the field is a
// value-type int the server rejects below 1, so a zero value fails the create
// with "priority: must be greater than or equal to 1".
func testAccCreatePackageOutOfBand(t *testing.T, displayName string) string {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	created, err := c.CreatePackageV1(context.Background(), &pro.Package{
		PackageName: displayName,
		FileName:    displayName + ".pkg",
		CategoryID:  "-1",
		Priority:    10,
	})
	if err != nil {
		t.Fatalf("creating a package outside Terraform: %v", err)
	}
	if created == nil || created.ID == "" {
		t.Fatal("creating a package outside Terraform: no id in the response")
	}
	t.Cleanup(func() {
		if err := c.DeletePackageV1(context.Background(), created.ID); err != nil {
			t.Logf("cleaning up package %s: %v", created.ID, err)
		}
	})
	return created.ID
}

// testAccCreatePatchTitleOutOfBand mints a patch software title the way an
// administrator would and assigns packageID to accTitleVersion, then returns the
// title id. Two calls, because that is what minting a title takes: the classic
// POST is the only id-minting create, and package assignments live on the v3
// configuration.
//
// The cleanup is a safety net rather than the normal path — the test imports the
// title, so CheckDestroy is what proves Terraform destroyed it.
func testAccCreatePatchTitleOutOfBand(t *testing.T, name, packageID string) string {
	t.Helper()
	ctx := context.Background()
	classic := proclassic.New(testhelpers.NewAcceptanceClient(t))
	nameID, sourceID := accTitleNameID, accTitleSourceID

	created, err := classic.CreatePatchSoftwareTitleByID(ctx, "0", &proclassic.PatchSoftwareTitle{ //nolint:staticcheck // SA1019: the classic POST is the only id-minting create, as crud.go Create documents
		Name:     &name,
		NameID:   &nameID,
		SourceID: &sourceID,
	})
	if err != nil {
		t.Fatalf("creating a patch software title outside Terraform: %v", err)
	}
	if created == nil || created.ID == nil {
		t.Fatal("creating a patch software title outside Terraform: no id in the response")
	}
	id := strconv.Itoa(*created.ID)

	c := pro.New(testhelpers.NewAcceptanceClient(t))
	t.Cleanup(func() {
		if err := c.DeletePatchSoftwareTitleConfigurationV3(context.Background(), id); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleaning up patch software title %s: %v", id, err)
		}
	})

	version := accTitleVersion
	packages := []pro.PatchSoftwareTitlePackages{{Version: &version, PackageID: &packageID}}
	if _, err := c.UpdatePatchSoftwareTitleConfigurationV3(ctx, id, &pro.PatchSoftwareTitleConfigurationPatch{
		Packages: &packages,
	}); err != nil {
		t.Fatalf("assigning package %s to version %s of title %s: %v", packageID, version, id, err)
	}
	return id
}

// TestAccResource_ProPatchSoftwareTitle_OutOfBandAssignmentSurvives pins the
// managed-subset contract version_packages documents: only the versions
// Terraform declares are managed, and a package an admin assigns to some other
// version through the UI must still be there after the next apply.
//
// It earns an acceptance test because the v3 packages array is a full
// replacement — the naive migration, sending the plan's assignments alone,
// silently wipes every assignment Terraform does not know about, and no unit
// test over the request builder can catch that. Step two mutates the title
// outside Terraform, then forces an Update by renaming, so the read-modify-write
// fold in crud.go Update is what keeps the out-of-band assignment alive.
func TestAccResource_ProPatchSoftwareTitle_OutOfBandAssignmentSurvives(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	pkgA := "tf-acc-pst-oob-pkgA-" + suffix
	pkgB := "tf-acc-pst-oob-pkgB-" + suffix

	var titleID, pkgBID string

	config := func(name string) string {
		return fmt.Sprintf(`
		resource "jamfplatform_pro_package" "pkg_a" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_package" "pkg_b" {
			display_name = %q
			file_name    = "%s.pkg"
		}

		resource "jamfplatform_pro_patch_software_title" "test" {
			name      = %q
			name_id   = %q
			source_id = %d

			version_packages = {
				%q = jamfplatform_pro_package.pkg_a.id
			}
		}
	`, pkgA, pkgA, pkgB, pkgB, name, accTitleNameID, accTitleSourceID, accTitleVersion)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchSoftwareTitleDestroy(t),
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_13_0),
		},
		Steps: []resource.TestStep{
			{
				Config: config("tf-acc 8x8 Work oob " + suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(patchSoftwareType+".test", "version_packages."+accTitleVersion, "jamfplatform_pro_package.pkg_a", "id"),
					testAccCapturePatchTitleAndPackage(&titleID, &pkgBID),
				),
			},
			{
				PreConfig: func() {
					if err := testAccAssignPatchPackageOutOfBand(t, titleID, accTitleVersionAlt, pkgBID); err != nil {
						t.Fatalf("assigning a package outside Terraform: %v", err)
					}
				},
				Config: config("tf-acc 8x8 Work oob renamed " + suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "name", "tf-acc 8x8 Work oob renamed "+suffix),
					// The managed key is unchanged and still the only one in state.
					resource.TestCheckResourceAttrPair(patchSoftwareType+".test", "version_packages."+accTitleVersion, "jamfplatform_pro_package.pkg_a", "id"),
					resource.TestCheckResourceAttr(patchSoftwareType+".test", "version_packages.%", "1"),
					// The unmanaged key is untouched on the server.
					testAccCheckPatchTitleAssignment(t, &titleID, accTitleVersionAlt, &pkgBID),
				),
			},
		},
	})
}

// testAccCapturePatchTitleAndPackage records the title id and the id of the
// package the next step assigns outside Terraform. PreConfig takes no state, so
// the ids have to be captured from a Check in the preceding step.
func testAccCapturePatchTitleAndPackage(titleID, pkgBID *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		title, ok := s.RootModule().Resources[patchSoftwareType+".test"]
		if !ok {
			return fmt.Errorf("patch software title not found in state")
		}
		pkg, ok := s.RootModule().Resources["jamfplatform_pro_package.pkg_b"]
		if !ok {
			return fmt.Errorf("package pkg_b not found in state")
		}
		*titleID = title.Primary.ID
		*pkgBID = pkg.Primary.ID
		return nil
	}
}

// testAccAssignPatchPackageOutOfBand assigns a package to one version of a title
// the way an administrator would in the UI: a v3 merge-patch the provider had no
// part in. It reads the current assignments first and appends, because the
// packages array replaces rather than merges.
func testAccAssignPatchPackageOutOfBand(t *testing.T, titleID, version, packageID string) error {
	t.Helper()
	if titleID == "" || packageID == "" {
		return fmt.Errorf("nothing captured from the previous step (title %q, package %q)", titleID, packageID)
	}

	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	current, err := c.GetPatchSoftwareTitleConfigurationV3(ctx, titleID)
	if err != nil {
		return fmt.Errorf("reading title %s: %w", titleID, err)
	}

	packages := append([]pro.PatchSoftwareTitlePackages{}, current.Packages...)
	v, p := version, packageID
	packages = append(packages, pro.PatchSoftwareTitlePackages{Version: &v, PackageID: &p})

	if _, err := c.UpdatePatchSoftwareTitleConfigurationV3(ctx, titleID, &pro.PatchSoftwareTitleConfigurationPatch{
		Packages: &packages,
	}); err != nil {
		return fmt.Errorf("assigning package %s to version %s of title %s: %w", packageID, version, titleID, err)
	}
	return nil
}

// testAccCheckPatchTitleAssignment asserts the server still reports the given
// version pointing at the given package. The ids are read through pointers
// because they are only known once an earlier step has run.
func testAccCheckPatchTitleAssignment(t *testing.T, titleID *string, version string, packageID *string) resource.TestCheckFunc {
	t.Helper()
	return func(*terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetPatchSoftwareTitleConfigurationV3(context.Background(), *titleID)
		if err != nil {
			return fmt.Errorf("reading title %s: %w", *titleID, err)
		}
		for _, pkg := range got.Packages {
			if pkg.Version == nil || *pkg.Version != version {
				continue
			}
			if pkg.PackageID == nil || *pkg.PackageID != *packageID {
				return fmt.Errorf("version %s points at package %v, want %s", version, pkg.PackageID, *packageID)
			}
			return nil
		}
		return fmt.Errorf("version %s has no package assignment; the apply wiped an assignment Terraform did not manage", version)
	}
}

// TestAccResource_ProPatchSoftwareTitle_InvalidPackageID asserts the
// version_packages value validator rejects a non-positive-integer package id at
// plan time. The error detail wraps at ~80 cols; the regex avoids whitespace at
// the wrap point by anchoring on the no-space token "positive".
func TestAccResource_ProPatchSoftwareTitle_InvalidPackageID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_software_title" "test" {
						name      = "tf-acc 8x8 Work invalid %[1]s"
						name_id   = %q
						source_id = %d

						version_packages = {
							%q = "0"
						}
					}
				`, suffix, accTitleNameID, accTitleSourceID, accTitleVersion),
				ExpectError: regexp.MustCompile(`positive`),
			},
		},
	})
}

// TestAccDataSource_ProPatchSoftwareTitle_ByID looks up a freshly-created title
// by ID.
func TestAccDataSource_ProPatchSoftwareTitle_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := fmt.Sprintf("tf-acc 8x8 Work ds %s", suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchSoftwareTitleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_software_title" "src" {
						name      = %q
						name_id   = %q
						source_id = %d
					}

					data "jamfplatform_pro_patch_software_title" "by_id" {
						id = jamfplatform_pro_patch_software_title.src.id
					}

					data "jamfplatform_pro_patch_software_title" "by_name" {
						name = jamfplatform_pro_patch_software_title.src.name
					}
				`, name, accTitleNameID, accTitleSourceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_patch_software_title.by_id", "name_id", "jamfplatform_pro_patch_software_title.src", "name_id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_patch_software_title.by_id", "name_id", accTitleNameID),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_patch_software_title.by_id", "name", name),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_patch_software_title.by_id", "available_versions.#"),

					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_patch_software_title.by_name", "id", "jamfplatform_pro_patch_software_title.src", "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_patch_software_title.by_name", "name_id", accTitleNameID),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_patch_software_title.by_name", "available_versions.#"),
				),
			},
		},
	})
}

// TestAccDataSource_ProPatchSoftwareTitle_AmbiguousSelector asserts the data
// source's ExactlyOneOf selector validator: supplying both id and name is
// refused at plan time. The positive cases above cover each selector on its own,
// which is exactly the coverage a silent removal of the validator would keep
// green.
//
// The diagnostic wraps at ~80 cols, so the regex matches only the summary.
func TestAccDataSource_ProPatchSoftwareTitle_AmbiguousSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "jamfplatform_pro_patch_software_title" "bad" {
  id   = "1"
  name = "x"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// TestAccListResource_ProPatchSoftwareTitle_Basic exercises the list resource via
// the `terraform query` workflow. DisplayName / the filter match on the title's
// display name.
//
// The title is created with no package assignments, which is the case that made
// generated configuration unusable: version_packages is Optional-only with a
// minimum of one entry, so it has to stream as null rather than an empty map.
func TestAccListResource_ProPatchSoftwareTitle_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := fmt.Sprintf("tf-acc 8x8 Work list %s", suffix)

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchSoftwareTitleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_software_title" "src" {
						name      = %q
						name_id   = %q
						source_id = %d
					}
				`, name, accTitleNameID, accTitleSourceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_patch_software_title.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_patch_software_title" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_patch_software_title.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("name_id"), KnownValue: knownvalue.StringExact(accTitleNameID)},
							{Path: tfjsonpath.New("version_packages"), KnownValue: knownvalue.Null()},
						},
					),
				},
			},
		},
	})
}

// TestAccResource_ProPatchSoftwareTitle_ExtensionAttributeAccept exercises the
// v2 extension-attribute side-channel. "Adobe AIR" (name_id 0AE) is a title for
// which Jamf supplies an extension attribute that must be accepted. Step 1
// creates the title without accepting (extension_attributes present, accepted =
// false); step 2 sets accept_extension_attributes = true and asserts the EA
// flips to accepted = true. Accepting is one-way, so there is no revert step.
func TestAccResource_ProPatchSoftwareTitle_ExtensionAttributeAccept(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const (
		eaTitleNameID   = "0AE" // Adobe AIR
		eaTitleSourceID = 1
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchSoftwareTitleDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create without accepting — the EA is present but not accepted.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_software_title" "ea" {
						name      = "Adobe AIR"
						name_id   = %q
						source_id = %d
					}
				`, eaTitleNameID, eaTitleSourceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchSoftwareType+".ea", "extension_attributes.#", "1"),
					resource.TestCheckResourceAttr(patchSoftwareType+".ea", "extension_attributes.0.accepted", "false"),
					resource.TestCheckResourceAttrSet(patchSoftwareType+".ea", "extension_attributes.0.ea_id"),
					resource.TestCheckResourceAttrSet(patchSoftwareType+".ea", "extension_attributes.0.display_name"),
				),
			},
			{
				// Accept — the EA flips to accepted = true.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_software_title" "ea" {
						name                        = "Adobe AIR"
						name_id                     = %q
						source_id                   = %d
						accept_extension_attributes = true
					}
				`, eaTitleNameID, eaTitleSourceID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchSoftwareType+".ea", "accept_extension_attributes", "true"),
					resource.TestCheckResourceAttr(patchSoftwareType+".ea", "extension_attributes.#", "1"),
					resource.TestCheckResourceAttr(patchSoftwareType+".ea", "extension_attributes.0.accepted", "true"),
				),
			},
		},
	})
}
