// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package re_enrollment_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestBuildReenrollmentInput_FullRoundTrip confirms all six fields are
// populated from a fully-authored plan, with the bools sent as concrete
// pointers and the enum passed through verbatim.
func TestBuildReenrollmentInput_FullRoundTrip(t *testing.T) {
	plan := ReEnrollmentSettingsResourceModel{
		ClearPolicyLogs:                 types.BoolValue(true),
		ClearLocationInformation:        types.BoolValue(false),
		ClearLocationInformationHistory: types.BoolValue(true),
		ClearExtensionAttributes:        types.BoolValue(false),
		ClearSoftwareUpdatePlans:        types.BoolValue(true),
		ClearManagementHistory:          types.StringValue("DELETE_EVERYTHING"),
	}

	out := buildReenrollmentInput(plan, nil)

	if out.IsFlushPolicyHistoryEnabled == nil || *out.IsFlushPolicyHistoryEnabled != true {
		t.Errorf("IsFlushPolicyHistoryEnabled = %v, want true", out.IsFlushPolicyHistoryEnabled)
	}
	if out.IsFlushLocationInformationEnabled == nil || *out.IsFlushLocationInformationEnabled != false {
		t.Errorf("IsFlushLocationInformationEnabled = %v, want false", out.IsFlushLocationInformationEnabled)
	}
	if out.IsFlushLocationInformationHistoryEnabled == nil || *out.IsFlushLocationInformationHistoryEnabled != true {
		t.Errorf("IsFlushLocationInformationHistoryEnabled = %v, want true", out.IsFlushLocationInformationHistoryEnabled)
	}
	if out.IsFlushExtensionAttributesEnabled == nil || *out.IsFlushExtensionAttributesEnabled != false {
		t.Errorf("IsFlushExtensionAttributesEnabled = %v, want false", out.IsFlushExtensionAttributesEnabled)
	}
	if out.IsFlushSoftwareUpdatePlansEnabled == nil || *out.IsFlushSoftwareUpdatePlansEnabled != true {
		t.Errorf("IsFlushSoftwareUpdatePlansEnabled = %v, want true", out.IsFlushSoftwareUpdatePlansEnabled)
	}
	if out.FlushMDMQueue != "DELETE_EVERYTHING" {
		t.Errorf("FlushMDMQueue = %q, want DELETE_EVERYTHING", out.FlushMDMQueue)
	}
}

// TestBuildReenrollmentInput_OmittedBoolsDroppedWhenNoCurrent confirms that with
// no merge base (current nil) a null/unknown toggle produces a nil pointer so
// omitempty drops it. This is the update path: UseStateForUnknown has already made
// omitted toggles known prior values (FullRoundTrip), so a still-null/unknown plan
// value genuinely has nothing to send.
func TestBuildReenrollmentInput_OmittedBoolsDroppedWhenNoCurrent(t *testing.T) {
	plan := ReEnrollmentSettingsResourceModel{
		ClearPolicyLogs:                 types.BoolNull(),
		ClearLocationInformation:        types.BoolUnknown(),
		ClearLocationInformationHistory: types.BoolNull(),
		ClearExtensionAttributes:        types.BoolNull(),
		ClearSoftwareUpdatePlans:        types.BoolUnknown(),
		ClearManagementHistory:          types.StringValue("DELETE_NOTHING"),
	}

	out := buildReenrollmentInput(plan, nil)

	for name, p := range map[string]*bool{
		"IsFlushPolicyHistoryEnabled":              out.IsFlushPolicyHistoryEnabled,
		"IsFlushLocationInformationEnabled":        out.IsFlushLocationInformationEnabled,
		"IsFlushLocationInformationHistoryEnabled": out.IsFlushLocationInformationHistoryEnabled,
		"IsFlushExtensionAttributesEnabled":        out.IsFlushExtensionAttributesEnabled,
		"IsFlushSoftwareUpdatePlansEnabled":        out.IsFlushSoftwareUpdatePlansEnabled,
	} {
		if p != nil {
			t.Errorf("%s = %v, want nil pointer (dropped) for null/unknown plan input", name, *p)
		}
	}
}

// TestBuildReenrollmentInput_OmittedBoolsAdoptCurrent confirms the GET-on-create
// merge: when a toggle is omitted (null/unknown plan) but a current settings read
// is supplied, the payload carries the current value forward rather than dropping
// it — so the full-replace write preserves undeclared toggles on first create.
// A declared toggle still wins over current.
func TestBuildReenrollmentInput_OmittedBoolsAdoptCurrent(t *testing.T) {
	tr, fa := true, false
	current := &pro.Reenrollment{
		IsFlushPolicyHistoryEnabled:              &tr, // omitted in plan -> adopt true
		IsFlushLocationInformationEnabled:        &tr, // declared false in plan -> plan wins
		IsFlushLocationInformationHistoryEnabled: &fa, // omitted -> adopt false
		IsFlushExtensionAttributesEnabled:        &tr, // omitted -> adopt true
		IsFlushSoftwareUpdatePlansEnabled:        &tr, // omitted -> adopt true
	}
	plan := ReEnrollmentSettingsResourceModel{
		ClearPolicyLogs:                 types.BoolNull(),
		ClearLocationInformation:        types.BoolValue(false),
		ClearLocationInformationHistory: types.BoolUnknown(),
		ClearExtensionAttributes:        types.BoolNull(),
		ClearSoftwareUpdatePlans:        types.BoolNull(),
		ClearManagementHistory:          types.StringValue("DELETE_NOTHING"),
	}

	out := buildReenrollmentInput(plan, current)

	want := map[string]struct {
		got  *bool
		want bool
	}{
		"IsFlushPolicyHistoryEnabled":              {out.IsFlushPolicyHistoryEnabled, true},
		"IsFlushLocationInformationEnabled":        {out.IsFlushLocationInformationEnabled, false}, // plan wins
		"IsFlushLocationInformationHistoryEnabled": {out.IsFlushLocationInformationHistoryEnabled, false},
		"IsFlushExtensionAttributesEnabled":        {out.IsFlushExtensionAttributesEnabled, true},
		"IsFlushSoftwareUpdatePlansEnabled":        {out.IsFlushSoftwareUpdatePlansEnabled, true},
	}
	for name, c := range want {
		if c.got == nil {
			t.Errorf("%s = nil, want %v (adopt current / plan)", name, c.want)
			continue
		}
		if *c.got != c.want {
			t.Errorf("%s = %v, want %v", name, *c.got, c.want)
		}
	}
}

// TestBuildReenrollmentInput_EnumPassthrough confirms the Required
// clear_management_history enum is sent verbatim. It is always known (Required),
// so there is no default-substitution path.
func TestBuildReenrollmentInput_EnumPassthrough(t *testing.T) {
	for _, v := range validClearManagementHistory {
		t.Run(v, func(t *testing.T) {
			plan := ReEnrollmentSettingsResourceModel{ClearManagementHistory: types.StringValue(v)}
			out := buildReenrollmentInput(plan, nil)
			if out.FlushMDMQueue != v {
				t.Errorf("FlushMDMQueue = %q, want %q", out.FlushMDMQueue, v)
			}
		})
	}
}
