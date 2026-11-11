// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package validators provides shared Terraform Plugin Framework validators
// for use across all resource packages.
package validators

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UniqueStringFieldList returns a validator.List enforcing that the named
// string attribute is unique across every element of a ListNestedAttribute.
// Duplicates are reported at the offending element's field path, citing the
// index of the first occurrence.
func UniqueStringFieldList(fieldName string) validator.List {
	return uniqueStringFieldListValidator{uniqueStringField{fieldName: fieldName}}
}

// UniqueStringFieldSet returns a validator.Set enforcing that the named
// string attribute is unique across every element of a SetNestedAttribute.
// Set elements have no stable index, so duplicates are reported at the set
// path. Note the Set type alone cannot catch these: elements differing in any
// other attribute are distinct set members even when the named field collides.
func UniqueStringFieldSet(fieldName string) validator.Set {
	return uniqueStringFieldSetValidator{fieldName: fieldName}
}

// UniqueFieldRule configures UniqueStringFieldSetRule: which string attribute
// keys a set element, how two values are compared, and the diagnostic that
// says what Jamf Pro does with a repeated key and how to write it instead.
type UniqueFieldRule struct {
	// Field is the string attribute that keys each element.
	Field string
	// Key maps a value to its comparison key, for a server that matches
	// values loosely (email addresses without regard to case, numeric ids
	// written with leading zeros). Nil compares the values exactly.
	Key func(string) string
	// Summary is the diagnostic summary.
	Summary string
	// Detail is the diagnostic detail: a format string whose one %s verb is
	// the repeated value as written in the configuration.
	Detail string
}

// UniqueStringFieldSetRule is UniqueStringFieldSet with a comparison key and
// a diagnostic of the caller's choosing. It exists for sets that Jamf Pro
// stores keyed by one attribute: two elements differing anywhere else are
// distinct set members, so Terraform keeps both, while Jamf Pro merges them,
// keeps one, or refuses the write. Each repeated key is reported once.
func UniqueStringFieldSetRule(rule UniqueFieldRule) validator.Set {
	return uniqueStringFieldSetValidator{fieldName: rule.Field, rule: &rule}
}

// uniqueStringField holds the shared extraction and description logic. Per
// STYLE_GUIDE §Config-time validators the check defers on unknown values: an
// unknown collection, element, or field is skipped rather than treated as a
// duplicate, so values sourced from variables or other resources do not
// false-error at plan time.
type uniqueStringField struct {
	fieldName string
}

func (v uniqueStringField) description() string {
	return fmt.Sprintf("every element must have a unique %s", v.fieldName)
}

// fieldValue extracts the validated string field from one collection element,
// returning ok=false (defer) when the element or field is null, unknown,
// absent, or not a string.
func (v uniqueStringField) fieldValue(elem attr.Value) (string, bool) {
	obj, ok := elem.(types.Object)
	if !ok || obj.IsNull() || obj.IsUnknown() {
		return "", false
	}
	raw, ok := obj.Attributes()[v.fieldName]
	if !ok {
		return "", false
	}
	str, ok := raw.(types.String)
	if !ok || str.IsNull() || str.IsUnknown() {
		return "", false
	}
	return str.ValueString(), true
}

type uniqueStringFieldListValidator struct {
	uniqueStringField
}

var _ validator.List = uniqueStringFieldListValidator{}

// Description returns a plain-text description of the validator.
func (v uniqueStringFieldListValidator) Description(context.Context) string {
	return v.description()
}

// MarkdownDescription returns the markdown description of the validator.
func (v uniqueStringFieldListValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateList implements validator.List.
func (v uniqueStringFieldListValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	elements := req.ConfigValue.Elements()
	seen := make(map[string]int, len(elements))
	for i, elem := range elements {
		value, ok := v.fieldValue(elem)
		if !ok {
			continue
		}
		if first, dup := seen[value]; dup {
			resp.Diagnostics.AddAttributeError(
				req.Path.AtListIndex(i).AtName(v.fieldName),
				fmt.Sprintf("Duplicate %s within list", v.fieldName),
				fmt.Sprintf("%s %q was already used at index %d. Each element must have a unique %s.", v.fieldName, value, first, v.fieldName),
			)
			continue
		}
		seen[value] = i
	}
}

type uniqueStringFieldSetValidator struct {
	uniqueStringField
	rule *UniqueFieldRule
}

var _ validator.Set = uniqueStringFieldSetValidator{}

// Description returns a plain-text description of the validator.
func (v uniqueStringFieldSetValidator) Description(context.Context) string {
	return v.description()
}

// MarkdownDescription returns the markdown description of the validator.
func (v uniqueStringFieldSetValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateSet implements validator.Set.
func (v uniqueStringFieldSetValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	seen := make(map[string]int)
	for _, elem := range req.ConfigValue.Elements() {
		value, ok := v.fieldValue(elem)
		if !ok {
			continue
		}
		key := value
		if v.rule != nil && v.rule.Key != nil {
			key = v.rule.Key(value)
		}
		seen[key]++
		if seen[key] != 2 {
			continue
		}
		if v.rule != nil {
			resp.Diagnostics.AddAttributeError(req.Path, v.rule.Summary, fmt.Sprintf(v.rule.Detail, value))
			continue
		}
		resp.Diagnostics.AddAttributeError(
			req.Path,
			fmt.Sprintf("Duplicate %s within set", v.fieldName),
			fmt.Sprintf("%s %q appears more than once. Each element must have a unique %s.", v.fieldName, value, v.fieldName),
		)
	}
}
