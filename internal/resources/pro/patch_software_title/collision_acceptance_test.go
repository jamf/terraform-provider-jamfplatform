// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package patch_software_title_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingPatchSoftwareTitle(t *testing.T, name, nameID string, sourceID int) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreatePatchSoftwareTitleByID(ctx, "0", &proclassic.PatchSoftwareTitle{Name: &name, NameID: &nameID, SourceID: &sourceID})
	if err != nil || created.ID == nil {
		t.Fatalf("creating patch software title %q outside Terraform: %v", name, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeletePatchSoftwareTitleByID(ctx, id) })
}

func collisionConfig(name, nameID string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_patch_software_title" "test" {
			name      = %q
			name_id   = %q
			source_id = %d
		}
	`, name, nameID, accTitleSourceID)
}

func TestAccResource_ProPatchSoftwareTitle_TitleCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					createExistingPatchSoftwareTitle(t, "tf-acc-collide-pst-"+suffix, accTitleNameID, accTitleSourceID)
				},
				Config:      collisionConfig("tf-acc-collide-pst-other-"+suffix, accTitleNameID),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Software\s+title\s+already\s+in\s+use.*patch\s+software\s+title.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProPatchSoftwareTitle_DifferentTitleDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					createExistingPatchSoftwareTitle(t, "tf-acc-collide-pst-diff-"+suffix, accTitleNameID, accTitleSourceID)
				},
				Config:             collisionConfig("tf-acc-collide-pst-diff-"+suffix, "286"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
