// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// ReferenceIDClearValue is the value that clears a Jamf Pro category or site
// reference on every endpoint ClearableReferenceID guards.
const ReferenceIDClearValue = "-1"

// clearableReferenceIDValidator accepts a positive Jamf Pro object ID or the
// clear value "-1".
type clearableReferenceIDValidator struct {
	noun string
}

// ClearableReferenceID returns a validator.String for a Jamf Pro category or
// site reference whose clear value is "-1". noun names the referenced object
// in the diagnostic ("category", "site").
//
// "-1" is the only value that clears the reference on every endpoint the
// validator is wired onto, wire-probed for issue #567. The other candidates
// fail at apply time, each differently: "" draws a 500 from /pro/v1/scripts
// and a 400 INVALID_ID from /pro/v1/app-installers/deployments and
// /pro/v1/packages, and on the classic resources the input builders drop it,
// so the old value is kept and the apply ends in "Provider produced
// inconsistent result after apply". "0" draws a 400 or a 409 ("No match found
// for category 0", "Problem with site ID"), except on /pro/v1/scripts, which
// stores the literal "0". So everything but a positive decimal integer and
// "-1" is refused at plan time, with a diagnostic naming "-1".
//
// A positive ID must be written in canonical form, without a sign, leading
// zeros or whitespace: Jamf Pro echoes the integer, so "007" would read back
// as "7" and fail the post-apply consistency check.
//
// Null and unknown values defer, per STYLE_GUIDE §Config-time validators.
func ClearableReferenceID(noun string) validator.String {
	return clearableReferenceIDValidator{noun: noun}
}

// Description returns a plain-text description of the validator.
func (v clearableReferenceIDValidator) Description(_ context.Context) string {
	return fmt.Sprintf("must be a positive %s ID, or %q to clear the %s", v.noun, ReferenceIDClearValue, v.noun)
}

// MarkdownDescription returns the markdown description of the validator.
func (v clearableReferenceIDValidator) MarkdownDescription(_ context.Context) string {
	return fmt.Sprintf("must be a positive %s ID, or `%s` to clear the %s", v.noun, ReferenceIDClearValue, v.noun)
}

// ValidateString implements validator.String.
func (v clearableReferenceIDValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if s == ReferenceIDClearValue || isCanonicalPositiveInt(s) {
		return
	}
	var detail string
	switch s {
	case "":
		detail = fmt.Sprintf("Jamf Pro does not clear the %[1]s when it receives an empty value. Set %[2]q to clear the %[1]s.", v.noun, ReferenceIDClearValue)
	case "0":
		detail = fmt.Sprintf("Jamf Pro does not accept 0 as a %[1]s ID. Set %[2]q to clear the %[1]s.", v.noun, ReferenceIDClearValue)
	default:
		detail = fmt.Sprintf("%q is not a %[2]s ID. Set the numeric ID of a Jamf Pro %[2]s, or %[3]q to clear the %[2]s.", s, v.noun, ReferenceIDClearValue)
	}
	resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("Invalid %s ID", v.noun), detail)
}

// isCanonicalPositiveInt reports whether s is a decimal integer above zero
// written with digits only and no leading zero.
func isCanonicalPositiveInt(s string) bool {
	if s == "" || s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}
