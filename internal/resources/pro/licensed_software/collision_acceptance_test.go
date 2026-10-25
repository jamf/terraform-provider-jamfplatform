// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package licensed_software_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"strconv"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingLicensedSoftware(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	created, err := c.CreateLicensedSoftwareByID(ctx, "0", &proclassic.LicensedSoftware{General: &proclassic.LicensedSoftwareGeneral{Name: &name}})
	if err != nil || created.ID == nil {
		t.Fatalf("creating licensed software %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteLicensedSoftwareByID(ctx, id) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name = %q
			software_definitions = [
				{ name = "Acme Editor", version = "1.0", compare_type = "is" },
			]
		}
	`, name)
}

func TestAccResource_ProLicensedSoftware_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-licensed-software-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingLicensedSoftware(t, name) },
				Config:      collisionConfig(strings.ToUpper(name) + " "),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*licensed\s+software\s+record.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProLicensedSoftware_NameDifferingByAccentDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-licensed-software-acc-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingLicensedSoftware(t, name) },
				Config:             collisionConfig(strings.Replace(name, "e", "é", 1)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
