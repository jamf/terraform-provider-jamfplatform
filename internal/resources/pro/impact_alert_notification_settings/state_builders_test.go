// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact_alert_notification_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// fullResponse returns an ImpactAlertNotificationSettingsV1 with distinct bool values per
// field so a swapped mapping is caught.
func fullResponse() *pro.ImpactAlertNotificationSettingsV1 {
	return &pro.ImpactAlertNotificationSettingsV1{
		DeployableObjectsAlertEnabled:            true,
		DeployableObjectsConfirmationCodeEnabled: false,
		ScopeableObjectsAlertEnabled:             true,
		ScopeableObjectsConfirmationCodeEnabled:  false,
	}
}

func TestAssignImpactAlertNotificationSettingsResourceModel_AllFields(t *testing.T) {
	var state ImpactAlertNotificationSettingsResourceModel
	assignImpactAlertNotificationSettingsResourceModel(&state, fullResponse())

	checks := []struct {
		name string
		got  types.Bool
		want bool
	}{
		{"deployable_objects_alert_enabled", state.DeployableObjectsAlertEnabled, true},
		{"deployable_objects_confirmation_code_enabled", state.DeployableObjectsConfirmationCodeEnabled, false},
		{"scopeable_objects_alert_enabled", state.ScopeableObjectsAlertEnabled, true},
		{"scopeable_objects_confirmation_code_enabled", state.ScopeableObjectsConfirmationCodeEnabled, false},
	}
	for _, c := range checks {
		if c.got.IsNull() || c.got.IsUnknown() {
			t.Errorf("%s: expected concrete bool, got null/unknown", c.name)
			continue
		}
		if c.got.ValueBool() != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got.ValueBool(), c.want)
		}
	}
}

func TestAssignImpactAlertNotificationSettingsDataSourceModel_AllFields(t *testing.T) {
	var state ImpactAlertNotificationSettingsDataSourceModel
	assignImpactAlertNotificationSettingsDataSourceModel(&state, fullResponse())

	checks := []struct {
		name string
		got  types.Bool
		want bool
	}{
		{"deployable_objects_alert_enabled", state.DeployableObjectsAlertEnabled, true},
		{"deployable_objects_confirmation_code_enabled", state.DeployableObjectsConfirmationCodeEnabled, false},
		{"scopeable_objects_alert_enabled", state.ScopeableObjectsAlertEnabled, true},
		{"scopeable_objects_confirmation_code_enabled", state.ScopeableObjectsConfirmationCodeEnabled, false},
	}
	for _, c := range checks {
		if c.got.ValueBool() != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got.ValueBool(), c.want)
		}
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched.
func TestAssign_DoesNotClobberID(t *testing.T) {
	state := ImpactAlertNotificationSettingsResourceModel{ID: types.StringValue("singleton")}
	assignImpactAlertNotificationSettingsResourceModel(&state, fullResponse())
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := ImpactAlertNotificationSettingsDataSourceModel{ID: types.StringValue("singleton")}
	assignImpactAlertNotificationSettingsDataSourceModel(&dsState, fullResponse())
	if dsState.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered on data source: got %q, want %q", dsState.ID.ValueString(), "singleton")
	}
}

// TestSingletonIDConstant pins the import identifier.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
