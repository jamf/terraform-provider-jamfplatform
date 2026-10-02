// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// notEmptyStringValidator rejects an empty string, naming what Jamf Pro does
// with one.
type notEmptyStringValidator struct {
	detail string
}

// NotEmptyString returns a validator.String that rejects "" at plan time.
//
// It exists for attributes where Jamf Pro does not store an empty value as
// sent: it refuses the write, replaces "" with a default, or drops the object.
// Each one was wire-probed on 2026-10-01 (#445), and in every case the apply
// failed, either on the server's error or on the post-apply consistency check
// once the read disagreed with the plan. detail completes the diagnostic: say
// what Jamf Pro does with "" and what to write instead.
//
// stringvalidator.LengthAtLeast(1) would catch the same input, but its message
// names a length and leaves the operator to guess why an empty value matters.
//
// Null and unknown values defer to the server, per STYLE_GUIDE §Config-time
// validators.
func NotEmptyString(detail string) validator.String {
	return notEmptyStringValidator{detail: detail}
}

// Description returns a plain-text description of the validator.
func (notEmptyStringValidator) Description(_ context.Context) string {
	return "must not be an empty string"
}

// MarkdownDescription returns the markdown description of the validator.
func (v notEmptyStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString implements validator.String.
func (v notEmptyStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || req.ConfigValue.ValueString() != "" {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Empty value not accepted", v.detail)
}
