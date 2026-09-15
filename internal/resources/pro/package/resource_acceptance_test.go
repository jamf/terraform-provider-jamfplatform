// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro /v1/packages endpoint and (for
// JCDS scenarios) the cloud distribution point. Keep these serial with
// other package acceptance work — JCDS does not enforce serial behaviour
// but the maintainer convention is to keep all Pro classic + V1 acc tests
// at -parallel 1.
//
// Real .pkg fixtures live under test_fixtures/ (local) and are fetched
// from the public jamf-cli GitHub releases (URL). Fixture paths are
// resolved via fixturePath / fixtureSHA3 / downloadAndHashURL in
// fixtures_acceptance_test.go.
//
// Every test that uploads a binary gates on testhelpers.RequireJCDSUploads:
// without a Jamf Cloud distribution point the upload's verification poll can
// never converge, so the test would spend the resource's full create timeout
// before failing on something that is an estate condition, not a defect.

package pkg_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckPackageDestroy verifies packages created during the test were
// destroyed.
func testAccCheckPackageDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_package" {
				continue
			}
			_, err := c.GetPackageV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro package %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro package %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// testCheckPackageHashConverged verifies via a direct SDK GET that the
// post-upload hash on the server exactly matches the closure-captured
// expected SHA-3-512 digest. Load-bearing post-condition for every JCDS
// upload step — without it the test wouldn't catch a "verification poll
// declared success on the wrong hash" regression.
func testCheckPackageHashConverged(t *testing.T, resourceName string, expectFn func() string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("resource %s has empty ID", resourceName)
		}

		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		p, err := c.GetPackageV1(ctx, rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("fetching package %s: %w", rs.Primary.ID, err)
		}

		want := strings.ToLower(expectFn())
		got := strings.ToLower(helpers.DerefString(p.HashValue))
		if got != want {
			return fmt.Errorf("hash mismatch on %s: server reports %q, expected %q", rs.Primary.ID, got, want)
		}
		if hashType := helpers.DerefString(p.HashType); hashType != "SHA3_512" {
			return fmt.Errorf("hash_type mismatch on %s: server reports %q, expected %q", rs.Primary.ID, hashType, "SHA3_512")
		}
		if status := helpers.DerefString(p.CloudTransferStatus); status != "READY" {
			return fmt.Errorf("cloud_transfer_status mismatch on %s: server reports %q, expected %q", rs.Primary.ID, status, "READY")
		}
		return nil
	}
}

// testCheckPackageHashEquals asserts the live server hash equals a
// closure-captured digest WITHOUT requiring the converged JCDS state —
// used for assertions on a metadata-only update where we expect the hash
// to remain on its previous value.
func testCheckPackageHashEquals(t *testing.T, resourceName string, expectFn func() string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		p, err := c.GetPackageV1(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("fetching package %s: %w", rs.Primary.ID, err)
		}
		want := strings.ToLower(expectFn())
		got := strings.ToLower(helpers.DerefString(p.HashValue))
		if got != want {
			return fmt.Errorf("hash drift on %s: server reports %q, expected %q (re-upload should NOT have triggered)", rs.Primary.ID, got, want)
		}
		return nil
	}
}

const packageResourceAddr = "jamfplatform_pro_package.test"

// hclLocalUpload renders a JCDS resource block backed by a local-path
// fixture. info is woven through so metadata-only step assertions have a
// scalar field to perturb.
func hclLocalUpload(displayName, fileName, src, info string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name        = %q
  file_name           = %q
  package_file_source = %q
  info                = %q
}
`, displayName, fileName, src, info)
}

// hclURLUpload renders a JCDS resource block backed by an http(s):// URL.
// The streaming flag drives the stream-direct-vs-disk-stage path; pass
// false for the default disk-staging behaviour.
func hclURLUpload(displayName, fileName, url, info string, streaming bool) string {
	if streaming {
		return fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name        = %q
  file_name           = %q
  package_file_source = %q
  info                = %q
  stream_url_directly = true
}
`, displayName, fileName, url, info)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name        = %q
  file_name           = %q
  package_file_source = %q
  info                = %q
}
`, displayName, fileName, url, info)
}

// hclFSDPMetadataOnly renders a no-source / no-hash record.
func hclFSDPMetadataOnly(displayName, fileName, info string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name = %q
  file_name    = %q
  info         = %q
}
`, displayName, fileName, info)
}

// hclFSDPWithHashes renders an FSDP record where the user supplies
// hash_type + hash_value + md5/sha256/sha3_512 explicitly.
func hclFSDPWithHashes(displayName, fileName, hashType, hashValue, md5, sha256, sha3 string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name = %q
  file_name    = %q
  hash_type    = %q
  hash_value   = %q
  md5          = %q
  sha256       = %q
  sha3_512     = %q
}
`, displayName, fileName, hashType, hashValue, md5, sha256, sha3)
}

// TestAccResource_ProPackage_LocalUploadCreateAndUpdate exercises the JCDS
// happy path with local-filesystem `package_file_source` values across
// Create, file-replace Update, and metadata-only Update. Asserts the
// verification poll converges on each upload step and stays steady on the
// metadata-only step (no re-upload).
func TestAccResource_ProPackage_LocalUploadCreateAndUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireJCDSUploads(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-local-" + suffix
	fileName := name + ".pkg"

	src161 := fixturePath(t, localFixture161)
	src150 := fixturePath(t, localFixture150)
	sha161 := fixtureSHA3(t, src161)
	sha150 := fixtureSHA3(t, src150)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				// Step 1 — create with 1.16.1 local.
				Config: hclLocalUpload(name, fileName, src161, "initial create"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(packageResourceAddr, "id"),
					resource.TestCheckResourceAttr(packageResourceAddr, "display_name", name),
					resource.TestCheckResourceAttr(packageResourceAddr, "file_name", fileName),
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "initial create"),
					resource.TestCheckResourceAttr(packageResourceAddr, "hash_type", "SHA3_512"),
					resource.TestCheckResourceAttr(packageResourceAddr, "cloud_transfer_status", "READY"),
					// Jamf Pro derives size from the uploaded binary; it must be
					// populated after a JCDS upload.
					resource.TestCheckResourceAttrSet(packageResourceAddr, "size"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha161)),
				),
			},
			{
				// Step 2 — replace with 1.15.0 (different bytes, same display_name).
				Config: hclLocalUpload(name, fileName, src150, "post-replace"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "post-replace"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha150)),
				),
			},
			{
				// Step 3 — metadata-only update (info change). Hash must NOT
				// change — the resource handler's hash-aware idempotency check
				// should short-circuit the upload.
				Config: hclLocalUpload(name, fileName, src150, "metadata-only-edit"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "metadata-only-edit"),
					// Regression guard: a metadata-only update blanks Jamf Pro's
					// server-managed size; the resource re-derives it from a
					// cloud distribution point refresh, so it must remain
					// populated (not collapse to empty) after the update.
					resource.TestCheckResourceAttrSet(packageResourceAddr, "size"),
					testCheckPackageHashEquals(t, packageResourceAddr, staticDigest(sha150)),
				),
			},
			{
				// Step 4 — import.
				ResourceName:      packageResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"package_file_source", "package_file_source_checksum",
					"stream_url_directly", "manifest_file_source", "timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProPackage_URLUploadCreateAndUpdate exercises the JCDS
// URL-source path via the DEFAULT disk-staging mode. Streaming is
// exercised in TestAccResource_ProPackage_URLStreamingUpload below.
func TestAccResource_ProPackage_URLUploadCreateAndUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireJCDSUploads(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-url-" + suffix
	fileName := name + ".pkg"

	_, sha170 := downloadAndHashURL(t, urlFixture170)
	_, sha160 := downloadAndHashURL(t, urlFixture160)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: hclURLUpload(name, fileName, urlFixture170, "url-create", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(packageResourceAddr, "id"),
					resource.TestCheckResourceAttr(packageResourceAddr, "hash_type", "SHA3_512"),
					resource.TestCheckResourceAttr(packageResourceAddr, "cloud_transfer_status", "READY"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha170)),
				),
			},
			{
				Config: hclURLUpload(name, fileName, urlFixture160, "url-replace", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "url-replace"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha160)),
				),
			},
			{
				Config: hclURLUpload(name, fileName, urlFixture160, "url-meta-only", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "url-meta-only"),
					// Regression guard: metadata-only update must not blank the
					// server-derived size (see local-upload Step 3).
					resource.TestCheckResourceAttrSet(packageResourceAddr, "size"),
					testCheckPackageHashEquals(t, packageResourceAddr, staticDigest(sha160)),
				),
			},
		},
	})
}

// TestAccResource_ProPackage_URLStreamingUploadCreateAndUpdate exercises
// the streaming-URL path (stream_url_directly = true). The streaming path
// does not perform a hash-aware idempotency check because the hash is
// only known after the stream completes — every Update WILL re-upload
// the binary. The test confirms each step's hash converges correctly via
// the inline tee.
func TestAccResource_ProPackage_URLStreamingUploadCreateAndUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireJCDSUploads(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-stream-" + suffix
	fileName := name + ".pkg"

	_, sha170 := downloadAndHashURL(t, urlFixture170)
	_, sha160 := downloadAndHashURL(t, urlFixture160)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: hclURLUpload(name, fileName, urlFixture170, "stream-create", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(packageResourceAddr, "id"),
					resource.TestCheckResourceAttr(packageResourceAddr, "stream_url_directly", "true"),
					resource.TestCheckResourceAttr(packageResourceAddr, "hash_type", "SHA3_512"),
					resource.TestCheckResourceAttr(packageResourceAddr, "cloud_transfer_status", "READY"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha170)),
				),
			},
			{
				// Streaming replace: different URL, different bytes. The
				// streaming path always re-uploads on Update (no local hash
				// gate); confirms server converges on the new hash.
				Config: hclURLUpload(name, fileName, urlFixture160, "stream-replace", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "stream-replace"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha160)),
				),
			},
		},
	})
}

// TestAccResource_ProPackage_MixedSourceSwap verifies cross-mode transitions
// don't confuse the hash-aware idempotency check:
// local → URL (disk) → local. Streaming swap is covered by the dedicated
// streaming test above.
func TestAccResource_ProPackage_MixedSourceSwap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireJCDSUploads(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-mixed-" + suffix
	fileName := name + ".pkg"

	src161 := fixturePath(t, localFixture161)
	src150 := fixturePath(t, localFixture150)
	sha161 := fixtureSHA3(t, src161)
	sha150 := fixtureSHA3(t, src150)
	_, sha170 := downloadAndHashURL(t, urlFixture170)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: hclLocalUpload(name, fileName, src161, "mixed-step1-local"),
				Check:  testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha161)),
			},
			{
				Config: hclURLUpload(name, fileName, urlFixture170, "mixed-step2-url", false),
				Check:  testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha170)),
			},
			{
				Config: hclLocalUpload(name, fileName, src150, "mixed-step3-local"),
				Check:  testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha150)),
			},
		},
	})
}

// Verification corruption coverage lives in the unit-test suite as
// TestClassifyPollTick (helpers_test.go). It exercises every branch of
// the convergence decision tree (converged / continue / corruption)
// without requiring a live tenant or an SDK-boundary mock.

// TestAccResource_ProPackage_MetadataOnly_FSDP exercises the pure-metadata
// path (no `package_file_source`, no hashes). Asserts no upload code runs:
// hash attributes stay null in state throughout, and `cloud_transfer_status`
// remains empty.
func TestAccResource_ProPackage_MetadataOnly_FSDP(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-fsdp-meta-" + suffix
	fileName := name + ".pkg"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: hclFSDPMetadataOnly(name, fileName, "fsdp-create"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(packageResourceAddr, "id"),
					resource.TestCheckResourceAttr(packageResourceAddr, "display_name", name),
					resource.TestCheckResourceAttr(packageResourceAddr, "file_name", fileName),
					// Hash attrs must collapse to null on a never-uploaded record.
					resource.TestCheckNoResourceAttr(packageResourceAddr, "sha3_512"),
					resource.TestCheckNoResourceAttr(packageResourceAddr, "sha256"),
					resource.TestCheckNoResourceAttr(packageResourceAddr, "md5"),
					resource.TestCheckNoResourceAttr(packageResourceAddr, "hash_value"),
					resource.TestCheckNoResourceAttr(packageResourceAddr, "size"),
				),
			},
			{
				Config: hclFSDPMetadataOnly(name, fileName, "fsdp-metadata-edit"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(packageResourceAddr, "info", "fsdp-metadata-edit"),
					resource.TestCheckNoResourceAttr(packageResourceAddr, "sha3_512"),
				),
			},
			{
				ResourceName:      packageResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"package_file_source", "package_file_source_checksum",
					"stream_url_directly", "manifest_file_source", "timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProPackage_FSDPUserSuppliedHashes exercises the
// FSDP-with-hashes path: no source, user explicitly supplies hash
// attributes. Asserts the server stores them verbatim (§13.8 A.7).
func TestAccResource_ProPackage_FSDPUserSuppliedHashes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-fsdp-hashes-" + suffix
	fileName := name + ".pkg"

	// Fabricated hashes — server stores verbatim without validation.
	const (
		md5Hex    = "0123456789abcdef0123456789abcdef"
		sha256Hex = "abc123abc123abc123abc123abc123abc123abc123abc123abc123abc123abc1"
		sha3Hex   = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: hclFSDPWithHashes(name, fileName, "SHA3_512", sha3Hex, md5Hex, sha256Hex, sha3Hex),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(packageResourceAddr, "id"),
					resource.TestCheckResourceAttr(packageResourceAddr, "hash_type", "SHA3_512"),
					resource.TestCheckResourceAttr(packageResourceAddr, "hash_value", sha3Hex),
					resource.TestCheckResourceAttr(packageResourceAddr, "md5", md5Hex),
					resource.TestCheckResourceAttr(packageResourceAddr, "sha256", sha256Hex),
					resource.TestCheckResourceAttr(packageResourceAddr, "sha3_512", sha3Hex),
				),
			},
		},
	})
}

// TestAccResource_ProPackage_ManifestLifecycle exercises the nested
// manifest sub-resource:
//   - create with a JCDS binary upload AND a manifest_file_source
//   - update to clear manifest_file_source (server-side manifest DELETE)
//
// The manifest fixture is a tiny synthetic plist generated at runtime so
// the test does not depend on a real .plist on disk.
func TestAccResource_ProPackage_ManifestLifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireJCDSUploads(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-package-manifest-" + suffix
	fileName := name + ".pkg"

	src161 := fixturePath(t, localFixture161)
	sha161 := fixtureSHA3(t, src161)

	// Build a synthetic plist fixture in t.TempDir() so this test owns its
	// disk footprint and cleans up automatically.
	manifestBody := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>items</key>
    <array>
      <dict>
        <key>metadata</key>
        <dict>
          <key>bundle-identifier</key>
          <string>com.jamf.acc-test</string>
          <key>bundle-version</key>
          <string>1.0.0</string>
        </dict>
      </dict>
    </array>
  </dict>
</plist>
`
	manifestPath := writeTempManifest(t, manifestBody)

	hclWithManifest := func(srcPath, manifestSrc, info string) string {
		if manifestSrc == "" {
			return hclLocalUpload(name, fileName, srcPath, info)
		}
		return fmt.Sprintf(`
resource "jamfplatform_pro_package" "test" {
  display_name         = %q
  file_name            = %q
  package_file_source  = %q
  manifest_file_source = %q
  info                 = %q
}
`, name, fileName, srcPath, manifestSrc, info)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPackageDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: hclWithManifest(src161, manifestPath, "manifest-create"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(packageResourceAddr, "manifest"),
					resource.TestCheckResourceAttrSet(packageResourceAddr, "manifest_file_name"),
					resource.TestCheckResourceAttrSet(packageResourceAddr, "size"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha161)),
				),
			},
			{
				// Drop manifest_file_source → provider issues DeletePackageManifestV1.
				// Manifest fields collapse to null in state (not empty
				// string) per ReconcileOptionalString — the server returns
				// "" and the helper turns that into types.StringNull().
				Config: hclWithManifest(src161, "", "manifest-removed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(packageResourceAddr, "manifest"),
					resource.TestCheckNoResourceAttr(packageResourceAddr, "manifest_file_name"),
					// Removing the manifest clears the server-managed size AND
					// the metadata update blanks it; the resource must re-derive
					// it from a cloud distribution point refresh so it stays
					// populated rather than collapsing to empty.
					resource.TestCheckResourceAttrSet(packageResourceAddr, "size"),
					testCheckPackageHashConverged(t, packageResourceAddr, staticDigest(sha161)),
				),
			},
		},
	})
}
