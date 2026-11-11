// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	bpSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const (
	orderedAddr  = "jamfplatform_blueprints_blueprint.ordered"
	orderedHashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	orderedHashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// ordered joins HCL list elements in the order given, or reversed.
func ordered(reversed bool, elements ...string) string {
	if reversed {
		elements = slices.Clone(elements)
		slices.Reverse(elements)
	}
	return "[\n" + strings.Join(elements, ",\n") + "\n]"
}

// orderedCollectionsConfig builds a blueprint whose every ordered collection holds an identical
// repeat and a pair differing only in a secondary field, issue #568's two failing shapes, in an
// order that is not sorted by any field. reversed reverses every collection at every level.
func orderedCollectionsConfig(name string, reversed bool) string {
	bookmark := func(title, url string) string {
		return fmt.Sprintf(`{ type = "bookmark", title = %q, url = %q }`, title, url)
	}
	folderItem := func(title, url string) string { return fmt.Sprintf(`{ title = %q, url = %q }`, title, url) }
	folder := fmt.Sprintf(`{ type = "folder", title = "F", folder = %s }`, ordered(reversed,
		folderItem("m", "https://m.example.com"), folderItem("b", "https://b.example.com"), folderItem("b", "https://b.example.com")))
	group := func(id, title, bookmarks string) string {
		return fmt.Sprintf(`{ group_identifier = %q, title = %q, bookmarks = %s }`, id, title, bookmarks)
	}
	work := ordered(reversed, bookmark("Z", "https://z.example.com"), bookmark("A", "https://a.example.com"),
		bookmark("A", "https://a.example.com"), bookmark("A", "https://a2.example.com"), folder)
	single := ordered(reversed, bookmark("Z", "https://z.example.com"))

	domain := func(d string) string { return fmt.Sprintf(`{ domain = %q }`, d) }

	launchd := func(context, url string) string {
		return fmt.Sprintf(`{ context = %q, file_asset_reference = { data_url = %q, hash_sha_256 = %q, content_type = "application/x-plist" } }`, context, url, orderedHashA)
	}
	task := func(taskType, description, url, hash, launchds string) string {
		extra := ""
		if launchds != "" {
			extra = ", launchd_configurations = " + launchds
		}
		return fmt.Sprintf(`{ task_type = %q, task_description = %q, executable_asset_reference = { data_url = %q, hash_sha_256 = %q }%s }`, taskType, description, url, hash, extra)
	}
	launchds := ordered(reversed, launchd("daemon", "https://e.example.com/2.plist"), launchd("daemon", "https://e.example.com/1.plist"),
		launchd("daemon", "https://e.example.com/1.plist"), launchd("agent", "https://e.example.com/1.plist"))

	file := func(service, url string) string {
		return fmt.Sprintf(`{ service_type = %q, data_asset_reference = { data_url = %q, hash_sha_256 = %q } }`, service, url, orderedHashB)
	}
	program := func(token, description string) string {
		return fmt.Sprintf(`{ token = %q, description = %q }`, token, description)
	}
	declaration := func(declarationType, payload string) string {
		return fmt.Sprintf(`{ channel = "SYSTEM", kind = "CONFIGURATION", type = %q, payload = jsonencode(%s) }`, declarationType, payload)
	}

	return testBlueprintConfig(smartGroupHCL("ordered"), fmt.Sprintf(`
resource "jamfplatform_blueprints_blueprint" "ordered" {
  name          = %q
  description   = "Acceptance test — safe to delete"
  deployed      = false
  device_groups = [jamfplatform_device_group.scope.id]

  component_blocks = [
    {
      name = "Ordered"
      safari_bookmarks = {
        managed_bookmarks = %s
      }
      safari_extensions = {
        managed_extensions = [{
          extension_id    = "com.example.ordered.extension (ABC1234567)"
          state           = "Allowed"
          allowed_domains = %s
          denied_domains  = %s
        }]
      }
      service_background_tasks = {
        background_tasks = %s
      }
      service_configuration_files = {
        service_config_files = %s
      }
      software_update_settings = {
        beta_program_enrollment = "AlwaysOn"
        beta_offer_programs     = %s
      }
      custom_declarations = {
        declaration = %s
      }
    },
  ]
}
`, name,
		ordered(reversed, group("g1", "Work", work), group("g1", "Work", single), group("g1", "Work", single), group("g0", "Home", single)),
		ordered(reversed, domain("z.example.com"), domain("a.example.com"), domain("a.example.com"), domain("m.example.com")),
		ordered(reversed, domain("d.example.com"), domain("d.example.com"), domain("c.example.com")),
		ordered(reversed,
			task("com.example.z", "Z", "https://e.example.com/z.zip", orderedHashA, launchds),
			task("com.example.a", "A", "https://e.example.com/a.zip", orderedHashB, ""),
			task("com.example.a", "A", "https://e.example.com/a.zip", orderedHashB, ""),
			task("com.example.a", "A2", "https://e.example.com/a.zip", orderedHashB, "")),
		ordered(reversed, file("com.apple.sshd", "https://e.example.com/z.zip"), file("com.apple.bash", "https://e.example.com/a.zip"),
			file("com.apple.bash", "https://e.example.com/a.zip"), file("com.apple.bash", "https://e.example.com/b.zip")),
		ordered(reversed, program("tok-z", "Zed"), program("tok-a", "Ay"), program("tok-a", "Ay"), program("tok-a", "Ay two")),
		ordered(reversed,
			declaration("com.apple.configuration.siri.settings", `{ Enabled = true }`),
			declaration("com.apple.configuration.softwareupdate.settings", `{ Beta = { ProgramEnrollment = "AlwaysOff" } }`),
			declaration("com.apple.configuration.softwareupdate.settings", `{ Beta = { ProgramEnrollment = "AlwaysOff" } }`),
			declaration("com.apple.configuration.softwareupdate.settings", `{ Beta = { ProgramEnrollment = "AlwaysOn" } }`)),
	))
}

// orderedChecks asserts the first and last element of every ordered collection, which pins both
// the order and that no repeat was dropped (the last index exists only if every entry survived).
func orderedChecks(t *testing.T, reversed bool) resource.TestCheckFunc {
	pick := func(first, last string) string {
		if reversed {
			return last
		}
		return first
	}
	block := "component_blocks.0."
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_bookmarks.managed_bookmarks.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_bookmarks.managed_bookmarks.0.group_identifier", pick("g1", "g0")),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_bookmarks.managed_bookmarks."+pick("0", "3")+".bookmarks.#", "5"),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_bookmarks.managed_bookmarks."+pick("0", "3")+".bookmarks.0.title", pick("Z", "F")),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_bookmarks.managed_bookmarks."+pick("0", "3")+".bookmarks."+pick("4", "0")+".folder.#", "3"),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_bookmarks.managed_bookmarks."+pick("0", "3")+".bookmarks."+pick("4", "0")+".folder.0.title", pick("m", "b")),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_extensions.managed_extensions.0.allowed_domains.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_extensions.managed_extensions.0.allowed_domains.0.domain", pick("z.example.com", "m.example.com")),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_extensions.managed_extensions.0.denied_domains.#", "3"),
		resource.TestCheckResourceAttr(orderedAddr, block+"safari_extensions.managed_extensions.0.denied_domains.0.domain", pick("d.example.com", "c.example.com")),
		resource.TestCheckResourceAttr(orderedAddr, block+"service_background_tasks.background_tasks.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"service_background_tasks.background_tasks.0.task_description", pick("Z", "A2")),
		resource.TestCheckResourceAttr(orderedAddr, block+"service_background_tasks.background_tasks."+pick("0", "3")+".launchd_configurations.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"service_background_tasks.background_tasks."+pick("0", "3")+".launchd_configurations.0.context", pick("daemon", "agent")),
		resource.TestCheckResourceAttr(orderedAddr, block+"service_configuration_files.service_config_files.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"service_configuration_files.service_config_files.0.service_type", pick("com.apple.sshd", "com.apple.bash")),
		resource.TestCheckResourceAttr(orderedAddr, block+"software_update_settings.beta_offer_programs.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"software_update_settings.beta_offer_programs.0.description", pick("Zed", "Ay two")),
		resource.TestCheckResourceAttr(orderedAddr, block+"custom_declarations.declaration.#", "4"),
		resource.TestCheckResourceAttr(orderedAddr, block+"custom_declarations.declaration.0.type", pick("com.apple.configuration.siri.settings", "com.apple.configuration.softwareupdate.settings")),
		checkStoredOrder(t, reversed),
	)
}

// checkStoredOrder reads the blueprint back from the service and asserts the beta programs and the
// custom declarations are stored in configuration order, with each declaration's payloadKey its
// 1-based position, so state agreeing with configuration is not a coincidence of the read path.
func checkStoredOrder(t *testing.T, reversed bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[orderedAddr]
		if !ok {
			return fmt.Errorf("%s not in state", orderedAddr)
		}
		detail, err := bpSDK.New(testhelpers.NewAcceptanceClient(t)).GetBlueprint(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		wantDescriptions := []string{"Zed", "Ay", "Ay", "Ay two"}
		wantTypes := []string{"siri", "softwareupdate", "softwareupdate", "softwareupdate"}
		if reversed {
			slices.Reverse(wantDescriptions)
			slices.Reverse(wantTypes)
		}
		for _, component := range detail.Steps[0].Components {
			switch component.Identifier {
			case bpSDK.SoftwareUpdateSettingsComponentIdentifierComJamfDdmSoftwareUpdateSettings:
				var cfg bpSDK.SoftwareUpdateSettingsConfiguration
				if err := json.Unmarshal(component.Configuration, &cfg); err != nil {
					return err
				}
				var got []string
				for _, p := range *cfg.Beta.Value.OfferPrograms {
					got = append(got, *p.Description)
				}
				if !slices.Equal(got, wantDescriptions) {
					return fmt.Errorf("stored beta programs %v, want %v", got, wantDescriptions)
				}
			case bpSDK.CustomDeclarationsComponentIdentifierComJamfDdmCustomDeclarations:
				var cfg bpSDK.CustomDeclarationsConfiguration
				if err := json.Unmarshal(component.Configuration, &cfg); err != nil {
					return err
				}
				for i, d := range cfg.Declarations {
					if !strings.Contains(d.Type, wantTypes[i]) || d.PayloadKey != i+1 {
						return fmt.Errorf("stored declaration %d is %s with payloadKey %d, want a %s declaration keyed %d", i, d.Type, d.PayloadKey, wantTypes[i], i+1)
					}
				}
			}
		}
		return nil
	}
}

// addDuplicateBetaProgram repeats the first stored beta program out of band, the way an admin
// editing the blueprint in Jamf Pro could.
func addDuplicateBetaProgram(t *testing.T, id *string) func() {
	return func() {
		ctx := context.Background()
		client := bpSDK.New(testhelpers.NewAcceptanceClient(t))
		detail, err := client.GetBlueprint(ctx, *id)
		if err != nil {
			t.Fatalf("reading blueprint: %v", err)
		}
		steps := detail.Steps
		for i, component := range steps[0].Components {
			if component.Identifier != bpSDK.SoftwareUpdateSettingsComponentIdentifierComJamfDdmSoftwareUpdateSettings {
				continue
			}
			var cfg map[string]any
			if err := json.Unmarshal(component.Configuration, &cfg); err != nil {
				t.Fatal(err)
			}
			programs := cfg["Beta"].(map[string]any)["Value"].(map[string]any)["OfferPrograms"].([]any)
			cfg["Beta"].(map[string]any)["Value"].(map[string]any)["OfferPrograms"] = append(programs, programs[0])
			encoded, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			steps[0].Components[i].Configuration = encoded
		}
		if err := client.UpdateBlueprint(ctx, *id, &bpSDK.UpdateBlueprintRequest{Steps: &steps}); err != nil {
			t.Fatalf("adding a duplicate beta program out of band: %v", err)
		}
	}
}

// TestAccResource_Blueprint_OrderedCollections covers issue #568 for every blueprint collection the
// service stores as written: identical repeats and secondary-field variants round-trip in order with
// an empty plan, a reorder plans one in-place update and then plans clean before and after refresh,
// the blueprint adopts cleanly through generated configuration, and a repeat added outside
// Terraform reads as drift rather than an error.
func TestAccResource_Blueprint_OrderedCollections(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-bp-ordered-" + testhelpers.RunSuffix()
	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: orderedCollectionsConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					orderedChecks(t, false),
					func(s *terraform.State) error {
						id = s.RootModule().Resources[orderedAddr].Primary.ID
						return nil
					},
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: orderedCollectionsConfig(name, true),
				Check:  orderedChecks(t, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(orderedAddr, plancheck.ResourceActionUpdate)},
					PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			testhelpers.GenerateConfigStep(orderedAddr),
			{
				PreConfig:          addDuplicateBetaProgram(t, &id),
				Config:             orderedCollectionsConfig(name, true),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
