// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package platform_environment_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_AccountPlatformEnvironment_NameCollisionIsStrictOnly(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("collide")
	c := account.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	config := func(provider string) string {
		return fmt.Sprintf(`
			%s
			resource "jamfplatform_account_platform_environment" "test" {
				name = %q
				type = %q
			}
		`, provider, strings.ToUpper(name), account.EnvironmentLabelSandbox)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnvironmentDestroy(t),
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					created, err := c.CreateEnvironment(ctx, &account.CreateEnvironmentRequest{Name: name, Label: account.EnvironmentLabelSandbox})
					if err != nil {
						t.Fatalf("creating environment %q outside Terraform: %v", name, err)
					}
					t.Cleanup(func() { _ = c.DeleteEnvironment(ctx, created.ID) })
				},
				Config: config(`provider "jamfplatform" {
					name_collision_check = { include_unenforced = true }
				}`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Account.*accepts\s+duplicates`),
			},
			{
				Config:             config(""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
