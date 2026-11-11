// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// SelfServiceCategories returns the validators for a classic Self Service
// category set: a SetNestedAttribute whose elements carry a string `id` and
// the display and feature flags named here. featureField is "" for a
// resource with no feature flag (mobile device apps).
//
// Jamf Pro rewrites two shapes of that set on every write, so a plan holding
// either never matches what it stores. Wire-probed on the EU sandbox,
// 2026-11-11, on create and update alike for policies, Mac App Store apps,
// ebooks, mobile device apps and both configuration profiles, with identical
// results on all six:
//
//   - It stores one entry per category id. Repeated entries merge, and each
//     flag is the OR of the entries (`feature_in` on one and `display_in` on
//     another store one entry with both; `display_in = false` beside
//     `display_in = true` stores `true`). Terraform keeps two elements that
//     differ in a flag as two set members, so the next plan re-adds the one
//     Jamf Pro dropped, forever, or the plan is refused outright when the
//     keyed flag modifier fills both elements to one value.
//   - It drops an entry whose display flag is false, whatever its feature
//     flag says. The apply fails with "inconsistent result after apply",
//     or, when the set held nothing else, "invalid result object".
//
// Identical entries need no check: Terraform collapses them before a
// validator sees the set. The ids compare as the integers the write sends,
// so "012" repeats "12".
func SelfServiceCategories(displayField, featureField string) []validator.Set {
	return []validator.Set{
		UniqueStringFieldSetRule(categoryIDRule(displayField, featureField)),
		selfServiceCategoryDisplayedValidator{displayField: displayField, featureField: featureField},
	}
}

// SelfServiceCategoriesList is SelfServiceCategories for a category list, as
// the two configuration profiles model it. A list keeps identical elements,
// so the repeated-id check covers those as well: with the provider built from
// main, 2026-11-11, two identical entries failed the apply with "element 1
// has vanished". displayField is "" for a resource that exposes no display
// flag (mobile device configuration profiles), which leaves only the
// repeated-id check. Errors are reported at the offending element's index.
func SelfServiceCategoriesList(displayField, featureField string) []validator.List {
	out := []validator.List{UniqueStringFieldListRule(categoryIDRule(displayField, featureField))}
	if displayField != "" {
		out = append(out, selfServiceCategoryDisplayedValidator{displayField: displayField, featureField: featureField})
	}
	return out
}

func categoryIDRule(displayField, featureField string) UniqueFieldRule {
	detail := "Category %s is listed more than once. Jamf Pro keeps one entry per category, so it would store a single entry and the apply or the next plan would fail to match. List the category once."
	if featureField != "" {
		detail = fmt.Sprintf("Category %%s is listed more than once. Jamf Pro keeps one entry per category and merges the flags of repeated entries, so the apply or the next plan would fail to match. Combine them into one entry with both %s and %s set as you want them stored.", displayField, featureField)
	}
	return UniqueFieldRule{
		Field:   "id",
		Key:     canonicalIntegerID,
		Summary: "Self Service category listed more than once",
		Detail:  detail,
	}
}

// canonicalIntegerID keys an id by the integer a classic write sends, falling
// back to the raw string for a value that is not one.
func canonicalIntegerID(value string) string {
	if n, err := strconv.Atoi(value); err == nil {
		return strconv.Itoa(n)
	}
	return value
}

// selfServiceCategoryDisplayedValidator rejects an element whose display flag
// is explicitly false. See SelfServiceCategories. An unknown or null flag
// passes: the write sends an unset display flag as true.
type selfServiceCategoryDisplayedValidator struct {
	displayField string
	featureField string
}

var (
	_ validator.Set  = selfServiceCategoryDisplayedValidator{}
	_ validator.List = selfServiceCategoryDisplayedValidator{}
)

// Description returns a plain-text description of the validator.
func (v selfServiceCategoryDisplayedValidator) Description(context.Context) string {
	return fmt.Sprintf("%s must not be false; remove the entry instead", v.displayField)
}

// MarkdownDescription returns the markdown description of the validator.
func (v selfServiceCategoryDisplayedValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateSet implements validator.Set. Set elements have no stable index,
// so errors are reported at the set path.
func (v selfServiceCategoryDisplayedValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, elem := range req.ConfigValue.Elements() {
		v.check(elem, req.Path, &resp.Diagnostics)
	}
}

// ValidateList implements validator.List.
func (v selfServiceCategoryDisplayedValidator) ValidateList(_ context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for i, elem := range req.ConfigValue.Elements() {
		v.check(elem, req.Path.AtListIndex(i).AtName(v.displayField), &resp.Diagnostics)
	}
}

func (v selfServiceCategoryDisplayedValidator) check(elem attr.Value, at path.Path, diags *diag.Diagnostics) {
	obj, ok := elem.(types.Object)
	if !ok || obj.IsNull() || obj.IsUnknown() {
		return
	}
	attrs := obj.Attributes()
	display, ok := attrs[v.displayField].(types.Bool)
	if !ok || display.IsNull() || display.IsUnknown() || display.ValueBool() {
		return
	}
	id := "with an unknown id"
	if s, ok := attrs["id"].(types.String); ok && !s.IsNull() && !s.IsUnknown() {
		id = s.ValueString()
	}
	if feature, ok := attrs[v.featureField].(types.Bool); ok && !feature.IsNull() && !feature.IsUnknown() && feature.ValueBool() {
		diags.AddAttributeError(
			at,
			fmt.Sprintf("%s requires %s", v.featureField, v.displayField),
			fmt.Sprintf("Category %s sets %s = true and %s = false. Jamf Pro drops a featured category that is not displayed, so the apply would fail. Set %s = true, or remove the entry.", id, v.featureField, v.displayField, v.displayField),
		)
		return
	}
	diags.AddAttributeError(
		at,
		fmt.Sprintf("Self Service category with %s = false", v.displayField),
		fmt.Sprintf("Category %s sets %s = false. Jamf Pro does not store a category that is not displayed, so the apply would fail. To take the item out of the category, remove the entry.", id, v.displayField),
	)
}
