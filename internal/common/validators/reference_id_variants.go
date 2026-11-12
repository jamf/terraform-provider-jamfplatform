// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// referenceIDVariantValidator is the String validator behind
// ClearableReferenceIDWith and PositiveReferenceID: a positive canonical ID,
// plus any of the extra sentinels.
type referenceIDVariantValidator struct {
	noun      string
	sentinels []string
	hint      string
}

// ClearableReferenceIDWith is ClearableReferenceID for a field whose wire
// accepts further sentinels beside "-1". Each extra is accepted verbatim and
// named in the diagnostic. "" and "0" stay refused.
func ClearableReferenceIDWith(noun string, extra ...string) validator.String {
	return referenceIDVariantValidator{
		noun:      noun,
		sentinels: append([]string{ReferenceIDClearValue}, extra...),
	}
}

// PositiveReferenceID returns a validator.String accepting only a positive
// canonical decimal ID, for a reference Jamf Pro cannot clear to a sentinel.
// hint finishes the diagnostic and may be empty.
func PositiveReferenceID(noun, hint string) validator.String {
	return referenceIDVariantValidator{noun: noun, hint: hint}
}

// PositiveReferenceIDOr returns a validator.String accepting a positive
// canonical decimal ID or one of the given sentinels, and refusing "-1" unless it
// is listed. It fits a field where "none" is a sentinel other than "-1", such as
// "0" on an enrollment customization. hint finishes the diagnostic and may be
// empty.
func PositiveReferenceIDOr(noun, hint string, sentinels ...string) validator.String {
	return referenceIDVariantValidator{noun: noun, sentinels: sentinels, hint: hint}
}

func (v referenceIDVariantValidator) quoted() string {
	q := make([]string, len(v.sentinels))
	for i, s := range v.sentinels {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}

// Description returns a plain-text description of the validator.
func (v referenceIDVariantValidator) Description(_ context.Context) string {
	if len(v.sentinels) == 0 {
		return fmt.Sprintf("must be a positive %s ID", v.noun)
	}
	return fmt.Sprintf("must be a positive %s ID, or one of %s", v.noun, v.quoted())
}

// MarkdownDescription returns the markdown description of the validator.
func (v referenceIDVariantValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (v referenceIDVariantValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if slices.Contains(v.sentinels, s) || isCanonicalPositiveInt(s) {
		return
	}
	var detail string
	if len(v.sentinels) == 0 {
		detail = fmt.Sprintf("%q is not a %[2]s ID. Set the numeric ID of a Jamf Pro %[2]s.", s, v.noun)
	} else {
		detail = fmt.Sprintf("%q is not a %[2]s ID. Set the numeric ID of a Jamf Pro %[2]s, or one of %[3]s.", s, v.noun, v.quoted())
	}
	if v.hint != "" {
		detail += " " + v.hint
	}
	resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s ID", v.noun), detail)
}

// int64ReferenceIDValidator is the Int64 twin of the String validators above.
type int64ReferenceIDValidator struct {
	noun      string
	sentinels []int64
	hint      string
}

// ClearableReferenceIDInt64 is ClearableReferenceID for an Int64 reference
// attribute: a positive ID or -1. Zero and every other negative are refused.
func ClearableReferenceIDInt64(noun string) validator.Int64 {
	return int64ReferenceIDValidator{noun: noun, sentinels: []int64{-1}}
}

// PositiveReferenceIDInt64 accepts only a positive ID, for an Int64 reference
// Jamf Pro cannot clear with a sentinel. hint finishes the diagnostic and may
// be empty.
func PositiveReferenceIDInt64(noun, hint string) validator.Int64 {
	return int64ReferenceIDValidator{noun: noun, hint: hint}
}

// Description returns a plain-text description of the validator.
func (v int64ReferenceIDValidator) Description(_ context.Context) string {
	if len(v.sentinels) == 0 {
		return fmt.Sprintf("must be a positive %s ID", v.noun)
	}
	return fmt.Sprintf("must be a positive %s ID, or %d to clear the %s", v.noun, v.sentinels[0], v.noun)
}

// MarkdownDescription returns the markdown description of the validator.
func (v int64ReferenceIDValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateInt64 implements validator.Int64.
func (v int64ReferenceIDValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	n := req.ConfigValue.ValueInt64()
	if n > 0 || slices.Contains(v.sentinels, n) {
		return
	}
	var detail string
	if len(v.sentinels) == 0 {
		detail = fmt.Sprintf("%d is not a %[2]s ID. Set the numeric ID of a Jamf Pro %[2]s.", n, v.noun)
	} else {
		detail = fmt.Sprintf("%d is not a %[2]s ID. Set the numeric ID of a Jamf Pro %[2]s, or %[3]d to clear the %[2]s.", n, v.noun, v.sentinels[0])
	}
	if v.hint != "" {
		detail += " " + v.hint
	}
	resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s ID", v.noun), detail)
}
