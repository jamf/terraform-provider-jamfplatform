// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignManagedSoftwareUpdateResourceModel_AllFields uses distinct values per field so a
// swapped mapping is caught, and confirms the SDK `toggle` maps to the `enabled` attribute.
func TestAssignManagedSoftwareUpdateResourceModel_AllFields(t *testing.T) {
	var state ManagedSoftwareUpdateResourceModel
	assignManagedSoftwareUpdateResourceModel(&state, &pro.ManagedSoftwareUpdatePlanToggle{
		Toggle:                       true,
		DssEnabled:                   new(true),
		RecipeEnabled:                new(false),
		ForceInstallLocalDateEnabled: new(true),
		CustomVersionEnabled:         new(false),
	})

	checks := []struct {
		name string
		got  types.Bool
		want bool
	}{
		{"enabled", state.Enabled, true},
		{"dss_enabled", state.DssEnabled, true},
		{"recipe_enabled", state.RecipeEnabled, false},
		{"force_install_local_date_enabled", state.ForceInstallLocalDateEnabled, true},
		{"custom_version_enabled", state.CustomVersionEnabled, false},
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

// TestAssignManagedSoftwareUpdateResourceModel_NilSubEnables verifies a missing *bool maps
// to a concrete false rather than null, keeping the Computed attribute deterministic.
func TestAssignManagedSoftwareUpdateResourceModel_NilSubEnables(t *testing.T) {
	var state ManagedSoftwareUpdateResourceModel
	assignManagedSoftwareUpdateResourceModel(&state, &pro.ManagedSoftwareUpdatePlanToggle{Toggle: false})

	for _, c := range []struct {
		name string
		got  types.Bool
	}{
		{"dss_enabled", state.DssEnabled},
		{"recipe_enabled", state.RecipeEnabled},
		{"force_install_local_date_enabled", state.ForceInstallLocalDateEnabled},
		{"custom_version_enabled", state.CustomVersionEnabled},
	} {
		if c.got.IsNull() || c.got.IsUnknown() {
			t.Errorf("%s: expected concrete false, got null/unknown", c.name)
			continue
		}
		if c.got.ValueBool() {
			t.Errorf("%s = true, want false for nil pointer", c.name)
		}
	}
}
