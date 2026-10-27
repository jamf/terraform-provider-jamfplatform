// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package file_share_distribution_point_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingFileShareDistributionPoint(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	created, err := c.CreateDistributionPointV1(ctx, &pro.DistributionPoint{
		Name:                      name,
		ServerName:                "smb.acc.example.com",
		FileSharingConnectionType: "SMB",
		ShareName:                 new("CasperShare"),
		Port:                      new(445),
		Workgroup:                 new("WORKGROUP"),
		ReadWriteUsername:         new("rwuser"),
		ReadWritePassword:         new("rw-secret-1"),
		ReadOnlyUsername:          new("rouser"),
		ReadOnlyPassword:          new("ro-secret-1"),
		HttpsEnabled:              new(false),
	})
	if err != nil {
		t.Fatalf("creating distribution point %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteDistributionPointV1(ctx, created.ID) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_file_share_distribution_point" "test" {
			name                         = %q
			server_name                  = "smb.acc.example.com"
			file_sharing_connection_type = "SMB"
			share_name                   = "CasperShare"
			port                         = 445
			workgroup                    = "WORKGROUP"

			read_write_username            = "rwuser"
			read_write_password            = "rw-secret-1"
			read_write_password_wo_version = 1
			read_only_username             = "rouser"
			read_only_password             = "ro-secret-1"
			read_only_password_wo_version  = 1
		}
	`, name)
}

func TestAccResource_ProFileShareDistributionPoint_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-file-share-distribution-point-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingFileShareDistributionPoint(t, name) },
				Config:      collisionConfig(" " + strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*file\s+share\s+distribution\s+point.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

// twoDistributionPointsConfig declares two distribution points. The second
// depends on the first so the creates are sequential: Jamf's duplicate check is
// not atomic, and two concurrent creates of one name can both succeed, which
// would leave a test asserting on a race instead of on the refusal.
func twoDistributionPointsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_file_share_distribution_point" "one" {
			name                         = %q
			server_name                  = "smb.acc.example.com"
			file_sharing_connection_type = "SMB"
			share_name                   = "CasperShare"
			port                         = 445
			workgroup                    = "WORKGROUP"

			read_write_username            = "rwuser"
			read_write_password            = "rw-secret-1"
			read_write_password_wo_version = 1
			read_only_username             = "rouser"
			read_only_password             = "ro-secret-1"
			read_only_password_wo_version  = 1
		}

		resource "jamfplatform_pro_file_share_distribution_point" "two" {
			depends_on = [jamfplatform_pro_file_share_distribution_point.one]

			name                         = %q
			server_name                  = "smb.acc.example.com"
			file_sharing_connection_type = "SMB"
			share_name                   = "CasperShare"
			port                         = 445
			workgroup                    = "WORKGROUP"

			read_write_username            = "rwuser"
			read_write_password            = "rw-secret-1"
			read_write_password_wo_version = 1
			read_only_username             = "rouser"
			read_only_password             = "ro-secret-1"
			read_only_password_wo_version  = 1
		}
	`, nameOne, nameTwo)
}

func TestAccResource_ProFileShareDistributionPoint_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoDistributionPointsConfig(one, two),
			},
			{
				Config:      twoDistributionPointsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProFileShareDistributionPoint_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoDistributionPointsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating\s+Jamf\s+Pro\s+file\s+share\s+distribution\s+point.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
