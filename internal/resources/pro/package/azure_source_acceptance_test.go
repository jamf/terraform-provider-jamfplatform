// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// The Azure Blob Storage counterparts of the S3 source tests. They read from
// the account and container named in JAMFPLATFORM_ACC_CLOUDSTORAGE_AZURE_ACCOUNT
// and _AZURE_CONTAINER with the ambient Microsoft Entra credential, the ARM_*
// variables or an Azure CLI login, and skip when either is unset.
//
// The behaviour under test is the resource's, which is the same for both
// stores, so only the paths a store can change are repeated here: the read,
// the marker and its comparison, and the refusal of a missing object. An Azure
// ETag changes on every write, where an S3 ETag follows the content, and the
// lifecycle test's replace step is where that difference would show.

package pkg_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProPackage_AzureBlobSource_Lifecycle is the update
// round-trip for an azblob:// source: create, change only metadata, replace
// the blob, import. The import step ignores the source and the marker for the
// reason the S3 lifecycle test gives.
func TestAccResource_ProPackage_AzureBlobSource_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireAzureBlobFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("azure")
	object := fixture.PutRandom(t, name+".pkg", s3ObjectSize)
	provider := fixture.ProviderBlock("")
	var firstMarker string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: s3PackageConfig(provider, name, object.Source, "created"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(s3ResourceName, "package_file_source_version", s3ETagMarker),
					resource.TestCheckResourceAttr(s3ResourceName, "size", fmt.Sprint(s3ObjectSize)),
					s3CaptureAttr("package_file_source_version", &firstMarker),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				Config: s3PackageConfig(provider, name, object.Source, "edited"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(s3ResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(s3ResourceName, s3PlanPath("md5"), s3StringExact(func() string { return object.MD5 })),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(s3ResourceName, "notes", "edited"),
					s3CheckAttrIs("package_file_source_version", &firstMarker, true),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				PreConfig: func() { object = fixture.PutRandom(t, name+".pkg", s3ObjectSize) },
				Config:    s3PackageConfig(provider, name, object.Source, "edited"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(s3ResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(s3ResourceName, s3PlanPath("md5")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					s3CheckAttrIs("package_file_source_version", &firstMarker, false),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				ResourceName:            s3ResourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"package_file_source", "package_file_source_version", "timeouts"},
			},
		},
	})
}

// TestAccResource_ProPackage_AzureBlobSource_MissingObjectLeavesNoRecord is the
// azblob:// form of the S3 test of the same name: the plan warns, the apply
// fails, and no package record is left behind.
func TestAccResource_ProPackage_AzureBlobSource_MissingObjectLeavesNoRecord(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireAzureBlobFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("azure-missing")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             s3CheckNoPackageNamed(t, name),
		Steps: []resource.TestStep{
			{
				Config:      s3PackageConfig(fixture.ProviderBlock(""), name, fixture.Source(name+"-absent.pkg"), "missing"),
				ExpectError: regexp.MustCompile(`Package\s+file\s+not\s+found\s+in\s+cloud\s+storage`),
			},
		},
	})
}
