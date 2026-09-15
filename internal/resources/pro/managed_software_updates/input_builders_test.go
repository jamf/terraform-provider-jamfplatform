// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestBuildManagedSoftwareUpdateInput_DeclaredValueWins verifies a declared (known) enabled
// value is sent verbatim and the merge base is ignored.
func TestBuildManagedSoftwareUpdateInput_DeclaredValueWins(t *testing.T) {
	in := buildManagedSoftwareUpdateInput(
		ManagedSoftwareUpdateResourceModel{Enabled: types.BoolValue(true)},
		&pro.ManagedSoftwareUpdatePlanToggle{Toggle: false},
	)
	if !in.Toggle {
		t.Errorf("expected declared enabled=true to win, got %v", in.Toggle)
	}
}

// TestBuildManagedSoftwareUpdateInput_AdoptsCurrentWhenUnknown verifies an omitted value on
// first create (unknown, no prior state) adopts the live value via the merge base.
func TestBuildManagedSoftwareUpdateInput_AdoptsCurrentWhenUnknown(t *testing.T) {
	in := buildManagedSoftwareUpdateInput(
		ManagedSoftwareUpdateResourceModel{Enabled: types.BoolUnknown()},
		&pro.ManagedSoftwareUpdatePlanToggle{Toggle: true},
	)
	if !in.Toggle {
		t.Errorf("expected adopted enabled=true from merge base, got %v", in.Toggle)
	}
}

// TestBuildManagedSoftwareUpdateInput_NeverEmitsSubEnables verifies the server-coerced
// sub-enables are never authored on the write — they stay nil (omitted on the wire).
func TestBuildManagedSoftwareUpdateInput_NeverEmitsSubEnables(t *testing.T) {
	in := buildManagedSoftwareUpdateInput(ManagedSoftwareUpdateResourceModel{Enabled: types.BoolValue(true)}, nil)
	if in.DssEnabled != nil || in.RecipeEnabled != nil || in.ForceInstallLocalDateEnabled != nil || in.CustomVersionEnabled != nil {
		t.Errorf("sub-enables must not be emitted on the write; got dss=%v recipe=%v force=%v custom=%v",
			in.DssEnabled, in.RecipeEnabled, in.ForceInstallLocalDateEnabled, in.CustomVersionEnabled)
	}
}
