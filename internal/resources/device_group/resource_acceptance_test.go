// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package device_group_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// logAttrValue returns a TestCheckFunc that logs the value of a state attribute.
// Use to emit resolved server-derived values (e.g. jamf_pro_id) into the test
// output for cross-checking against Jamf Pro UI / API.
func logAttrValue(t *testing.T, resourceName, attribute string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", resourceName)
		}
		v, ok := rs.Primary.Attributes[attribute]
		if !ok {
			t.Logf("%s.%s: <absent>", resourceName, attribute)
			return nil
		}
		if v == "" {
			t.Logf("%s.%s: <null>", resourceName, attribute)
			return nil
		}
		t.Logf("%s.%s = %q", resourceName, attribute, v)
		return nil
	}
}

// testAccCheckDeviceGroupDestroy verifies that device groups created during the test
// have been destroyed.
func testAccCheckDeviceGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testhelpers.NewAcceptanceClient(t)
		dgClient := devicegroups.New(c)
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_device_group" {
				continue
			}
			deadline := time.Now().Add(60 * time.Second)
			for time.Now().Before(deadline) {
				_, err := dgClient.GetDeviceGroup(ctx, rs.Primary.ID)
				if err != nil {
					if helpers.IsNotFoundError(err) {
						break
					}
					return fmt.Errorf("error checking device group %s: %s", rs.Primary.ID, err)
				}
				time.Sleep(2 * time.Second)
			}
		}
		return nil
	}
}

func TestAccResource_DeviceGroup_StaticComputer(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.SkipUnlessProGroupsReadable(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-static-computer-" + suffix
	nameUpdated := "tf-acc-static-computer-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test_static" {
						name        = %q
						description = "Acceptance test — safe to delete"
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.test_static", "id"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_static", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_static", "group_type", "static"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_static", "device_type", "computer"),
					// jamf_pro_id is resolved via Pro /v2/groups and depends on the test
					// tenant having the Device groups Read permission wired up on the API
					// client. If this assertion fails on a tenant that lacks Jamf Pro
					// entirely or lacks the privilege, see the Pro forbidden warning
					// surfaced during the plan output.
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.test_static", "jamf_pro_id"),
					logAttrValue(t, "jamfplatform_device_group.test_static", "id"),
					logAttrValue(t, "jamfplatform_device_group.test_static", "jamf_pro_id"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test_static" {
						name        = %q
						description = "Updated description"
						group_type  = "static"
						device_type = "computer"
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_static", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_static", "description", "Updated description"),
				),
			},
		},
	})
}

func TestAccResource_DeviceGroup_SmartComputer(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-smart-computer-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test_smart" {
						name        = %q
						description = "Acceptance test — safe to delete"
						group_type  = "smart"
						device_type = "computer"
						criteria = [{
							criteria = "Serial Number"
							operator = "like"
							value    = ""
						}]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.test_smart", "id"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_smart", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_smart", "group_type", "smart"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_smart", "device_type", "computer"),
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.test_smart", "member_count"),
					logAttrValue(t, "jamfplatform_device_group.test_smart", "id"),
					logAttrValue(t, "jamfplatform_device_group.test_smart", "jamf_pro_id"),
				),
			},
		},
	})
}

func TestAccResource_DeviceGroup_SmartMobile(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-smart-mobile-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test_mobile" {
						name        = %q
						description = "Acceptance test — safe to delete"
						group_type  = "smart"
						device_type = "mobile"
						criteria = [{
							criteria = "Serial Number"
							operator = "like"
							value    = ""
						}]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.test_mobile", "id"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_mobile", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_mobile", "device_type", "mobile"),
					logAttrValue(t, "jamfplatform_device_group.test_mobile", "id"),
					logAttrValue(t, "jamfplatform_device_group.test_mobile", "jamf_pro_id"),
				),
			},
		},
	})
}

func TestAccResource_DeviceGroup_ImportState(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-import-test-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test_import" {
						name        = %q
						description = "Acceptance test — safe to delete"
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					logAttrValue(t, "jamfplatform_device_group.test_import", "id"),
					logAttrValue(t, "jamfplatform_device_group.test_import", "jamf_pro_id"),
				),
			},
			{
				ResourceName:      "jamfplatform_device_group.test_import",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccResource_DeviceGroup_ImportStateSmartWithDescription is the acceptance
// half of the issue #372 regression. The description is what the defect dropped,
// and the smart shape is where it was reported, so this imports a group whose
// criteria hydrate on the same path. `ImportStateVerify` compares the imported
// state against the state the create left; the second import step then names the
// description, because a future change that dropped it on both paths at once
// would satisfy the comparison while managing nothing.
//
// `order` is declared rather than left to the element index because the criteria
// flatten reconciles it against prior state on create (an omitted order stays
// null) and adopts the wire value on import (0). Declaring it makes both sides 0,
// which is the attribute's own contract rather than a concession to the test.
func TestAccResource_DeviceGroup_ImportStateSmartWithDescription(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-import-smart-desc-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test_import_smart" {
						name        = %q
						description = "Acceptance test — safe to delete"
						group_type  = "smart"
						device_type = "computer"
						criteria = [{
							order    = 0
							criteria = "Serial Number"
							operator = "is"
							value    = "C02IMPORT0001"
						}]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_device_group.test_import_smart", "description", "Acceptance test — safe to delete"),
					logAttrValue(t, "jamfplatform_device_group.test_import_smart", "id"),
				),
			},
			{
				ResourceName:      "jamfplatform_device_group.test_import_smart",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName: "jamfplatform_device_group.test_import_smart",
				ImportState:  true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected one imported instance, got %d", len(states))
					}
					if got := states[0].Attributes["description"]; got != "Acceptance test — safe to delete" {
						return fmt.Errorf("imported description = %q, want the configured description", got)
					}
					return nil
				},
			},
		},
	})
}

func TestAccDataSource_DeviceGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.SkipUnlessProGroupsReadable(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ds-device-group-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "source" {
						name        = %q
						group_type  = "static"
						device_type = "computer"
					}

					data "jamfplatform_device_group" "test" {
						id = jamfplatform_device_group.source.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_device_group.test", "name", name),
					resource.TestCheckResourceAttrSet("data.jamfplatform_device_group.test", "group_type"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_device_group.test", "device_type"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_device_group.test", "jamf_pro_id"),
					logAttrValue(t, "jamfplatform_device_group.source", "id"),
					logAttrValue(t, "jamfplatform_device_group.source", "jamf_pro_id"),
					logAttrValue(t, "data.jamfplatform_device_group.test", "jamf_pro_id"),
				),
			},
		},
	})
}

func TestAccDataSource_DeviceGroups(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_device_groups" "all" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.jamfplatform_device_groups.all", "device_groups.#"),
				),
			},
		},
	})
}

func TestAccResource_DeviceGroup_DescriptionNullVsEmpty(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-desc-nullempty-" + suffix
	rn := "jamfplatform_device_group.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			// Step 1: create with a real description value
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test" {
						name        = %q
						description = "initial value"
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(rn, "id"),
					resource.TestCheckResourceAttr(rn, "description", "initial value"),
					logAttrValue(t, rn, "id"),
					logAttrValue(t, rn, "jamf_pro_id"),
				),
			},
			// Step 2: set description to explicit empty string — must be preserved as ""
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test" {
						name        = %q
						description = ""
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "description", ""),
				),
			},
			// Step 3: set description to explicit null — must become unset in state
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test" {
						name        = %q
						description = null
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(rn, "description"),
				),
			},
			// Step 4: omit description entirely — equivalent to null, plan must be empty
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test" {
						name        = %q
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Step 5: re-add description to verify it can be restored after being unset
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_device_group" "test" {
						name        = %q
						description = "restored"
						group_type  = "static"
						device_type = "computer"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "description", "restored"),
				),
			},
		},
	})
}

// Directory-service group criteria acceptance coverage.
//
// Requires a tenant with a directory service configured and a real resolvable
// group. Stands up the shared Okta LDAP directory-service fixture (via the SDK, so
// the directory exists before the pre-apply group resolve) and resolves
// JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME against it. The equivalent base64 is resolved +
// encoded in-test (same path the provider uses) so the swap value always matches;
// the live apply is the independent check that the encoding is server-acceptable.
// Real group names are never committed — see memory: no real LDAP names in public
// files.

// TestAccResource_DeviceGroup_DSGroupCriteria exercises a directory-service group
// smart-group criterion authored by NAME, asserts the provider stores the name
// (not the base64) in state, and — the crux — that swapping the config to the
// EQUIVALENT raw base64 value produces an EMPTY plan (the ModifyPlan semantic
// equality suppression).
func TestAccResource_DeviceGroup_DSGroupCriteria(t *testing.T) {
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	groupName := testhelpers.RequireLdapGroupName(t)
	testhelpers.AccPreCheck(t)
	// The "Username directory service group" criterion is rejected (400
	// INVALID_FIELD) before Jamf Pro 11.29.
	testhelpers.RequireMinJamfProVersion(t, "11.29.0")

	suffix := testhelpers.RunSuffix()
	name := "tf-acc-dsgroup-" + suffix
	testhelpers.EnsureLdapServerFixture(t, name, ldapEnv)

	// Resolve the group exactly as the provider does, so the step-2 base64 is
	// byte-identical to what the provider resolves the name to.
	groupValue := testhelpers.ResolveDSGroupWireValue(t, groupName)

	const criterion = "Username directory service group" // accepted on the computer surface

	cfg := func(value string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_device_group" "dsgroup" {
				name        = %q
				description = "Acceptance test — safe to delete"
				group_type  = "smart"
				device_type = "computer"
				criteria = [{
					criteria = %q
					operator = "member of"
					value    = %q
				}]
			}
		`, name, criterion, value)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				// Author by NAME. State must round-trip back to the NAME (not base64).
				Config: cfg(groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_device_group.dsgroup", "id"),
					resource.TestCheckResourceAttr("jamfplatform_device_group.dsgroup", "criteria.0.criteria", criterion),
					resource.TestCheckResourceAttr("jamfplatform_device_group.dsgroup", "criteria.0.value", groupName),
				),
			},
			{
				// Swap the config to the equivalent raw base64. Same group → no diff.
				Config: cfg(groupValue),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				// And back to the NAME again — also a no-op.
				Config: cfg(groupName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}
