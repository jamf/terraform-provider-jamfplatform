// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /ldapservers endpoint.
// Creating an LDAP server stores the supplied configuration without verifying
// that the directory is reachable (connection verification is not modelled —
// the classic lookup endpoints give no usable bind signal), so these tests use
// a dummy hostname and dummy credentials and still exercise full CRUD.
//
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with other classic acceptance work.

package ldap_server_test

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

const ldapServerResource = "jamfplatform_pro_ldap_server.test"

// testAccCheckLdapServerDestroy verifies servers created during the test were
// destroyed.
func testAccCheckLdapServerDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_ldap_server" {
				continue
			}
			_, err := c.GetLDAPServerByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro LDAP server %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro LDAP server %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProLdapServer_Anonymous covers the anonymous-bind shape
// (authentication_type = "none", no account block) plus an ImportStateVerify
// round-trip.
func TestAccResource_ProLdapServer_Anonymous(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ldap-anon-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLdapServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Open Directory"
							hostname            = "ldap.acc-anon.example.com"
							port                = 389
							use_ssl             = false
							authentication_type = "none"
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ldapServerResource, "id"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.display_name", name),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.directory_service", "Open Directory"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.authentication_type", "none"),
					// Server-managed echo is self-healing and present after apply.
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.is_enabled", "true"),
				),
			},
			{
				ResourceName:      ldapServerResource,
				ImportState:       true,
				ImportStateVerify: true,
				// This server declares no mappings_for_users, so its managed state
				// has none — but Jamf Pro fills server-side mapping defaults, and
				// import surfaces them (full fidelity). The managed (mapping-less)
				// state therefore cannot equal the imported state; ignore mappings
				// here. (The SimpleUpdate test declares all three blocks and DOES
				// verify mappings on import.)
				ImportStateVerifyIgnore: []string{"timeouts", "mappings_for_users"},
			},
		},
	})
}

// TestAccResource_ProLdapServer_SimpleUpdate covers the authenticated-bind
// shape with a full mappings tree, then a multi-attribute update that mutates
// every non-RequiresReplace surface: connection fields, the bind account,
// every mapping sub-block, and a password rotation (wo_version bump).
func TestAccResource_ProLdapServer_SimpleUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ldap-simple-" + suffix
	renamed := "tf-acc-ldap-simple-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLdapServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-simple.example.com"
							port                = 636
							use_ssl             = true
							authentication_type = "simple"
							connection_timeout  = 15
							search_timeout      = 60
							referral_response   = "follow"
							use_wildcards       = true
							account = {
								distinguished_username = "CN=svc,DC=example,DC=com"
								password               = "initial-pw"
								password_wo_version    = 1
							}
						}
						mappings_for_users = {
							user_mappings = {
								object_class_limitation = "any"
								object_classes          = "organizationalPerson"
								search_base              = "OU=Users,DC=example,DC=com"
								search_scope             = "All Subtrees"
								username                 = "mail"
								real_name                = "displayName"
								email_address            = "mail"
								phone                    = "telephoneNumber"
								user_uuid                = "objectGUID"
							}
							user_group_mappings = {
								object_class_limitation = "any"
								object_classes          = "group"
								search_base              = "OU=Groups,DC=example,DC=com"
								search_scope             = "All Subtrees"
								group_name               = "sAMAccountName"
								group_uuid               = "objectGUID"
							}
							user_group_membership_mappings = {
								membership_location  = "group object"
								member_user_mapping  = "member"
								use_dn               = true
								use_ldap_compare     = true
								recursive_lookups    = true
							}
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ldapServerResource, "id"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.display_name", name),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.port", "636"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.use_ssl", "true"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.referral_response", "follow"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.account.distinguished_username", "CN=svc,DC=example,DC=com"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.phone", "telephoneNumber"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.username", "mail"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_mappings.group_name", "sAMAccountName"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.member_user_mapping", "member"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.is_enabled", "true"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-simple-2.example.com"
							port                = 3269
							use_ssl             = true
							authentication_type = "simple"
							connection_timeout  = 30
							search_timeout      = 120
							referral_response   = "ignore"
							use_wildcards       = false
							account = {
								distinguished_username = "CN=svc2,DC=example,DC=com"
								password               = "rotated-pw"
								password_wo_version    = 2
							}
						}
						mappings_for_users = {
							user_mappings = {
								object_class_limitation = "all"
								object_classes          = "user"
								search_base              = "OU=People,DC=example,DC=com"
								search_scope             = "First Level Only"
								username                 = "userPrincipalName"
								real_name                = "cn"
								phone                    = "mobile"
							}
							user_group_mappings = {
								object_class_limitation = "all"
								group_name               = "cn"
							}
							user_group_membership_mappings = {
								membership_location                 = "user object"
								group_membership_mapping            = "memberOf"
								map_user_membership_use_dn          = true
								recursive_lookups                   = true
								use_member_field_for_select_queries = true
							}
						}
					}
				`, renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.display_name", renamed),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.hostname", "ldap.acc-simple-2.example.com"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.port", "3269"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.connection_timeout", "30"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.search_timeout", "120"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.referral_response", "ignore"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.use_wildcards", "false"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.account.distinguished_username", "CN=svc2,DC=example,DC=com"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.object_class_limitation", "all"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.search_scope", "First Level Only"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.username", "userPrincipalName"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.membership_location", "user object"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.use_member_field_for_select_queries", "true"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.group_membership_mapping", "memberOf"),
				),
			},
			{
				ResourceName:      ldapServerResource,
				ImportState:       true,
				ImportStateVerify: true,
				// password is WriteOnly (never in state); password_wo_version is
				// not echoed by Jamf Pro so import cannot reconstruct it. Mappings
				// ARE verified: this config declares all three sub-blocks, and
				// import populates every block from the server (full fidelity).
				ImportStateVerifyIgnore: []string{"timeouts", "connection_settings.account.password", "connection_settings.account.password_wo_version"},
			},
		},
	})
}

// TestAccResource_ProLdapServer_ReferralResponseExplicitDefault pins the
// regression behind https://github.com/jamf/terraform-provider-jamfplatform/issues/270:
// `referral_response = ""` is the documented way to select "use default from
// LDAP service," but Classic always echoes an empty <referral_response/> on
// read regardless of what was configured. Before the fix, decoding that empty
// echo with `StringPointerValueOrNull` collapsed the explicitly-configured ""
// to Null, which failed apply with "Provider produced inconsistent result
// after apply" — reported against the whole connection_settings block because
// account.password is Sensitive, masking the real attribute. Step 1 sets ""
// on Create; step 2 changes an unrelated field while keeping "" through
// Update, so both write paths are covered and the implicit post-apply plan
// check pins no permadiff.
func TestAccResource_ProLdapServer_ReferralResponseExplicitDefault(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ldap-referral-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLdapServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-referral.example.com"
							authentication_type = "none"
							referral_response   = "" # use default from LDAP service
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ldapServerResource, "id"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.referral_response", ""),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-referral-2.example.com"
							authentication_type = "none"
							referral_response   = ""
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.hostname", "ldap.acc-referral-2.example.com"),
					resource.TestCheckResourceAttr(ldapServerResource, "connection_settings.referral_response", ""),
				),
			},
		},
	})
}

// TestAccResource_ProLdapServer_PartialMappingsGating pins the mappings
// gating: the server echoes all three mapping sub-blocks, but the provider
// keeps only the ones the user declared. Step 1 declares only user_mappings
// (the other two must be absent from state). Step 2 drops user_mappings and
// declares only user_group_membership_mappings — exercising sub-block remove
// + add and the membership "Other-mode" fields (object_classes / search_base /
// username_mapping / group_id_mapping) under a real membership_location. The
// framework's implicit post-apply plan check pins no permadiff for each
// partial shape — the regression path the gating fix addresses.
func TestAccResource_ProLdapServer_PartialMappingsGating(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ldap-gate-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLdapServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-gate.example.com"
							authentication_type = "none"
						}
						mappings_for_users = {
							user_mappings = {
								object_class_limitation = "any"
								search_scope            = "All Subtrees"
								username                = "uid"
							}
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.username", "uid"),
					// Undeclared sub-blocks (echoed by the server) must be gated out.
					resource.TestCheckNoResourceAttr(ldapServerResource, "mappings_for_users.user_group_mappings.group_name"),
					resource.TestCheckNoResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.membership_location"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-gate.example.com"
							authentication_type = "none"
						}
						mappings_for_users = {
							user_group_membership_mappings = {
								membership_location     = "group object"
								object_class_limitation = "any"
								object_classes          = "posixGroup"
								search_base             = "OU=Groups,DC=example,DC=com"
								search_scope            = "All Subtrees"
								username_mapping        = "uid"
								group_id_mapping        = "gidNumber"
							}
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					// user_mappings was declared in step 1, dropped here → absent.
					resource.TestCheckNoResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.username"),
					// Membership "Other-mode" fields round-trip under a real location.
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.membership_location", "group object"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.object_classes", "posixGroup"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.username_mapping", "uid"),
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.group_id_mapping", "gidNumber"),
				),
			},
		},
	})
}

// TestAccResource_ProLdapServer_AccountForbiddenForNone asserts the
// account-vs-auth cross-field validator rejects an account block for an
// anonymous bind.
func TestAccResource_ProLdapServer_AccountForbiddenForNone(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = "tf-acc-ldap-bad"
							directory_service   = "Active Directory"
							hostname            = "ldap.example.com"
							authentication_type = "none"
							account = {
								distinguished_username = "CN=svc"
							}
						}
					}
				`,
				ExpectError: regexp.MustCompile(`account forbidden for anonymous bind`),
			},
		},
	})
}

// TestAccResource_ProLdapServer_AccountRequiredForSimple asserts the validator
// requires an account block for a non-anonymous bind.
func TestAccResource_ProLdapServer_AccountRequiredForSimple(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = "tf-acc-ldap-bad"
							directory_service   = "Active Directory"
							hostname            = "ldap.example.com"
							authentication_type = "simple"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`account required for authenticated bind`),
			},
		},
	})
}

// TestAccResource_ProLdapServer_InvalidDirectoryService asserts the OneOf
// validator blocks an unknown server_type (which the server would silently
// coerce to Active Directory).
func TestAccResource_ProLdapServer_InvalidDirectoryService(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name      = "tf-acc-ldap-bad"
							directory_service = "Microsoft Active Directory"
							hostname          = "ldap.example.com"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute connection_settings.directory_service value must be one of`),
			},
		},
	})
}

// TestAccResource_ProLdapServer_InvalidReferralResponse asserts the OneOf
// validator blocks a non-lowercase referral_response (the server lower-cases
// it, which would otherwise be an inconsistent-result-after-apply).
func TestAccResource_ProLdapServer_InvalidReferralResponse(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name      = "tf-acc-ldap-bad"
							directory_service = "Active Directory"
							hostname          = "ldap.example.com"
							referral_response = "Follow"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

// TestAccResource_ProLdapServer_InvalidAuthType asserts the OneOf validator
// preserves the load-bearing mixed case of the authentication_type values.
func TestAccResource_ProLdapServer_InvalidAuthType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_ldap_server" "test" {
						connection_settings = {
							display_name        = "tf-acc-ldap-bad"
							directory_service   = "Active Directory"
							hostname            = "ldap.example.com"
							authentication_type = "Simple"
							account = {
								distinguished_username = "CN=svc"
							}
						}
					}
				`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

const ldapOmitRetainsConnection = `
						connection_settings = {
							display_name        = %q
							directory_service   = "Active Directory"
							hostname            = "ldap.acc-omit.example.com"
							authentication_type = "none"
						}`

// ldapOmitRetainsConfig declares every gated mapping sub-block with a
// distinctive value, so a server that stopped retaining an omitted element is
// caught on content rather than presence.
func ldapOmitRetainsConfig(name string) string {
	return fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {`+ldapOmitRetainsConnection+`
						mappings_for_users = {
							user_mappings = {
								object_class_limitation = "any"
								object_classes          = "inetOrgPerson"
								search_base             = "OU=People,DC=omit,DC=example,DC=com"
								search_scope            = "All Subtrees"
								username                = "uid"
								user_id                 = "uidNumber"
							}
							user_group_mappings = {
								object_class_limitation = "any"
								object_classes          = "groupOfNames"
								search_base             = "OU=Groups,DC=omit,DC=example,DC=com"
								search_scope            = "All Subtrees"
								group_name              = "cn"
								group_id                = "gidNumber"
							}
							user_group_membership_mappings = {
								membership_location     = "group object"
								object_class_limitation = "any"
								object_classes          = "posixGroup"
								search_base             = "OU=Groups,DC=omit,DC=example,DC=com"
								search_scope            = "All Subtrees"
								username_mapping        = "uid"
								group_id_mapping        = "gidNumber"
							}
						}
					}
				`, name)
}

// ldapOmitRetainsChildrenDroppedConfig keeps mappings_for_users but drops two
// of its three sub-blocks and one Optional+Computed leaf, so the PUT carries a
// partial <mappings_for_users>.
func ldapOmitRetainsChildrenDroppedConfig(name string) string {
	return fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {`+ldapOmitRetainsConnection+`
						mappings_for_users = {
							user_mappings = {
								object_class_limitation = "any"
								object_classes          = "inetOrgPerson"
								search_base             = "OU=People,DC=omit,DC=example,DC=com"
								search_scope            = "All Subtrees"
								username                = "uid"
							}
						}
					}
				`, name)
}

// ldapOmitRetainsConnectionOnlyConfig drops mappings_for_users entirely.
func ldapOmitRetainsConnectionOnlyConfig(name string) string {
	return fmt.Sprintf(`
					resource "jamfplatform_pro_ldap_server" "test" {`+ldapOmitRetainsConnection+`
					}
				`, name)
}

// ldapRetainedOnServer asserts the server's copy still carries every mapping
// value the omit-retains config declared in its first step.
func ldapRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := testhelpers.NewProClassicClient(t)
	return testhelpers.CheckLiveObject(ldapServerResource,
		func(ctx context.Context, id string) (*proclassic.LdapServer, error) {
			return c.GetLDAPServerByID(ctx, id)
		},
		func(s *proclassic.LdapServer) error {
			m := s.MappingsForUsers
			if m == nil {
				return fmt.Errorf("mappings_for_users: absent")
			}
			if m.UserMappings == nil {
				return fmt.Errorf("mappings_for_users.user_mappings: absent")
			}
			if err := testhelpers.RequireEqual("user_mappings.username", "uid", testhelpers.Deref(m.UserMappings.MapUsername)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("user_mappings.user_id", "uidNumber", testhelpers.Deref(m.UserMappings.MapUserID)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("user_mappings.search_base", "OU=People,DC=omit,DC=example,DC=com", testhelpers.Deref(m.UserMappings.SearchBase)); err != nil {
				return err
			}
			if m.UserGroupMappings == nil {
				return fmt.Errorf("mappings_for_users.user_group_mappings: absent")
			}
			if err := testhelpers.RequireEqual("user_group_mappings.group_name", "cn", testhelpers.Deref(m.UserGroupMappings.MapGroupName)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("user_group_mappings.object_classes", "groupOfNames", testhelpers.Deref(m.UserGroupMappings.ObjectClasses)); err != nil {
				return err
			}
			if m.UserGroupMembershipMappings == nil {
				return fmt.Errorf("mappings_for_users.user_group_membership_mappings: absent")
			}
			if err := testhelpers.RequireEqual("user_group_membership_mappings.membership_location", "group object", testhelpers.Deref(m.UserGroupMembershipMappings.UserGroupMembershipStoredIn)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("user_group_membership_mappings.object_classes", "posixGroup", testhelpers.Deref(m.UserGroupMembershipMappings.ObjectClasses)); err != nil {
				return err
			}
			return testhelpers.RequireEqual("user_group_membership_mappings.group_id_mapping", "gidNumber", testhelpers.Deref(m.UserGroupMembershipMappings.GroupID))
		})
}

// TestAccResource_ProLdapServer_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping mapping sub-blocks, and then
// mappings_for_users itself, plans them as removed, but the classic PUT omits
// the elements and the server keeps every value. Step 2 keeps
// mappings_for_users with only user_mappings, so the PUT carries a partial
// parent; step 3 drops the block so the PUT carries <connection> alone. Each
// step's implicit post-apply plan must be empty. If this test fails on
// content, the endpoint no longer merges at that level and nothing that
// suppresses the removal plan may ship for this resource.
func TestAccResource_ProLdapServer_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ldap-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLdapServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ldapOmitRetainsConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_group_mappings.group_name", "cn"),
					ldapRetainedOnServer(t),
				),
			},
			{
				Config: ldapOmitRetainsChildrenDroppedConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.username", "uid"),
					resource.TestCheckNoResourceAttr(ldapServerResource, "mappings_for_users.user_group_mappings.group_name"),
					resource.TestCheckNoResourceAttr(ldapServerResource, "mappings_for_users.user_group_membership_mappings.membership_location"),
					ldapRetainedOnServer(t),
				),
			},
			{
				Config: ldapOmitRetainsConnectionOnlyConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(ldapServerResource, "mappings_for_users.user_mappings.username"),
					ldapRetainedOnServer(t),
				),
			},
		},
	})
}
