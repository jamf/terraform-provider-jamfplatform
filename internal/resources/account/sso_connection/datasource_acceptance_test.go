// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package sso_connection_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The read path is the half of this construct that works. Jamf refuses every
// connection write, so nothing in this file creates anything: each test reads a
// connection the organization already holds.
//
// That makes the fixture the organization's own live configuration, which
// constrains what may be asserted. Nothing here assumes a count, a name, a
// provider family or a region — those are the maintainer's estate and not this
// suite's to depend on. What is asserted is that a read returns a coherent
// connection, that the two lookup keys agree, and that the collection and the
// single read line up where they overlap.

// singularDataSourceAddress and pluralDataSourceAddress are the addresses these
// tests use.
const (
	singularDataSourceAddress = "data.jamfplatform_account_sso_connection.test"
	pluralDataSourceAddress   = "data.jamfplatform_account_sso_connections.test"
)

// readableConnection returns one connection the organization holds that this
// provider can also manage, or skips.
//
// The skip is deliberate rather than defensive. An organization with no
// connections, or one whose only connections are the two kinds that cannot be
// managed, is a legitimate acceptance environment — but it is not one these tests
// can say anything about, and returning early on an empty read would let them
// pass while asserting nothing. So the absence is reported as a skip naming what
// was looked for.
//
// The two unmanageable kinds are excluded here for the same reason the list
// resource excludes them: a connection built with Microsoft admin consent and one
// Jamf lists but cannot read on its identifier both behave differently on purpose,
// and a test that happened to pick one would be testing the refusal rather than
// the read.
func readableConnection(t *testing.T) *account.Connection {
	t.Helper()
	ctx := context.Background()
	c := account.New(testhelpers.NewAcceptanceClient(t))

	summaries, err := c.ListConnections(ctx)
	if err != nil {
		t.Fatalf("listing the organization's SSO connections: %v", err)
	}

	for i := range summaries {
		found, readErr := c.GetConnection(ctx, summaries[i].ID)
		if readErr != nil {
			continue
		}
		if found.ConsentFlow {
			continue
		}
		return found
	}

	t.Skipf(
		"this Jamf Account organization holds no SSO connection that can be read on its own identifier and is "+
			"not a Microsoft admin-consent connection, so there is nothing for these read tests to look at. "+
			"%d connections were listed. This is a legitimate environment rather than a failure — Jamf refuses "+
			"every connection write, so this suite cannot make one to read.",
		len(summaries),
	)
	return nil
}

// TestAccDataSource_AccountSSOConnection_ByIdentifier reads a connection the
// organization holds, and checks the values that are true of every connection
// whatever its family.
func TestAccDataSource_AccountSSOConnection_ByIdentifier(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	connection := readableConnection(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "jamfplatform_account_sso_connection" "test" {
						id = %q
					}
				`, connection.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(singularDataSourceAddress, "id", connection.ID),
					resource.TestCheckResourceAttr(singularDataSourceAddress, "name", connection.Name),
					resource.TestCheckResourceAttr(singularDataSourceAddress, "consent_flow", "false"),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "connection_type"),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "hosting_region"),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "sync_attributes_at_login"),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "omit_login_hint"),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "domains.#"),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "enabled_product_names.#"),
					checkRenamedConnectionType(singularDataSourceAddress),
					checkOneSettingsBlock(singularDataSourceAddress),
				),
			},
		},
	})
}

// TestAccDataSource_AccountSSOConnection_ByName pins the second lookup key
// against the first: both have to resolve to the same connection, since the name
// is resolved through the collection and the identifier is not.
//
// Note the name matched is the one Jamf stores, which may be a uniquified form of
// whatever the connection was created with — that is why the fixture reads it back
// rather than assuming one.
func TestAccDataSource_AccountSSOConnection_ByName(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	connection := readableConnection(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "jamfplatform_account_sso_connection" "test" {
						name = %q
					}
				`, connection.Name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(singularDataSourceAddress, "name", connection.Name),
					resource.TestCheckResourceAttrSet(singularDataSourceAddress, "id"),
				),
			},
		},
	})
}

// TestAccDataSource_AccountSSOConnection_BothKeysRefused pins the configuration
// validator against the real provider, which is the only place the framework's
// own refusal text can be confirmed.
func TestAccDataSource_AccountSSOConnection_BothKeysRefused(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_sso_connection" "test" {
						id   = "con_tfaccbothkeys001"
						name = "tfAccBothKeys"
					}
				`,
				ExpectError: regexpInvalidCombination,
			},
		},
	})
}

// TestAccDataSource_AccountSSOConnection_NeitherKeyRefused is the other half:
// exactly one key means neither is optional in practice.
func TestAccDataSource_AccountSSOConnection_NeitherKeyRefused(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_sso_connection" "test" {}
				`,
				ExpectError: regexpMissingAttribute,
			},
		},
	})
}

// TestAccDataSource_AccountSSOConnection_UnknownIdentifierIsReported pins the
// not-found path against the real collection, and that it lands on the attribute
// the practitioner supplied.
func TestAccDataSource_AccountSSOConnection_UnknownIdentifierIsReported(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_sso_connection" "test" {
						id = "con_tfaccabsent000001"
					}
				`,
				ExpectError: regexpConnectionNotFound,
			},
		},
	})
}

// TestAccDataSource_AccountSSOConnections_List reads the whole collection and
// checks it against a connection known to be in it.
//
// The count is deliberately not asserted: it is the maintainer's estate. What is
// asserted is that the collection reports the connection the fixture found, which
// is the property that would break if the entry mapping were wrong.
func TestAccDataSource_AccountSSOConnections_List(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	connection := readableConnection(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_sso_connections" "test" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(pluralDataSourceAddress, "id", "sso_connections"),
					resource.TestCheckResourceAttrSet(pluralDataSourceAddress, "sso_connections.#"),
					checkCollectionHolds(pluralDataSourceAddress, connection.ID, connection.Name),
				),
			},
		},
	})
}

// TestAccListResource_AccountSSOConnection reads the list resource, which is the
// one entry point whose extra cost is worth confirming live: it reads each
// connection individually, and the two kinds it leaves out are only
// distinguishable that way.
//
// The connection asserted on is one the fixture already established is neither
// kind, so its presence is the assertion — an omission would mean the filter is
// dropping something it should keep.
func TestAccListResource_AccountSSOConnection(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	connection := readableConnection(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
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
						queryfilter.ByDisplayName(knownvalue.StringExact(connection.Name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("id"), KnownValue: knownvalue.StringExact(connection.ID)},
							{Path: tfjsonpath.New("consent_flow"), KnownValue: knownvalue.Bool(false)},
						},
					),
				},
			},
		},
	})
}

// checkRenamedConnectionType asserts the family is reported in this provider's
// own vocabulary rather than Jamf's.
//
// It is worth asserting live because the rename is the thing most likely to look
// right in a unit test and be wrong against the estate: a family Jamf added since
// this provider was built is carried through under Jamf's spelling, which is
// honest but is a gap, and this is where it would show.
func checkRenamedConnectionType(address string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(address, "connection_type", func(value string) error {
		if slices.Contains([]string{"generic_oidc", "entra", "okta", "google_workspace"}, value) {
			return nil
		}
		return fmt.Errorf(
			"connection_type = %q, which is not one of this provider's own names — Jamf may have added a "+
				"provider family this release does not rename",
			value,
		)
	})
}

// checkOneSettingsBlock asserts exactly one per-family settings block is
// reported, which is the read shape Jamf documents and the one the four-block
// schema depends on.
func checkOneSettingsBlock(address string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("data source %s not found in state", address)
		}

		present := 0
		var names []string
		for _, block := range []string{"generic_oidc", "entra", "okta", "google_workspace"} {
			if rs.Primary.Attributes[block+".%"] != "" {
				present++
				names = append(names, block)
			}
		}
		if present != 1 {
			return fmt.Errorf("%d settings blocks were reported (%v), want exactly the connection's own", present, names)
		}
		return nil
	}
}

// checkCollectionHolds asserts the plural read reports one particular
// connection, matched by identifier, and that its name agrees with the single
// read.
func checkCollectionHolds(address, id, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("data source %s not found in state", address)
		}

		count := rs.Primary.Attributes["sso_connections.#"]
		if count == "" || count == "0" {
			return fmt.Errorf("the collection reported %q connections, but %s is known to be in it", count, id)
		}

		for i := 0; ; i++ {
			key := fmt.Sprintf("sso_connections.%d.id", i)
			value, present := rs.Primary.Attributes[key]
			if !present {
				break
			}
			if value != id {
				continue
			}
			gotName := rs.Primary.Attributes[fmt.Sprintf("sso_connections.%d.name", i)]
			if gotName != name {
				return fmt.Errorf("the collection reports %s as %q, the single read as %q", id, gotName, name)
			}
			return nil
		}
		return fmt.Errorf("the collection does not report %s, which the single read found", id)
	}
}

// Expected-error patterns for the data source's own refusals. Terraform wraps
// diagnostic text at roughly 80 columns, so each pattern matches a short phrase
// that cannot be split across a line break.
var (
	regexpInvalidCombination = regexp.MustCompile(`Invalid Attribute Combination`)
	regexpMissingAttribute   = regexp.MustCompile(`Missing Attribute Configuration`)
	regexpConnectionNotFound = regexp.MustCompile(`Unable to find Jamf Account SSO connection`)
)
