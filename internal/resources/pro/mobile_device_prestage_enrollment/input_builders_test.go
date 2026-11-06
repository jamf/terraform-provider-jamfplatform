// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCreateStorageQuota(t *testing.T) {
	cases := []struct {
		name string
		in   types.Int64
		want int
	}{
		{"null floors to 1024", types.Int64Null(), minStorageQuotaMegabytes},
		{"unknown floors to 1024", types.Int64Unknown(), minStorageQuotaMegabytes},
		{"below floor 512 -> 1024", types.Int64Value(512), minStorageQuotaMegabytes},
		{"exact floor 1024", types.Int64Value(1024), 1024},
		{"above floor 4096", types.Int64Value(4096), 4096},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := createStorageQuota(tc.in); got != tc.want {
				t.Errorf("createStorageQuota(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestStringOrSentinel(t *testing.T) {
	if got := stringOrSentinel(types.StringNull(), "-1"); got != "-1" {
		t.Errorf("null -> sentinel: got %q", got)
	}
	if got := stringOrSentinel(types.StringUnknown(), "-1"); got != "-1" {
		t.Errorf("unknown -> sentinel: got %q", got)
	}
	if got := stringOrSentinel(types.StringValue(""), "-1"); got != "-1" {
		t.Errorf("empty -> sentinel: got %q", got)
	}
	if got := stringOrSentinel(types.StringValue("42"), "-1"); got != "42" {
		t.Errorf("value pass-through: got %q", got)
	}
}

func TestBuildSkipSetupItemsMap(t *testing.T) {
	if buildSkipSetupItemsMap(nil) != nil {
		t.Errorf("nil input must return nil map (so SDK omits the field)")
	}

	plan := &SkipSetupItemsModel{
		Biometric:           types.BoolValue(true),
		AppleID:             types.BoolValue(true),
		IMessageAndFaceTime: types.BoolValue(true),
		EnableLockdownMode:  types.BoolValue(false),
		TOS:                 types.BoolValue(false),
	}
	out := buildSkipSetupItemsMap(plan)
	if out == nil {
		t.Fatalf("expected non-nil map")
	}
	wire := *out
	cases := map[string]bool{
		"Biometric":           true,
		"AppleID":             true,
		"iMessageAndFaceTime": true, // wire-side lowercase 'i'
		"EnableLockdownMode":  false,
		"TOS":                 false,
	}
	for k, want := range cases {
		if got, ok := wire[k]; !ok || got != want {
			t.Errorf("wire key %q: want %v, got (ok=%v, val=%v)", k, want, ok, got)
		}
	}
	// All 47 wire keys must be present even when zero-valued in the model.
	if len(wire) != 47 {
		t.Errorf("map should always carry all 47 wire keys; got %d", len(wire))
	}
}

func TestBuildLocationInformation_FreshAndPopulated(t *testing.T) {
	fresh := buildLocationInformation(nil, sentinelNestedIDForCreate, 0)
	if fresh.ID != "-1" || fresh.VersionLock != 0 {
		t.Errorf("fresh: id/lock zero-value expected, got %+v", fresh)
	}
	if fresh.BuildingID != "-1" || fresh.DepartmentID != "-1" {
		t.Errorf("fresh: building/department sentinels expected, got %+v", fresh)
	}

	plan := &LocationInformationModel{
		Username:     types.StringValue("alice"),
		Realname:     types.StringValue("Alice Example"),
		BuildingID:   types.StringValue("3"),
		DepartmentID: types.StringValue("7"),
	}
	pop := buildLocationInformation(plan, "99", 4)
	if pop.ID != "99" || pop.VersionLock != 4 {
		t.Errorf("populated: id/lock not passed through")
	}
	if pop.Username != "alice" || pop.Realname != "Alice Example" {
		t.Errorf("populated: scalar fields not copied: %+v", pop)
	}
	if pop.BuildingID != "3" || pop.DepartmentID != "7" {
		t.Errorf("populated: building/department user values not preserved: %+v", pop)
	}
}

func TestBuildPurchasingInformation_DateDefaults(t *testing.T) {
	fresh := buildPurchasingInformation(nil, sentinelNestedIDForCreate, 0)
	if fresh.LeaseDate != sentinelDateUnset || fresh.PoDate != sentinelDateUnset || fresh.WarrantyDate != sentinelDateUnset {
		t.Errorf("fresh: date sentinels missing, got %+v", fresh)
	}

	plan := &PurchasingInformationModel{
		Leased:         types.BoolValue(true),
		Purchased:      types.BoolValue(false),
		LeaseDate:      types.StringValue("2025-01-01"),
		LifeExpectancy: types.Int64Value(5),
	}
	pop := buildPurchasingInformation(plan, "101", 3)
	if pop.ID != "101" || pop.VersionLock != 3 {
		t.Errorf("populated: id/lock not passed through")
	}
	if !pop.Leased || pop.Purchased {
		t.Errorf("populated: leased/purchased not copied: %+v", pop)
	}
	if pop.LifeExpectancy != 5 || pop.LeaseDate != "2025-01-01" {
		t.Errorf("populated: scalar fields not copied: %+v", pop)
	}
}

func TestBuildNames_NilSynthesizesDefault(t *testing.T) {
	out := buildNames(nil, false, true)
	if out == nil {
		t.Fatalf("buildNames(nil) must synthesize a populated object (empty names:{} 500s the server)")
	}
	if out.AssignNamesUsing == nil || *out.AssignNamesUsing != defaultAssignNamesUsing {
		t.Errorf("synthesized assign_names_using = %v, want %q", out.AssignNamesUsing, defaultAssignNamesUsing)
	}
	if out.ManageNames == nil || *out.ManageNames {
		t.Errorf("synthesized manage_names must be false, got %v", out.ManageNames)
	}
	if out.PrestageDeviceNames == nil {
		t.Errorf("synthesized prestage_device_names must be a non-nil (empty) slice")
	} else if len(*out.PrestageDeviceNames) != 0 {
		t.Errorf("synthesized prestage_device_names must be empty, got %d", len(*out.PrestageDeviceNames))
	}
	if out.DeviceNamingConfigured == nil || *out.DeviceNamingConfigured {
		t.Errorf("synthesized device_naming_configured must be an explicit false, got %v", out.DeviceNamingConfigured)
	}
}

// namingIntended decides deviceNamingConfigured, which the server stores
// verbatim rather than deriving — and which the admin UI keys the whole "Mobile
// device names" payload off.
func TestNamingIntended(t *testing.T) {
	tests := []struct {
		name  string
		names *NamesModel
		want  bool
		// wantBesidesMode is namingIntentBesidesMode's answer, which differs only
		// where assign_names_using is the sole evidence.
		wantBesidesMode bool
	}{
		{
			name:  "nil block",
			names: nil,
			want:  false,
		},
		{
			name:  "bare names = {} expresses no naming intent",
			names: &NamesModel{},
			want:  false,
		},
		{
			name:  "assign_names_using set",
			names: &NamesModel{AssignNamesUsing: types.StringValue("Serial Numbers")},
			want:  true,
			// Indistinguishable from the server's echo once in state.
			wantBesidesMode: false,
		},
		{
			name:            "assign_names_using set to Default Names is still a choice",
			names:           &NamesModel{AssignNamesUsing: types.StringValue("Default Names")},
			want:            true,
			wantBesidesMode: false,
		},
		{
			name:            "manage_names true",
			names:           &NamesModel{ManageNames: types.BoolValue(true)},
			want:            true,
			wantBesidesMode: true,
		},
		{
			name:  "manage_names explicitly false is not intent on its own",
			names: &NamesModel{ManageNames: types.BoolValue(false)},
			want:  false,
		},
		{
			name:            "device_name_prefix set",
			names:           &NamesModel{DeviceNamePrefix: types.StringValue("SSC-")},
			want:            true,
			wantBesidesMode: true,
		},
		{
			name:            "device_name_suffix set",
			names:           &NamesModel{DeviceNameSuffix: types.StringValue("-lab")},
			want:            true,
			wantBesidesMode: true,
		},
		{
			name:            "single_device_name set",
			names:           &NamesModel{SingleDeviceName: types.StringValue("Shared-iPad")},
			want:            true,
			wantBesidesMode: true,
		},
		{
			name: "prestage_device_names populated",
			names: &NamesModel{PrestageDeviceNames: []PrestageDeviceNameModel{
				{DeviceName: types.StringValue("iPad-1")},
			}},
			want:            true,
			wantBesidesMode: true,
		},
		{
			name:  "empty-string prefix is not intent",
			names: &NamesModel{DeviceNamePrefix: types.StringValue("")},
			want:  false,
		},
		{
			name:  "unknown sibling awaiting the post-apply GET is not intent",
			names: &NamesModel{AssignNamesUsing: types.StringUnknown()},
			want:  false,
		},
		{
			name: "the reported config: Serial Numbers + prefix + manage_names",
			names: &NamesModel{
				AssignNamesUsing: types.StringValue("Serial Numbers"),
				DeviceNamePrefix: types.StringValue("SSC-"),
				ManageNames:      types.BoolValue(true),
			},
			want:            true,
			wantBesidesMode: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := namingIntended(tc.names); got != tc.want {
				t.Errorf("namingIntended = %v, want %v", got, tc.want)
			}
			if got := namingIntentBesidesMode(tc.names); got != tc.wantBesidesMode {
				t.Errorf("namingIntentBesidesMode = %v, want %v", got, tc.wantBesidesMode)
			}
		})
	}
}

// buildNames must put deviceNamingConfigured on the wire unconditionally —
// omitting it is what made the naming payload invisible in the admin UI.
func TestBuildNames_AlwaysSerialisesDeviceNamingConfigured(t *testing.T) {
	for _, configured := range []bool{true, false} {
		for _, isCreate := range []bool{true, false} {
			for _, m := range []*NamesModel{nil, {}, {DeviceNamePrefix: types.StringValue("SSC-")}} {
				out := buildNames(m, configured, isCreate)
				if out.DeviceNamingConfigured == nil {
					t.Fatalf("configured=%v isCreate=%v names=%+v: must always serialise, got nil", configured, isCreate, m)
				}
				if *out.DeviceNamingConfigured != configured {
					t.Errorf("configured=%v isCreate=%v names=%+v: got %v", configured, isCreate, m, *out.DeviceNamingConfigured)
				}
			}
		}
	}
}

// Regression: deviceNamingConfigured must be derived from the CONFIG, not the
// plan. assign_names_using is Optional+Computed with UseStateForUnknown, so on
// update the plan carries the server's echoed mode even for a bare
// `names = {}` — deriving from the plan flipped an unconfigured PreStage to
// configured on any unrelated edit.
func TestBuildPutInput_NamingConfiguredComesFromConfigNotPlan(t *testing.T) {
	base := MobileDevicePrestageEnrollmentResourceModel{
		DisplayName:                       types.StringValue("bare names block"),
		DeviceEnrollmentProgramInstanceID: types.StringValue("1"),
	}

	// Config said `names = {}`; the plan holds the server's echoed mode.
	plan := base
	plan.Names = &NamesModel{AssignNamesUsing: types.StringValue("Serial Numbers")}
	cfg := base
	cfg.Names = &NamesModel{}

	put, d := buildPutInput(context.Background(), plan, cfg)
	if d.HasError() {
		t.Fatalf("put build diags: %v", d)
	}
	if put.Names == nil || put.Names.DeviceNamingConfigured == nil {
		t.Fatalf("names.deviceNamingConfigured must always serialise")
	}
	if *put.Names.DeviceNamingConfigured {
		t.Errorf("a bare `names = {}` config must stay unconfigured; the plan's echoed assign_names_using leaked through")
	}

	// Same plan, but the config really does ask for naming.
	cfg.Names = &NamesModel{AssignNamesUsing: types.StringValue("Serial Numbers")}
	put, d = buildPutInput(context.Background(), plan, cfg)
	if d.HasError() {
		t.Fatalf("put build diags: %v", d)
	}
	if !*put.Names.DeviceNamingConfigured {
		t.Errorf("config-declared naming must write deviceNamingConfigured=true")
	}
}

func TestBuildNames_CreateForcesSentinelID(t *testing.T) {
	plan := &NamesModel{
		AssignNamesUsing: types.StringValue("List of Names"),
		ManageNames:      types.BoolValue(true),
		PrestageDeviceNames: []PrestageDeviceNameModel{
			{DeviceName: types.StringValue("iPad-1"), ID: types.StringValue("42"), Used: types.BoolValue(true)},
			{DeviceName: types.StringValue("iPad-2"), ID: types.StringNull(), Used: types.BoolNull()},
		},
	}
	out := buildNames(plan, true, true)
	if out.AssignNamesUsing == nil || *out.AssignNamesUsing != "List of Names" {
		t.Errorf("assign_names_using not copied")
	}
	if out.ManageNames == nil || !*out.ManageNames {
		t.Errorf("manage_names not copied")
	}
	if out.PrestageDeviceNames == nil || len(*out.PrestageDeviceNames) != 2 {
		t.Fatalf("expected 2 prestage device names")
	}
	for i, el := range *out.PrestageDeviceNames {
		if el.ID == nil || *el.ID != sentinelNameIDForCreate {
			t.Errorf("element %d: isCreate=true must force id=%q, got %v", i, sentinelNameIDForCreate, el.ID)
		}
	}
	// `used` must serialise on every element (§F4b).
	if el := (*out.PrestageDeviceNames)[0]; el.Used == nil || !*el.Used {
		t.Errorf("element 0: used should round-trip true, got %v", el.Used)
	}
	if el := (*out.PrestageDeviceNames)[1]; el.Used == nil || *el.Used {
		t.Errorf("element 1: null used should serialise as false, got %v", el.Used)
	}
}

func TestBuildNames_UpdateEchoesModelID(t *testing.T) {
	plan := &NamesModel{
		AssignNamesUsing: types.StringValue("List of Names"),
		PrestageDeviceNames: []PrestageDeviceNameModel{
			{DeviceName: types.StringValue("iPad-1"), ID: types.StringValue("42")},
			{DeviceName: types.StringValue("iPad-2"), ID: types.StringNull()},
		},
	}
	out := buildNames(plan, true, false)
	if out.PrestageDeviceNames == nil || len(*out.PrestageDeviceNames) != 2 {
		t.Fatalf("expected 2 prestage device names")
	}
	if el := (*out.PrestageDeviceNames)[0]; el.ID == nil || *el.ID != "42" {
		t.Errorf("element 0: isCreate=false must echo the model id %q, got %v", "42", el.ID)
	}
	// null id on a not-yet-server-assigned element still falls back to "-1".
	if el := (*out.PrestageDeviceNames)[1]; el.ID == nil || *el.ID != sentinelNameIDForCreate {
		t.Errorf("element 1: null id must fall back to %q, got %v", sentinelNameIDForCreate, el.ID)
	}
}

func TestBuildPostInput_NamesAndNestedDefaults(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		DisplayName:                       types.StringValue("smoke"),
		DeviceEnrollmentProgramInstanceID: types.StringValue("1"),
		// Names omitted, storage unset, skip omitted.
	}
	post, d := buildPostInput(context.Background(), plan, plan)
	if d.HasError() {
		t.Fatalf("post build diags: %v", d)
	}
	if post.DisplayName != "smoke" {
		t.Errorf("display_name not copied")
	}

	// names must ALWAYS be a populated object on POST, even when plan.Names
	// is nil — the synthesized default block (§F2).
	if post.Names == nil {
		t.Fatalf("post.Names must be a populated object even when plan.Names is nil")
	}
	if post.Names.AssignNamesUsing == nil || *post.Names.AssignNamesUsing != defaultAssignNamesUsing {
		t.Errorf("synthesized names.assign_names_using = %v, want %q", post.Names.AssignNamesUsing, defaultAssignNamesUsing)
	}

	// location / purchasing fully populated with create sentinel id + lock 0.
	if post.LocationInformation.ID != sentinelNestedIDForCreate || post.LocationInformation.VersionLock != 0 {
		t.Errorf("location id/lock must be create sentinel %q + 0, got id=%q lock=%d",
			sentinelNestedIDForCreate, post.LocationInformation.ID, post.LocationInformation.VersionLock)
	}
	if post.PurchasingInformation.ID != sentinelNestedIDForCreate || post.PurchasingInformation.VersionLock != 0 {
		t.Errorf("purchasing id/lock must be create sentinel %q + 0, got id=%q lock=%d",
			sentinelNestedIDForCreate, post.PurchasingInformation.ID, post.PurchasingInformation.VersionLock)
	}

	// storage floors to 1024 when unset.
	if post.StorageQuotaSizeMegabytes != minStorageQuotaMegabytes {
		t.Errorf("unset storage must floor to %d, got %d", minStorageQuotaMegabytes, post.StorageQuotaSizeMegabytes)
	}

	// skip_setup_items omitted -> nil map.
	if post.SkipSetupItems != nil {
		t.Errorf("nil skip_setup_items must produce nil map, got %v", *post.SkipSetupItems)
	}
}

func TestBuildPostInput_SkipSetupItemsPopulated(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		DisplayName:                       types.StringValue("smoke"),
		DeviceEnrollmentProgramInstanceID: types.StringValue("1"),
		StorageQuotaSizeMegabytes:         types.Int64Value(2048),
		SkipSetupItems: &SkipSetupItemsModel{
			Biometric: types.BoolValue(true),
		},
	}
	post, d := buildPostInput(context.Background(), plan, plan)
	if d.HasError() {
		t.Fatalf("post build diags: %v", d)
	}
	if post.SkipSetupItems == nil {
		t.Fatalf("populated skip_setup_items must produce a non-nil map")
	}
	if len(*post.SkipSetupItems) != 47 {
		t.Errorf("skip_setup_items map must carry all 47 keys, got %d", len(*post.SkipSetupItems))
	}
	if post.StorageQuotaSizeMegabytes != 2048 {
		t.Errorf("storage 2048 must pass through, got %d", post.StorageQuotaSizeMegabytes)
	}
}

func TestBuildPutInput_Smoke(t *testing.T) {
	plan := MobileDevicePrestageEnrollmentResourceModel{
		DisplayName:                       types.StringValue("put"),
		DeviceEnrollmentProgramInstanceID: types.StringValue("1"),
	}
	put, d := buildPutInput(context.Background(), plan, plan)
	if d.HasError() {
		t.Fatalf("put build diags: %v", d)
	}
	if put.DisplayName != "put" {
		t.Errorf("display_name not copied to PUT")
	}
	// names must always be populated on PUT (full-replace).
	if put.Names == nil {
		t.Errorf("put.Names must be a populated object even when plan.Names is nil")
	}
	// versionLocks are NOT set in buildPutInput (caller injects them).
	if put.VersionLock != nil {
		t.Errorf("buildPutInput must leave VersionLock unset (caller injects), got %v", *put.VersionLock)
	}
	// anchor certificates serialise as a non-nil empty slice when omitted.
	if put.AnchorCertificates == nil {
		t.Errorf("anchor_certificates must serialise as a non-nil slice")
	}
}
