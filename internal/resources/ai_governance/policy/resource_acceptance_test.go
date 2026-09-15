// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const policyResource = "jamfplatform_ai_governance_policy.test"

// testAccCheckPolicyDestroy verifies policies created during the test were archived. An archived
// policy reads as absent, so a 404 is the pass condition.
func testAccCheckPolicyDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := aigovernance.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_ai_governance_policy" {
				continue
			}
			_, err := client.GetPolicy(ctx, rs.Primary.ID)
			if err != nil {
				if apiErr := jamfplatform.AsAPIError(err); apiErr != nil && apiErr.HasStatus(404) {
					continue
				}
				return fmt.Errorf("error checking AI policy %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("AI policy %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_AIGovernancePolicy_Basic covers create-and-publish, an in-place update of every
// writable field, and import.
//
// The update changes name, description and settings at once because the provider sends the whole
// object on every update: a step changing one field would pass even if the write dropped the others.
// It also asserts the version went from 1 to 2, which is what proves the publish actually happened —
// saving a draft alone leaves the published version where it was.
func TestAccResource_AIGovernancePolicy_Basic(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ai-policy-" + suffix
	nameUpdated := "tf-acc-ai-policy-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "test" {
						name           = %q
						description    = "created by the provider acceptance suite"
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({ verbose = true })
					}
				`, name, tool.ID, tool.SchemaVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(policyResource, "id"),
					resource.TestCheckResourceAttr(policyResource, "name", name),
					resource.TestCheckResourceAttr(policyResource, "description", "created by the provider acceptance suite"),
					resource.TestCheckResourceAttr(policyResource, "tool_id", tool.ID),
					resource.TestCheckResourceAttr(policyResource, "schema_version", tool.SchemaVersion),
					resource.TestCheckResourceAttr(policyResource, "publish", "true"),
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "false"),
					resource.TestCheckResourceAttr(policyResource, "schema_drift", "false"),
					resource.TestCheckResourceAttrSet(policyResource, "created_at"),
					resource.TestCheckResourceAttrSet(policyResource, "updated_at"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "test" {
						name           = %q
						description    = "updated by the provider acceptance suite"
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({ verbose = false, model = "sonnet" })
					}
				`, nameUpdated, tool.ID, tool.SchemaVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "name", nameUpdated),
					resource.TestCheckResourceAttr(policyResource, "description", "updated by the provider acceptance suite"),
					resource.TestCheckResourceAttr(policyResource, "published_version", "2"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "false"),
				),
			},
			{
				ResourceName:      policyResource,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// The ImportState step above passes the flat `id =` form, which
			// arrives with state already populated. GenerateConfigStep uses
			// ImportBlockWithResourceIdentity — the form
			// `terraform query -generate-config-out` emits — which leaves prior
			// state null, sends this Read down readIdentity, and used to write a
			// state the framework refused outright: "Value Conversion Error ...
			// Path: timeouts", taking the whole plan down. Nothing else in this
			// suite reaches that branch.
			testhelpers.GenerateConfigStep(policyResource),
		},
	})
}

// TestAccResource_AIGovernancePolicy_DescriptionClearedAndBlank covers the two description
// transitions the rest of the suite never reaches: removing the attribute from a configuration that
// had one, and setting it to an explicit blank.
//
// Both need pinning for the same reason. Every other configuration here either carries a description
// throughout or never carries one, so a policy whose description could be written but never cleared
// passed the whole suite — the apply failed only on the present-to-absent step, which nothing took.
//
// The two outcomes are deliberately different and asserted as such: an absent attribute reads back
// null, while an explicit blank is kept as a blank. Jamf reports both as an empty description, so it
// is the configuration that decides which, and a later simplification of that handling would break
// one without touching the other.
func TestAccResource_AIGovernancePolicy_DescriptionClearedAndBlank(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	name := "tf-acc-ai-policy-desc-" + testhelpers.RunSuffix()

	config := func(description string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_ai_governance_policy" "test" {
				name           = %q
				%s
				tool_id        = %q
				schema_version = %q
				settings_json  = jsonencode({ verbose = true })
			}
		`, name, description, tool.ID, tool.SchemaVersion)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(`description = "set by the provider acceptance suite"`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(policyResource, tfjsonpath.New("description"),
						knownvalue.StringExact("set by the provider acceptance suite")),
				},
			},
			{
				Config: config(""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(policyResource, tfjsonpath.New("description"), knownvalue.Null()),
				},
			},
			{
				Config: config(`description = ""`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(policyResource, tfjsonpath.New("description"), knownvalue.StringExact("")),
				},
			},
		},
	})
}

// TestAccResource_AIGovernancePolicy_UnchangedSettingsMintNoVersion pins the wire behaviour the
// publish default rests on: the platform diffs the settings itself, so an apply that changes only the
// name raises no draft and the publish that follows is a no-op rather than a version minted for
// nothing.
//
// Without this the resource would mint a version on every apply, and a blueprint pinning a version
// number would fall behind on each one.
func TestAccResource_AIGovernancePolicy_UnchangedSettingsMintNoVersion(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	suffix := testhelpers.RunSuffix()

	config := func(name string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_ai_governance_policy" "test" {
				name           = %q
				tool_id        = %q
				schema_version = %q
				settings_json  = jsonencode({ verbose = true })
			}
		`, name, tool.ID, tool.SchemaVersion)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("tf-acc-ai-nover-" + suffix),
				Check:  resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
			},
			{
				Config: config("tf-acc-ai-nover-renamed-" + suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "name", "tf-acc-ai-nover-renamed-"+suffix),
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "false"),
				),
			},
		},
	})
}

// TestAccResource_AIGovernancePolicy_DraftOnly covers publish = false: the policy exists with an
// unpublished draft and no published version, and enabling publish afterwards mints version 1.
func TestAccResource_AIGovernancePolicy_DraftOnly(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	name := "tf-acc-ai-draft-" + testhelpers.RunSuffix()

	config := func(publish bool) string {
		return fmt.Sprintf(`
			resource "jamfplatform_ai_governance_policy" "test" {
				name           = %q
				tool_id        = %q
				schema_version = %q
				settings_json  = jsonencode({ verbose = true })
				publish        = %t
			}
		`, name, tool.ID, tool.SchemaVersion, publish)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "publish", "false"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "true"),
					resource.TestCheckNoResourceAttr(policyResource, "published_version"),
				),
			},
			{
				Config: config(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "publish", "true"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "false"),
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
				),
			},
		},
	})
}

// TestAccResource_AIGovernancePolicy_ReformattedSettingsConverge covers the settings body being
// rewritten with the same content in a different shape — reindented, keys reordered — which is what
// happens when an operator moves from `jsonencode` to a file, or pastes a configuration exported out
// of the admin UI.
//
// Two things must hold, and only the pair is meaningful. The apply must converge: the framework's own
// post-step check fails if the following plan is not empty, which is what proves state adopted the new
// formatting rather than fighting it. And published_version must stay at 1: the platform diffs the
// settings it holds, sees no change, and mints no version — so the reformatting reaches nobody.
//
// A plan modifier that held the state value instead was tried and removed for failing the first of
// those: state never adopted the new formatting, so every later plan proposed the same update again.
func TestAccResource_AIGovernancePolicy_ReformattedSettingsConverge(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	name := "tf-acc-ai-fmt-" + testhelpers.RunSuffix()

	compact := fmt.Sprintf(`
		resource "jamfplatform_ai_governance_policy" "test" {
			name           = %q
			tool_id        = %q
			schema_version = %q
			settings_json  = jsonencode({ verbose = true, model = "sonnet" })
		}
	`, name, tool.ID, tool.SchemaVersion)

	reordered := fmt.Sprintf(`
		resource "jamfplatform_ai_governance_policy" "test" {
			name           = %q
			tool_id        = %q
			schema_version = %q
			settings_json  = <<-JSON
				{
				  "model": "sonnet",
				  "verbose":       true
				}
			JSON
		}
	`, name, tool.ID, tool.SchemaVersion)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: compact,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "settings_json", `{"model":"sonnet","verbose":true}`),
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
				),
			},
			{
				Config: reordered,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(policyResource, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "false"),
				),
			},
		},
	})
}

// TestAccResource_AIGovernancePolicy_ToolChangeReplaces pins that a policy's tool is immutable. The
// platform has no field for it on an update, so changing it must replace the resource rather than
// silently leave the tool as it was.
func TestAccResource_AIGovernancePolicy_ToolChangeReplaces(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	first := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	second := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudefordesktop")
	name := "tf-acc-ai-replace-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "test" {
						name           = %q
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({ verbose = true })
					}
				`, name, first.ID, first.SchemaVersion),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "test" {
						name           = %q
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({})
					}
				`, name, second.ID, second.SchemaVersion),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(policyResource, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr(policyResource, "tool_id", second.ID),
			},
		},
	})
}

// TestAccResource_AIGovernancePolicy_SchemaDrift covers a policy deliberately written against an
// older schema version: it applies, reports schema_drift, and the plural data source's filter finds
// it. Skips when the tool offers only one schema version, which is a legitimate state.
func TestAccResource_AIGovernancePolicy_SchemaDrift(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	if len(tool.SchemaVersions) < 2 {
		t.Skipf("%s offers only one settings schema version, so drift cannot be exercised", tool.DisplayName)
	}
	older := ""
	for _, version := range tool.SchemaVersions {
		if version != tool.SchemaVersion {
			older = version
			break
		}
	}
	name := "tf-acc-ai-drift-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "test" {
						name           = %q
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({ verbose = true })
					}

					data "jamfplatform_ai_governance_policies" "drifted" {
						schema_drift_only = true
						depends_on        = [jamfplatform_ai_governance_policy.test]
					}
				`, name, tool.ID, older),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "schema_version", older),
					resource.TestCheckResourceAttr(policyResource, "schema_drift", "true"),
					checkDriftedListingContains(name),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "test" {
						name           = %q
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({ verbose = false })
					}
				`, name, tool.ID, tool.SchemaVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "schema_version", tool.SchemaVersion),
					resource.TestCheckResourceAttr(policyResource, "schema_drift", "false"),
					resource.TestCheckResourceAttr(policyResource, "published_version", "2"),
				),
			},
		},
	})
}

// checkDriftedListingContains asserts the drift-filtered listing names the policy under test.
func checkDriftedListingContains(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["data.jamfplatform_ai_governance_policies.drifted"]
		if !ok {
			return fmt.Errorf("the drifted policies data source is not in state")
		}
		for key, value := range rs.Primary.Attributes {
			if len(key) > len("policies.") && key[len(key)-5:] == ".name" && value == name {
				return nil
			}
		}
		return fmt.Errorf("the drift-filtered listing does not contain %q", name)
	}
}

// TestAccResource_AIGovernancePolicy_PlanTimeValidation covers every check the provider performs
// before an apply, so each one is proved to fail the plan rather than the apply.
func TestAccResource_AIGovernancePolicy_PlanTimeValidation(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	if !slices.Contains(tool.SchemaVersions, tool.SchemaVersion) {
		t.Fatalf("the catalogue reports a current schema version %q that is not in its own version list", tool.SchemaVersion)
	}

	cases := []struct {
		name      string
		toolID    string
		version   string
		settings  string
		wantError *regexp.Regexp
	}{
		{
			name:      "unknown tool",
			toolID:    "com.example.nope",
			version:   tool.SchemaVersion,
			settings:  `jsonencode({})`,
			wantError: regexp.MustCompile(`Unknown\s+AI\s+tool`),
		},
		{
			name:      "unknown schema version",
			toolID:    tool.ID,
			version:   "1999-01-01",
			settings:  `jsonencode({})`,
			wantError: regexp.MustCompile(`Unknown\s+settings\s+schema`),
		},
		{
			name:      "setting of the wrong type",
			toolID:    tool.ID,
			version:   tool.SchemaVersion,
			settings:  `jsonencode({ verbose = "yes" })`,
			wantError: regexp.MustCompile(`does\s+not\s+match\s+the\s+tool's\s+schema`),
		},
		{
			name:      "settings are not an object",
			toolID:    tool.ID,
			version:   tool.SchemaVersion,
			settings:  `jsonencode([])`,
			wantError: regexp.MustCompile(`must\s+be\s+a\s+JSON\s+object`),
		},
		{
			name:      "settings are not JSON",
			toolID:    tool.ID,
			version:   tool.SchemaVersion,
			settings:  `"{oops"`,
			wantError: regexp.MustCompile(`not\s+valid\s+JSON`),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: fmt.Sprintf(`
							resource "jamfplatform_ai_governance_policy" "test" {
								name           = "tf-acc-ai-invalid"
								tool_id        = %q
								schema_version = %q
								settings_json  = %s
							}
						`, c.toolID, c.version, c.settings),
						PlanOnly:    true,
						ExpectError: c.wantError,
					},
				},
			})
		})
	}
}

// TestAccResource_AIGovernancePolicy_Disappears covers a policy archived outside Terraform: the read
// must treat it as gone and the plan must propose creating it again, rather than failing on the 404
// every operation on an archived policy returns.
func TestAccResource_AIGovernancePolicy_Disappears(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	name := "tf-acc-ai-gone-" + testhelpers.RunSuffix()

	config := fmt.Sprintf(`
		resource "jamfplatform_ai_governance_policy" "test" {
			name           = %q
			tool_id        = %q
			schema_version = %q
			settings_json  = jsonencode({ verbose = true })
		}
	`, name, tool.ID, tool.SchemaVersion)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
			},
			{
				Config:    config,
				PreConfig: func() { archiveOutOfBand(t, name) },
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(policyResource, plancheck.ResourceActionCreate),
					},
				},
				Check: resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
			},
		},
	})
}

// archiveOutOfBand archives every policy with the given name, simulating an administrator deleting
// it in the Jamf Account admin UI.
func archiveOutOfBand(t *testing.T, name string) {
	t.Helper()
	client := aigovernance.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	summaries, err := client.ListPolicies(ctx, nil, false)
	if err != nil {
		t.Fatalf("listing policies to archive %q: %s", name, err)
	}
	for _, summary := range summaries {
		if summary.Name != name {
			continue
		}
		if err := client.ArchivePolicy(ctx, summary.ID); err != nil {
			t.Fatalf("archiving policy %s: %s", summary.ID, err)
		}
	}
}

// TestAccResource_AIGovernancePolicy_SchemaVersionAloneMintsNoVersion pins a trap in the platform
// that no diagnostic can prevent: moving a policy forward to a newer settings schema version, without
// also changing the settings, raises no draft and therefore publishes nothing.
//
// The consequence is that the policy reports schema_drift = false while the version blueprints deploy
// is still the one published against the older schema. The provider records this honestly rather than
// pretending a version was minted; the guide tells operators to change a setting as well.
func TestAccResource_AIGovernancePolicy_SchemaVersionAloneMintsNoVersion(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	if len(tool.SchemaVersions) < 2 {
		t.Skipf("%s offers only one settings schema version", tool.DisplayName)
	}
	older := ""
	for _, version := range tool.SchemaVersions {
		if version != tool.SchemaVersion {
			older = version
			break
		}
	}
	name := "tf-acc-ai-schemaonly-" + testhelpers.RunSuffix()

	config := func(schemaVersion string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_ai_governance_policy" "test" {
				name           = %q
				tool_id        = %q
				schema_version = %q
				settings_json  = jsonencode({ verbose = true })
			}
		`, name, tool.ID, schemaVersion)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(older),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
					resource.TestCheckResourceAttr(policyResource, "schema_drift", "true"),
				),
			},
			{
				Config: config(tool.SchemaVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyResource, "schema_version", tool.SchemaVersion),
					resource.TestCheckResourceAttr(policyResource, "schema_drift", "false"),
					resource.TestCheckResourceAttr(policyResource, "has_draft", "false"),
					// No new version: the platform saw no settings change, so there was nothing to publish.
					resource.TestCheckResourceAttr(policyResource, "published_version", "1"),
				),
			},
		},
	})
}
