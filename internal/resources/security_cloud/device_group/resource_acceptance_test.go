// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package device_group_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// defaultGroupName is the built-in group Jamf Security Cloud gives every tenant.
// Duplicated from the package under test rather than exported, because a test
// asserting the reserved name against the constant it is compared to would pass by
// construction.
const defaultGroupName = "Default Group"

// testAccCheckDeviceGroupDestroy verifies device groups created during the test
// were destroyed.
//
// The read is what proves it: the delete endpoint answers 204 for a group that
// never existed, so a successful destroy call is no evidence on its own.
func testAccCheckDeviceGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_device_group" {
				continue
			}
			_, err := c.GetDeviceGroupV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Security Cloud device group %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Security Cloud device group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_SecurityCloudDeviceGroup_Basic covers create, in-place rename,
// and import.
//
// The rename step also pins that a rename is an update rather than a replacement:
// step 1 captures the ID and step 2 asserts it survived. Nothing in the schema
// carries RequiresReplace, so a regression there would silently start destroying
// and recreating groups — which, given that deleting a group quietly unassigns
// every app that named it, would be a genuinely destructive change.
func TestAccResource_SecurityCloudDeviceGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-group-" + suffix
	nameUpdated := "tf-acc-jsc-device-group-updated-" + suffix

	var groupID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "test" {
						name = %q
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_device_group.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_device_group.test", "name", name),
					captureDeviceGroupID("jamfplatform_security_cloud_device_group.test", &groupID),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "test" {
						name = %q
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_device_group.test", "name", nameUpdated),
					assertDeviceGroupIDUnchanged("jamfplatform_security_cloud_device_group.test", &groupID),
				),
			},
			{
				ResourceName:      "jamfplatform_security_cloud_device_group.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					// Timeouts are provider-side configuration with no wire
					// representation, so an import cannot recover them. There is
					// nothing else to ignore: every other attribute on this
					// resource is stored and read back verbatim.
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudDeviceGroup_Disappears covers the drift path: a
// group deleted out from under Terraform must be dropped from state and planned
// for re-creation rather than failing the refresh.
func TestAccResource_SecurityCloudDeviceGroup_Disappears(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-group-gone-" + suffix

	config := fmt.Sprintf(`
		resource "jamfplatform_security_cloud_device_group" "test" {
			name = %q
		}
	`, name)

	var groupID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_device_group.test", "id"),
					captureDeviceGroupID("jamfplatform_security_cloud_device_group.test", &groupID),
				),
			},
			{
				PreConfig: func() {
					c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
					if err := c.DeleteDeviceGroupV1(context.Background(), groupID); err != nil {
						t.Fatalf("drift preconfig: deleting device group %s: %v", groupID, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudDeviceGroup_WhitespaceNameRejectedAtPlan pins the
// plan-time refusal for surrounding whitespace. Jamf Security Cloud would accept
// these names and store them trimmed, and Terraform would then fail the apply with
// an inconsistent-result error naming neither the cause nor the fix. The config
// must never reach apply.
func TestAccResource_SecurityCloudDeviceGroup_WhitespaceNameRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "leading" {
						name = %q
					}
				`, " tf-acc-jsc-ws-"+suffix),
				ExpectError: regexpSurroundingWhitespace,
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "trailing" {
						name = %q
					}
				`, "tf-acc-jsc-ws-"+suffix+" "),
				ExpectError: regexpSurroundingWhitespace,
			},
		},
	})
}

// TestAccResource_SecurityCloudDeviceGroup_ReservedNameRejectedAtPlan pins the
// plan-time refusal for the built-in group's name, in both the exact and the
// lowercased form. Jamf Security Cloud's own check is case-insensitive, so a
// validator that only caught the exact spelling would let the second config
// through to a mid-apply 400.
func TestAccResource_SecurityCloudDeviceGroup_ReservedNameRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "reserved" {
						name = %q
					}
				`, defaultGroupName),
				ExpectError: regexpReservedName,
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_device_group" "reserved_lower" {
						name = "default group"
					}
				`,
				ExpectError: regexpReservedName,
			},
		},
	})
}

// TestAccResource_SecurityCloudDeviceGroup_DuplicateNameRejected exercises the
// apply-time 409. Uniqueness needs a tenant read, so unlike the two rules above
// this one cannot be caught at plan time — the diagnostic is what has to carry
// the explanation, including that the comparison is exact.
func TestAccResource_SecurityCloudDeviceGroup_DuplicateNameRejected(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-group-dup-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "first" {
						name = %q
					}

					resource "jamfplatform_security_cloud_device_group" "second" {
						name       = %q
						depends_on = [jamfplatform_security_cloud_device_group.first]
					}
				`, name, name),
				ExpectError: regexpDuplicateName,
			},
		},
	})
}

// TestAccResource_SecurityCloudDeviceGroup_CaseDifferingNamesCoexist pins a wire
// law that is easy to assume the other way round: Jamf Security Cloud's uniqueness
// check is case-SENSITIVE, even though its reserved-name check is not. Two groups
// differing only in capitalisation apply cleanly. If the server ever tightens this
// to case-insensitive, this test fails and the schema needs a warning rather than
// silently letting users create two groups nobody can tell apart in the UI.
func TestAccResource_SecurityCloudDeviceGroup_CaseDifferingNamesCoexist(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	lower := "tf-acc-jsc-case-" + suffix
	upper := "TF-ACC-JSC-CASE-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "lower" {
						name = %q
					}

					resource "jamfplatform_security_cloud_device_group" "upper" {
						name       = %q
						depends_on = [jamfplatform_security_cloud_device_group.lower]
					}
				`, lower, upper),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_device_group.lower", "name", lower),
					resource.TestCheckResourceAttr("jamfplatform_security_cloud_device_group.upper", "name", upper),
				),
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDeviceGroup_ByIDAndName covers both selectors on
// the singular data source.
func TestAccDataSource_SecurityCloudDeviceGroup_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-group-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "src" {
						name = %q
					}

					data "jamfplatform_security_cloud_device_group" "by_id" {
						id = jamfplatform_security_cloud_device_group.src.id
					}

					data "jamfplatform_security_cloud_device_group" "by_name" {
						name       = jamfplatform_security_cloud_device_group.src.name
						depends_on = [jamfplatform_security_cloud_device_group.src]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_device_group.by_id", "name", "jamfplatform_security_cloud_device_group.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_device_group.by_id", "name", name),
					resource.TestCheckResourceAttrPair("data.jamfplatform_security_cloud_device_group.by_name", "id", "jamfplatform_security_cloud_device_group.src", "id"),
				),
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDeviceGroup_NameLookupIsCaseSensitive pins that
// the by-name lookup does not quietly fall back to a case-insensitive match. Two
// groups may legitimately differ only in capitalisation, so a loose match here
// would return whichever the server happened to list first.
func TestAccDataSource_SecurityCloudDeviceGroup_NameLookupIsCaseSensitive(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-group-case-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "src" {
						name = %q
					}
				`, name),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "src" {
						name = %q
					}

					data "jamfplatform_security_cloud_device_group" "wrong_case" {
						name       = %q
						depends_on = [jamfplatform_security_cloud_device_group.src]
					}
				`, name, "TF-ACC-JSC-DEVICE-GROUP-CASE-DS-"+suffix),
				ExpectError: regexpGroupNotFound,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDeviceGroup_RequiresExactlyOneSelector pins the
// config validator on the singular data source, from both directions.
func TestAccDataSource_SecurityCloudDeviceGroup_RequiresExactlyOneSelector(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_security_cloud_device_group" "neither" {}
				`,
				ExpectError: regexpExactlyOneSelector,
			},
			{
				Config: `
					data "jamfplatform_security_cloud_device_group" "both" {
						id   = "57497e81-d499-4f99-8fe8-8f262d0f5b8f"
						name = "Some Group"
					}
				`,
				ExpectError: regexpExactlyOneSelector,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDeviceGroup_BuiltInGroupRefused pins the one
// result the singular data source has to reject. The built-in group resolves by
// name — it is in the list — but Jamf Security Cloud gives it no identifier, so
// handing it back would produce a config that plans and then fails against the API
// with an empty group ID.
func TestAccDataSource_SecurityCloudDeviceGroup_BuiltInGroupRefused(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "jamfplatform_security_cloud_device_group" "built_in" {
						name = %q
					}
				`, defaultGroupName),
				ExpectError: regexpNoIDForBuiltIn,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudDeviceGroups_ListsCreatedGroupAndBuiltIn checks
// the plural data source surfaces a group created in the same apply, and that it
// reports the built-in group rather than filtering it out — the decision that makes
// this data source disagree with the list resource on purpose.
//
// It asserts the created group is present rather than asserting a total, because
// the tenant may hold groups this test did not create. The built-in group's
// presence is asserted by name, which is the only handle it has.
func TestAccDataSource_SecurityCloudDeviceGroups_ListsCreatedGroupAndBuiltIn(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-groups-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "src" {
						name = %q
					}

					data "jamfplatform_security_cloud_device_groups" "all" {
						depends_on = [jamfplatform_security_cloud_device_group.src]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_security_cloud_device_groups.all", "id", "device_groups"),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_security_cloud_device_groups.all", "device_groups.*", map[string]string{
						"name":     name,
						"built_in": "false",
					}),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_security_cloud_device_groups.all", "device_groups.*", map[string]string{
						"name":     defaultGroupName,
						"built_in": "true",
					}),
					assertBuiltInGroupHasNullID("data.jamfplatform_security_cloud_device_groups.all"),
				),
			},
		},
	})
}

// TestAccListResource_SecurityCloudDeviceGroup_Basic exercises the list resource
// via the `terraform query` workflow. The endpoint takes no filter, so step 2
// asserts the created group appears among the results rather than pinning a total.
//
// The built-in group's exclusion is covered by the manageableGroups unit test
// rather than here: querycheck asserts what a result contains, and there is no
// clean way to assert a display name is absent.
//
// Requires Terraform 1.14+ (list resources).
func TestAccListResource_SecurityCloudDeviceGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-device-group-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "src" {
						name = %q
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_device_group.src", "id"),
				),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_security_cloud_device_group" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_security_cloud_device_group.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
						},
					),
				},
			},
		},
	})
}

// captureDeviceGroupID records the applied group ID so a later step can act on the
// group directly. PreConfig runs without access to state, so the ID has to be
// carried out of the apply step this way.
func captureDeviceGroupID(address string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// assertDeviceGroupIDUnchanged fails if the resource was replaced rather than
// updated in place.
func assertDeviceGroupIDUnchanged(address string, want *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		if *want == "" {
			return fmt.Errorf("no ID was captured from the earlier step, so the comparison would pass vacuously")
		}
		if rs.Primary.ID != *want {
			return fmt.Errorf("device group ID changed from %s to %s — a rename must be an in-place update, not a replacement", *want, rs.Primary.ID)
		}
		return nil
	}
}

// assertBuiltInGroupHasNullID checks the built-in group is reported with no ID.
//
// TestCheckTypeSetElemNestedAttrs cannot express "this attribute is null", so the
// state map is walked directly: a null element attribute is absent from it
// entirely, which is what distinguishes the built-in group from a stored group
// whose ID happened to be empty.
func assertBuiltInGroupHasNullID(address string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("data source %s not found in state", address)
		}

		count, ok := rs.Primary.Attributes["device_groups.#"]
		if !ok {
			return fmt.Errorf("%s reported no device_groups collection", address)
		}

		var total int
		if _, err := fmt.Sscanf(count, "%d", &total); err != nil {
			return fmt.Errorf("%s device_groups.# = %q: %w", address, count, err)
		}

		for i := range total {
			if rs.Primary.Attributes[fmt.Sprintf("device_groups.%d.name", i)] != defaultGroupName {
				continue
			}
			if rs.Primary.Attributes[fmt.Sprintf("device_groups.%d.built_in", i)] != "true" {
				return fmt.Errorf("%s: the built-in group must report built_in = true", address)
			}
			if id, present := rs.Primary.Attributes[fmt.Sprintf("device_groups.%d.id", i)]; present && id != "" {
				return fmt.Errorf("%s: the built-in group must report a null id, got %q", address, id)
			}
			return nil
		}

		return fmt.Errorf("%s did not report the built-in group %q; Jamf Security Cloud returns it on every tenant", address, defaultGroupName)
	}
}

// Expected-error patterns for the plan- and apply-time refusals. Terraform wraps
// diagnostic text at roughly 80 columns, so each pattern matches a short phrase
// that cannot be split across a line break.
var (
	regexpSurroundingWhitespace = regexp.MustCompile(`Group name has surrounding whitespace`)
	regexpReservedName          = regexp.MustCompile(`Group name is reserved`)
	regexpDuplicateName         = regexp.MustCompile(`Device group name already in use`)
	regexpNoIDForBuiltIn        = regexp.MustCompile(`Device group has no ID`)
	regexpGroupNotFound         = regexp.MustCompile(`Unable to find Jamf Security Cloud device`)
	regexpExactlyOneSelector    = regexp.MustCompile(`Exactly one of these attributes must be configured`)
)
