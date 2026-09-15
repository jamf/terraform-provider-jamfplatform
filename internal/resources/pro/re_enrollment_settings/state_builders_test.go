// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package re_enrollment_settings

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignReEnrollmentSettingsResourceModel_FullRoundTrip confirms a complete
// GET response lands in every state attribute.
func TestAssignReEnrollmentSettingsResourceModel_FullRoundTrip(t *testing.T) {
	wire := &pro.Reenrollment{
		FlushMDMQueue:                            "DELETE_ERRORS",
		IsFlushPolicyHistoryEnabled:              new(true),
		IsFlushLocationInformationEnabled:        new(false),
		IsFlushLocationInformationHistoryEnabled: new(true),
		IsFlushExtensionAttributesEnabled:        new(false),
		IsFlushSoftwareUpdatePlansEnabled:        new(true),
	}

	var state ReEnrollmentSettingsResourceModel
	assignReEnrollmentSettingsResourceModel(&state, wire)

	if state.ClearPolicyLogs.ValueBool() != true {
		t.Errorf("clear_policy_logs = %v, want true", state.ClearPolicyLogs.ValueBool())
	}
	if state.ClearLocationInformation.ValueBool() != false {
		t.Errorf("clear_location_information = %v, want false", state.ClearLocationInformation.ValueBool())
	}
	if state.ClearLocationInformationHistory.ValueBool() != true {
		t.Errorf("clear_location_information_history = %v, want true", state.ClearLocationInformationHistory.ValueBool())
	}
	if state.ClearExtensionAttributes.ValueBool() != false {
		t.Errorf("clear_extension_attributes = %v, want false", state.ClearExtensionAttributes.ValueBool())
	}
	if state.ClearSoftwareUpdatePlans.ValueBool() != true {
		t.Errorf("clear_software_update_plans = %v, want true", state.ClearSoftwareUpdatePlans.ValueBool())
	}
	if state.ClearManagementHistory.ValueString() != "DELETE_ERRORS" {
		t.Errorf("clear_management_history = %q, want DELETE_ERRORS", state.ClearManagementHistory.ValueString())
	}
}

// TestAssignReEnrollmentSettingsResourceModel_NilFlushFields confirms a nil
// *bool from the wire maps to a null state value rather than panicking.
func TestAssignReEnrollmentSettingsResourceModel_NilFlushFields(t *testing.T) {
	wire := &pro.Reenrollment{
		FlushMDMQueue: "",
	}
	var state ReEnrollmentSettingsResourceModel
	assignReEnrollmentSettingsResourceModel(&state, wire)

	if !state.ClearPolicyLogs.IsNull() {
		t.Errorf("clear_policy_logs should be null for nil wire pointer")
	}
	if !state.ClearManagementHistory.IsNull() {
		t.Errorf("clear_management_history should be null for empty wire string")
	}
}

// TestAssignReEnrollmentSettingsResourceModel_NilResponse is a no-op guard.
func TestAssignReEnrollmentSettingsResourceModel_NilResponse(t *testing.T) {
	var state ReEnrollmentSettingsResourceModel
	assignReEnrollmentSettingsResourceModel(&state, nil)
	if !state.ClearPolicyLogs.IsNull() {
		t.Errorf("nil response must leave state untouched (null)")
	}
}

// TestAssignReEnrollmentSettingsDataSourceModel_FullRoundTrip mirrors the
// resource assigner for the data source projection.
func TestAssignReEnrollmentSettingsDataSourceModel_FullRoundTrip(t *testing.T) {
	wire := &pro.Reenrollment{
		FlushMDMQueue:                     "DELETE_EVERYTHING",
		IsFlushPolicyHistoryEnabled:       new(false),
		IsFlushSoftwareUpdatePlansEnabled: new(true),
	}

	var state ReEnrollmentSettingsDataSourceModel
	assignReEnrollmentSettingsDataSourceModel(&state, wire)

	if state.ClearPolicyLogs.ValueBool() != false {
		t.Errorf("clear_policy_logs = %v, want false", state.ClearPolicyLogs.ValueBool())
	}
	if state.ClearSoftwareUpdatePlans.ValueBool() != true {
		t.Errorf("clear_software_update_plans = %v, want true", state.ClearSoftwareUpdatePlans.ValueBool())
	}
	if state.ClearManagementHistory.ValueString() != "DELETE_EVERYTHING" {
		t.Errorf("clear_management_history = %q, want DELETE_EVERYTHING", state.ClearManagementHistory.ValueString())
	}
}
