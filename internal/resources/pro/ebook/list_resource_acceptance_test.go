// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ebook_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccListResource_ProEbook_SelfServiceHydration is the ebook half of the
// #520 regression: with include_resource the list starts from empty state, and
// the category links used to stay null, so `-generate-config-out` dropped them.
func TestAccListResource_ProEbook_SelfServiceHydration(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ebook-listss-" + suffix
	cfg := ebookConfigCategorySwap(name, suffix, "a", "b")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Query: true,
				Config: `
provider "jamfplatform" {}

list "jamfplatform_pro_ebook" "test" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = {
      name_substring = "` + name + `"
    }
  }
}
`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_ebook.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_ebook.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("self_service").AtMapKey("categories"), KnownValue: knownvalue.SetSizeExact(2)},
						},
					),
				},
			},
		},
	})
}
