// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_parent_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// knownAppSet builds a known types.Set of safelisted_apps elements.
func knownAppSet(t *testing.T, apps ...safelistedAppModel) types.Set {
	t.Helper()
	set, diags := types.SetValueFrom(context.Background(), safelistedAppObjectType, apps)
	if diags.HasError() {
		t.Fatalf("building test set: %v", diags)
	}
	return set
}

// emptyAppSet builds a known empty types.Set (the `[]` clear shape).
func emptyAppSet(t *testing.T) types.Set {
	t.Helper()
	set, diags := types.SetValue(safelistedAppObjectType, []attr.Value{})
	if diags.HasError() {
		t.Fatalf("building empty test set: %v", diags)
	}
	return set
}

// knownTimesMap builds a known types.Map of restricted_times entries.
func knownTimesMap(t *testing.T, entries map[string]restrictedTimeModel) types.Map {
	t.Helper()
	m, diags := types.MapValueFrom(context.Background(), restrictedTimeObjectType, entries)
	if diags.HasError() {
		t.Fatalf("building test map: %v", diags)
	}
	return m
}

// baseCurrent returns a live-settings merge base with every field populated.
func baseCurrent() *pro.ParentApp {
	name, bundleID := "Live App", "com.example.live"
	begin, end := "07:00:00", "08:00:00"
	return &pro.ParentApp{
		IsEnabled:                     true,
		TimezoneID:                    "Europe/Berlin",
		DeviceGroupID:                 99,
		AllowClearPasscode:            new(true),
		DisassociateOnWipeAndReEnroll: new(true),
		AllowTemplates:                new(false),
		RestrictedTimes: map[string]pro.TimeFrame{
			"TUESDAY": {BeginTime: &begin, EndTime: &end},
		},
		SafelistedApps: &[]pro.SafelistedApp{{Name: &name, BundleID: &bundleID}},
	}
}

// TestBuildParentSettingsInput_FullRoundTrip confirms a fully-authored plan
// lands in every owned request field — the required trio always from the plan,
// never from current — while the unmodeled allowTemplates is carried from
// current.
func TestBuildParentSettingsInput_FullRoundTrip(t *testing.T) {
	plan := JamfParentSettingsResourceModel{
		Enabled:                 types.BoolValue(false),
		Timezone:                types.StringValue("Europe/London"),
		DeviceGroupID:           types.Int64Value(42),
		AllowClearPasscode:      types.BoolValue(false),
		RevokeOnWipeAndReEnroll: types.BoolValue(false),
		RestrictedTimes: knownTimesMap(t, map[string]restrictedTimeModel{
			"MONDAY": {BeginTime: types.StringValue("08:30:00"), EndTime: types.StringValue("15:30:00")},
			"FRIDAY": {BeginTime: types.StringValue("09:00:00"), EndTime: types.StringValue("14:00:00")},
		}),
		SafelistedApps: knownAppSet(t,
			safelistedAppModel{Name: types.StringValue("Example App"), BundleID: types.StringValue("com.example.app")},
		),
	}

	out, diags := buildParentSettingsInput(context.Background(), plan, baseCurrent())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if out.TimezoneID != "Europe/London" {
		t.Errorf("TimezoneID = %q, want Europe/London (plan, never current)", out.TimezoneID)
	}
	if out.DeviceGroupID != 42 {
		t.Errorf("DeviceGroupID = %d, want 42 (plan, never current)", out.DeviceGroupID)
	}
	if out.IsEnabled != false {
		t.Errorf("IsEnabled = %v, want false (declared plan wins over current true)", out.IsEnabled)
	}
	if out.AllowClearPasscode == nil || *out.AllowClearPasscode != false {
		t.Errorf("AllowClearPasscode = %v, want false (declared plan wins)", out.AllowClearPasscode)
	}
	if out.DisassociateOnWipeAndReEnroll == nil || *out.DisassociateOnWipeAndReEnroll != false {
		t.Errorf("DisassociateOnWipeAndReEnroll = %v, want false (declared plan wins)", out.DisassociateOnWipeAndReEnroll)
	}
	if out.AllowTemplates == nil || *out.AllowTemplates != false {
		t.Errorf("AllowTemplates = %v, want the live false carried verbatim (§768.3)", out.AllowTemplates)
	}
	if len(out.RestrictedTimes) != 2 {
		t.Fatalf("RestrictedTimes = %v, want 2 entries from the plan (current's TUESDAY must not leak)", out.RestrictedTimes)
	}
	if _, leaked := out.RestrictedTimes["TUESDAY"]; leaked {
		t.Errorf("RestrictedTimes contains current's TUESDAY — the required map must come from the plan only")
	}
	mon, ok := out.RestrictedTimes["MONDAY"]
	if !ok || mon.BeginTime == nil || *mon.BeginTime != "08:30:00" || mon.EndTime == nil || *mon.EndTime != "15:30:00" {
		t.Errorf("RestrictedTimes[MONDAY] = %+v, want {08:30:00 15:30:00}", mon)
	}
	if out.SafelistedApps == nil || len(*out.SafelistedApps) != 1 {
		t.Fatalf("SafelistedApps = %v, want 1 entry", out.SafelistedApps)
	}
	app := (*out.SafelistedApps)[0]
	if app.Name == nil || *app.Name != "Example App" || app.BundleID == nil || *app.BundleID != "com.example.app" {
		t.Errorf("SafelistedApps[0] = {%v %v}, want {Example App com.example.app}", app.Name, app.BundleID)
	}
}

// TestBuildParentSettingsInput_AllowTemplatesCarriedOnUpdateBuild pins the
// §768.3 round-trip on the update path: even when UseStateForUnknown has made
// every owned field a known plan value (so current's owned fields are never
// consulted), the unmodeled allowTemplates must still be carried from current
// — a stored false must survive an update build (omitting it would let the
// full-replace PUT reset it to the server default true).
func TestBuildParentSettingsInput_AllowTemplatesCarriedOnUpdateBuild(t *testing.T) {
	// Fully-known plan — the USFU-filled update shape.
	plan := JamfParentSettingsResourceModel{
		Enabled:                 types.BoolValue(true),
		Timezone:                types.StringValue("UTC"),
		DeviceGroupID:           types.Int64Value(7),
		AllowClearPasscode:      types.BoolValue(true),
		RevokeOnWipeAndReEnroll: types.BoolValue(true),
		RestrictedTimes:         knownTimesMap(t, map[string]restrictedTimeModel{}),
		SafelistedApps:          emptyAppSet(t),
	}
	current := baseCurrent() // AllowTemplates = false (non-default)

	out, diags := buildParentSettingsInput(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if out.AllowTemplates == nil || *out.AllowTemplates != false {
		t.Errorf("AllowTemplates = %v, want the stored false carried verbatim on the update build", out.AllowTemplates)
	}
}

// TestBuildParentSettingsInput_OmittedFieldsAdoptCurrent confirms the
// GET-on-create merge: when an Optional+Computed field is omitted
// (null/unknown plan) the payload carries the live value forward rather than
// dropping it — so the full-replace write preserves undeclared fields on
// first create.
func TestBuildParentSettingsInput_OmittedFieldsAdoptCurrent(t *testing.T) {
	plan := JamfParentSettingsResourceModel{
		Enabled:                 types.BoolNull(),    // omitted -> adopt true
		AllowClearPasscode:      types.BoolUnknown(), // omitted -> adopt true
		RevokeOnWipeAndReEnroll: types.BoolNull(),    // omitted -> adopt true
		Timezone:                types.StringValue("Europe/London"),
		DeviceGroupID:           types.Int64Value(42),
		RestrictedTimes:         knownTimesMap(t, map[string]restrictedTimeModel{}),
		SafelistedApps:          types.SetUnknown(safelistedAppObjectType), // omitted -> adopt live entry
	}

	out, diags := buildParentSettingsInput(context.Background(), plan, baseCurrent())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if out.IsEnabled != true {
		t.Errorf("IsEnabled = %v, want adopted true", out.IsEnabled)
	}
	if out.AllowClearPasscode == nil || *out.AllowClearPasscode != true {
		t.Errorf("AllowClearPasscode = %v, want adopted true", out.AllowClearPasscode)
	}
	if out.DisassociateOnWipeAndReEnroll == nil || *out.DisassociateOnWipeAndReEnroll != true {
		t.Errorf("DisassociateOnWipeAndReEnroll = %v, want adopted true", out.DisassociateOnWipeAndReEnroll)
	}
	if out.SafelistedApps == nil || len(*out.SafelistedApps) != 1 {
		t.Fatalf("SafelistedApps = %v, want the adopted live entry", out.SafelistedApps)
	}
	got := (*out.SafelistedApps)[0]
	if got.Name == nil || *got.Name != "Live App" || got.BundleID == nil || *got.BundleID != "com.example.live" {
		t.Errorf("SafelistedApps[0] = {%v %v}, want the live entry verbatim", got.Name, got.BundleID)
	}
}

// TestBuildParentSettingsInput_DeclaredEmptySetWinsOverCurrent confirms a
// known empty plan set ([] = clear) is not overridden by the adopt merge base.
func TestBuildParentSettingsInput_DeclaredEmptySetWinsOverCurrent(t *testing.T) {
	plan := JamfParentSettingsResourceModel{
		Timezone:        types.StringValue("UTC"),
		DeviceGroupID:   types.Int64Value(42),
		RestrictedTimes: knownTimesMap(t, map[string]restrictedTimeModel{}),
		SafelistedApps:  emptyAppSet(t),
	}

	out, diags := buildParentSettingsInput(context.Background(), plan, baseCurrent())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if out.SafelistedApps == nil || len(*out.SafelistedApps) != 0 {
		t.Errorf("SafelistedApps = %v, want declared empty slice (clear wins over adopt)", out.SafelistedApps)
	}
}

// TestBuildParentSettingsInput_RestrictedTimesRoundTrip confirms the map
// expansion: declared days land verbatim with both time pointers populated,
// and an empty plan map yields a non-nil empty wire map ({} is the valid "no
// restrictions" shape — restrictedTimes carries no omitempty and a JSON null
// would be rejected).
func TestBuildParentSettingsInput_RestrictedTimesRoundTrip(t *testing.T) {
	plan := JamfParentSettingsResourceModel{
		Timezone:      types.StringValue("UTC"),
		DeviceGroupID: types.Int64Value(42),
		RestrictedTimes: knownTimesMap(t, map[string]restrictedTimeModel{
			"SUNDAY": {BeginTime: types.StringValue("00:00:00"), EndTime: types.StringValue("12:05:00")},
		}),
	}

	out, diags := buildParentSettingsInput(context.Background(), plan, baseCurrent())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(out.RestrictedTimes) != 1 {
		t.Fatalf("RestrictedTimes = %v, want exactly the declared SUNDAY entry", out.RestrictedTimes)
	}
	sun := out.RestrictedTimes["SUNDAY"]
	if sun.BeginTime == nil || *sun.BeginTime != "00:00:00" || sun.EndTime == nil || *sun.EndTime != "12:05:00" {
		t.Errorf("RestrictedTimes[SUNDAY] = %+v, want {00:00:00 12:05:00}", sun)
	}

	// Empty map: non-nil, zero entries.
	plan.RestrictedTimes = knownTimesMap(t, map[string]restrictedTimeModel{})
	out, diags = buildParentSettingsInput(context.Background(), plan, baseCurrent())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if out.RestrictedTimes == nil {
		t.Fatalf("RestrictedTimes = nil, want non-nil empty map ({} on the wire, not null)")
	}
	if len(out.RestrictedTimes) != 0 {
		t.Errorf("RestrictedTimes = %v, want empty", out.RestrictedTimes)
	}
}

// TestBuildParentSettingsInput_NilCurrentDefensive confirms the builder does
// not panic without a merge base (the CRUD handlers always supply one): owned
// omitted pointers drop, the non-pointer bool zero-fills, and allowTemplates
// is omitted so the server applies its default.
func TestBuildParentSettingsInput_NilCurrentDefensive(t *testing.T) {
	plan := JamfParentSettingsResourceModel{
		Timezone:        types.StringValue("UTC"),
		DeviceGroupID:   types.Int64Value(42),
		RestrictedTimes: knownTimesMap(t, map[string]restrictedTimeModel{}),
	}

	out, diags := buildParentSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if out.IsEnabled != false {
		t.Errorf("IsEnabled = %v, want false zero value", out.IsEnabled)
	}
	if out.AllowClearPasscode != nil || out.DisassociateOnWipeAndReEnroll != nil || out.AllowTemplates != nil {
		t.Errorf("optional bools must drop (nil) without a merge base")
	}
	if out.SafelistedApps != nil {
		t.Errorf("SafelistedApps = %v, want nil (dropped)", *out.SafelistedApps)
	}
}
