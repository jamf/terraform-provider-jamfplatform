// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// noSurroundingWhitespaceValidator refuses a name Jamf Account would trim.
//
// Jamf Account trims the name when it records it, wire-probed 2026-10-22: a name
// sent as "  tf-probe-env  " is stored and read back as "tf-probe-env". `name` is
// Required, so STYLE_GUIDE §Plan-modifier rewrites are NOT a valid option for
// Required attributes rules out rewriting it in a plan modifier. Strict
// acceptance with a diagnostic naming the spelling to use is the remaining
// correct option, and it keeps plan and state equal.
type noSurroundingWhitespaceValidator struct{}

// NoSurroundingWhitespace returns a validator that rejects a value with leading
// or trailing whitespace.
func NoSurroundingWhitespace() validator.String {
	return noSurroundingWhitespaceValidator{}
}

// Description returns a plain-text description of the validator's behaviour.
func (v noSurroundingWhitespaceValidator) Description(_ context.Context) string {
	return "must not start or end with whitespace"
}

// MarkdownDescription returns a Markdown description of the validator's behaviour.
func (v noSurroundingWhitespaceValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString checks the configured value.
func (v noSurroundingWhitespaceValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	if trimmed := strings.TrimSpace(value); trimmed != value {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Name has leading or trailing whitespace",
			"Jamf Account trims the name when it stores it. The stored name would differ from your "+
				"configuration, and every apply would fail as inconsistent. Set `name` to \""+trimmed+"\".",
		)
	}
}
