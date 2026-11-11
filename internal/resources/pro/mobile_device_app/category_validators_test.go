// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/setvalidation"
)

func runCategoryValidators(t *testing.T, elems ...map[string]attr.Value) []string {
	t.Helper()
	var resp resource.SchemaResponse
	NewMobileAppResource().(*MobileAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return setvalidation.Summaries(setvalidation.Run(t, resp.Schema, []string{"self_service", "self_service_categories"}, elems...))
}

func TestSelfServiceCategories_Wired(t *testing.T) {
	id := func(s string) attr.Value { return types.StringValue(s) }
	cases := map[string]struct {
		elems []map[string]attr.Value
		want  string
	}{
		"same id twice": {
			elems: []map[string]attr.Value{
				{"id": id("12")},
				{"id": id("12"), "display_in": types.BoolValue(true)},
			},
			want: "Self Service category listed more than once",
		},
		"not displayed": {
			elems: []map[string]attr.Value{{"id": id("12"), "display_in": types.BoolValue(false)}},
			want:  "Self Service category with display_in = false",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := runCategoryValidators(t, tc.elems...); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("errors = %q, want [%q]", got, tc.want)
			}
		})
	}
	t.Run("two categories", func(t *testing.T) {
		got := runCategoryValidators(t,
			map[string]attr.Value{"id": id("12"), "display_in": types.BoolValue(true)},
			map[string]attr.Value{"id": id("13")},
		)
		if len(got) != 0 {
			t.Errorf("valid set rejected: %q", got)
		}
	})
}
