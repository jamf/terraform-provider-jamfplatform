// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_teacher_settings

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

// TestBuildTeacherSettingsInput_FullRoundTrip confirms a fully-authored plan
// lands in every request field, with timezoneId always present.
func TestBuildTeacherSettingsInput_FullRoundTrip(t *testing.T) {
	plan := JamfTeacherSettingsResourceModel{
		Enabled:                       types.BoolValue(true),
		Timezone:                      types.StringValue("Europe/London"),
		RestrictionsEndTime:           types.StringValue("17:30:00"),
		MaximumRestrictionTimeSeconds: types.Int64Value(3600),
		SafelistedApps: knownAppSet(t,
			safelistedAppModel{Name: types.StringValue("Example App"), BundleID: types.StringValue("com.example.app")},
		),
	}

	out, diags := buildTeacherSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if out.TimezoneID == nil || *out.TimezoneID != "Europe/London" {
		t.Errorf("TimezoneID = %v, want Europe/London", out.TimezoneID)
	}
	if out.IsEnabled == nil || *out.IsEnabled != true {
		t.Errorf("IsEnabled = %v, want true", out.IsEnabled)
	}
	if out.AutoClear == nil || *out.AutoClear != "17:30:00" {
		t.Errorf("AutoClear = %v, want 17:30:00", out.AutoClear)
	}
	if out.MaxRestrictionLengthSeconds == nil || *out.MaxRestrictionLengthSeconds != 3600 {
		t.Errorf("MaxRestrictionLengthSeconds = %v, want 3600", out.MaxRestrictionLengthSeconds)
	}
	if out.SafelistedApps == nil || len(*out.SafelistedApps) != 1 {
		t.Fatalf("SafelistedApps = %v, want 1 entry", out.SafelistedApps)
	}
	app := (*out.SafelistedApps)[0]
	if app.Name == nil || *app.Name != "Example App" || app.BundleID == nil || *app.BundleID != "com.example.app" {
		t.Errorf("SafelistedApps[0] = {%v %v}, want {Example App com.example.app}", app.Name, app.BundleID)
	}
}

// TestBuildTeacherSettingsInput_ClearSemantics confirms the two explicit clear
// shapes: restrictions_end_time = "" is sent verbatim (the server persists it
// as null) and safelisted_apps = [] is sent as a non-nil empty slice (clears
// the collection on the full-replace write).
func TestBuildTeacherSettingsInput_ClearSemantics(t *testing.T) {
	plan := JamfTeacherSettingsResourceModel{
		Enabled:                       types.BoolValue(false),
		Timezone:                      types.StringValue("UTC"),
		RestrictionsEndTime:           types.StringValue(""),
		MaximumRestrictionTimeSeconds: types.Int64Value(0),
		SafelistedApps:                emptyAppSet(t),
	}

	out, diags := buildTeacherSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if out.AutoClear == nil || *out.AutoClear != "" {
		t.Errorf("AutoClear = %v, want pointer to \"\" (clear sentinel sent verbatim)", out.AutoClear)
	}
	if out.MaxRestrictionLengthSeconds == nil || *out.MaxRestrictionLengthSeconds != 0 {
		t.Errorf("MaxRestrictionLengthSeconds = %v, want 0 (0 is a meaningful value, not a drop)", out.MaxRestrictionLengthSeconds)
	}
	if out.SafelistedApps == nil {
		t.Fatalf("SafelistedApps = nil, want non-nil empty slice ([] clears)")
	}
	if len(*out.SafelistedApps) != 0 {
		t.Errorf("SafelistedApps = %v, want empty", *out.SafelistedApps)
	}
}

// TestBuildTeacherSettingsInput_OmittedFieldsDroppedWhenNoCurrent confirms that
// with no merge base (the update path) null/unknown optional fields produce nil
// pointers so omitempty drops them, while timezoneId is still always sent. On a
// real update UseStateForUnknown has already made omitted fields known prior
// values, so this path only fires when there is genuinely nothing to send.
func TestBuildTeacherSettingsInput_OmittedFieldsDroppedWhenNoCurrent(t *testing.T) {
	plan := JamfTeacherSettingsResourceModel{
		Enabled:                       types.BoolNull(),
		Timezone:                      types.StringValue("America/Chicago"),
		RestrictionsEndTime:           types.StringUnknown(),
		MaximumRestrictionTimeSeconds: types.Int64Null(),
		SafelistedApps:                types.SetNull(safelistedAppObjectType),
	}

	out, diags := buildTeacherSettingsInput(context.Background(), plan, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if out.TimezoneID == nil || *out.TimezoneID != "America/Chicago" {
		t.Errorf("TimezoneID = %v, want America/Chicago (always sent)", out.TimezoneID)
	}
	if out.IsEnabled != nil {
		t.Errorf("IsEnabled = %v, want nil (dropped)", *out.IsEnabled)
	}
	if out.AutoClear != nil {
		t.Errorf("AutoClear = %v, want nil (dropped)", *out.AutoClear)
	}
	if out.MaxRestrictionLengthSeconds != nil {
		t.Errorf("MaxRestrictionLengthSeconds = %v, want nil (dropped)", *out.MaxRestrictionLengthSeconds)
	}
	if out.SafelistedApps != nil {
		t.Errorf("SafelistedApps = %v, want nil (dropped)", *out.SafelistedApps)
	}
}

// TestBuildTeacherSettingsInput_OmittedFieldsAdoptCurrent confirms the
// GET-on-create merge: when a field is omitted (null/unknown plan) but a
// current settings read is supplied, the payload carries the live value forward
// rather than dropping it — so the full-replace write preserves undeclared
// fields on first create. A declared field still wins over current.
func TestBuildTeacherSettingsInput_OmittedFieldsAdoptCurrent(t *testing.T) {
	name, bundleID := "Live App", "com.example.live"
	current := &pro.TeacherSettingsResponse{
		IsEnabled:                   true,
		AutoClear:                   "08:15:00",
		MaxRestrictionLengthSeconds: 28740,
		TimezoneID:                  "Europe/Berlin",
		SafelistedApps:              []pro.SafelistedApp{{Name: &name, BundleID: &bundleID}},
	}
	plan := JamfTeacherSettingsResourceModel{
		Enabled:                       types.BoolValue(false), // declared -> plan wins over current true
		Timezone:                      types.StringValue("Europe/London"),
		RestrictionsEndTime:           types.StringNull(),                        // omitted -> adopt "08:15:00"
		MaximumRestrictionTimeSeconds: types.Int64Unknown(),                      // omitted -> adopt 28740
		SafelistedApps:                types.SetUnknown(safelistedAppObjectType), // omitted -> adopt live entry
	}

	out, diags := buildTeacherSettingsInput(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if out.IsEnabled == nil || *out.IsEnabled != false {
		t.Errorf("IsEnabled = %v, want false (declared plan wins over current)", out.IsEnabled)
	}
	if out.TimezoneID == nil || *out.TimezoneID != "Europe/London" {
		t.Errorf("TimezoneID = %v, want Europe/London (plan, never current)", out.TimezoneID)
	}
	if out.AutoClear == nil || *out.AutoClear != "08:15:00" {
		t.Errorf("AutoClear = %v, want adopted 08:15:00", out.AutoClear)
	}
	if out.MaxRestrictionLengthSeconds == nil || *out.MaxRestrictionLengthSeconds != 28740 {
		t.Errorf("MaxRestrictionLengthSeconds = %v, want adopted 28740", out.MaxRestrictionLengthSeconds)
	}
	if out.SafelistedApps == nil || len(*out.SafelistedApps) != 1 {
		t.Fatalf("SafelistedApps = %v, want the adopted live entry", out.SafelistedApps)
	}
	got := (*out.SafelistedApps)[0]
	if got.Name == nil || *got.Name != "Live App" || got.BundleID == nil || *got.BundleID != "com.example.live" {
		t.Errorf("SafelistedApps[0] = {%v %v}, want the live entry verbatim", got.Name, got.BundleID)
	}
}

// TestBuildTeacherSettingsInput_DeclaredEmptySetWinsOverCurrent confirms a
// known empty plan set ([] = clear) is not overridden by the adopt merge base.
func TestBuildTeacherSettingsInput_DeclaredEmptySetWinsOverCurrent(t *testing.T) {
	name, bundleID := "Live App", "com.example.live"
	current := &pro.TeacherSettingsResponse{
		TimezoneID:     "UTC",
		SafelistedApps: []pro.SafelistedApp{{Name: &name, BundleID: &bundleID}},
	}
	plan := JamfTeacherSettingsResourceModel{
		Timezone:       types.StringValue("UTC"),
		SafelistedApps: emptyAppSet(t),
	}

	out, diags := buildTeacherSettingsInput(context.Background(), plan, current)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if out.SafelistedApps == nil || len(*out.SafelistedApps) != 0 {
		t.Errorf("SafelistedApps = %v, want declared empty slice (clear wins over adopt)", out.SafelistedApps)
	}
}
