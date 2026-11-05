// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// liveWireKeys is the skipSetupItems key set Jamf Pro returned from GET /api/v3/mobile-device-prestages,
// captured 2026-11-05. A key the tenant adds or drops fails TestSkipItems_CoverTheLiveWireKeys.
var liveWireKeys = []string{
	"Biometric",
	"SoftwareUpdate",
	"Diagnostics",
	"iMessageAndFaceTime",
	"Intelligence",
	"TVRoom",
	"Passcode",
	"SIMSetup",
	"ScreenTime",
	"RestoreCompleted",
	"TVProviderSignIn",
	"Siri",
	"Restore",
	"ScreenSaver",
	"HomeButtonSensitivity",
	"CloudStorage",
	"LiquidGlass",
	"Privacy",
	"ActionButton",
	"DeviceFeaturesTour",
	"TransferData",
	"EnableLockdownMode",
	"Zoom",
	"PreferredLanguage",
	"VoiceSelection",
	"TVHomeScreenSync",
	"Safety",
	"SpokenLanguage",
	"TermsOfAddress",
	"ExpressLanguage",
	"CameraButton",
	"AppleID",
	"Keyboard",
	"Multitasking",
	"DisplayTone",
	"WatchMigration",
	"UpdateCompleted",
	"Appearance",
	"Android",
	"Payment",
	"OnBoarding",
	"TOS",
	"OSShowcase",
	"Welcome",
	"SafetyAndHandling",
	"TapToSetup",
	"Location",
}

func TestSkipItems_CoverTheLiveWireKeys(t *testing.T) {
	t.Parallel()
	got := make([]string, 0, len(skipItems))
	for _, it := range skipItems {
		got = append(got, it.wire)
	}
	want := slices.Clone(liveWireKeys)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("skipItems wire keys differ from the live tenant\n got: %v\nwant: %v", got, want)
	}
}

func TestSkipItems_NamesAreUniqueAndFieldsMatchTheirTags(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	tagOf := func(it skipItem, deprecated bool) string {
		m := &SkipSetupItemsModel{}
		if deprecated {
			*it.deprecated.field(m) = types.BoolValue(true)
		} else {
			*it.field(m) = types.BoolValue(true)
		}
		rt := reflect.TypeOf(*m)
		rv := reflect.ValueOf(*m)
		var tags []string
		for i := range rt.NumField() {
			if v, ok := rv.Field(i).Interface().(types.Bool); ok && !v.IsNull() {
				tags = append(tags, rt.Field(i).Tag.Get("tfsdk"))
			}
		}
		if len(tags) != 1 {
			t.Fatalf("%s: accessor must address exactly one model field, got %v", it.name, tags)
		}
		return tags[0]
	}
	for _, it := range skipItems {
		if seen[it.name] {
			t.Errorf("duplicate attribute %q", it.name)
		}
		seen[it.name] = true
		if got := tagOf(it, false); got != it.name {
			t.Errorf("%s: accessor writes the field tagged %q", it.name, got)
		}
		if it.deprecated != nil {
			if seen[it.deprecated.name] {
				t.Errorf("duplicate attribute %q", it.deprecated.name)
			}
			seen[it.deprecated.name] = true
			if got := tagOf(it, true); got != it.deprecated.name {
				t.Errorf("%s: deprecated accessor writes the field tagged %q", it.deprecated.name, got)
			}
		}
	}
	rt := reflect.TypeFor[SkipSetupItemsModel]()
	for field := range rt.Fields() {
		if tag := field.Tag.Get("tfsdk"); !seen[tag] {
			t.Errorf("model field %q has no row in skipItems", tag)
		}
	}
}

func TestSkipItems_NewPanesReachTheWire(t *testing.T) {
	t.Parallel()
	m := &SkipSetupItemsModel{}
	*skipItemFor(t, "liquid_glass").field(m) = types.BoolValue(true)
	*skipItemFor(t, "device_features_tour").field(m) = types.BoolValue(true)
	wire := *buildSkipSetupItemsMap(m)
	if !wire["LiquidGlass"] {
		t.Errorf("`liquid_glass` must write wire key LiquidGlass")
	}
	if !wire["DeviceFeaturesTour"] {
		t.Errorf("`device_features_tour` must write wire key DeviceFeaturesTour")
	}
}

func TestBuildSkipSetupItemsMap_RenamedPaneKeepsItsWireKey(t *testing.T) {
	t.Parallel()
	m := &SkipSetupItemsModel{TermsAndConditions: types.BoolValue(true)}
	if got := (*buildSkipSetupItemsMap(m))["TOS"]; !got {
		t.Errorf("`terms_and_conditions` must write wire key TOS")
	}
}

func TestBuildSkipSetupItemsMap_DeprecatedNameStillWrites(t *testing.T) {
	t.Parallel()
	m := &SkipSetupItemsModel{TOS: types.BoolValue(true)}
	if got := (*buildSkipSetupItemsMap(m))["TOS"]; !got {
		t.Errorf("`tos` must still write wire key TOS")
	}
}

func TestBuildSkipSetupItemsMap_ReplacementWinsOverDeprecated(t *testing.T) {
	t.Parallel()
	m := &SkipSetupItemsModel{TermsAndConditions: types.BoolValue(false), TOS: types.BoolValue(true)}
	if got := (*buildSkipSetupItemsMap(m))["TOS"]; got {
		t.Errorf("the replacement must win when a plan somehow carries both")
	}
}

func TestFlattenSkipSetupItems_DeprecatedNamesStayNullWithoutHistory(t *testing.T) {
	t.Parallel()
	wire := map[string]bool{"TOS": true}
	for name, prior := range map[string]*SkipSetupItemsModel{"nil prior": nil, "import sentinel": {}} {
		m := flattenSkipSetupItems(wire, prior)
		if !m.TermsAndConditions.ValueBool() {
			t.Errorf("%s: `terms_and_conditions` must read the wire value", name)
		}
		for _, it := range skipItems {
			if it.deprecated != nil && !it.deprecated.field(m).IsNull() {
				t.Errorf("%s: deprecated `%s` must stay null so import never emits it", name, it.deprecated.name)
			}
		}
	}
}

func TestFlattenSkipSetupItems_DeprecatedNameRefreshesWhenStateHoldsIt(t *testing.T) {
	t.Parallel()
	prior := &SkipSetupItemsModel{TOS: types.BoolValue(false)}
	m := flattenSkipSetupItems(map[string]bool{"TOS": true}, prior)
	if !m.TOS.Equal(types.BoolValue(true)) || !m.TermsAndConditions.Equal(types.BoolValue(true)) {
		t.Errorf("both names must report the wire value, got old=%v new=%v", m.TOS, m.TermsAndConditions)
	}
}

func TestSkipSetupItems_OnlyDeprecatedNameRoundTrips(t *testing.T) {
	t.Parallel()
	plan := &SkipSetupItemsModel{TOS: types.BoolValue(true), TermsAndConditions: types.BoolValue(true)}
	wire := *buildSkipSetupItemsMap(plan)
	got := flattenSkipSetupItems(wire, plan)
	if !got.TOS.Equal(plan.TOS) || !got.TermsAndConditions.Equal(plan.TermsAndConditions) {
		t.Errorf("a configuration using the deprecated name must read back unchanged, got old=%v new=%v", got.TOS, got.TermsAndConditions)
	}
}

func TestSkipSetupItems_EveryPaneRoundTripsThroughTheWire(t *testing.T) {
	t.Parallel()
	plan := &SkipSetupItemsModel{}
	for _, it := range skipItems {
		*it.field(plan) = types.BoolValue(true)
	}
	got := flattenSkipSetupItems(*buildSkipSetupItemsMap(plan), plan)
	for _, it := range skipItems {
		if !it.field(got).Equal(types.BoolValue(true)) {
			t.Errorf("%s did not survive build and flatten", it.name)
		}
	}
}

func TestSkipAliases_PairEveryDeprecatedAttribute(t *testing.T) {
	t.Parallel()
	want := 0
	for _, it := range skipItems {
		if it.deprecated != nil {
			want++
		}
	}
	if got := len(skipAliases()); got != want || got != 17 {
		t.Errorf("skipAliases has %d pairs, table has %d, expected 17", got, want)
	}
}

func TestSkipSetupItemsSchema_DeprecatedAttributesNameTheirReplacement(t *testing.T) {
	t.Parallel()
	attrs := skipSetupItemsSchema().Attributes
	for _, it := range skipItems {
		repl, ok := attrs[it.name]
		if !ok {
			t.Fatalf("replacement %q missing from schema", it.name)
		}
		if repl.GetDeprecationMessage() != "" {
			t.Errorf("%s is current and must not be deprecated", it.name)
		}
		if it.deprecated == nil {
			continue
		}
		old, ok := attrs[it.deprecated.name].(rschema.BoolAttribute)
		if !ok {
			t.Fatalf("deprecated %q missing from schema", it.deprecated.name)
		}
		msg := old.GetDeprecationMessage()
		if !strings.Contains(msg, "`"+it.name+"`") || !strings.Contains(msg, "2027-02-03") {
			t.Errorf("%s: notice must name %q and the removal date, got %q", it.deprecated.name, it.name, msg)
		}
		if !strings.Contains(old.GetMarkdownDescription(), msg) {
			t.Errorf("%s: the rendered description must repeat the notice, because tfplugindocs omits DeprecationMessage", it.deprecated.name)
		}
		if !old.Optional || !old.Computed {
			t.Errorf("%s must be Optional and Computed so existing state keeps planning clean", it.deprecated.name)
		}
		if len(old.Validators) == 0 {
			t.Errorf("%s must conflict with %s", it.deprecated.name, it.name)
		}
		if !strings.Contains(repl.GetMarkdownDescription(), "`"+it.deprecated.name+"`") {
			t.Errorf("%s: description must say which attribute it replaces", it.name)
		}
	}
}

func TestSkipSetupItemsSchema_IsPartOfTheResourceSchema(t *testing.T) {
	t.Parallel()
	resp := &resource.SchemaResponse{}
	NewMobileDevicePrestageEnrollmentResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	block, ok := resp.Schema.Attributes["skip_setup_items"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("skip_setup_items missing or not a nested attribute")
	}
	if len(block.Attributes) != len(skipItems)+len(skipAliases()) {
		t.Errorf("schema has %d attributes, table implies %d", len(block.Attributes), len(skipItems)+len(skipAliases()))
	}
}

func skipItemFor(t *testing.T, name string) skipItem {
	t.Helper()
	for _, it := range skipItems {
		if it.name == name {
			return it
		}
	}
	t.Fatalf("no skip item named %q", name)
	return skipItem{}
}
