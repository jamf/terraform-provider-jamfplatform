// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package script_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingScript(t *testing.T, name string) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	contents := "#!/bin/sh\necho existing"
	created, err := c.CreateScriptV1(ctx, &pro.Script{Name: name, ScriptContents: &contents})
	if err != nil {
		t.Fatalf("creating script %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteScriptV1(ctx, created.ID) })
}

func TestAccResource_ProScript_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-script-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { createExistingScript(t, name) },
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						script_contents = "#!/bin/sh\necho hello"
					}
				`, strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
		},
	})
}

func TestAccResource_ProScript_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	mine := "tf-acc-collide-script-mine-" + suffix
	taken := "tf-acc-collide-script-taken-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						script_contents = "#!/bin/sh\necho hello"
					}
				`, mine),
			},
			{
				PreConfig: func() { createExistingScript(t, taken) },
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						script_contents = "#!/bin/sh\necho hello"
					}
				`, taken),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use`),
			},
		},
	})
}
