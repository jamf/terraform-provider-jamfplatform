// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package keyedcategories holds the shared schema assertion for the elements of
// a Self Service category set or list. It is a package of its own rather than
// part of testhelpers because testhelpers imports the provider under the
// acceptance tag, which would make every in-package unit test that uses it an
// import cycle.
package keyedcategories

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
)

// AssertCarriedByID checks the attributes of one Self Service category element.
//
// Jamf Pro resolves a Self Service category by id and ignores an authored name
// (wire-probed 2026-10-14 for Mac apps, mobile device apps and ebooks), and the
// framework pairs Set and List elements with prior state by position. So name
// must be Computed-only and every carried attribute, name and each of flags,
// must carry prior state only from the element with the same id. A positional
// carry hands a re-pointed element another category's values, and the apply
// fails with "Provider produced inconsistent result after apply".
func AssertCarriedByID(t *testing.T, elem map[string]schema.Attribute, flags ...string) {
	t.Helper()
	ctx := context.Background()
	keyedString := planmodifiers.UseKeyedStateForUnknownString("id").Description(ctx)
	keyedBool := planmodifiers.UseKeyedStateForUnknownBool("id").Description(ctx)

	if id, ok := elem["id"].(schema.StringAttribute); !ok || !id.Required {
		t.Errorf("id: want a Required string, got %+v", elem["id"])
	}
	n, ok := elem["name"].(schema.StringAttribute)
	if !ok || n.Optional || n.Required || !n.Computed || len(n.PlanModifiers) != 1 || n.PlanModifiers[0].Description(ctx) != keyedString {
		t.Errorf("name: want Computed-only carried by id, got %+v", elem["name"])
	}
	for _, f := range flags {
		b, ok := elem[f].(schema.BoolAttribute)
		if !ok || !b.Optional || !b.Computed || len(b.PlanModifiers) != 1 || b.PlanModifiers[0].Description(ctx) != keyedBool {
			t.Errorf("%s: want Optional+Computed carried by id, got %+v", f, elem[f])
		}
	}
}
