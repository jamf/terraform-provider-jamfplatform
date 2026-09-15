// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// Shared Okta LDAP directory-service fixture for every acceptance test that needs
// a directory service configured on the tenant — enrollment customization LDAP
// panes, user-initiated-enrollment LDAP access groups, scope blocks that reference
// a directory-service user group (VPP, Mac/mobile apps), and the directory-service
// group criteria tests.
//
// One Okta directory backs them all: the SAML metadata URL
// (JAMFPLATFORM_ACC_PRO_SSO_IDP_URL, also used by the SSO fixtures) yields the
// subdomain/SLD/TLD; the bind credentials come from JAMFPLATFORM_ACC_PRO_LDAP_USERNAME
// / JAMFPLATFORM_ACC_PRO_LDAP_PASSWORD; and JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME names a
// group that exists in that directory (used wherever a test references a real
// directory-service group by name).
//
// Two creation paths share one config derivation (deriveOktaLdapConfig) so they
// never drift:
//   - LdapServerFixture — HCL for the jamfplatform_pro_ldap_server resource
//     (labelled acc_ldap). Use when the provider resolves the group at APPLY time;
//     embed it and depend_on acc_ldap.
//   - EnsureLdapServerFixture — proclassic SDK POST + t.Cleanup. Use when the test
//     resolves the group through the live API BEFORE Terraform applies (the
//     directory must already exist), e.g. ResolveDSGroupWireValue.

const (
	// EnvSSOIdpURL is the SAML IdP metadata URL; its host is split into
	// <subdomain>.<sld>.<tld> to derive the Okta LDAP hostname and DN components.
	EnvSSOIdpURL = "JAMFPLATFORM_ACC_PRO_SSO_IDP_URL"
	// EnvLdapUsername / EnvLdapPassword are the Okta LDAP bind service-account
	// credentials.
	EnvLdapUsername = "JAMFPLATFORM_ACC_PRO_LDAP_USERNAME"
	EnvLdapPassword = "JAMFPLATFORM_ACC_PRO_LDAP_PASSWORD" //nolint:gosec // env var name, not a credential
	// EnvLdapGroupName names a directory-service group that exists in the Okta
	// directory; tests reference it wherever they need a real group by name.
	EnvLdapGroupName = "JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME"

	// LdapFixtureResourceLabel is the Terraform resource label of the shared LDAP
	// server fixture written by LdapServerFixture; depends_on / id references use
	// LdapFixtureResourceAddr.
	LdapFixtureResourceLabel = "acc_ldap"
	LdapFixtureResourceAddr  = "jamfplatform_pro_ldap_server." + LdapFixtureResourceLabel
)

// OktaLdapEnv carries the LDAP fixture inputs: the bind credentials plus the Okta
// host split into <subdomain>.<sld>.<tld> (e.g. dev-12345678.okta.com).
type OktaLdapEnv struct {
	Username, Password  string
	Subdomain, SLD, TLD string
}

// RequireOktaLdapEnv skips the calling test unless the bind credentials and the
// SAML IdP metadata URL are all set, then derives the Okta host components.
func RequireOktaLdapEnv(t *testing.T) OktaLdapEnv {
	t.Helper()
	username := AccEnv(EnvLdapUsername)
	password := AccEnv(EnvLdapPassword)
	idpURL := AccEnv(EnvSSOIdpURL)
	if username == "" || password == "" || idpURL == "" {
		t.Skipf("skipping LDAP test: set %s, %s, and %s so the test can stand up an Okta LDAP directory-service fixture", EnvLdapUsername, EnvLdapPassword, EnvSSOIdpURL)
	}
	u, err := url.Parse(idpURL)
	if err != nil || u.Hostname() == "" {
		t.Skipf("skipping LDAP test: %s=%q is not a valid URL: %v", EnvSSOIdpURL, idpURL, err)
	}
	labels := strings.Split(u.Hostname(), ".")
	if len(labels) < 3 {
		t.Skipf("skipping LDAP test: %s host %q must be <subdomain>.<sld>.<tld> (e.g. dev-12345678.okta.com)", EnvSSOIdpURL, u.Hostname())
	}
	return OktaLdapEnv{
		Username:  username,
		Password:  password,
		Subdomain: labels[0],
		SLD:       labels[len(labels)-2],
		TLD:       labels[len(labels)-1],
	}
}

// RequireLdapGroupName skips the calling test unless a directory-service group name
// is set.
func RequireLdapGroupName(t *testing.T) string {
	t.Helper()
	g := AccEnv(EnvLdapGroupName)
	if g == "" {
		t.Skipf("skipping LDAP-group test: set %s to a directory-service group that exists in the Okta directory", EnvLdapGroupName)
	}
	return g
}

// oktaLdapConfig holds the derived field values shared by the HCL fixture and the
// SDK pre-create so the two never drift.
type oktaLdapConfig struct {
	displayName, hostname, dn, username, password string
}

func deriveOktaLdapConfig(prefix string, e OktaLdapEnv) oktaLdapConfig {
	return oktaLdapConfig{
		displayName: prefix + "-Okta",
		hostname:    fmt.Sprintf("%s.ldap.%s.%s", e.Subdomain, e.SLD, e.TLD),
		dn:          fmt.Sprintf("dc=%s,dc=%s,dc=%s", e.Subdomain, e.SLD, e.TLD),
		username:    e.Username,
		password:    e.Password,
	}
}

// LdapServerFixture returns HCL for a jamfplatform_pro_ldap_server resource labelled
// acc_ldap, pointed at the Okta LDAP interface. prefix makes the display name
// unique per test. Classic /ldapservers does not test connectivity on save, so it
// creates regardless of whether Okta LDAP is reachable.
func LdapServerFixture(prefix string, e OktaLdapEnv) string {
	c := deriveOktaLdapConfig(prefix, e)
	return fmt.Sprintf(`
resource "jamfplatform_pro_ldap_server" %[1]q {
	connection_settings = {
		display_name        = %[2]q
		directory_service   = "Custom"
		hostname            = %[3]q
		port                = 636
		use_ssl             = true
		authentication_type = "simple"
		connection_timeout  = 15
		search_timeout      = 60
		use_wildcards       = true
		account = {
			distinguished_username = "uid=%[4]s,%[5]s"
			password               = %[6]q
			password_wo_version    = 1
		}
	}
	mappings_for_users = {
		user_mappings = {
			object_class_limitation = "all"
			object_classes          = "inetOrgPerson"
			search_base             = "ou=users,%[5]s"
			search_scope            = "All Subtrees"
			user_id                 = "uid"
			username                = "uid"
			real_name               = "cn"
			email_address           = "mail"
			department              = "department"
			position                = "title"
			user_uuid               = "uid"
		}
		user_group_mappings = {
			object_class_limitation = "all"
			object_classes          = "groupofUniqueNames"
			search_base             = "ou=groups,%[5]s"
			search_scope            = "All Subtrees"
			group_id                = "uniqueIdentifier"
			group_name              = "cn"
			group_uuid              = "uniqueIdentifier"
		}
		user_group_membership_mappings = {
			membership_location                 = "group object"
			use_dn                              = false
			recursive_lookups                   = false
			member_user_mapping                 = "uniqueMember"
			map_user_membership_use_dn          = true
			object_class_limitation             = "all"
			search_scope                        = "All Subtrees"
			use_ldap_compare                    = false
			membership_calculation_optimization = true
		}
	}
}
`, LdapFixtureResourceLabel, c.displayName, c.hostname, c.username, c.dn, c.password)
}

// EnsureLdapServerFixture creates the same Okta LDAP server via the proclassic SDK
// (POST id=0) and registers t.Cleanup to delete it. Use for tests that resolve a
// directory-service group through the live API before Terraform runs. Returns the
// created server id.
func EnsureLdapServerFixture(t *testing.T, prefix string, e OktaLdapEnv) string {
	t.Helper()
	c := deriveOktaLdapConfig(prefix, e)
	client := NewProClassicClient(t)
	ctx := context.Background()
	created, err := client.CreateLDAPServerByID(ctx, "0", oktaLdapServerPost(c))
	if err != nil {
		t.Fatalf("creating LDAP server fixture %q: %v", c.displayName, err)
	}
	if created == nil || created.ID == nil {
		t.Fatalf("LDAP server fixture %q: create response missing id", c.displayName)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() {
		ctx := context.Background()
		if err := client.DeleteLDAPServerByID(ctx, id); err == nil {
			return
		}
		// Delete can be refused when a scope-bearing object still references this
		// directory — e.g. an orphaned app->LDAP association left behind by a
		// killed/cancelled run (a server-side data-integrity bug the API cannot
		// otherwise clear). Fall back to DISABLING the server so the leak is benign:
		// a disabled directory is skipped for group resolution and so will not cause
		// "group name not unique" failures in later runs. Best-effort.
		if err := client.UpdateLDAPServerByID(ctx, id, &proclassic.LdapServerPost{
			Connection: &proclassic.LdapServerPostConnection{IsEnabled: new(false)},
		}); err != nil {
			t.Logf("LDAP fixture %s: delete blocked and disable fallback failed: %v", id, err)
		} else {
			t.Logf("LDAP fixture %s: delete blocked (likely orphaned reference); disabled it instead so it won't poison later runs", id)
		}
	})
	return id
}

// oktaLdapServerPost mirrors LdapServerFixture's HCL as the proclassic SDK payload.
func oktaLdapServerPost(c oktaLdapConfig) *proclassic.LdapServerPost {
	return &proclassic.LdapServerPost{
		Connection: &proclassic.LdapServerPostConnection{
			Name:               new(c.displayName),
			ServerType:         new(proclassic.LdapServerConnectionServerTypeCustom),
			Hostname:           new(c.hostname),
			Port:               new(636),
			UseSsl:             new(true),
			AuthenticationType: new(proclassic.LdapServerConnectionAuthenticationTypeSimple),
			OpenCloseTimeout:   new(15),
			SearchTimeout:      new(60),
			UseWildcards:       new(true),
			Account: &proclassic.LdapServerConnectionAccount{
				DistinguishedUsername: new(fmt.Sprintf("uid=%s,%s", c.username, c.dn)),
				Password:              new(c.password),
			},
		},
		MappingsForUsers: &proclassic.LdapServerPostMappingsForUsers{
			UserMappings: &proclassic.LdapServerMappingsForUsersUserMappings{
				MapObjectClassToAnyOrAll: new(proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllAll),
				ObjectClasses:            new("inetOrgPerson"),
				SearchBase:               new("ou=users," + c.dn),
				SearchScope:              new(proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeAllSubtrees),
				MapUserID:                new("uid"),
				MapUsername:              new("uid"),
				MapRealname:              new("cn"),
				MapEmailAddress:          new("mail"),
				MapDepartment:            new("department"),
				MapPosition:              new("title"),
				MapUserUUID:              new("uid"),
			},
			UserGroupMappings: &proclassic.LdapServerMappingsForUsersUserGroupMappings{
				MapObjectClassToAnyOrAll: new(proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllAll),
				ObjectClasses:            new("groupofUniqueNames"),
				SearchBase:               new("ou=groups," + c.dn),
				SearchScope:              new(proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeAllSubtrees),
				MapGroupID:               new("uniqueIdentifier"),
				MapGroupName:             new("cn"),
				MapGroupUUID:             new("uniqueIdentifier"),
			},
			UserGroupMembershipMappings: &proclassic.LdapServerMappingsForUsersUserGroupMembershipMappings{
				UserGroupMembershipStoredIn:   new(proclassic.LdapServerMappingsForUsersUserGroupMembershipMappingsUserGroupMembershipStoredInGroupObject),
				MapUserMembershipToGroupField: new("uniqueMember"),
				UseDn:                         new(false),
				RecursiveLookups:              new(false),
				MapUserMembershipUseDn:        new(true),
				MapObjectClassToAnyOrAll:      new(proclassic.LdapServerMappingsForUsersUserMappingsMapObjectClassToAnyOrAllAll),
				SearchScope:                   new(proclassic.LdapServerMappingsForUsersUserMappingsSearchScopeAllSubtrees),
				MembershipScopingOptimization: new(true),
			},
		},
	}
}
