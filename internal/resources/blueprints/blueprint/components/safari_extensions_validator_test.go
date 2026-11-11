// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// managedExtensionsValue builds a managed_extensions set from the live schema's element type, one
// element per {extension_id, state} pair; a nil id makes that element's extension_id unknown.
func managedExtensionsValue(t *testing.T, entries ...[2]*string) types.Set {
	t.Helper()
	elementType := SafariExtensionsComponentSchema()["managed_extensions"].(schema.SetNestedAttribute).NestedObject.Type().(types.ObjectType)

	elements := make([]attr.Value, 0, len(entries))
	for _, entry := range entries {
		values := map[string]attr.Value{}
		for name, attrType := range elementType.AttrTypes {
			values[name] = types.StringNull()
			if list, ok := attrType.(types.ListType); ok {
				values[name] = types.ListNull(list.ElemType)
			}
		}
		values["extension_id"] = types.StringUnknown()
		if entry[0] != nil {
			values["extension_id"] = types.StringValue(*entry[0])
		}
		values["state"] = types.StringValue(*entry[1])
		element, diags := types.ObjectValue(elementType.AttrTypes, values)
		if diags.HasError() {
			t.Fatalf("building an element: %v", diags)
		}
		elements = append(elements, element)
	}
	set, diags := types.SetValue(elementType, elements)
	if diags.HasError() {
		t.Fatalf("building the set: %v", diags)
	}
	return set
}

func validateManagedExtensions(value types.Set) []string {
	var resp validator.SetResponse
	uniqueExtensionIDs().ValidateSet(context.Background(), validator.SetRequest{
		Path:        path.Root("managed_extensions"),
		ConfigValue: value,
	}, &resp)
	var details []string
	for _, d := range resp.Diagnostics.Errors() {
		details = append(details, d.Summary()+": "+d.Detail())
	}
	return details
}

//go:fix inline

// TestUniqueExtensionIDs rejects two entries for one extension_id, whatever else differs, and
// lets distinct ids and an id still unknown at plan time through.
func TestUniqueExtensionIDs(t *testing.T) {
	dup := validateManagedExtensions(managedExtensionsValue(t,
		[2]*string{new("com.example.dup (ABC1234567)"), new("Allowed")},
		[2]*string{new("com.example.dup (ABC1234567)"), new("AlwaysOff")},
	))
	if len(dup) != 1 {
		t.Fatalf("a repeated extension_id produced %d errors, want 1: %v", len(dup), dup)
	}
	if !strings.Contains(dup[0], "configured more than once") || !strings.Contains(dup[0], `"com.example.dup (ABC1234567)"`) || !strings.Contains(dup[0], "merge these entries") {
		t.Errorf("the finding does not name the extension and the fix: %s", dup[0])
	}

	if got := validateManagedExtensions(managedExtensionsValue(t,
		[2]*string{new("com.example.a (ABC1234567)"), new("Allowed")},
		[2]*string{new("com.example.b (ABC1234567)"), new("Allowed")},
	)); len(got) != 0 {
		t.Errorf("distinct extension ids were rejected: %v", got)
	}

	if got := validateManagedExtensions(managedExtensionsValue(t,
		[2]*string{new("com.example.a (ABC1234567)"), new("Allowed")},
		[2]*string{nil, new("AlwaysOff")},
	)); len(got) != 0 {
		t.Errorf("an unknown extension id was rejected: %v", got)
	}

	if got := validateManagedExtensions(types.SetUnknown(managedExtensionsValue(t).ElementType(context.Background()))); len(got) != 0 {
		t.Errorf("an unknown set was rejected: %v", got)
	}
}
