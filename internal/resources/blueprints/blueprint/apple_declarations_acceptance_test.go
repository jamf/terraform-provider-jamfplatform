// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// appleDeclarationsConfig builds a blueprint carrying one apple_declarations component. The caller
// supplies the declaration list body so a step can vary only the declarations.
func appleDeclarationsConfig(scopeSuffix, name, declarations string) string {
	return testBlueprintConfig(smartGroupHCL(scopeSuffix), fmt.Sprintf(`
		resource "jamfplatform_blueprints_blueprint" "test_apple_decl" {
			name          = %q
			description   = "Acceptance test — safe to delete"
			deployed      = false
			device_groups = [jamfplatform_device_group.scope.id]

			component_blocks = [
				{
					name               = "Apple Declarations"
					apple_declarations = %s
				},
			]
		}
	`, name, declarations))
}

// TestAccResource_Blueprint_AppleDeclarations covers create, update and plan stability for the
// component. The second step changes one payload and adds a declaration, so the ordered list is
// exercised rather than just a single entry; the third proves the round-trip settles, which is what
// would break if the platform reformatted a payload or echoed back the derived kind and payloadKey.
func TestAccResource_Blueprint_AppleDeclarations(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-" + suffix

	const oneDeclaration = `[
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.siri.settings"
			payload = jsonencode({
				Enabled              = true
				ForceProfanityFilter = true
			})
		},
	]`

	const twoDeclarations = `[
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.siri.settings"
			payload = jsonencode({
				Enabled                   = true
				ForceProfanityFilter      = true
				AllowUserGeneratedContent = false
			})
		},
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.diskmanagement.settings"
			payload = jsonencode({
				Restrictions = {
					ExternalStorage = "ReadOnly"
					NetworkStorage  = "Allowed"
				}
			})
		},
	]`

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: appleDeclarationsConfig("appledecl", name, oneDeclaration),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.#", "1"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.type",
						"com.apple.configuration.siri.settings"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.channel", "SYSTEM"),
				),
			},
			{
				Config: appleDeclarationsConfig("appledecl", name, twoDeclarations),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.#", "2"),
					// Order is meaningful — $PAYLOAD_n counts positions — so the second entry must
					// come back second rather than wherever a set would have put it.
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.1.type",
						"com.apple.configuration.diskmanagement.settings"),
				),
			},
			{
				Config: appleDeclarationsConfig("appledecl", name, twoDeclarations),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_HTMLEscapedPayload pins that a payload carrying &, <
// or > survives a real apply and then plans empty.
//
// Both Terraform's jsonencode() and Go's encoding/json HTML-escape those three characters, so the
// canonical encoding the provider derives from the wire matches what jsonencode() produced. An
// import relies on that: flattenAppleDeclarations keeps the author's bytes when the two are
// semantically equal, but an import has none to keep, so the canonical encoding is what lands in
// state. Every other fixture in this file is free of those characters.
//
// The declaration type also gives the MANAGEMENT kind its only acceptance coverage. That kind is
// absent from the SDK's DeclarationKindValues() and was wire-verified accepted on 2026-09-10.
func TestAccResource_Blueprint_AppleDeclarations_HTMLEscapedPayload(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-esc-" + suffix

	const escapedDeclaration = `[
		{
			channel = "SYSTEM"
			type    = "com.apple.management.organization-info"
			payload = jsonencode({
				Name = "Example & Co <EMEA>"
				URL  = "https://example.com/ddm?token=x&v=2&scope=a<b"
			})
		},
	]`

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				// The apply is the assertion: a mismatched round trip fails here with
				// "inconsistent result after apply" before any Check function runs.
				Config: appleDeclarationsConfig("appledeclesc", name, escapedDeclaration),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.type",
						"com.apple.management.organization-info"),
				),
			},
			{
				Config: appleDeclarationsConfig("appledeclesc", name, escapedDeclaration),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_SeedOnlyKeys pins that keys Apple publishes only on
// its pre-release branch are accepted. This is the test that fails if the embedded table is ever
// regenerated from Apple's release branch alone: AllowSiriAI and ForceReduceSensitiveContent are
// offered by the Jamf interface today and absent from release, so a release-only table would reject
// a configuration that works.
func TestAccResource_Blueprint_AppleDeclarations_SeedOnlyKeys(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-seed-" + suffix

	const seedKeys = `[
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.siri.settings"
			payload = jsonencode({
				Enabled                     = true
				AllowSiriAI                 = false
				ForceReduceSensitiveContent = true
			})
		},
	]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: appleDeclarationsConfig("appleseed", name, seedKeys),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"jamfplatform_blueprints_blueprint.test_apple_decl", "id"),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_PayloadReferences covers an asset reference resolved
// by position. `$PAYLOAD_1` names the first declaration in the same component, which is why the
// list is ordered and payload_key is derived from the index rather than authored.
func TestAccResource_Blueprint_AppleDeclarations_PayloadReferences(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-ref-" + suffix

	const withReference = `[
		{
			channel = "SYSTEM"
			type    = "com.apple.asset.data"
			payload = jsonencode({
				Reference = {
					DataURL        = "https://cdn.example.com/ddm/sudoers-config.zip"
					ContentType    = "application/zip"
					"Hash-SHA-256" = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
				}
				Authentication = { Type = "MDM" }
			})
		},
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.services.configuration-files"
			payload = jsonencode({
				ServiceType        = "com.apple.sudo"
				DataAssetReference = "$PAYLOAD_1"
			})
		},
	]`

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: appleDeclarationsConfig("appleref", name, withReference),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.#", "2"),
					// The asset must stay first, or the reference below it points elsewhere.
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.type",
						"com.apple.asset.data"),
				),
			},
			{
				Config: appleDeclarationsConfig("appleref", name, withReference),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_SchemaValidation is the plan-time rejection matrix.
// Each step corresponds to a rule established by probing the live service and reading the result
// out of the Jamf Pro editor — the platform accepts every one of these bodies and reports success,
// so a plan error is the only thing standing between the author and a declaration that silently
// never applies.
//
// The expected patterns are kept short on purpose: Terraform wraps diagnostic text at roughly 80
// columns, so a longer pattern can straddle a line break and never match.
func TestAccResource_Blueprint_AppleDeclarations_SchemaValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-schema-" + suffix

	declaration := func(declarationType, payload string) string {
		return fmt.Sprintf(`[
			{
				channel = "SYSTEM"
				type    = %q
				payload = %s
			},
		]`, declarationType, payload)
	}
	config := func(declarationType, payload string) string {
		return appleDeclarationsConfig("appleschema", name, declaration(declarationType, payload))
	}

	const siri = "com.apple.configuration.siri.settings"
	const passcode = "com.apple.configuration.passcode.settings"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				// A key Apple does not declare. The platform discards it silently.
				Config:      config(siri, `jsonencode({ ZzNotAKey = true })`),
				ExpectError: regexp.MustCompile(`does not declare`),
				PlanOnly:    true,
			},
			{
				// Wrong case. Unlike a configuration profile payload, where Jamf restores Apple's
				// spelling, a declaration key is discarded outright.
				Config:      config(siri, `jsonencode({ forceprofanityfilter = true })`),
				ExpectError: regexp.MustCompile(`not spelled the way`),
				PlanOnly:    true,
			},
			{
				// A wrong-cased declaration type renders no card at all in the editor.
				Config:      config("com.apple.configuration.Siri.Settings", `jsonencode({ Enabled = true })`),
				ExpectError: regexp.MustCompile(`Unknown Apple declaration type`),
				PlanOnly:    true,
			},
			{
				Config:      config("com.apple.configuration.not.a.thing", `jsonencode({ Enabled = true })`),
				ExpectError: regexp.MustCompile(`Unknown Apple declaration type`),
				PlanOnly:    true,
			},
			{
				// A string where Apple declares a boolean. The key is recognised, the value dropped.
				Config:      config(siri, `jsonencode({ Enabled = "yes" })`),
				ExpectError: regexp.MustCompile(`declares this as a boolean`),
				PlanOnly:    true,
			},
			{
				// Outside a declared rangelist. The editor leaves the field unpopulated.
				Config: config("com.apple.configuration.diskmanagement.settings",
					`jsonencode({ Restrictions = { ExternalStorage = "Fortnightly" } })`),
				ExpectError: regexp.MustCompile(`not one of the values`),
				PlanOnly:    true,
			},
			{
				// Outside a declared numeric range. Jamf stores this unchanged — the device is what
				// rejects it — which is why it is an error here rather than a note.
				Config:      config(passcode, `jsonencode({ MinimumLength = 999 })`),
				ExpectError: regexp.MustCompile(`outside the range`),
				PlanOnly:    true,
			},
			{
				// A required key omitted, reported on absence rather than on anything present.
				Config: config("com.apple.configuration.math.settings",
					`jsonencode({ Calculator = { InputModes = { UnitConversion = true } } })`),
				ExpectError: regexp.MustCompile(`required`),
				PlanOnly:    true,
			},
			{
				// A status subscription naming an item Apple does not publish. Apple types these as
				// plain strings, so nothing else can catch the typo.
				Config: config("com.apple.configuration.management.status-subscriptions",
					`jsonencode({ StatusItems = [{ Name = "device.identifier.not-a-thing" }] })`),
				ExpectError: regexp.MustCompile(`Unknown status item`),
				PlanOnly:    true,
			},
			{
				// Not JSON at all.
				Config:      config(siri, `"not json"`),
				ExpectError: regexp.MustCompile(`not a JSON object`),
				PlanOnly:    true,
			},
			{
				// A clean declaration plans and applies, so the matrix above cannot be satisfied by
				// a validator that simply rejects everything. A null is tolerated: the platform
				// discards one rather than storing it, so it is never a type error.
				Config: config(siri, `jsonencode({ Enabled = true, ForceProfanityFilter = true, AllowWhileLocked = null })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"jamfplatform_blueprints_blueprint.test_apple_decl", "id"),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_CustomDeclarations_SchemaValidation pins that the same checks apply to
// the older component. It delivers identical declarations to the same devices with the same absence
// of server-side validation, so leaving it unchecked would only move the silent failure.
//
// The diagnostic addresses the element, `declaration[n].payload`, as it does for apple_declarations:
// custom_declarations backs its declarations with a list since issue #568.
func TestAccResource_Blueprint_CustomDeclarations_SchemaValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-custom-decl-schema-" + suffix

	config := func(payload string) string {
		return testBlueprintConfig(smartGroupHCL("customschema"), fmt.Sprintf(`
			resource "jamfplatform_blueprints_blueprint" "test_custom_schema" {
				name          = %q
				description   = "Acceptance test — safe to delete"
				deployed      = false
				device_groups = [jamfplatform_device_group.scope.id]

				component_blocks = [
					{
						name = "Custom Declarations"
						custom_declarations = {
							declaration = [{
								channel = "SYSTEM"
								kind    = "CONFIGURATION"
								type    = "com.apple.configuration.siri.settings"
								payload = %s
							}]
						}
					},
				]
			}
		`, name, payload))
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config(`jsonencode({ ZzNotAKey = true })`),
				ExpectError: regexp.MustCompile(`does not declare`),
				PlanOnly:    true,
			},
			{
				// The finding is attributed through its path, to the payload line of the declaration it
				// came from, so the message carries no declaration name.
				Config:      config(`jsonencode({ Enabled = "yes" })`),
				ExpectError: regexp.MustCompile(`(?s)payload = jsonencode\(\{ Enabled = "yes" \}\).*Apple declares this as a\s+boolean`),
				PlanOnly:    true,
			},
			{
				Config: config(`jsonencode({ Enabled = true })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"jamfplatform_blueprints_blueprint.test_custom_schema", "id"),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_RawComponentSkipsSchemaValidation pins the escape hatch, which is what
// makes erroring on an unrecognised name defensible: there is no switch to soften a finding, so
// there has to be somewhere to put a declaration the tables are wrong about. The payload below
// would fail every check in the matrix above.
//
// It also covers the nesting idiom, which is easy to get wrong: raw_component's configuration is a
// map of strings, and a value is nested by JSON-encoding it. That round-trips byte-identically,
// which the second step proves.
func TestAccResource_Blueprint_RawComponentSkipsSchemaValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-raw-decl-" + suffix

	config := testBlueprintConfig(smartGroupHCL("rawdecl"), fmt.Sprintf(`
		resource "jamfplatform_blueprints_blueprint" "test_raw_decl" {
			name          = %q
			description   = "Acceptance test — safe to delete"
			deployed      = false
			device_groups = [jamfplatform_device_group.scope.id]

			component_blocks = [
				{
					name = "Unchecked"
					raw_component = [
						{
							identifier = "com.jamf.ddm-strict"
							configuration = {
								declarations = jsonencode([
									{
										channelType = "SYSTEM"
										kind        = "CONFIGURATION"
										type        = "com.apple.configuration.siri.settings"
										payload = {
											ZzNotAKey = true
											Enabled   = "yes"
										}
									},
								])
							}
						},
					]
				},
			]
		}
	`, name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"jamfplatform_blueprints_blueprint.test_raw_decl", "id"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_FilePayload covers a payload read from a .json file
// with file(), which is how one exported from a tool such as DDM Explorer arrives.
//
// The file's own formatting is the assertion: it is indented with unsorted keys, while the platform
// re-serialises what it stored compact with keys sorted. Without flattenAppleDeclarations keeping
// the author's bytes, the first apply fails as inconsistent. The second step proves the round trip
// settles; the third proves an edit to the file is still seen, so formatting is preserved without
// content being ignored.
func TestAccResource_Blueprint_AppleDeclarations_FilePayload(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-file-" + suffix

	// Deliberately indented, with keys out of alphabetical order and a trailing newline: every
	// difference from the canonical encoding the platform hands back.
	const payloadJSON = `{
  "ForceProfanityFilter": true,
  "Enabled": true,
  "AllowUserGeneratedContent": false
}
`
	const editedPayloadJSON = `{
  "ForceProfanityFilter": false,
  "Enabled": true,
  "AllowUserGeneratedContent": false
}
`

	directory := t.TempDir()
	payloadPath := filepath.Join(directory, "siri.settings.json")
	writePayloadFile := func(t *testing.T, contents string) {
		t.Helper()
		if err := os.WriteFile(payloadPath, []byte(contents), 0o600); err != nil {
			t.Fatalf("writing the payload fixture: %v", err)
		}
	}
	writePayloadFile(t, payloadJSON)

	declaration := fmt.Sprintf(`[
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.siri.settings"
			payload = file(%q)
		},
	]`, payloadPath)

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				// A rewritten payload fails this step with "inconsistent result after apply".
				Config: appleDeclarationsConfig("appledeclfile", name, declaration),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.payload", payloadJSON),
				),
			},
			{
				Config: appleDeclarationsConfig("appledeclfile", name, declaration),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				PreConfig: func() { writePayloadFile(t, editedPayloadJSON) },
				Config:    appleDeclarationsConfig("appledeclfile", name, declaration),
				Check: resource.TestCheckResourceAttr(resourceName,
					"component_blocks.0.apple_declarations.0.payload", editedPayloadJSON),
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_NullPayloadKey covers a payload carrying an explicit
// null, which the schema check treats as absent and therefore lets through to the wire.
//
// A declaration payload's null key is stored and echoed back verbatim, unlike a legacy
// configuration profile payload's, which the service discards. Comparing an authored payload
// against the server's with nulls pruned from one side only rejects the author's bytes, state takes
// the canonical encoding instead, and because payload is Required the apply fails as an
// inconsistent result. The file is indented with unsorted keys, so only the reconciliation can keep
// it. The second step proves the round trip settles rather than diffing on the null for ever.
func TestAccResource_Blueprint_AppleDeclarations_NullPayloadKey(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-null-" + suffix

	const payloadJSON = `{
  "ForceProfanityFilter": null,
  "Enabled": true
}
`

	directory := t.TempDir()
	payloadPath := filepath.Join(directory, "siri.settings.json")
	if err := os.WriteFile(payloadPath, []byte(payloadJSON), 0o600); err != nil {
		t.Fatalf("writing the payload fixture: %v", err)
	}

	declaration := fmt.Sprintf(`[
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.siri.settings"
			payload = file(%q)
		},
	]`, payloadPath)

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				// A pruned comparison fails this step with "inconsistent result after apply".
				Config: appleDeclarationsConfig("appledeclnull", name, declaration),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.payload", payloadJSON),
				),
			},
			{
				Config: appleDeclarationsConfig("appledeclnull", name, declaration),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// Declaration fixtures for the ordering test. Each payload is distinct, so an alignment that picks
// the wrong element shows up as a wrong payload rather than only a wrong type.
const (
	siriDeclaration = `{
			channel = "SYSTEM"
			type    = "com.apple.configuration.siri.settings"
			payload = jsonencode({ Enabled = true })
		}`
	passcodeDeclaration = `{
			channel = "SYSTEM"
			type    = "com.apple.configuration.passcode.settings"
			payload = jsonencode({ MinimumLength = 8 })
		}`
	diskDeclaration = `{
			channel = "SYSTEM"
			type    = "com.apple.configuration.diskmanagement.settings"
			payload = jsonencode({ Restrictions = { ExternalStorage = "ReadOnly" } })
		}`
)

// declarationList renders declaration fixtures as an HCL list body.
func declarationList(declarations ...string) string {
	return "[\n\t\t" + strings.Join(declarations, ",\n\t\t") + ",\n\t]"
}

// TestAccResource_Blueprint_AppleDeclarations_ListOrdering is the test a set could not pass.
//
// Order is the component's meaning: payloadKey is the 1-based position and a $PAYLOAD_n reference
// resolves against it, so reordering the configuration has to reorder what Jamf stores and what
// comes back into state. Three declarations rather than two, because a two-element reorder is also
// a swap and a wrong implementation can survive that.
//
// The reorder and the removal from the middle are also the two edits that exercise the read path's
// alignment — one moves every position's prior payload to a different declaration, the other
// returns fewer declarations than state holds. Each step asserts the payload as well as the type.
func TestAccResource_Blueprint_AppleDeclarations_ListOrdering(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-order-" + suffix

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"
	attr := func(index int, field string) string {
		return fmt.Sprintf("component_blocks.0.apple_declarations.%d.%s", index, field)
	}
	declarationAt := func(index int, declarationType, payload string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(resourceName, attr(index, "type"), declarationType),
			resource.TestCheckResourceAttr(resourceName, attr(index, "payload"), payload),
		)
	}

	const (
		siriPayload     = `{"Enabled":true}`
		passcodePayload = `{"MinimumLength":8}`
		diskPayload     = `{"Restrictions":{"ExternalStorage":"ReadOnly"}}`

		siriType     = "com.apple.configuration.siri.settings"
		passcodeType = "com.apple.configuration.passcode.settings"
		diskType     = "com.apple.configuration.diskmanagement.settings"
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: appleDeclarationsConfig("appledeclorder", name,
					declarationList(siriDeclaration, passcodeDeclaration, diskDeclaration)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "component_blocks.0.apple_declarations.#", "3"),
					declarationAt(0, siriType, siriPayload),
					declarationAt(1, passcodeType, passcodePayload),
					declarationAt(2, diskType, diskPayload),
				),
			},
			{
				Config: appleDeclarationsConfig("appledeclorder", name,
					declarationList(siriDeclaration, passcodeDeclaration, diskDeclaration)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				// Reordered, not edited. A set would report no change at all here; a list must
				// apply one, and every position's declaration must move with it.
				Config: appleDeclarationsConfig("appledeclorder", name,
					declarationList(diskDeclaration, siriDeclaration, passcodeDeclaration)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectNonEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "component_blocks.0.apple_declarations.#", "3"),
					declarationAt(0, diskType, diskPayload),
					declarationAt(1, siriType, siriPayload),
					declarationAt(2, passcodeType, passcodePayload),
				),
			},
			{
				// The middle declaration removed, so the server returns fewer than state holds.
				Config: appleDeclarationsConfig("appledeclorder", name,
					declarationList(diskDeclaration, passcodeDeclaration)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "component_blocks.0.apple_declarations.#", "2"),
					declarationAt(0, diskType, diskPayload),
					declarationAt(1, passcodeType, passcodePayload),
				),
			},
			{
				Config: appleDeclarationsConfig("appledeclorder", name,
					declarationList(diskDeclaration, passcodeDeclaration)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccResource_Blueprint_AppleDeclarations_RepeatedType covers two declarations of one type in a
// component, which the platform accepts. It is why the read path aligns a prior payload by position:
// by type, the first entry would reconcile against both and the second would diff for ever.
func TestAccResource_Blueprint_AppleDeclarations_RepeatedType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-apple-decl-repeat-" + suffix

	const repeated = `[
		{
			channel = "SYSTEM"
			type    = "com.apple.configuration.passcode.settings"
			payload = jsonencode({ MinimumLength = 8 })
		},
		{
			channel = "USER"
			type    = "com.apple.configuration.passcode.settings"
			payload = jsonencode({ MinimumLength = 6 })
		},
	]`

	resourceName := "jamfplatform_blueprints_blueprint.test_apple_decl"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: appleDeclarationsConfig("appledeclrepeat", name, repeated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "component_blocks.0.apple_declarations.#", "2"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.channel", "SYSTEM"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.0.payload", `{"MinimumLength":8}`),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.1.channel", "USER"),
					resource.TestCheckResourceAttr(resourceName,
						"component_blocks.0.apple_declarations.1.payload", `{"MinimumLength":6}`),
				),
			},
			{
				Config: appleDeclarationsConfig("appledeclrepeat", name, repeated),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
