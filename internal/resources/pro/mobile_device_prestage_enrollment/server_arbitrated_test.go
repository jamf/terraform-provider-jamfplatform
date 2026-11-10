// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"errors"
	"strings"

	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestIsKnownTrue(t *testing.T) {
	cases := map[string]struct {
		in   types.Bool
		want bool
	}{
		"known true":  {types.BoolValue(true), true},
		"known false": {types.BoolValue(false), false},
		"null":        {types.BoolNull(), false},
		"unknown":     {types.BoolUnknown(), false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isKnownTrue(tc.in); got != tc.want {
				t.Errorf("isKnownTrue(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveIntentBool(t *testing.T) {
	cases := map[string]struct {
		plan, cfg, state types.Bool
		want             types.Bool
	}{
		"known plan wins":                       {types.BoolValue(true), types.BoolValue(false), types.BoolValue(false), types.BoolValue(true)},
		"unknown plan -> config":                {types.BoolUnknown(), types.BoolValue(true), types.BoolValue(false), types.BoolValue(true)},
		"unknown plan, null config -> state":    {types.BoolUnknown(), types.BoolNull(), types.BoolValue(true), types.BoolValue(true)},
		"unknown plan, all null -> false":       {types.BoolUnknown(), types.BoolNull(), types.BoolNull(), types.BoolValue(false)},
		"unknown plan, unknown config -> state": {types.BoolUnknown(), types.BoolUnknown(), types.BoolValue(true), types.BoolValue(true)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := resolveIntentBool(tc.plan, tc.cfg, tc.state)
			if !got.Equal(tc.want) {
				t.Errorf("resolveIntentBool(%v,%v,%v) = %v, want %v", tc.plan, tc.cfg, tc.state, got, tc.want)
			}
		})
	}
}

// TestRestoreServerArbitrated_StateCarryPath models the §F9 mutual-exclusion
// state-carry case: ModifyPlan rendered both shared-iPad mode flags Unknown;
// the user's config enabled temporary_session_only and omitted
// use_storage_quota_size (carried true from prior state). Restore must send
// BOTH true on the wire (the server then forces one false) — a bare Unknown
// would serialise as false and silently disable the user's intent.
func TestRestoreServerArbitrated_StateCarryPath(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		DefaultPrestage:      types.BoolValue(true),
		UseStorageQuotaSize:  types.BoolUnknown(),
		TemporarySessionOnly: types.BoolUnknown(),
	}
	cfg := MobileDevicePrestageEnrollmentResourceModel{
		DefaultPrestage:      types.BoolValue(true),
		UseStorageQuotaSize:  types.BoolNull(), // user omitted
		TemporarySessionOnly: types.BoolValue(true),
	}
	state := MobileDevicePrestageEnrollmentResourceModel{
		UseStorageQuotaSize: types.BoolValue(true), // carried from prior apply
	}

	restoreServerArbitrated(&plan, cfg, state)

	// default_prestage is not server-arbitrated and is never stamped Unknown, so
	// restore must pass it through untouched.
	if !plan.DefaultPrestage.Equal(types.BoolValue(true)) {
		t.Errorf("default_prestage should pass through untouched: %v", plan.DefaultPrestage)
	}
	if !plan.UseStorageQuotaSize.Equal(types.BoolValue(true)) {
		t.Errorf("use_storage_quota_size intent lost (state-carry): %v", plan.UseStorageQuotaSize)
	}
	if !plan.TemporarySessionOnly.Equal(types.BoolValue(true)) {
		t.Errorf("temporary_session_only intent lost: %v", plan.TemporarySessionOnly)
	}
}

// TestRestoreServerArbitrated_CreateNoState confirms create (zero-value state)
// sources intent from config and defaults a wholly-absent field to false
// rather than leaking Unknown into the POST body.
func TestRestoreServerArbitrated_CreateNoState(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		DefaultPrestage:      types.BoolValue(true),
		UseStorageQuotaSize:  types.BoolValue(false),
		TemporarySessionOnly: types.BoolUnknown(),
	}
	cfg := MobileDevicePrestageEnrollmentResourceModel{
		DefaultPrestage:      types.BoolValue(true),
		UseStorageQuotaSize:  types.BoolValue(false),
		TemporarySessionOnly: types.BoolNull(),
	}

	restoreServerArbitrated(&plan, cfg, MobileDevicePrestageEnrollmentResourceModel{})

	if !plan.DefaultPrestage.Equal(types.BoolValue(true)) {
		t.Errorf("default_prestage = %v, want true (passed through)", plan.DefaultPrestage)
	}
	if !plan.TemporarySessionOnly.Equal(types.BoolValue(false)) {
		t.Errorf("temporary_session_only = %v, want false (no config/state)", plan.TemporarySessionOnly)
	}
}

// TestDiagnoseAlreadyDefault covers the ALREADY_DEFAULT mapping. Jamf Pro
// rejects a claim on a default another PreStage holds with 400 ALREADY_DEFAULT
// and writes nothing, so the error must be surfaced (not swallowed as a
// warning) and must name the fix.
func TestDiagnoseAlreadyDefault(t *testing.T) {
	apiErr := errors.New(`CreateMobileDevicePrestageV3: 400 {"httpStatus":400,"errors":[{"code":"ALREADY_DEFAULT","description":"Another prestage is already the default prestage","id":"0","field":"defaultPrestage"}]}`)

	t.Run("recognised, with config intent", func(t *testing.T) {
		var diags diag.Diagnostics
		cfg := MobileDevicePrestageEnrollmentResourceModel{DefaultPrestage: types.BoolValue(true)}
		if !diagnoseAlreadyDefault(&diags, cfg, apiErr) {
			t.Fatalf("must recognise ALREADY_DEFAULT")
		}
		if !diags.HasError() {
			t.Fatalf("must raise an ERROR, not a warning — the write did not happen")
		}
		detail := diags.Errors()[0].Detail()
		if !strings.Contains(detail, "default_prestage = false") {
			t.Errorf("detail should name the fix, got: %s", detail)
		}
		// The ordering hazard only applies when this resource is the claimant.
		if !strings.Contains(detail, "depends_on") {
			t.Errorf("detail should cover the release-before-claim ordering, got: %s", detail)
		}
	})

	t.Run("recognised, no config intent", func(t *testing.T) {
		var diags diag.Diagnostics
		if !diagnoseAlreadyDefault(&diags, MobileDevicePrestageEnrollmentResourceModel{}, apiErr) {
			t.Fatalf("must recognise ALREADY_DEFAULT regardless of config")
		}
		if strings.Contains(diags.Errors()[0].Detail(), "depends_on") {
			t.Errorf("ordering advice is noise when the user did not ask for the default")
		}
	})

	t.Run("passes other errors through", func(t *testing.T) {
		var diags diag.Diagnostics
		for _, err := range []error{nil, errors.New("500 internal server error"), errors.New("ALREADY_SCOPED")} {
			if diagnoseAlreadyDefault(&diags, MobileDevicePrestageEnrollmentResourceModel{}, err) {
				t.Errorf("must not claim unrelated error: %v", err)
			}
		}
		if diags.HasError() {
			t.Errorf("must not add diagnostics for unrelated errors")
		}
	})
}

func TestStorageQuotaWillBeRepaired(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		state   types.Int64
		changes bool
		want    bool
	}{
		{"UI-created zero, update planned", types.Int64Value(0), true, true},
		{"UI-created zero, no change planned", types.Int64Value(0), false, false},
		{"at floor, update planned", types.Int64Value(1024), true, false},
		{"above floor, update planned", types.Int64Value(2048), true, false},
		{"null state (create)", types.Int64Null(), true, false},
		{"unknown state", types.Int64Unknown(), true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := storageQuotaWillBeRepaired(c.state, c.changes); got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}
