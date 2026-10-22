// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package pkg_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProPackage_LocalReplaceAtSamePath is the regression test for
// #475: the file behind an unchanged package_file_source path is replaced. The
// path never changes, so only the plan-time content check can see it.
//
// Step 2 changes the bytes alone, which planned nothing before the fix. Step 3
// does the same but restores the original modification time, the way `cp -p`
// or an archive extraction does, so only the content decides. Step 4 changes the bytes and info together, which raised "Provider
// produced inconsistent result after apply" before the fix. Step 5 asserts
// the converged state plans nothing.
func TestAccResource_ProPackage_LocalReplaceAtSamePath(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireDirectUploads(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-replace-" + suffix
	fileName := name + ".pkg"

	src161 := fixturePath(t, localFixture161)
	src150 := fixturePath(t, localFixture150)
	sha161 := fixtureSHA3(t, src161)
	sha150 := fixtureSHA3(t, src150)

	target := filepath.Join(t.TempDir(), "package.pkg")
	var originalModTime time.Time
	copyTo := func(src string) func() {
		return func() {
			body, err := os.ReadFile(src) //nolint:gosec // test fixture path
			if err != nil {
				t.Fatalf("reading fixture %q: %v", src, err)
			}
			if err := os.WriteFile(target, body, 0o600); err != nil {
				t.Fatalf("writing %q: %v", target, err)
			}
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					copyTo(src161)()
					fi, err := os.Stat(target)
					if err != nil {
						t.Fatalf("stat %q: %v", target, err)
					}
					originalModTime = fi.ModTime()
				},
				Config: hclLocalUpload(name, fileName, target, "initial"),
				Check:  testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha161)),
			},
			{
				PreConfig: copyTo(src150),
				Config:    hclLocalUpload(name, fileName, target, "initial"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(packageResourceAddr, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(packageResourceAddr, tfjsonpath.New("sha3_512")),
					},
				},
				Check: testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha150)),
			},
			{
				PreConfig: func() {
					copyTo(src161)()
					if err := os.Chtimes(target, originalModTime, originalModTime); err != nil {
						t.Fatalf("restoring mtime on %q: %v", target, err)
					}
				},
				Config: hclLocalUpload(name, fileName, target, "initial"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(packageResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha161)),
			},
			{
				PreConfig: copyTo(src150),
				Config:    hclLocalUpload(name, fileName, target, "bytes and info"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "bytes and info"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha150)),
				),
			},
			{
				Config: hclLocalUpload(name, fileName, target, "bytes and info"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
