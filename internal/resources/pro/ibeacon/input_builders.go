// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

import (
	"strconv"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildIbeaconInput converts the Terraform plan model into the SDK Ibeacon
// payload used for both Create and Update. ID is omitted on write — Create
// uses path id="0" and Update derives it from state.
//
// Major and Minor are encoded as strings (the SDK type uses *string with
// omitempty). The two `include_any_*_value` bools are independent — each
// trumps its own concrete int value, emitting the sentinel "-1" instead.
// The cross-field rules (validateIbeaconPlan + includeAnyMajorMinorConfigValidator)
// guarantee mutually-exclusive shape before we get here.
func buildIbeaconInput(plan IbeaconResourceModel) *proclassic.Ibeacon {
	out := &proclassic.Ibeacon{
		Name: helpers.OptionalStringPointer(plan.Name),
		UUID: helpers.OptionalStringPointer(plan.UUID),
	}

	if plan.IncludeAnyMajorValue.ValueBool() {
		sentinel := anyMajorMinorSentinel
		out.Major = &sentinel
	} else if !plan.Major.IsNull() && !plan.Major.IsUnknown() {
		s := strconv.FormatInt(plan.Major.ValueInt64(), 10)
		out.Major = &s
	}

	if plan.IncludeAnyMinorValue.ValueBool() {
		sentinel := anyMajorMinorSentinel
		out.Minor = &sentinel
	} else if !plan.Minor.IsNull() && !plan.Minor.IsUnknown() {
		s := strconv.FormatInt(plan.Minor.ValueInt64(), 10)
		out.Minor = &s
	}

	return out
}
