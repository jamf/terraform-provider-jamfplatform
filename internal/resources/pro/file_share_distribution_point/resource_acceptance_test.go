// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package file_share_distribution_point_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const dpType = "jamfplatform_pro_file_share_distribution_point"

// testAccCheckDistributionPointDestroy verifies distribution points created
// during the test were destroyed.
func testAccCheckDistributionPointDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != dpType {
				continue
			}
			_, err := c.GetDistributionPointV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro distribution point %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro distribution point %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

var importIgnore = []string{
	"timeouts",
	// The three plaintext passwords are WriteOnly — never persisted, never
	// imported. Their *_wo_version companions are import-stable.
	"read_write_password",
	"read_only_password",
	"https_password",
}

// TestAccResource_ProFileShareDistributionPoint_SMB exercises the full SMB
// lifecycle: create with read/write + read-only accounts and HTTPS
// username/password, then update (rename, change the share, rotate all three
// passwords via their *_wo_version companions), then import.
func TestAccResource_ProFileShareDistributionPoint_SMB(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-smb-" + suffix
	renamed := name + "-renamed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDistributionPointDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
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

						https_enabled       = true
						https_port          = 443
						https_context       = "casper"
						https_security_type = "USERNAME_PASSWORD"
						https_username      = "httpsuser"
						https_password      = "https-secret-1"
						https_password_wo_version = 1
					}
				`, dpType, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dpType+".test", "id"),
					resource.TestCheckResourceAttr(dpType+".test", "name", name),
					resource.TestCheckResourceAttr(dpType+".test", "file_sharing_connection_type", "SMB"),
					resource.TestCheckResourceAttr(dpType+".test", "share_name", "CasperShare"),
					resource.TestCheckResourceAttr(dpType+".test", "port", "445"),
					resource.TestCheckResourceAttr(dpType+".test", "workgroup", "WORKGROUP"),
					resource.TestCheckResourceAttr(dpType+".test", "read_write_username", "rwuser"),
					resource.TestCheckResourceAttr(dpType+".test", "https_security_type", "USERNAME_PASSWORD"),
					resource.TestCheckResourceAttr(dpType+".test", "backup_distribution_point_id", "-1"),
					resource.TestCheckResourceAttr(dpType+".test", "principal", "false"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "smb.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "CasperShareV2"
						port                         = 445
						workgroup                    = "WORKGROUP"

						read_write_username            = "rwuser"
						read_write_password            = "rw-secret-2"
						read_write_password_wo_version = 2
						read_only_username             = "rouser2"
						read_only_password             = "ro-secret-2"
						read_only_password_wo_version  = 2

						https_enabled       = true
						https_port          = 8443
						https_context       = "casper"
						https_security_type = "USERNAME_PASSWORD"
						https_username      = "httpsuser"
						https_password      = "https-secret-2"
						https_password_wo_version = 2
					}
				`, dpType, renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dpType+".test", "name", renamed),
					resource.TestCheckResourceAttr(dpType+".test", "share_name", "CasperShareV2"),
					resource.TestCheckResourceAttr(dpType+".test", "read_only_username", "rouser2"),
					resource.TestCheckResourceAttr(dpType+".test", "https_port", "8443"),
					resource.TestCheckResourceAttr(dpType+".test", "read_write_password_wo_version", "2"),
				),
			},
			{
				ResourceName:            dpType + ".test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importIgnore,
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_AFP covers the AFP protocol.
func TestAccResource_ProFileShareDistributionPoint_AFP(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-afp-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDistributionPointDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "afp.acc.example.com"
						file_sharing_connection_type = "AFP"
						share_name                   = "AFPShare"
						port                         = 548

						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
					}
				`, dpType, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dpType+".test", "file_sharing_connection_type", "AFP"),
					resource.TestCheckResourceAttr(dpType+".test", "port", "548"),
					resource.TestCheckResourceAttr(dpType+".test", "share_name", "AFPShare"),
				),
			},
			{
				ResourceName:            dpType + ".test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importIgnore,
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_HTTPSOnly covers a NONE
// connection type that serves packages over HTTPS only.
func TestAccResource_ProFileShareDistributionPoint_HTTPSOnly(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-https-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDistributionPointDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "https.acc.example.com"
						file_sharing_connection_type = "NONE"
						https_enabled                = true
						https_port                   = 443
						https_context                = "downloads"
						https_security_type          = "NONE"
					}
				`, dpType, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dpType+".test", "file_sharing_connection_type", "NONE"),
					resource.TestCheckResourceAttr(dpType+".test", "https_enabled", "true"),
					resource.TestCheckResourceAttr(dpType+".test", "https_security_type", "NONE"),
				),
			},
			{
				ResourceName:            dpType + ".test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importIgnore,
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_Failover stands up two
// distribution points, points the second at the first as its failover, and
// enables randomized load sharing.
func TestAccResource_ProFileShareDistributionPoint_Failover(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	primary := "tf-acc-fsdp-primary-" + suffix
	secondary := "tf-acc-fsdp-secondary-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDistributionPointDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %[1]q "primary" {
						name                         = %[2]q
						server_name                  = "primary.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "Primary"
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
					}

					resource %[1]q "secondary" {
						name                         = %[3]q
						server_name                  = "secondary.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "Secondary"
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1

						backup_distribution_point_id = %[1]s.primary.id
						enable_load_balancing        = true
					}
				`, dpType, primary, secondary),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dpType+".secondary", "backup_distribution_point_id", dpType+".primary", "id"),
					resource.TestCheckResourceAttr(dpType+".secondary", "enable_load_balancing", "true"),
				),
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_SMBToNone converts an SMB
// distribution point to HTTPS-only (file_sharing_connection_type = NONE). The
// server blanks the file-sharing fields; the provider must omit them on the
// write (no "port should be blank" 400) and predict their clearing (no
// inconsistent-result), even though the user omits them from the new config and
// UseStateForUnknown would otherwise carry the prior SMB values forward.
func TestAccResource_ProFileShareDistributionPoint_SMBToNone(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-s2n-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDistributionPointDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "s2n.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "Share"
						port                         = 445
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
					}
				`, dpType, name),
				Check: resource.TestCheckResourceAttr(dpType+".test", "port", "445"),
			},
			{
				// Switch to HTTPS-only; omit every file-sharing field.
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "s2n.acc.example.com"
						file_sharing_connection_type = "NONE"
						https_enabled                = true
						https_port                   = 443
						https_security_type          = "NONE"
					}
				`, dpType, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dpType+".test", "file_sharing_connection_type", "NONE"),
					resource.TestCheckResourceAttr(dpType+".test", "https_enabled", "true"),
					resource.TestCheckNoResourceAttr(dpType+".test", "share_name"),
				),
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_NoTransport asserts the
// plan-time transport validator rejects NONE with HTTPS disabled.
func TestAccResource_ProFileShareDistributionPoint_NoTransport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-notransport-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "none.acc.example.com"
						file_sharing_connection_type = "NONE"
						https_enabled                = false
					}
				`, dpType, name),
				ExpectError: regexp.MustCompile(`transport`),
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_LoadBalancingNoFailover
// asserts the plan-time load-balancing validator rejects load sharing without
// a real failover distribution point.
func TestAccResource_ProFileShareDistributionPoint_LoadBalancingNoFailover(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-lb-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "lb.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "Share"
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
						backup_distribution_point_id = "-1"
						enable_load_balancing        = true
					}
				`, dpType, name),
				ExpectError: regexp.MustCompile(`failover`),
			},
		},
	})
}

// TestAccResource_ProFileShareDistributionPoint_SplitOwnership proves
// omit=preserve on a representative co-managed field: an out-of-band change to
// an omitted field survives an update to an unrelated managed field, then an
// explicit empty string clears it. (STYLE_GUIDE §Full-replace acceptance
// coverage — the merge analogue.)
func TestAccResource_ProFileShareDistributionPoint_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-fsdp-split-" + suffix

	setWorkgroupOutOfBand := func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		rs := s.RootModule().Resources[dpType+".test"]
		if rs == nil {
			return fmt.Errorf("resource not found in state")
		}
		wg := "OUT-OF-BAND"
		_, err := c.PatchDistributionPointV1(ctx, rs.Primary.ID, &pro.DistributionPoint{
			Name:                      name,
			ServerName:                "split.acc.example.com",
			FileSharingConnectionType: "SMB",
			Workgroup:                 &wg,
		})
		return err
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDistributionPointDestroy(t),
		Steps: []resource.TestStep{
			{
				// workgroup omitted from config.
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "split.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "Share"
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
					}
				`, dpType, name),
				Check: setWorkgroupOutOfBand,
			},
			{
				// Change an unrelated managed field (share_name); workgroup is
				// still omitted, so the out-of-band value must survive.
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "split.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "ShareV2"
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
					}
				`, dpType, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dpType+".test", "share_name", "ShareV2"),
					resource.TestCheckResourceAttr(dpType+".test", "workgroup", "OUT-OF-BAND"),
				),
			},
			{
				// Explicitly clear workgroup with an empty string.
				Config: fmt.Sprintf(`
					resource %q "test" {
						name                         = %q
						server_name                  = "split.acc.example.com"
						file_sharing_connection_type = "SMB"
						share_name                   = "ShareV2"
						workgroup                    = ""
						read_write_username            = "rwuser"
						read_write_password            = "rw-secret"
						read_write_password_wo_version = 1
						read_only_username             = "rouser"
						read_only_password             = "ro-secret"
						read_only_password_wo_version  = 1
					}
				`, dpType, name),
				Check: resource.TestCheckResourceAttr(dpType+".test", "workgroup", ""),
			},
		},
	})
}
