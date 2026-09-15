// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package cloud_distribution_point_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// destructiveEnv gates the lifecycle acceptance tests behind an explicit opt-in.
// Managing this singleton is destructive to shared tenant state: destroying it
// (cdn_type → NONE) PERMANENTLY DELETES every package, in-house app, and eBook
// hosted in Jamf Cloud (Jamf Pro warns of this in the UI) and turns off the JCDS
// direct upload the `package` acceptance tests depend on. `restoreJamfCloud`
// re-enables JCDS afterward but CANNOT recover the deleted content. The tests
// also cannot create idempotently while the tenant already has a CDP configured
// (POST returns "already configured"), so they delete-to-NONE first and restore
// JAMF_CLOUD on cleanup. Run ONLY against a tenant with no hosted content you
// care about:
//
//	JAMFPLATFORM_ACC_PRO_CDP_WRITE_OK=1 make testacc-run \
//	  RUN=TestAccResource_ProCloudDistributionPoint \
//	  PKG=./internal/resources/pro/cloud_distribution_point/
const destructiveEnv = "JAMFPLATFORM_ACC_PRO_CDP_WRITE_OK"

func requireDestructive(t *testing.T) {
	t.Helper()
	if testhelpers.AccEnv(destructiveEnv) == "" {
		t.Skipf("skipping destructive cloud distribution point test; set %s=1 to run. WARNING: destroy PERMANENTLY DELETES all packages/apps/eBooks hosted in Jamf Cloud and cannot be undone", destructiveEnv)
	}
}

func cdpClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

// resetToNone ensures the tenant starts at cdn_type NONE so the test's create
// step (POST) succeeds. A 204 or an already-NONE state are both fine.
func resetToNone(t *testing.T) {
	t.Helper()
	_ = cdpClient(t).DeleteCloudDistributionPointV1(context.Background())
}

// restoreJamfCloud re-enables JCDS after the test so the shared tenant returns
// to its original configuration (the `package` tests depend on it).
func restoreJamfCloud(t *testing.T) {
	t.Helper()
	master := false
	if _, err := cdpClient(t).CreateCloudDistributionPointV1(context.Background(), &pro.CloudDistributionPoint{
		CdnType: "JAMF_CLOUD",
		Master:  &master,
	}); err != nil {
		t.Logf("warning: failed to restore JAMF_CLOUD cloud distribution point after test: %v", err)
	}
}

// checkIsNone is the CheckDestroy: after Terraform destroys the resource, the
// real DELETE must have run, leaving the tenant at cdn_type NONE.
func checkIsNone(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got, err := cdpClient(t).GetCloudDistributionPointV1(context.Background())
		if err != nil {
			return fmt.Errorf("reading cloud distribution point post-destroy: %w", err)
		}
		if got.CdnType != "NONE" {
			return fmt.Errorf("expected cdn_type NONE after destroy, got %q", got.CdnType)
		}
		return nil
	}
}

func jcdsConfig(master bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_cloud_distribution_point" "test" {
  cdn_type = "JAMF_CLOUD"
  master   = %t
}
`, master)
}

// TestAccResource_ProCloudDistributionPoint_JamfCloud exercises the full
// lifecycle against a real tenant for the JCDS type (the only type testable
// without third-party CDN credentials): create → update (master toggle, the
// sole JCDS-applicable mutable field) → import. CheckDestroy asserts the real
// DELETE ran (cdn_type → NONE).
func TestAccResource_ProCloudDistributionPoint_JamfCloud(t *testing.T) {
	requireDestructive(t)
	testhelpers.AccPreCheck(t)
	resetToNone(t)
	t.Cleanup(func() { restoreJamfCloud(t) })

	const rn = "jamfplatform_pro_cloud_distribution_point.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIsNone(t),
		Steps: []resource.TestStep{
			{
				Config: jcdsConfig(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "id", "singleton"),
					resource.TestCheckResourceAttr(rn, "cdn_type", "JAMF_CLOUD"),
					resource.TestCheckResourceAttr(rn, "master", "false"),
					// Computed echoes resolve from the server.
					resource.TestCheckResourceAttrSet(rn, "has_connection_succeeded"),
					resource.TestCheckResourceAttrSet(rn, "inventory_id"),
				),
			},
			{
				Config: jcdsConfig(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "cdn_type", "JAMF_CLOUD"),
					resource.TestCheckResourceAttr(rn, "master", "true"),
				),
			},
			{
				ResourceName:      rn,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				// timeouts has no remote equivalent; password/private_key are
				// WriteOnly (never in state, never returned).
				ImportStateVerifyIgnore: []string{"timeouts", "password", "private_key"},
			},
		},
	})
}

// TestAccResource_ProCloudDistributionPoint_RejectsNonSingletonImport verifies
// the ImportState guard.
func TestAccResource_ProCloudDistributionPoint_RejectsNonSingletonImport(t *testing.T) {
	requireDestructive(t)
	testhelpers.AccPreCheck(t)
	resetToNone(t)
	t.Cleanup(func() { restoreJamfCloud(t) })

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIsNone(t),
		Steps: []resource.TestStep{
			{Config: jcdsConfig(false)},
			{
				ResourceName:  "jamfplatform_pro_cloud_distribution_point.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProCloudDistributionPoint_AkamaiRequiresFields exercises the
// per-cdn_type required-field config validator. This is a plan-time check only
// (no apply, no API mutation), so it is NOT gated behind the destructive env.
func TestAccResource_ProCloudDistributionPoint_AkamaiRequiresFields(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_cloud_distribution_point" "test" {
  cdn_type = "AKAMAI"
  username = "admin"
  password = "secret"
  # upload_url / directory / download_url omitted on purpose
}
`,
				ExpectError: regexp.MustCompile(`upload_url is required when cdn_type`),
			},
		},
	})
}

// TestAccResource_ProCloudDistributionPoint_AmazonSignedURLsRequirePrivateKey
// verifies the AMAZON_S3 + require_signed_urls → private_key conditional. Plan
// time only; not destructive.
func TestAccResource_ProCloudDistributionPoint_AmazonSignedURLsRequirePrivateKey(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_cloud_distribution_point" "test" {
  cdn_type            = "AMAZON_S3"
  username            = "AKIAEXAMPLE"
  password            = "secret"
  require_signed_urls = true
  # private_key omitted on purpose
}
`,
				ExpectError: regexp.MustCompile(`private_key is required`),
			},
		},
	})
}

// TestAccDataSource_ProCloudDistributionPoint reads the current cloud
// distribution point. Non-destructive (GET only), so not env-gated.
func TestAccDataSource_ProCloudDistributionPoint(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "jamfplatform_pro_cloud_distribution_point" "lookup" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_cloud_distribution_point.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_cloud_distribution_point.lookup", "cdn_type"),
				),
			},
		},
	})
}
