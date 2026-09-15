// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignIbeaconResourceModel populates a resource model from an Ibeacon
// response. state.ID is only overwritten when the API ID is non-nil so a
// transient GET that drops the ID does not clobber the value persisted from
// Create.
//
// Major and Minor are derived from the SDK's *string fields *independently*:
// each field's sentinel ("-1") flips its own `include_any_*_value` bool to
// true and nulls the int64 field. Concrete values parse to int64 with
// `include_any_*_value = false`. The two toggles are independent — Jamf
// supports e.g. major=42, minor=-1 (specific major, any minor).
func assignIbeaconResourceModel(state *IbeaconResourceModel, ib *proclassic.Ibeacon) diag.Diagnostics {
	var diags diag.Diagnostics
	if ib == nil {
		return diags
	}
	if ib.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(ib.ID)
	}
	// Required attributes on the resource — server is authoritative; direct
	// copy keeps the import path aligned with the post-apply refresh path.
	if ib.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(ib.Name)
	}
	if ib.UUID != nil {
		state.UUID = helpers.StringPointerValueOrNull(ib.UUID)
	}

	major, includeAnyMajor, mErr := decodeIbeaconAxisValue(ib.Major)
	if mErr != nil {
		diags.AddError("Invalid iBeacon major value", helpers.APIErrorDetail(mErr))
	}
	minor, includeAnyMinor, nErr := decodeIbeaconAxisValue(ib.Minor)
	if nErr != nil {
		diags.AddError("Invalid iBeacon minor value", helpers.APIErrorDetail(nErr))
	}
	if diags.HasError() {
		return diags
	}
	state.Major = major
	state.Minor = minor
	state.IncludeAnyMajorValue = types.BoolValue(includeAnyMajor)
	state.IncludeAnyMinorValue = types.BoolValue(includeAnyMinor)

	return diags
}

// assignIbeaconDataSourceModel populates a data source model from an Ibeacon
// response. Symmetric with the resource builder but always copies the API
// value over the user's selector (the selector is just an input — output is
// Computed).
func assignIbeaconDataSourceModel(state *IbeaconDataSourceModel, ib *proclassic.Ibeacon) diag.Diagnostics {
	var diags diag.Diagnostics
	if ib == nil {
		return diags
	}
	if ib.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(ib.ID)
	}
	if ib.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(ib.Name)
	}
	state.UUID = helpers.StringPointerValueOrNull(ib.UUID)

	major, includeAnyMajor, mErr := decodeIbeaconAxisValue(ib.Major)
	if mErr != nil {
		diags.AddError("Invalid iBeacon major value", helpers.APIErrorDetail(mErr))
	}
	minor, includeAnyMinor, nErr := decodeIbeaconAxisValue(ib.Minor)
	if nErr != nil {
		diags.AddError("Invalid iBeacon minor value", helpers.APIErrorDetail(nErr))
	}
	if diags.HasError() {
		return diags
	}
	state.Major = major
	state.Minor = minor
	state.IncludeAnyMajorValue = types.BoolValue(includeAnyMajor)
	state.IncludeAnyMinorValue = types.BoolValue(includeAnyMinor)

	return diags
}

// decodeIbeaconAxisValue maps an SDK Major/Minor *string into the Terraform
// representation: (int64-or-null, includeAny bool, error). The sentinel "-1"
// produces (null, true, nil); a concrete integer string produces
// (Int64Value, false, nil); nil/empty produces (null, false, nil).
func decodeIbeaconAxisValue(s *string) (types.Int64, bool, error) {
	if s == nil || *s == "" {
		return types.Int64Null(), false, nil
	}
	if *s == anyMajorMinorSentinel {
		return types.Int64Null(), true, nil
	}
	v, err := parseIbeaconNumeric(s)
	if err != nil {
		return types.Int64Null(), false, err
	}
	return v, false, nil
}

// parseIbeaconNumeric parses a non-sentinel iBeacon major/minor string into a
// types.Int64. Empty/nil pointers map to a null int64. Returns a diagnostic
// error when the string is non-empty but not a valid integer in [0, 65535].
func parseIbeaconNumeric(s *string) (types.Int64, error) {
	if s == nil || *s == "" {
		return types.Int64Null(), nil
	}
	v, err := strconv.ParseInt(*s, 10, 64)
	if err != nil {
		return types.Int64Null(), fmt.Errorf("expected a 0-65535 integer, got %q", *s)
	}
	if v < 0 || v > 65535 {
		return types.Int64Null(), fmt.Errorf("value %d outside the supported [0, 65535] range", v)
	}
	return types.Int64Value(v), nil
}
