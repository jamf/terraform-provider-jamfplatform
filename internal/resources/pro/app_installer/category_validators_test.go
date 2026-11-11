// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

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
	NewAppInstallerResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return setvalidation.Summaries(setvalidation.Run(t, resp.Schema, []string{"self_service_settings", "categories"}, elems...))
}

func TestSelfServiceCategories_Wired(t *testing.T) {
	id := func(s string) attr.Value { return types.StringValue(s) }
	got := runCategoryValidators(t,
		map[string]attr.Value{"category_id": id("12"), "featured": types.BoolValue(true)},
		map[string]attr.Value{"category_id": id("12"), "featured": types.BoolValue(false)},
	)
	if want := []string{"Self Service category listed more than once"}; !slices.Equal(got, want) {
		t.Errorf("repeated category_id: errors = %q, want %q", got, want)
	}
	got = runCategoryValidators(t,
		map[string]attr.Value{"category_id": id("12"), "featured": types.BoolValue(true)},
		map[string]attr.Value{"category_id": id("13")},
	)
	if len(got) != 0 {
		t.Errorf("valid set rejected: %q", got)
	}
	got = runCategoryValidators(t,
		map[string]attr.Value{"category_id": types.StringUnknown(), "featured": types.BoolValue(true)},
		map[string]attr.Value{"category_id": types.StringUnknown(), "featured": types.BoolValue(false)},
	)
	if len(got) != 0 {
		t.Errorf("unknown ids must defer: %q", got)
	}
}
