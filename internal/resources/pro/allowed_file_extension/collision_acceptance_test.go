// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package allowed_file_extension_test

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

func createExistingAllowedFileExtension(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	created, err := c.CreateAllowedFileExtensionByID(ctx, "0", &proclassic.AllowedFileExtension{Extension: &name})
	if err != nil || created.ID == nil {
		t.Fatalf("creating allowed file extension %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteAllowedFileExtensionByID(ctx, id) })
}

func collisionConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_allowed_file_extension" "test" {
			extension = %q
		}
	`, name)
}

func TestAccResource_ProAllowedFileExtension_ExtensionCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "zzcollide" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingAllowedFileExtension(t, name) },
				Config:      collisionConfig(strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Extension\s+already\s+in\s+use.*allowed\s+file\s+extension.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}
