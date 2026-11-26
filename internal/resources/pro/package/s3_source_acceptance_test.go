// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Every test here uploads a file to the tenant's cloud distribution point, so
// each gates on testhelpers.RequireDirectUploads as the package resource's
// other upload tests do, and reads that file from the bucket named in
// JAMFPLATFORM_ACC_CLOUDSTORAGE_S3_BUCKET with the AWS default credential
// chain. With no bucket named, testhelpers.RequireCloudStorageFixture skips
// the test. The objects are random bytes under a .pkg name: Jamf Pro hashes
// and stores them without looking inside, and nothing is installed anywhere.
//
// Assertions on the uploaded file compare MD5, the one digest both a Jamf
// Cloud and an Amazon S3 distribution point report.

package pkg_test

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // MD5 is the digest every distribution point reports, not a security control
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jcdsupload"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const (
	s3ResourceName = "jamfplatform_pro_package.test"
	s3ObjectSize   = 256 * 1024
)

var s3ETagMarker = regexp.MustCompile(`^etag:.+`)

// s3TestName returns a unique tf-acc- name for one test's package and object.
func s3TestName(suffix string) string {
	return "tf-acc-s3-" + suffix + "-" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
}

// s3PackageConfig is a package read from source, under the given provider
// block.
func s3PackageConfig(providerBlock, name, source, notes string) string {
	return providerBlock + fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name        = %[1]q
  file_name           = "%[1]s.pkg"
  notes               = %[3]q
  package_file_source = %[2]q
}
`, name, source, notes)
}

// s3CheckNoPackageNamed fails when a package with the display name exists. It
// is the destroy check for a test whose create is expected to fail, where
// state holds nothing to look up and a record left behind is the defect.
func s3CheckNoPackageNamed(t *testing.T, name string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		packages, err := pro.New(testhelpers.NewAcceptanceClient(t)).ListPackagesV1(context.Background(), nil, "")
		if err != nil {
			return fmt.Errorf("listing Jamf Pro packages: %w", err)
		}
		for _, p := range packages {
			if p.PackageName == name {
				return fmt.Errorf("package %q was left behind by a create that failed", name)
			}
		}
		return nil
	}
}

// s3CheckHolds asserts that Jamf Pro itself, not only state, reports the file
// as READY with the object's MD5.
func s3CheckHolds(t *testing.T, object func() testhelpers.CloudStorageObject) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[s3ResourceName]
		if !ok {
			return fmt.Errorf("%s not in state", s3ResourceName)
		}
		p, err := pro.New(testhelpers.NewAcceptanceClient(t)).GetPackageV1(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading package %s: %w", rs.Primary.ID, err)
		}
		want := object()
		if got := strings.ToLower(helpers.DerefString(p.Md5)); got != want.MD5 {
			return fmt.Errorf("Jamf Pro holds a file with MD5 %q, want the object's %q", got, want.MD5)
		}
		if status := helpers.DerefString(p.CloudTransferStatus); status != "READY" {
			return fmt.Errorf("cloud_transfer_status = %q, want READY", status)
		}
		return nil
	}
}

// s3CaptureAttr stores an attribute of the test resource for a later step.
func s3CaptureAttr(attr string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[s3ResourceName]
		if !ok {
			return fmt.Errorf("%s not in state", s3ResourceName)
		}
		*into = rs.Primary.Attributes[attr]
		return nil
	}
}

// s3CheckAttrIs compares an attribute with a value known only once an earlier
// step has run.
func s3CheckAttrIs(attr string, want *string, equal bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[s3ResourceName]
		if !ok {
			return fmt.Errorf("%s not in state", s3ResourceName)
		}
		got := rs.Primary.Attributes[attr]
		if (got == *want) != equal {
			return fmt.Errorf("%s = %q; equal to %q is %v, want %v", attr, got, *want, got == *want, equal)
		}
		return nil
	}
}

// TestAccResource_ProPackage_S3Source_Lifecycle is the update round-trip:
// create from an object, change only metadata, replace the object, import.
//
// The metadata step is the one that proves the marker is doing its job. The
// plan must show notes changing and the file not, and the marker in state must
// be the one the create recorded.
//
// The import step ignores package_file_source and
// package_file_source_version. The importer reads the record from Jamf Pro,
// which knows nothing of where its file came from: the source is
// configuration only, and the marker is recorded by the apply that reads the
// source.
func TestAccResource_ProPackage_S3Source_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("lifecycle")
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
					resource.TestCheckResourceAttr(s3ResourceName, "cloud_transfer_status", "READY"),
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
					resource.TestMatchResourceAttr(s3ResourceName, "package_file_source_version", s3ETagMarker),
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

// TestAccResource_ProPackage_S3Source_Adoption gives a package that already
// holds a file an S3 source with the same bytes. The apply must record the
// marker and leave the file in Jamf Pro alone, which the unchanged MD5 and the
// clean plan after it show; that it did not upload is asserted at the unit
// level, where the upload call can be counted.
func TestAccResource_ProPackage_S3Source_Adoption(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("adopt")
	body := make([]byte, s3ObjectSize)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("generating bytes: %v", err)
	}
	local := filepath.Join(t.TempDir(), name+".pkg")
	if err := os.WriteFile(local, body, 0o600); err != nil {
		t.Fatalf("writing the local file: %v", err)
	}
	object := fixture.Put(t, name+".pkg", body)
	provider := fixture.ProviderBlock("")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: s3PackageConfig(provider, name, local, "local"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(s3ResourceName, "package_file_source_version"),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				Config: s3PackageConfig(provider, name, object.Source, "local"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(s3ResourceName, "package_file_source_version", s3ETagMarker),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				Config: s3PackageConfig(provider, name, local, "local"),
				Check:  resource.TestCheckNoResourceAttr(s3ResourceName, "package_file_source_version"),
			},
		},
	})
}

// TestAccResource_ProPackage_S3Source_RestoresAFileReplacedInJamfPro is the
// drift recovery test. Between two applies of one unchanged configuration,
// another file is uploaded to the package record behind Terraform's back. The
// object in the bucket has not changed, so only the check in Read can notice,
// and the second apply must put the object's bytes back.
func TestAccResource_ProPackage_S3Source_RestoresAFileReplacedInJamfPro(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("drift")
	object := fixture.PutRandom(t, name+".pkg", s3ObjectSize)
	config := s3PackageConfig(fixture.ProviderBlock(""), name, object.Source, "drift")
	var id, marker string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					s3CaptureAttr("id", &id),
					s3CaptureAttr("package_file_source_version", &marker),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				PreConfig: func() { s3ReplaceFileInJamfPro(t, id, name+".pkg") },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(s3ResourceName, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					s3CheckAttrIs("package_file_source_version", &marker, true),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
		},
	})
}

// s3ReplaceFileInJamfPro uploads different bytes to a package record directly,
// by the route the tenant's distribution point takes, and waits until Jamf Pro
// reports their digest. It is the change made outside Terraform.
func s3ReplaceFileInJamfPro(t *testing.T, id, fileName string) {
	t.Helper()
	body := make([]byte, s3ObjectSize)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("generating bytes: %v", err)
	}
	sum := md5.Sum(body) //nolint:gosec // see the md5 import
	want := hex.EncodeToString(sum[:])

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := pro.New(testhelpers.NewAcceptanceClient(t))
	cdp, err := client.GetCloudDistributionPointV1(ctx)
	if err != nil {
		t.Fatalf("reading the cloud distribution point: %v", err)
	}
	if strings.EqualFold(cdp.CdnType, pro.CloudDistributionPointCdnTypeJamfCloud) {
		err = jcdsupload.Upload(ctx, client, fileName, bytes.NewReader(body))
	} else {
		_, err = client.UploadPackageV1(ctx, id, fileName, bytes.NewReader(body))
	}
	if err != nil {
		t.Fatalf("uploading a file outside Terraform: %v", err)
	}
	for {
		_ = client.RefreshCloudDistributionPointInventoryV1(ctx, fileName)
		p, getErr := client.GetPackageV1(ctx, id)
		if getErr != nil {
			t.Fatalf("reading package %s: %v", id, getErr)
		}
		if strings.EqualFold(helpers.DerefString(p.Md5), want) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Jamf Pro did not report the file uploaded outside Terraform: MD5 is %q, want %q", helpers.DerefString(p.Md5), want)
		case <-time.After(10 * time.Second):
		}
	}
}

// TestAccResource_ProPackage_S3Source_PinnedVersion reads one version of an
// object. Replacing the object afterwards must plan nothing, because the
// source still names the version first read.
func TestAccResource_ProPackage_S3Source_PinnedVersion(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("version")
	object := fixture.PutRandom(t, name+".pkg", s3ObjectSize)
	if object.VersionID == "" {
		t.Skipf("skipping: bucket %s has no versioning, so an object has no version to name", fixture.Bucket)
	}
	config := s3PackageConfig(fixture.ProviderBlock(""), name, object.Source+"?versionId="+object.VersionID, "version")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(s3ResourceName, "package_file_source_version", s3ETagMarker),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				PreConfig: func() { fixture.PutRandom(t, name+".pkg", s3ObjectSize) },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
			},
		},
	})
}

// TestAccResource_ProPackage_S3Source_MissingObjectLeavesNoRecord covers the
// one plan-time problem that is deferred to the apply. The plan warns, the
// apply fails, and no package record may exist afterwards: a record left
// behind would collide with the next apply by name.
func TestAccResource_ProPackage_S3Source_MissingObjectLeavesNoRecord(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("missing")

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

// TestAccResource_ProPackage_S3Source_AssumeRole reads the object through a
// role named in file_sources.s3.assume_role. It needs a role the ambient
// identity may assume, which the suite cannot create, so it is skipped unless
// one is named.
func TestAccResource_ProPackage_S3Source_AssumeRole(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	role := testhelpers.AccEnv(testhelpers.EnvCloudStorageS3AssumeRoleARN)
	if role == "" {
		t.Skipf("skipping: %s names no role to assume", testhelpers.EnvCloudStorageS3AssumeRoleARN)
	}
	block := fmt.Sprintf("assume_role = {\n        role_arn     = %q\n        session_name = \"jamfplatform-acceptance\"", role)
	if externalID := testhelpers.AccEnv(testhelpers.EnvCloudStorageS3AssumeRoleExternalID); externalID != "" {
		block += fmt.Sprintf("\n        external_id  = %q", externalID)
	}
	block += "\n      }"

	name := s3TestName("role")
	object := fixture.PutRandom(t, name+".pkg", s3ObjectSize)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: s3PackageConfig(fixture.ProviderBlock("", block), name, object.Source, "role"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(s3ResourceName, "package_file_source_version", s3ETagMarker),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
		},
	})
}

// TestAccResource_ProPackage_S3Source_TwoProviderAliases configures the
// provider twice with a different AWS profile each, and reads one object
// through both. Each provider instance has its own resolver, so neither
// profile may leak into the other's client. It needs two named profiles,
// which the suite cannot create, so it is skipped unless both are named.
func TestAccResource_ProPackage_S3Source_TwoProviderAliases(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	profileA := testhelpers.AccEnv(testhelpers.EnvCloudStorageS3ProfileA)
	profileB := testhelpers.AccEnv(testhelpers.EnvCloudStorageS3ProfileB)
	if profileA == "" || profileB == "" {
		t.Skipf("skipping: %s and %s do not both name a profile", testhelpers.EnvCloudStorageS3ProfileA, testhelpers.EnvCloudStorageS3ProfileB)
	}

	name := s3TestName("alias")
	object := fixture.PutRandom(t, name+".pkg", s3ObjectSize)
	config := fixture.ProviderBlock("", fmt.Sprintf("profile = %q", profileA)) +
		fixture.ProviderBlock("second", fmt.Sprintf("profile = %q", profileB)) +
		fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name        = %[1]q
  file_name           = "%[1]s.pkg"
  package_file_source = %[2]q
}

resource "jamfplatform_pro_package" "second" {
  provider            = jamfplatform.second
  display_name        = "%[1]s-second"
  file_name           = "%[1]s-second.pkg"
  package_file_source = %[2]q
}
`, name, object.Source)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(s3ResourceName, "package_file_source_version", "jamfplatform_pro_package.second", "package_file_source_version"),
					resource.TestCheckResourceAttr("jamfplatform_pro_package.second", "md5", object.MD5),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
		},
	})
}

// TestAccResource_ProPackage_S3Source_Streamed is the lifecycle with
// stream_url_directly set, so the object goes from the bucket to the
// distribution point without touching local disk. The metadata step is the
// reason the marker matters most here: without it a streamed package is sent
// again on every update, and the plan check holds the file to "unchanged".
func TestAccResource_ProPackage_S3Source_Streamed(t *testing.T) {
	testhelpers.AccPreCheck(t)
	fixture := testhelpers.RequireCloudStorageFixture(t)
	testhelpers.RequireDirectUploads(t)

	name := s3TestName("stream")
	object := fixture.PutRandom(t, name+".pkg", s3ObjectSize)
	config := func(notes string) string {
		return fixture.ProviderBlock("") + fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name        = %[1]q
  file_name           = "%[1]s.pkg"
  notes               = %[3]q
  package_file_source = %[2]q
  stream_url_directly = true
}
`, name, object.Source, notes)
	}
	var firstMarker string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("created"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr(s3ResourceName, "package_file_source_version", s3ETagMarker),
					s3CaptureAttr("package_file_source_version", &firstMarker),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				Config: config("edited"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(s3ResourceName, plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue(s3ResourceName, s3PlanPath("md5"), s3StringExact(func() string { return object.MD5 })),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					s3CheckAttrIs("package_file_source_version", &firstMarker, true),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
			{
				PreConfig: func() { object = fixture.PutRandom(t, name+".pkg", s3ObjectSize) },
				Config:    config("edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					s3CheckAttrIs("package_file_source_version", &firstMarker, false),
					s3CheckHolds(t, func() testhelpers.CloudStorageObject { return object }),
				),
			},
		},
	})
}
