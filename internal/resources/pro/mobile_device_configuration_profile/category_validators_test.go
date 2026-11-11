// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/setvalidation"
)

func runCategoryValidators(t *testing.T, ids ...attr.Value) []string {
	t.Helper()
	var resp resource.SchemaResponse
	NewResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	elems := make([]map[string]attr.Value, 0, len(ids))
	for _, id := range ids {
		elems = append(elems, map[string]attr.Value{"id": id})
	}
	return setvalidation.Summaries(setvalidation.RunList(t, resp.Schema, []string{"self_service", "categories"}, elems...))
}

func TestSelfServiceCategories_Wired(t *testing.T) {
	want := []string{"Self Service category listed more than once"}
	if got := runCategoryValidators(t, types.StringValue("12"), types.StringValue("12")); !slices.Equal(got, want) {
		t.Errorf("repeated id: errors = %q, want %q", got, want)
	}
	if got := runCategoryValidators(t, types.StringValue("13"), types.StringValue("12")); len(got) != 0 {
		t.Errorf("valid list rejected: %q", got)
	}
	if got := runCategoryValidators(t, types.StringUnknown(), types.StringUnknown()); len(got) != 0 {
		t.Errorf("unknown ids must defer: %q", got)
	}
}
