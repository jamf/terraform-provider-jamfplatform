// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package sso_connection_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// connectionResourceAddress is the address every test in this file uses for the
// subject resource.
const connectionResourceAddress = "jamfplatform_account_sso_connection.test"

// acceptanceDomainVariable names the environment variable holding a domain the
// acceptance organization has claimed *and verified*.
//
// It cannot be derived and it cannot be invented. A connection needs at least one
// verified domain, verification needs a public DNS record, and pointing a probe
// connection at one of the organization's live domains would repoint real
// sign-in. So the operator declares which domain is safe to attach a throwaway
// connection to, and these tests skip without it — the same declaration pattern
// the Security Cloud suites use for a scope only the operator can vouch for.
const acceptanceDomainVariable = "JAMFPLATFORM_ACC_ORGANIZATION_SSO_VERIFIED_DOMAIN"

// probeConnectionID is an identifier no connection has. It is the target of the
// update-path probe below, which is deliberately aimed at something that does not
// exist.
const probeConnectionID = "con_tfaccprobe000001"

// upstreamErrorCode is the code Jamf answers every connection update with, and
// one of the several things it answers a refused create with — an unclaimed or
// unverified domain, a missing required field, a settings payload disagreeing
// with the connection type, a name carrying anything but letters and digits, and
// the organization being at its connection limit all share it. Restated because
// the Jamf Account SDK generates no error-code vocabulary; see mappings.go, which
// records the same exemption for the non-test code.
const upstreamErrorCode = "UPSTREAM_ERROR"

// acceptanceConnectionName builds a name unlikely to collide with a live
// connection, or with a leftover from an interrupted run.
//
// It cannot use the repository's usual hyphenated tf-acc- prefix: Jamf accepts
// only letters and digits in a connection name and refuses anything else with an
// unattributed 500, so a hyphenated fixture would be rejected at plan time by the
// provider's own validator. The run suffix is an epoch timestamp, so it is safe
// here. Domain fixtures keep the hyphenated form — the constraint is on
// connection names alone.
func acceptanceConnectionName(part string) string {
	return "tfAcc" + strings.ToUpper(part[:1]) + part[1:] + testhelpers.RunSuffix()
}

// verifiedDomain returns the declared verified domain, or skips.
func verifiedDomain(t *testing.T) string {
	t.Helper()
	domain := testhelpers.AccEnv(acceptanceDomainVariable)
	if domain == "" {
		t.Skipf(
			"%s is not set. A Jamf Account SSO connection requires at least one domain the organization has "+
				"claimed and verified, verification requires a public DNS record this suite cannot publish, and "+
				"attaching a probe connection to one of the organization's live domains would repoint real "+
				"sign-in. Set it to a verified domain that is safe to attach a throwaway connection to.",
			acceptanceDomainVariable,
		)
	}
	return domain
}

// skipUnlessConnectionUpdatesWork is the gate the update-dependent work in this
// file opens with, and it exists because of a fault on Jamf's side rather than
// anything about this provider.
//
// Only changing an existing connection is impossible. Re-probed live on
// 2026-09-03: a create answers 201 for a valid body, and reading, importing, the
// data sources, the list resource and removing a connection all work — so every
// test here that creates, reads, imports, queries or destroys runs whatever the
// update path is doing, and only a step that edits an existing connection is
// gated. The update is refused with an internal failure carrying no detail for
// every request, including the verbatim body a create had just accepted, so a
// gated step would fail rather than report — and a red suite that is red for a
// reason nobody can fix is worse than a skipped one, because it hides every
// genuine failure beside it.
//
// The probe is an update aimed at an identifier that does not exist, carrying the
// body oidcConnectionConfig renders — the operator's verified domain included, so
// a working endpoint has nothing in it to object to. Reading or removing that
// identifier reports it missing, as it should; the update refuses it with the
// internal failure instead. So the probe tells the two states apart with no
// throwaway connection, and nothing created even if it succeeds:
//
//   - the internal failure means the fault is still there, and the suite skips;
//   - a not-found means the update path has been fixed, and the suite runs;
//   - anything else is a third state nobody has seen, so the suite skips saying
//     what it saw rather than failing on an unexamined difference.
//
// One residual ambiguity is worth knowing about, because the internal failure is
// an overloaded catch-all rather than one fault: an endpoint that had been fixed
// but validated the body before looking the identifier up would answer it too,
// for any body Jamf disliked. That is why the probe body mirrors a create Jamf is
// known to accept rather than being an obviously invalid one. If Jamf ever
// reports the endpoint fixed and this gate still skips, compare the probe body
// against oidcConnectionConfig before believing the gate.
//
// Delete this function, and the call to it, when Jamf fixes the update path.
func skipUnlessConnectionUpdatesWork(t *testing.T, domain string) {
	t.Helper()
	c := account.New(testhelpers.NewAcceptanceClient(t))

	_, err := c.UpdateConnection(context.Background(), probeConnectionID, probeConnectionRequest(domain))
	if err == nil {
		t.Fatalf(
			"the update-path probe updated %s, an identifier no connection has. That should be impossible; "+
				"check whether a real connection is using this identifier before running the suite again.",
			probeConnectionID,
		)
	}

	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		t.Skipf("the update-path probe could not reach Jamf Account, so an update cannot be verified: %v", err)
	}
	for _, detail := range apiErr.Details() {
		if detail.Code == upstreamErrorCode {
			t.Skipf(
				"Jamf Account cannot change an existing SSO connection: the update path answers an internal "+
					"failure for every request, including this probe aimed at %s, an identifier no connection "+
					"has — while creating, reading, listing and removing connections all work, and reading or "+
					"removing that same identifier reports it missing as it should. The fault is on Jamf's side "+
					"and has been reported. Trace identifier: %s. Reported by Jamf Account: %s",
				probeConnectionID, apiErr.TraceID, detail.Description,
			)
		}
	}
	if apiErr.HasStatus(404) {
		return
	}
	t.Skipf(
		"the update-path probe answered %d rather than the expected not-found or internal failure, which is a "+
			"state this suite has not seen. Skipping rather than failing on an unexamined difference. Body: %s",
		apiErr.StatusCode, apiErr.Body,
	)
}

// probeConnectionRequest is a complete, semantically valid generic OpenID Connect
// body for the update-path probe, aimed at the operator's verified domain.
//
// It mirrors what oidcConnectionConfig renders, which a create is known to accept
// with a 201, so that an internal failure from the probe cannot be read as Jamf
// disliking the body. That matters more than it used to: the same failure now
// covers an unclaimed domain, a missing field, a mismatched settings payload, an
// illegal name and a full organization, so a deliberately invalid probe body
// would make a fixed endpoint indistinguishable from a broken one.
func probeConnectionRequest(domain string) *account.ConnectionRequest {
	return &account.ConnectionRequest{
		ConnectionType: account.ConnectionTypeOidc,
		Domains:        []string{domain},
		EnabledProducts: []account.EnabledProduct{{
			Product:        account.ProductAccount,
			EnabledTenants: []string{},
		}},
		Connection: account.ConnectionRequestConnection{
			OidcConnectionSettings: &account.OidcConnectionSettings{
				Name:                  "tfAccUpdateProbe",
				Region:                account.RegionUs,
				ClientID:              new("tfAccClient"),
				ClientSecret:          new("tfAccClientSecret"),
				IssuerURL:             "idp.example",
				AuthorizationEndpoint: "idp.example/authorize",
				TokenEndpoint:         "idp.example/token",
				JwksUri:               "idp.example/keys",
				Scopes:                "openid email profile",
				AliasLoginHintToIdp:   true,
			},
		},
	}
}

// testAccCheckSSOConnectionDestroy verifies the connections made during the test
// were removed.
//
// It scans the organization's collection rather than reading each identifier,
// because Jamf is known to list a connection it cannot read on its own
// identifier — so a per-identifier check would report a connection as gone when
// Jamf still holds it.
func testAccCheckSSOConnectionDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := account.New(testhelpers.NewAcceptanceClient(t))

		var wanted []string
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_account_sso_connection" {
				continue
			}
			if id := rs.Primary.Attributes["id"]; id != "" {
				wanted = append(wanted, id)
			}
		}
		if len(wanted) == 0 {
			return nil
		}

		summaries, err := c.ListConnections(context.Background())
		if err != nil {
			return fmt.Errorf("error listing Jamf Account SSO connections: %w", err)
		}
		for _, summary := range summaries {
			for _, want := range wanted {
				if summary.ID == want {
					return fmt.Errorf("Jamf Account SSO connection %s still exists", want)
				}
			}
		}
		return nil
	}
}

// oidcConnectionConfig renders a generic OpenID Connect connection.
//
// `attribute_map` is in the fixture, authored with `jsonencode`, so the
// connection's only JSON-object attribute is actually sent and read back rather
// than left unset in every acceptance run. `bind_all` is the simplest of the
// three shapes the attribute's validator recognises, and every test here
// inherits it. It is not what exercises the JSON normalisation — see the
// GenerateConfigStep comment in TestAccResource_AccountSSOConnection_Basic for
// why no acceptance step can, while the update endpoint refuses every write.
func oidcConnectionConfig(name, domain string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_account_sso_connection" "test" {
			name            = %q
			connection_type = "generic_oidc"
			hosting_region  = "US"

			client_id     = "tfAccClient"
			client_secret = "tfAccClientSecret"
			scopes        = "openid email profile"

			domains = [%q]

			attribute_map = jsonencode({ mapping_mode = "bind_all" })

			generic_oidc = {
				issuer_url             = "idp.example"
				authorization_endpoint = "idp.example/authorize"
				token_endpoint         = "idp.example/token"
				jwks_uri               = "idp.example/keys"
			}
		}
	`, name, domain)
}

// TestAccResource_AccountSSOConnection_Basic covers the create and the import
// round trip. It is ungated: a create answers 201, so the round trip runs against
// any organization the operator has declared a verified domain for.
//
// Four of the assertions are worth stating the reasoning for.
//
// `name` is asserted equal to the configured name on both the create and the
// import, which is a claim about the provider rather than about Jamf: Jamf stores
// the name with a uniquifying suffix appended, and the provider reports the
// configured base name so that a plan does not perpetually differ from state.
//
// `internal_name` is asserted to be the configured name plus a suffix, which is
// what Jamf was observed to store — `tfReviewMin` came back as
// `tfReviewMin-jqxld7tl4m454ed7s35647nmjssypo`. Pinning the shape rather than an
// exact value is as tight as this can be: the suffix is Jamf's and unpredictable.
//
// `enabled_product_names` is asserted present rather than equal to anything,
// because it reports the products Jamf holds and the tenants of them are never
// returned. Asserting a value would be asserting half a fact.
//
// The import step ignores `enabled_products` and the write-only secret. Neither
// can be recovered by any read: the tenants are never echoed and the secret is
// never returned, which is exactly the drift blindness the resource documents.
//
// One failure this test cannot tell apart from a defect: a create is refused with
// the same internal failure when the organization is at its connection limit — an
// identical body answered 201 at twenty-four connections, the internal failure at
// twenty-five, and 201 again after one was removed. A create failing here is
// worth checking against the organization's connection count before it is read as
// a provider fault.
func TestAccResource_AccountSSOConnection_Basic(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := verifiedDomain(t)
	name := acceptanceConnectionName("basic")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSOConnectionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcConnectionConfig(name, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(connectionResourceAddress, "name", name),
					resource.TestCheckResourceAttr(connectionResourceAddress, "connection_type", "generic_oidc"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "hosting_region", account.RegionUs),
					resource.TestCheckResourceAttr(connectionResourceAddress, "auth_method", "client_secret"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "consent_flow", "false"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "easy_config", "false"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "domains.#", "1"),
					resource.TestCheckTypeSetElemAttr(connectionResourceAddress, "domains.*", domain),
					resource.TestCheckResourceAttrSet(connectionResourceAddress, "attribute_map"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "generic_oidc.issuer_url", "idp.example"),
					resource.TestCheckResourceAttrSet(connectionResourceAddress, "id"),
					resource.TestMatchResourceAttr(connectionResourceAddress, "internal_name",
						regexp.MustCompile(`^`+regexp.QuoteMeta(name)+`-[A-Za-z0-9]+$`)),
					resource.TestCheckResourceAttrSet(connectionResourceAddress, "enabled_product_names.#"),
					resource.TestCheckNoResourceAttr(connectionResourceAddress, "client_secret"),
				),
			},
			{
				ResourceName:      connectionResourceAddress,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					"client_secret",
					"client_secret_wo_version",
					"enabled_products",
					"enabled_environments",
				},
			},
			// The step above verifies imported state against the configuration
			// that built it. GenerateConfigStep goes the other way: it writes
			// state back out as configuration and requires the result to plan as
			// a NO-OP.
			//
			// It is deliberately NOT the guard for attribute_map's JSON
			// normalisation, and it cannot be, because the two requirements
			// exclude each other. Generation re-emits the value and Terraform
			// compares what that evaluates to against state. Where the two are
			// byte-identical, as they are for this single-key fixture, nothing
			// reaches jsonComparable. Where they are not, the in-place
			// formatting diff that jsonComparable does not remove — see its doc
			// comment, and it cannot be removed while the update endpoint fails
			// — is what this step's no-op assertion then reports. So the
			// executed guard for the normalisation is
			// TestAttributeMapReindentingIsNotAReplacement, and this step guards
			// the round-trip of every other attribute, which is what caught the
			// ungated Entra group options.
			//
			// attribute_map is in the fixture regardless, so the value is
			// exercised on create, read and import rather than never being sent.
			testhelpers.GenerateConfigStep(connectionResourceAddress),
		},
	})
}

// TestAccResource_AccountSSOConnection_Update covers the change path: the second
// step changes the name, adds a group filter and adds an optional endpoint. It is
// the one test here still gated on the update probe, because it is the only one
// that edits an existing connection — every other test creates, reads, imports,
// queries or destroys, all of which work.
//
// Two of its expectations are pinned to plan_modifiers.go being present, and
// whoever deletes that file has to restore both.
//
// The plan check expects a replacement rather than an in-place update, because
// `name` is one of the attributes plan_modifiers.go compares and any change to it
// forces replacement while that file exists. Expecting an update would fail even
// on the day Jamf fixes the endpoint, which is the opposite of what a gated test
// is for. Restore plancheck.ResourceActionUpdate when plan_modifiers.go goes.
//
// The second step supplies the client secret for the same reason. The behaviour
// worth proving is that omitting it keeps the stored one, but while every change
// is a replacement the step is a create, and a create with no secret is a
// different test with an unattributable failure. Remove `client_secret` from the
// second step, along with the replacement expectation, to recover the original
// assertion.
//
// The changed name is `name` plus a suffix of letters, not a hyphenated one:
// Jamf accepts only letters and digits in a connection name and the provider's
// own validator refuses anything else at plan time, so a hyphen here would fail
// before the update was ever attempted. See acceptanceConnectionName.
func TestAccResource_AccountSSOConnection_Update(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := verifiedDomain(t)
	skipUnlessConnectionUpdatesWork(t, domain)
	name := acceptanceConnectionName("update")
	changedName := name + "Changed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSOConnectionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcConnectionConfig(name, domain),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_connection" "test" {
						name            = %q
						connection_type = "generic_oidc"
						hosting_region  = "US"

						client_id     = "tfAccClient"
						client_secret = "tfAccClientSecret"
						scopes        = "openid email profile groups"

						send_nonce               = true
						sync_attributes_at_login = false
						omit_login_hint          = true
						pkce                     = "s256"

						session_duration_minutes   = 480
						inactivity_timeout_minutes = 30

						group_name_filter = {
							operator = "and"
							groups   = ["tf-acc-one", "tf-acc-two"]
						}

						domains = [%q]

						generic_oidc = {
							issuer_url             = "idp.example"
							authorization_endpoint = "idp.example/authorize"
							token_endpoint         = "idp.example/token"
							jwks_uri               = "idp.example/keys"
							user_info_endpoint     = "idp.example/userinfo"
						}
					}
				`, changedName, domain),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(connectionResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(connectionResourceAddress, "name", changedName),
					resource.TestCheckResourceAttr(connectionResourceAddress, "send_nonce", "true"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "sync_attributes_at_login", "false"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "omit_login_hint", "true"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "pkce", "s256"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "session_duration_minutes", "480"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "inactivity_timeout_minutes", "30"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "group_name_filter.operator", "and"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "group_name_filter.groups.#", "2"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "generic_oidc.user_info_endpoint", "idp.example/userinfo"),
				),
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_EmptyGroupFilterRoundTrips pins the
// distinction the group filter block exists to preserve, on the only surface that
// can actually prove it: an operator with no groups is a real value and is
// different from the filter being absent. Every connection read carried the
// former, so getting this wrong would rewrite live configuration.
func TestAccResource_AccountSSOConnection_EmptyGroupFilterRoundTrips(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := verifiedDomain(t)
	name := acceptanceConnectionName("filter")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSOConnectionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_connection" "test" {
						name            = %q
						connection_type = "generic_oidc"
						hosting_region  = "US"

						client_id     = "tfAccClient"
						client_secret = "tfAccClientSecret"
						scopes        = "openid email profile"

						group_name_filter = {
							operator = "or"
							groups   = []
						}

						domains = [%q]

						generic_oidc = {
							issuer_url             = "idp.example"
							authorization_endpoint = "idp.example/authorize"
							token_endpoint         = "idp.example/token"
							jwks_uri               = "idp.example/keys"
						}
					}
				`, name, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(connectionResourceAddress, "group_name_filter.operator", "or"),
					resource.TestCheckResourceAttr(connectionResourceAddress, "group_name_filter.groups.#", "0"),
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_ImmutableAttributesReplace stands in for
// the two changes an update cannot make. Both are specification-derived: Jamf says
// an update can move a connection to neither another family nor another region,
// and the refusal was never observed because the update path is refused for every
// request.
//
// So this asserts what the provider plans rather than what Jamf answers, which is
// the honest thing to assert either way — a replacement is correct whether Jamf
// would have refused the in-place change or silently ignored it. It is ungated,
// because a replacement is a destroy and a create and both work.
//
// While plan_modifiers.go is present the replacement is over-determined: that
// file replaces the connection on any change at all, so this step would plan a
// replacement even if `connection_type` were not immutable. What makes the test
// meaningful either way is the RequiresReplace plan modifier the attribute itself
// carries in resource.go, which is what this test still exercises once
// plan_modifiers.go is deleted.
func TestAccResource_AccountSSOConnection_ImmutableAttributesReplace(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := verifiedDomain(t)
	name := acceptanceConnectionName("replace")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSOConnectionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcConnectionConfig(name, domain),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_sso_connection" "test" {
						name            = %q
						connection_type = "okta"
						hosting_region  = "US"

						client_id     = "tfAccClient"
						client_secret = "tfAccClientSecret"
						scopes        = "openid email profile"

						domains = [%q]

						okta = {
							domain = "tf-acc.okta.example"
						}
					}
				`, name, domain),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(connectionResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(connectionResourceAddress, "connection_type", "okta"),
					resource.TestCheckResourceAttrSet(connectionResourceAddress, "okta.issuer_url"),
					resource.TestCheckResourceAttrSet(connectionResourceAddress, "okta.token_endpoint"),
				),
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_Disappears covers the drift path: a
// connection removed out from under Terraform has to be dropped from state and
// planned afresh rather than failing the refresh.
//
// It is worth its own test here because the not-found on a single connection is
// deliberately *not* trusted on its own — Jamf is known to list a connection it
// cannot read on its identifier, and the provider keeps such a connection in
// state. So this exercises the other branch of that decision, where the
// collection agrees the connection is gone.
func TestAccResource_AccountSSOConnection_Disappears(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := verifiedDomain(t)
	name := acceptanceConnectionName("gone")

	var connectionID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSOConnectionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcConnectionConfig(name, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					captureAttribute(connectionResourceAddress, "id", &connectionID),
				),
			},
			{
				PreConfig: func() {
					c := account.New(testhelpers.NewAcceptanceClient(t))
					if err := c.DeleteConnection(context.Background(), connectionID); err != nil {
						t.Fatalf("drift preconfig: removing SSO connection %s: %v", connectionID, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_Query covers the list resource against a
// connection this test made, which is the only way to assert on a known entry:
// the organization's own connections are live configuration and their names are
// not this suite's to depend on.
func TestAccResource_AccountSSOConnection_Query(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	domain := verifiedDomain(t)
	name := acceptanceConnectionName("query")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSOConnectionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: oidcConnectionConfig(name, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(connectionResourceAddress, "id"),
				),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_account_sso_connection" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_account_sso_connection.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("connection_type"), KnownValue: knownvalue.StringExact("generic_oidc")},
							{Path: tfjsonpath.New("hosting_region"), KnownValue: knownvalue.StringExact(account.RegionUs)},
						},
					),
				},
			},
		},
	})
}

// --- plan-time refusals, which need no write and so run whatever Jamf is doing ---

// TestAccResource_AccountSSOConnection_MismatchedSettingsBlockRefusedAtPlan is
// the most valuable of the plan-time tests, because it is the mistake Jamf
// handles worst: a settings block disagreeing with the declared family is
// accepted and then fails with an internal failure naming nothing, which is
// indistinguishable from any other failure.
func TestAccResource_AccountSSOConnection_MismatchedSettingsBlockRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccMismatch"
						connection_type = "generic_oidc"
						hosting_region  = "US"
						client_id       = "tfAccClient"
						scopes          = "openid"
						domains         = ["tf-acc.example"]

						entra = {
							domain        = "contoso.example"
							tenant_domain = "contoso.example"
						}
					}
				`,
				ExpectError: regexpBlockDoesNotMatch,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_MissingSettingsBlockRefusedAtPlan is the
// other half of the same rule.
func TestAccResource_AccountSSOConnection_MissingSettingsBlockRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccMissingBlock"
						connection_type = "okta"
						hosting_region  = "US"
						client_id       = "tfAccClient"
						scopes          = "openid"
						domains         = ["tf-acc.example"]
					}
				`,
				ExpectError: regexpBlockRequired,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_ScopesRefusedForEntraAtPlan drives the
// rule that came from the survey rather than from documentation: no Entra
// connection read carried any scopes, and the settings Jamf accepts for one have
// nowhere to put them.
func TestAccResource_AccountSSOConnection_ScopesRefusedForEntraAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccEntraScopes"
						connection_type = "entra"
						hosting_region  = "US"
						client_id       = "tfAccClient"
						scopes          = "openid"
						domains         = ["tf-acc.example"]

						entra = {
							domain        = "contoso.example"
							tenant_domain = "contoso.example"
						}
					}
				`,
				ExpectError: regexpScopesRefused,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_SecretRefusedWithSignedAssertionAtPlan
// drives the one rule Jamf's own documentation is explicit about.
func TestAccResource_AccountSSOConnection_SecretRefusedWithSignedAssertionAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccJwtSecret"
						connection_type = "generic_oidc"
						hosting_region  = "US"
						auth_method     = "private_key_jwt"
						client_id       = "tfAccClient"
						client_secret   = "tfAccClientSecret"
						scopes          = "openid"
						domains         = ["tf-acc.example"]

						generic_oidc = {
							issuer_url             = "idp.example"
							authorization_endpoint = "idp.example/authorize"
							token_endpoint         = "idp.example/token"
							jwks_uri               = "idp.example/keys"
						}
					}
				`,
				ExpectError: regexpSecretRefused,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_MixedCaseDomainRefusedAtPlan pins the
// normalisation guard on the domain names. Jamf holds a claimed domain in lower
// case, so a mixed-case entry names a domain the organization does not hold under
// that spelling.
func TestAccResource_AccountSSOConnection_MixedCaseDomainRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccMixedCase"
						connection_type = "generic_oidc"
						hosting_region  = "US"
						client_id       = "tfAccClient"
						scopes          = "openid"
						domains         = ["TF-Acc.Example"]

						generic_oidc = {
							issuer_url             = "idp.example"
							authorization_endpoint = "idp.example/authorize"
							token_endpoint         = "idp.example/token"
							jwks_uri               = "idp.example/keys"
						}
					}
				`,
				ExpectError: regexpMixedCaseDomain,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_EmptyDomainsRefusedAtPlan pins the one
// collection constraint Jamf does report by name, caught at plan time so the
// operator does not have to read a refusal to learn it.
func TestAccResource_AccountSSOConnection_EmptyDomainsRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccNoDomains"
						connection_type = "generic_oidc"
						hosting_region  = "US"
						client_id       = "tfAccClient"
						scopes          = "openid"
						domains         = []

						generic_oidc = {
							issuer_url             = "idp.example"
							authorization_endpoint = "idp.example/authorize"
							token_endpoint         = "idp.example/token"
							jwks_uri               = "idp.example/keys"
						}
					}
				`,
				ExpectError: regexpAtLeastOneDomain,
			},
		},
	})
}

// TestAccResource_AccountSSOConnection_UnknownRegionRefusedAtPlan pins the
// vocabulary check. Jamf names the offending value in its own refusal but never
// the field it was on, so catching it at plan time is what turns an unattributed
// failure into something a practitioner can act on.
func TestAccResource_AccountSSOConnection_UnknownRegionRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_sso_connection" "test" {
						name            = "tfAccBadRegion"
						connection_type = "generic_oidc"
						hosting_region  = "MARS"
						client_id       = "tfAccClient"
						scopes          = "openid"
						domains         = ["tf-acc.example"]

						generic_oidc = {
							issuer_url             = "idp.example"
							authorization_endpoint = "idp.example/authorize"
							token_endpoint         = "idp.example/token"
							jwks_uri               = "idp.example/keys"
						}
					}
				`,
				ExpectError: regexpInvalidValue,
			},
		},
	})
}

// captureAttribute records an applied attribute so a later step can act on it.
// PreConfig runs without access to state, so a value has to be carried out of the
// apply step this way.
func captureAttribute(address, name string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		value, ok := rs.Primary.Attributes[name]
		if !ok {
			return fmt.Errorf("resource %s has no %s attribute", address, name)
		}
		*into = value
		return nil
	}
}

// Expected-error patterns for the plan-time refusals. Terraform wraps diagnostic
// text at roughly 80 columns, so each pattern matches a short phrase that cannot
// be split across a line break.
var (
	regexpBlockDoesNotMatch = regexp.MustCompile(`does not match this connection type`)
	regexpBlockRequired     = regexp.MustCompile(`required for this connection type`)
	regexpScopesRefused     = regexp.MustCompile(`not accepted for an Entra`)
	regexpSecretRefused     = regexp.MustCompile(`no shared secret`)
	regexpMixedCaseDomain   = regexp.MustCompile(`must be lower case`)
	regexpAtLeastOneDomain  = regexp.MustCompile(`at least 1 element`)
	regexpInvalidValue      = regexp.MustCompile(`Invalid Attribute Value Match`)
)
