// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package pkg_test

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

func createExistingPackage(t *testing.T, displayName, fileName string) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreatePackageV1(ctx, &pro.Package{PackageName: displayName, FileName: fileName, CategoryID: "-1", Priority: 10})
	if err != nil {
		t.Fatalf("creating package %q outside Terraform: %v", displayName, err)
	}
	t.Cleanup(func() { _ = c.DeletePackageV1(ctx, created.ID) })
}

func collisionConfig(displayName, fileName string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_package" "test" {
			display_name = %q
			file_name    = %q
		}
	`, displayName, fileName)
}

func TestAccResource_ProPackage_NameCollisionOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	displayName := "tf-acc-collide-pkg-" + suffix
	fileName := "tf-acc-collide-pkg-" + suffix + ".pkg"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingPackage(t, displayName, fileName) },
				Config:      collisionConfig(strings.ToUpper(displayName), "tf-acc-other-"+suffix+".pkg"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
			{
				Config:      collisionConfig("tf-acc-other-"+suffix, strings.ToUpper(strings.TrimSuffix(fileName, ".pkg"))+".pkg"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)File\s+name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
		},
	})
}

func TestAccResource_ProPackage_NameCollisionOnRename(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	taken := "tf-acc-collide-pkg-taken-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: collisionConfig("tf-acc-collide-pkg-mine-"+suffix, "tf-acc-collide-pkg-mine-"+suffix+".pkg")},
			{
				PreConfig:   func() { createExistingPackage(t, taken, taken+".pkg") },
				Config:      collisionConfig(taken, "tf-acc-collide-pkg-mine-"+suffix+".pkg"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use`),
			},
		},
	})
}
