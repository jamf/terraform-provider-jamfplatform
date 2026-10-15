// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// CategorySwapFixtures returns three jamfplatform_pro_category resources named
// a, b and c, for a test that re-points a Self Service category element.
func CategorySwapFixtures(prefix, suffix string) string {
	var out strings.Builder
	for _, which := range []string{"a", "b", "c"} {
		fmt.Fprintf(&out, `
resource "jamfplatform_pro_category" %[1]q {
  name     = "%[2]s-%[1]s-%[3]s"
  priority = 9
}
`, which, prefix, suffix)
	}
	return out.String()
}

// CheckCategoryElement checks that the collection at collection on addr holds
// an element whose id is the id of jamfplatform_pro_category.<which>, carrying
// that category's own name and, for each entry of want, that attribute value.
func CheckCategoryElement(addr, collection, which string, want map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		cat, ok := s.RootModule().Resources["jamfplatform_pro_category."+which]
		if !ok {
			return fmt.Errorf("category %s not in state", which)
		}
		res, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s not in state", addr)
		}
		attrs := res.Primary.Attributes
		n, err := strconv.Atoi(attrs[collection+".#"])
		if err != nil {
			return fmt.Errorf("%s count: %w", collection, err)
		}
		for i := range n {
			prefix := fmt.Sprintf("%s.%d.", collection, i)
			if attrs[prefix+"id"] != cat.Primary.ID {
				continue
			}
			if got, wantName := attrs[prefix+"name"], cat.Primary.Attributes["name"]; got != wantName {
				return fmt.Errorf("%s category %s: name %q, want %q", collection, which, got, wantName)
			}
			for k, v := range want {
				if got := attrs[prefix+k]; got != v {
					return fmt.Errorf("%s category %s: %s %q, want %q", collection, which, k, got, v)
				}
			}
			return nil
		}
		return fmt.Errorf("%s has no element with category %s's id %s", collection, which, cat.Primary.ID)
	}
}
