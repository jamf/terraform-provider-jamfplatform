// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package cloud_distribution_point_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// s3EnvKeyID and s3EnvSecret name the AWS access key pair an AMAZON_S3 test
// signs in with. They are read from the environment, never written into code.
const (
	s3EnvKeyID  = "JAMFPLATFORM_ACC_PRO_CDP_S3_ACCESS_KEY_ID"
	s3EnvSecret = "JAMFPLATFORM_ACC_PRO_CDP_S3_SECRET_ACCESS_KEY"
)

// s3Credentials reads the AWS key pair, skipping the test when it is not set.
func s3Credentials(t *testing.T) (string, string) {
	t.Helper()
	keyID := testhelpers.AccEnv(s3EnvKeyID)
	secret := testhelpers.AccEnv(s3EnvSecret)
	if keyID == "" || secret == "" {
		t.Skipf("skipping: set %s and %s to run the Amazon S3 cloud distribution point test", s3EnvKeyID, s3EnvSecret)
	}
	return keyID, secret
}

func hclS3Upload(keyID, secret string, master bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_cloud_distribution_point" "test" {
  cdn_type = "AMAZON_S3"
  username = %q
  password = %q
  master   = %t
}
`, keyID, secret, master)
}

// TestAccResource_ProCloudDistributionPoint_AmazonS3 exercises the lifecycle of
// an Amazon S3 distribution point: create → update (master) → destroy → recreate.
// The tenant is restored to JAMF_CLOUD afterwards, as the other lifecycle tests
// leave it. Destructive: gated on the same opt-in as the JCDS lifecycle test.
func TestAccResource_ProCloudDistributionPoint_AmazonS3(t *testing.T) {
	requireDestructive(t)
	testhelpers.AccPreCheck(t)
	keyID, secret := s3Credentials(t)
	resetToNone(t)
	t.Cleanup(func() { restoreJamfCloud(t) })

	rn := "jamfplatform_pro_cloud_distribution_point.test"
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIsNone(t),
		Steps: []resource.TestStep{
			{
				Config: hclS3Upload(keyID, secret, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "cdn_type", "AMAZON_S3"),
					resource.TestCheckResourceAttr(rn, "has_connection_succeeded", "true"),
					resource.TestCheckResourceAttr(rn, "master", "true"),
				),
			},
			{
				Config: hclS3Upload(keyID, secret, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(rn, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr(rn, "master", "false"),
			},
			{
				Config: "# destroyed\n",
			},
			{
				Config: hclS3Upload(keyID, secret, true),
				Check:  resource.TestCheckResourceAttr(rn, "has_connection_succeeded", "true"),
			},
		},
	})
}

// TestAccResource_ProCloudDistributionPoint_AmazonS3RejectsUnsignedFields checks
// the plan-time refusals for an Amazon S3 distribution point. Plan only: nothing
// is written to the tenant.
func TestAccResource_ProCloudDistributionPoint_AmazonS3RejectsUnsignedFields(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`expiration_seconds requires require_signed_urls = true`),
				Config: `
resource "jamfplatform_pro_cloud_distribution_point" "test" {
  cdn_type           = "AMAZON_S3"
  username           = "AKIAEXAMPLE"
  password           = "secret"
  master             = true
  expiration_seconds = 3600
}
`,
			},
			{
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`directory is not used when cdn_type = "AMAZON_S3"`),
				Config: `
resource "jamfplatform_pro_cloud_distribution_point" "test" {
  cdn_type  = "AMAZON_S3"
  username  = "AKIAEXAMPLE"
  password  = "secret"
  master    = true
  directory = "bucket"
}
`,
			},
		},
	})
}
