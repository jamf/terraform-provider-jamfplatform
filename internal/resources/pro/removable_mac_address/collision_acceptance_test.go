// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package removable_mac_address_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingRemovableMacAddress(t *testing.T, value string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateRemovableMacAddressByID(ctx, "0", &proclassic.RemovableMacAddress{Name: &value})
	if err != nil || created.ID == nil {
		t.Fatalf("creating removable MAC address %q outside Terraform: %v", value, err)
	}
	id := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeleteRemovableMacAddressByID(ctx, id) })
}

func TestAccResource_ProRemovableMacAddress_ValueCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	existing := strings.ToLower(macFor(testhelpers.RunSuffix(), 0xC1))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() { createExistingRemovableMacAddress(t, existing) },
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_removable_mac_address" "test" {
						mac_address = %q
					}
				`, strings.ToUpper(existing)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Value\s+already\s+in\s+use.*removable\s+MAC\s+address.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}
