// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact_alert_notification_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestBuildImpactAlertNotificationSettingsInput verifies every plan field maps to the
// correct SDK field. Distinct bool values per field catch a swapped mapping.
func TestBuildImpactAlertNotificationSettingsInput(t *testing.T) {
	plan := ImpactAlertNotificationSettingsResourceModel{
		DeployableObjectsAlertEnabled:            types.BoolValue(true),
		DeployableObjectsConfirmationCodeEnabled: types.BoolValue(false),
		ScopeableObjectsAlertEnabled:             types.BoolValue(true),
		ScopeableObjectsConfirmationCodeEnabled:  types.BoolValue(false),
	}

	out := buildImpactAlertNotificationSettingsInput(plan, nil)

	checks := []struct {
		name string
		got  bool
		want bool
	}{
		{"DeployableObjectsAlertEnabled", out.DeployableObjectsAlertEnabled, true},
		{"DeployableObjectsConfirmationCodeEnabled", out.DeployableObjectsConfirmationCodeEnabled, false},
		{"ScopeableObjectsAlertEnabled", out.ScopeableObjectsAlertEnabled, true},
		{"ScopeableObjectsConfirmationCodeEnabled", out.ScopeableObjectsConfirmationCodeEnabled, false},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestBuildImpactAlertNotificationSettingsInput_OmittedAdoptsCurrent verifies the
// GET-on-create merge: a toggle omitted from the plan (Unknown/Null, as on first create)
// takes the value from the live settings `current` rather than defaulting to false, so
// the singleton is adopted, not reset. A declared toggle still wins over current.
func TestBuildImpactAlertNotificationSettingsInput_OmittedAdoptsCurrent(t *testing.T) {
	current := &pro.ImpactAlertNotificationSettingsV1{
		DeployableObjectsAlertEnabled:            true,  // omitted in plan -> adopt true
		DeployableObjectsConfirmationCodeEnabled: true,  // omitted -> adopt true
		ScopeableObjectsAlertEnabled:             true,  // declared false in plan -> plan wins
		ScopeableObjectsConfirmationCodeEnabled:  false, // omitted -> adopt false
	}
	plan := ImpactAlertNotificationSettingsResourceModel{
		DeployableObjectsAlertEnabled:            types.BoolUnknown(),
		DeployableObjectsConfirmationCodeEnabled: types.BoolNull(),
		ScopeableObjectsAlertEnabled:             types.BoolValue(false), // declared
		ScopeableObjectsConfirmationCodeEnabled:  types.BoolUnknown(),
	}

	out := buildImpactAlertNotificationSettingsInput(plan, current)

	checks := []struct {
		name string
		got  bool
		want bool
	}{
		{"DeployableObjectsAlertEnabled", out.DeployableObjectsAlertEnabled, true},
		{"DeployableObjectsConfirmationCodeEnabled", out.DeployableObjectsConfirmationCodeEnabled, true},
		{"ScopeableObjectsAlertEnabled (plan wins)", out.ScopeableObjectsAlertEnabled, false},
		{"ScopeableObjectsConfirmationCodeEnabled", out.ScopeableObjectsConfirmationCodeEnabled, false},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestBuildImpactAlertNotificationSettingsInput_NilCurrentOmittedFalse verifies that with
// a nil merge base (update path) an omitted toggle falls back to false. In practice this
// never happens on update because UseStateForUnknown fills omitted toggles with their
// prior known value before the plan reaches the builder.
func TestBuildImpactAlertNotificationSettingsInput_NilCurrentOmittedFalse(t *testing.T) {
	plan := ImpactAlertNotificationSettingsResourceModel{
		DeployableObjectsAlertEnabled:            types.BoolUnknown(),
		DeployableObjectsConfirmationCodeEnabled: types.BoolUnknown(),
		ScopeableObjectsAlertEnabled:             types.BoolValue(true),
		ScopeableObjectsConfirmationCodeEnabled:  types.BoolNull(),
	}

	out := buildImpactAlertNotificationSettingsInput(plan, nil)

	if out.DeployableObjectsAlertEnabled {
		t.Errorf("omitted toggle with nil current should be false")
	}
	if !out.ScopeableObjectsAlertEnabled {
		t.Errorf("declared true toggle should be true")
	}
}
