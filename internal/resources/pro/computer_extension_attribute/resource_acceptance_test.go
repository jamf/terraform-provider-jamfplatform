// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf Pro /v1/computer-extension-attributes
// endpoint. Load-bearing behaviours the acc run verifies (see the build spike):
//   - input-type discriminator validators (FIELD_REQUIRED / INVALID_CONTENT mirror)
//   - full-replace PUT + GET-after-write
//   - SCRIPT→POPUP transition auto-clears the orphaned script (step 4)
//   - empty-echo normalization (no perma-diff on a bare TEXT EA)
//   - script trailing-newline tolerance (import ignores `script`; server appends \n)
//   - manage_existing_data is write-only (import ignores it; never returned)
//   - manage_existing_data is sent only on an update that disables a SCRIPT EA
//     (issue #302 — see TestAccResource_ProComputerExtensionAttribute_ScriptEnabledUpdate)

package computer_extension_attribute_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const ceaResource = "jamfplatform_pro_computer_extension_attribute.test"

func testAccCheckCEADestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_computer_extension_attribute" {
				continue
			}
			_, err := c.GetComputerExtensionAttributeV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking computer EA %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("computer EA %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// Step 1: bare TEXT EA — exercises empty-echo normalization (no companion fields).
func ceaText(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name              = %q
			description       = "probe text"
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = "GENERAL"
		}
	`, name)
}

// Step 2: → SCRIPT (disabled, manage_existing_data) — mutates most attributes.
func ceaScript(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name                 = %q
			description          = "probe script"
			data_type            = "STRING"
			input_type           = "SCRIPT"
			inventory_display    = "HARDWARE"
			enabled              = false
			script               = "#!/bin/bash\necho \"<result>ok</result>\""
			manage_existing_data = "RETAIN"
		}
	`, name)
}

// Step 4: SCRIPT→POPUP transition — server auto-clears the orphaned script.
func ceaPopup(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name               = %q
			data_type          = "STRING"
			input_type         = "POPUP"
			inventory_display  = "GENERAL"
			popup_menu_choices = ["Red", "Green", "Blue"]
		}
	`, name)
}

// Step 6: POPUP→DSAM transition with allow_multiple_values.
//
// A DIRECTORY_SERVICE_ATTRIBUTE_MAPPING extension attribute requires LDAP to be
// configured on the tenant (else Create 400s with "[INVALID_CONTENT] inputType:
// Input type can not be 'DIRECTORY_SERVICE_ATTRIBUTE_MAPPING' if LDAP is not
// configured"). "Configured" here means a directory service exists AND computer
// inventory is set to collect user/location from it — an LDAP server record alone
// is not enough. So the config stands up two ordered fixtures the EA depends_on:
// the shared Okta LDAP server fixture (with full user/group mappings), then the
// computer inventory collection setting that enables directory-service
// user/location collection (which must be applied after the server exists). The
// inventory-settings Delete is state-only (singleton); the LDAP server is removed
// on teardown.
func ceaDSAM(name string, e testhelpers.OktaLdapEnv) string {
	return testhelpers.LdapServerFixture("tf-acc-cea-dsam", e) + fmt.Sprintf(`
		resource "jamfplatform_pro_computer_inventory_collection_settings" "ea_fixture" {
			depends_on                                       = [%[1]s]
			collect_user_and_location_from_directory_service = true
		}

		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			depends_on                  = [jamfplatform_pro_computer_inventory_collection_settings.ea_fixture]
			name                        = %[2]q
			data_type                   = "STRING"
			input_type                  = "DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"
			inventory_display           = "USER_AND_LOCATION"
			directory_service_attribute = "mail"
			allow_multiple_values       = true
		}
	`, testhelpers.LdapFixtureResourceAddr, name)
}

func TestAccResource_ProComputerExtensionAttribute_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	// The final step transitions to a DIRECTORY_SERVICE_ATTRIBUTE_MAPPING EA, which
	// needs a real directory service configured; skip the whole test unless the Okta
	// LDAP fixture env is set.
	e := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-cea-" + suffix
	renamed := "tf-acc-pro-cea-renamed-" + suffix

	// script and manage_existing_data are excluded from ImportStateVerify:
	//   - script: Jamf appends a trailing newline; reconcileScript keeps the
	//     config value during apply, but import has no prior value so it takes
	//     the server's "…\n" verbatim — a benign mismatch.
	//   - manage_existing_data: write-only; never returned by the API.
	importIgnore := []string{"timeouts", "script", "manage_existing_data"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ceaText(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ceaResource, "id"),
					resource.TestCheckResourceAttr(ceaResource, "name", name),
					resource.TestCheckResourceAttr(ceaResource, "input_type", "TEXT"),
					resource.TestCheckResourceAttr(ceaResource, "enabled", "true"),
					// Empty-echo normalization: companion fields stay null.
					resource.TestCheckNoResourceAttr(ceaResource, "script"),
					resource.TestCheckNoResourceAttr(ceaResource, "popup_menu_choices.0"),
					resource.TestCheckNoResourceAttr(ceaResource, "directory_service_attribute"),
				),
			},
			{
				Config: ceaScript(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "name", renamed),
					resource.TestCheckResourceAttr(ceaResource, "input_type", "SCRIPT"),
					resource.TestCheckResourceAttr(ceaResource, "inventory_display", "HARDWARE"),
					resource.TestCheckResourceAttr(ceaResource, "enabled", "false"),
					resource.TestCheckResourceAttrSet(ceaResource, "script"),
				),
			},
			{
				ResourceName:            ceaResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importIgnore,
			},
			{
				Config: ceaPopup(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "input_type", "POPUP"),
					resource.TestCheckResourceAttr(ceaResource, "enabled", "true"),
					resource.TestCheckResourceAttr(ceaResource, "popup_menu_choices.#", "3"),
					// popup_menu_choices is a Set (server sorts) — match by element.
					resource.TestCheckTypeSetElemAttr(ceaResource, "popup_menu_choices.*", "Red"),
					resource.TestCheckTypeSetElemAttr(ceaResource, "popup_menu_choices.*", "Green"),
					resource.TestCheckTypeSetElemAttr(ceaResource, "popup_menu_choices.*", "Blue"),
					// Orphaned script auto-cleared by the server on transition.
					resource.TestCheckNoResourceAttr(ceaResource, "script"),
				),
			},
			{
				Config: ceaDSAM(renamed, e),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "input_type", "DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"),
					resource.TestCheckResourceAttr(ceaResource, "directory_service_attribute", "mail"),
					resource.TestCheckResourceAttr(ceaResource, "allow_multiple_values", "true"),
					resource.TestCheckNoResourceAttr(ceaResource, "popup_menu_choices.0"),
				),
			},
			{
				ResourceName:            ceaResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importIgnore,
			},
		},
	})
}

// ceaSplitOwn renders a TEXT EA that OMITS description, varying only
// inventory_display so the split-ownership test can change an unrelated field.
func ceaSplitOwn(name, inventoryDisplay string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name              = %q
			data_type         = "STRING"
			input_type        = "TEXT"
			inventory_display = %q
		}
	`, name, inventoryDisplay)
}

// TestAccResource_ProComputerExtensionAttribute_SplitOwnership proves the
// omit=preserve contract for the Optional+Computed `description` on this
// full-replace endpoint: with description omitted from HCL, an out-of-band edit
// (simulating the Jamf Pro UI) survives an unrelated Terraform change
// (inventory_display) rather than being wiped — and an explicit "" still clears it.
// (The discriminator-gated companion fields are intentionally NOT Optional+Computed
// — they must drop on omit to clear on an input_type transition — so description is
// the representative co-managed field here.)
func TestAccResource_ProComputerExtensionAttribute_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-cea-split-" + suffix
	const uiDesc = "UI edited description"

	var eaID string

	setDescriptionOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetComputerExtensionAttributeV1(ctx, eaID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		v := uiDesc
		got.Description = &v
		if _, err := c.UpdateComputerExtensionAttributeV1(ctx, eaID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerDescription := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetComputerExtensionAttributeV1(context.Background(), eaID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if helpers.DerefString(got.Description) != want {
				return fmt.Errorf("description = %q, want %q", helpers.DerefString(got.Description), want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCEADestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with description omitted.
				Config: ceaSplitOwn(name, "GENERAL"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ceaResource, "id"),
					func(s *terraform.State) error {
						eaID = s.RootModule().Resources[ceaResource].Primary.ID
						return nil
					},
				),
			},
			{
				// UI sets description out of band; config still omits it and changes
				// only inventory_display. The out-of-band value must survive.
				PreConfig: setDescriptionOutOfBand,
				Config:    ceaSplitOwn(name, "HARDWARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "inventory_display", "HARDWARE"),
					resource.TestCheckResourceAttr(ceaResource, "description", uiDesc),
					checkServerDescription(uiDesc),
				),
			},
			{
				// Explicit "" clears it (full-replace), proving Terraform can take over.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_computer_extension_attribute" "test" {
						name              = %q
						description       = ""
						data_type         = "STRING"
						input_type        = "TEXT"
						inventory_display = "HARDWARE"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "description", ""),
					checkServerDescription(""),
				),
			},
		},
	})
}

// ceaScriptEnabled renders an ENABLED SCRIPT EA with the given script body and
// no manage_existing_data — the shape from issue #302.
func ceaScriptEnabled(name, script string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name              = %q
			description       = "issue 302 regression"
			data_type         = "STRING"
			input_type        = "SCRIPT"
			inventory_display = "GENERAL"
			enabled           = true
			script            = %q
		}
	`, name, script)
}

// TestAccResource_ProComputerExtensionAttribute_ScriptEnabledUpdate is the
// issue #302 regression test: updating an ENABLED SCRIPT EA used to 400 with
// "[INVALID_CONTENT] manageExistingData: This field should be blank if the input
// type is not 'SCRIPT' and enabled value is not false" because the provider sent
// manageExistingData = RETAIN on every SCRIPT update. Jamf Pro accepts the field
// only on an update that lands the EA disabled, and requires it on the
// enabled→disabled transition. This walks the whole enabled/disabled matrix:
//
//	step 1  create enabled            (field must be absent on POST)
//	step 2  edit script, still enabled (field must be absent — the #302 repro)
//	step 3  disable                    (field REQUIRED; provider defaults to RETAIN)
//	step 4  edit script while disabled (field sent, already-disabled no-op)
//	step 5  re-enable + edit script    (field must be absent again)
func TestAccResource_ProComputerExtensionAttribute_ScriptEnabledUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-cea-script-enabled-" + suffix

	const scriptV1 = "#!/bin/sh\necho \"<result>v1</result>\""
	const scriptV2 = "#!/bin/sh\necho \"<result>v2</result>\""
	const scriptV3 = "#!/bin/sh\necho \"<result>v3</result>\""
	const scriptV4 = "#!/bin/sh\necho \"<result>v4</result>\""

	// Disabled steps carry manage_existing_data explicitly (step 3) and omitted
	// (step 4) to cover both the "user supplied" and "provider defaults to
	// RETAIN" paths of the required-on-disable rule.
	disabled := func(script, medLine string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_computer_extension_attribute" "test" {
				name              = %q
				description       = "issue 302 regression"
				data_type         = "STRING"
				input_type        = "SCRIPT"
				inventory_display = "GENERAL"
				enabled           = false
				script            = %q
				%s
			}
		`, name, script, medLine)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ceaScriptEnabled(name, scriptV1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ceaResource, "id"),
					resource.TestCheckResourceAttr(ceaResource, "enabled", "true"),
					resource.TestCheckResourceAttr(ceaResource, "script", scriptV1),
				),
			},
			{
				// The #302 repro: change only the script, EA stays enabled.
				Config: ceaScriptEnabled(name, scriptV2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "enabled", "true"),
					resource.TestCheckResourceAttr(ceaResource, "script", scriptV2),
				),
			},
			{
				// enabled true→false: Jamf Pro requires manage_existing_data.
				Config: disabled(scriptV3, `manage_existing_data = "RETAIN"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "enabled", "false"),
					resource.TestCheckResourceAttr(ceaResource, "script", scriptV3),
				),
			},
			{
				// Already disabled, manage_existing_data omitted: the provider
				// still sends RETAIN, which Jamf Pro accepts as a no-op.
				Config: disabled(scriptV4, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "enabled", "false"),
					resource.TestCheckResourceAttr(ceaResource, "script", scriptV4),
				),
			},
			{
				// Re-enable: the field must drop off the payload again.
				Config: ceaScriptEnabled(name, scriptV1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "enabled", "true"),
					resource.TestCheckResourceAttr(ceaResource, "script", scriptV1),
				),
			},
		},
	})
}

// ceaPopupNoChoices renders a POPUP EA that OMITS popup_menu_choices, varying
// inventory_display so an unrelated change can be applied.
func ceaPopupNoChoices(name, inventoryDisplay string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name              = %q
			data_type         = "STRING"
			input_type        = "POPUP"
			inventory_display = %q
		}
	`, name, inventoryDisplay)
}

// TestAccResource_ProComputerExtensionAttribute_PopupSplitOwnership proves the
// omit=preserve contract for popup_menu_choices (Optional+Computed Set, gated by
// input_type = POPUP): with the choices omitted from HCL, out-of-band choices
// (simulating the Jamf Pro UI) survive an unrelated change (inventory_display),
// and a transition away from POPUP cleanly clears them without a consistency error
// (the input_type-aware plan modifier predicts the cleared result).
func TestAccResource_ProComputerExtensionAttribute_PopupSplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-cea-popup-split-" + suffix
	uiChoices := []string{"Red", "Green", "Blue"}

	var eaID string

	setChoicesOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetComputerExtensionAttributeV1(ctx, eaID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		cs := append([]string(nil), uiChoices...)
		got.PopupMenuChoices = &cs
		if _, err := c.UpdateComputerExtensionAttributeV1(ctx, eaID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerChoices := func(wantLen int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetComputerExtensionAttributeV1(context.Background(), eaID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			n := 0
			if got.PopupMenuChoices != nil {
				n = len(*got.PopupMenuChoices)
			}
			if n != wantLen {
				return fmt.Errorf("server popup_menu_choices len = %d, want %d", n, wantLen)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCEADestroy(t),
		Steps: []resource.TestStep{
			{
				// Create a POPUP EA with no choices declared.
				Config: ceaPopupNoChoices(name, "GENERAL"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ceaResource, "id"),
					func(s *terraform.State) error {
						eaID = s.RootModule().Resources[ceaResource].Primary.ID
						return nil
					},
				),
			},
			{
				// UI adds choices out of band; config still omits them and changes only
				// inventory_display. The out-of-band choices must survive.
				PreConfig: setChoicesOutOfBand,
				Config:    ceaPopupNoChoices(name, "HARDWARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "inventory_display", "HARDWARE"),
					resource.TestCheckResourceAttr(ceaResource, "popup_menu_choices.#", "3"),
					resource.TestCheckTypeSetElemAttr(ceaResource, "popup_menu_choices.*", "Red"),
					checkServerChoices(3),
				),
			},
			{
				// Explicit [] while staying POPUP clears the choices and round-trips as
				// an empty set (no "inconsistent result after apply").
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_computer_extension_attribute" "test" {
						name               = %q
						data_type          = "STRING"
						input_type         = "POPUP"
						inventory_display  = "HARDWARE"
						popup_menu_choices = []
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "popup_menu_choices.#", "0"),
					checkServerChoices(0),
				),
			},
			{
				// Transition POPUP→TEXT: the choices clear with no "inconsistent result
				// after apply" (the plan modifier predicts the cleared SetNull).
				Config: ceaSplitOwn(name, "HARDWARE"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ceaResource, "input_type", "TEXT"),
					resource.TestCheckNoResourceAttr(ceaResource, "popup_menu_choices.0"),
					checkServerChoices(0),
				),
			},
		},
	})
}

// Validator matrix: each ExpectError matches a single no-space token to dodge
// Terraform's ~80-col diagnostic wrap.
func TestAccResource_ProComputerExtensionAttribute_ValidatorErrors(t *testing.T) {
	testhelpers.AccPreCheck(t)

	cases := []struct {
		name   string
		config string
		expect *regexp.Regexp
	}{
		{
			name:   "bad data_type",
			config: ceaBad(`data_type = "BOOLEAN"`, `input_type = "TEXT"`, `inventory_display = "GENERAL"`),
			expect: regexp.MustCompile(`must be one of`),
		},
		{
			name:   "script required when SCRIPT",
			config: ceaBad(`data_type = "STRING"`, `input_type = "SCRIPT"`, `inventory_display = "GENERAL"`),
			expect: regexp.MustCompile(`required`),
		},
		{
			name:   "dsa required when DSAM",
			config: ceaBad(`data_type = "STRING"`, `input_type = "DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"`, `inventory_display = "USER_AND_LOCATION"`),
			expect: regexp.MustCompile(`required`),
		},
		{
			name:   "script forbidden on TEXT",
			config: ceaBad(`data_type = "STRING"`, `input_type = "TEXT"`, `inventory_display = "GENERAL"`, `script = "echo hi"`),
			expect: regexp.MustCompile(`SCRIPT`),
		},
		{
			name:   "popup forbidden on TEXT",
			config: ceaBad(`data_type = "STRING"`, `input_type = "TEXT"`, `inventory_display = "GENERAL"`, `popup_menu_choices = ["a"]`),
			expect: regexp.MustCompile(`POPUP`),
		},
		{
			name:   "enabled false forbidden on TEXT",
			config: ceaBad(`data_type = "STRING"`, `input_type = "TEXT"`, `inventory_display = "GENERAL"`, `enabled = false`),
			expect: regexp.MustCompile(`SCRIPT`),
		},
		{
			// Issue #302: Jamf Pro only accepts manage_existing_data on an update
			// that disables the EA, so an enabled SCRIPT EA must not declare it.
			name: "manage_existing_data forbidden while enabled",
			config: ceaBad(`data_type = "STRING"`, `input_type = "SCRIPT"`, `inventory_display = "GENERAL"`,
				`script = "echo hi"`, `enabled = true`, `manage_existing_data = "RETAIN"`),
			expect: regexp.MustCompile(`manage_existing_data`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: tc.config, ExpectError: tc.expect},
				},
			})
		})
	}
}

// ceaBad assembles a resource block from arbitrary attribute lines, for the
// negative validator cases.
func ceaBad(lines ...string) string {
	var body strings.Builder
	for _, l := range lines {
		body.WriteString("\t\t\t" + l + "\n")
	}
	return fmt.Sprintf(`
		resource "jamfplatform_pro_computer_extension_attribute" "test" {
			name = "tf-acc-cea-bad"
%s		}
	`, body.String())
}

func TestAccDataSource_ProComputerExtensionAttribute_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-cea-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_computer_extension_attribute" "test" {
						name               = %q
						data_type          = "STRING"
						input_type         = "POPUP"
						inventory_display  = "GENERAL"
						popup_menu_choices = ["a", "b"]
					}

					data "jamfplatform_pro_computer_extension_attribute" "by_id" {
						id = jamfplatform_pro_computer_extension_attribute.test.id
					}

					data "jamfplatform_pro_computer_extension_attribute" "by_name" {
						name = jamfplatform_pro_computer_extension_attribute.test.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_computer_extension_attribute.by_id", "name", ceaResource, "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_computer_extension_attribute.by_id", "input_type", "POPUP"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_computer_extension_attribute.by_name", "id", ceaResource, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_computer_extension_attribute.by_name", "popup_menu_choices.#", "2"),
				),
			},
		},
	})
}

// List endpoint returns full objects; include_resource hydrates every attribute.
func TestAccListResource_ProComputerExtensionAttribute_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-cea-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCEADestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ceaText(name),
				Check:  resource.TestCheckResourceAttrSet(ceaResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_computer_extension_attribute" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_computer_extension_attribute.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_computer_extension_attribute.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("input_type"), KnownValue: knownvalue.StringExact("TEXT")},
						},
					),
				},
			},
		},
	})
}
