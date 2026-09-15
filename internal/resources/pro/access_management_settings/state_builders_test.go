// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package access_management_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// TestAssignResourceModel_ConcreteUUID pins that a non-empty wire UUID lands in state verbatim.
func TestAssignResourceModel_ConcreteUUID(t *testing.T) {
	state := AccessManagementSettingsResourceModel{}
	assignAccessManagementSettingsResourceModel(&state, &pro.AccessManagementSetting{
		AutomatedDeviceEnrollmentServerUUID: new("ABCD-1234-EF56"),
	})
	if state.AutomatedDeviceEnrollmentServerUUID.IsNull() || state.AutomatedDeviceEnrollmentServerUUID.ValueString() != "ABCD-1234-EF56" {
		t.Errorf("expected concrete UUID, got %#v", state.AutomatedDeviceEnrollmentServerUUID)
	}
}

// TestAssignResourceModel_EmptyWithNoPriorConfig pins that a fresh-tenant "" (no ADE
// server configured) with no prior user config maps to null.
func TestAssignResourceModel_EmptyWithNoPriorConfig(t *testing.T) {
	state := AccessManagementSettingsResourceModel{
		AutomatedDeviceEnrollmentServerUUID: types.StringNull(),
	}
	assignAccessManagementSettingsResourceModel(&state, &pro.AccessManagementSetting{
		AutomatedDeviceEnrollmentServerUUID: new(""),
	})
	if !state.AutomatedDeviceEnrollmentServerUUID.IsNull() {
		t.Errorf("expected null for empty wire value with no prior config, got %#v", state.AutomatedDeviceEnrollmentServerUUID)
	}
}

// TestAssignResourceModel_EmptyPreservesExplicitClear pins the consistency-critical case:
// when the user explicitly declared "" (to clear the setting), the empty wire value is
// kept as "" — not collapsed to null — so the planned value equals the post-apply state.
func TestAssignResourceModel_EmptyPreservesExplicitClear(t *testing.T) {
	state := AccessManagementSettingsResourceModel{
		AutomatedDeviceEnrollmentServerUUID: types.StringValue(""),
	}
	assignAccessManagementSettingsResourceModel(&state, &pro.AccessManagementSetting{
		AutomatedDeviceEnrollmentServerUUID: new(""),
	})
	if state.AutomatedDeviceEnrollmentServerUUID.IsNull() {
		t.Errorf("expected explicit \"\" to be preserved, got null")
	}
	if state.AutomatedDeviceEnrollmentServerUUID.ValueString() != "" {
		t.Errorf("expected \"\", got %q", state.AutomatedDeviceEnrollmentServerUUID.ValueString())
	}
}

// TestAssignResourceModel_NilPointer pins that a nil wire pointer maps to null.
func TestAssignResourceModel_NilPointer(t *testing.T) {
	state := AccessManagementSettingsResourceModel{
		AutomatedDeviceEnrollmentServerUUID: types.StringNull(),
	}
	assignAccessManagementSettingsResourceModel(&state, &pro.AccessManagementSetting{})
	if !state.AutomatedDeviceEnrollmentServerUUID.IsNull() {
		t.Errorf("expected null for nil wire pointer, got %#v", state.AutomatedDeviceEnrollmentServerUUID)
	}
}

// TestAssignResourceModel_NilResponse pins the nil-response guard (no panic, no mutation).
func TestAssignResourceModel_NilResponse(t *testing.T) {
	state := AccessManagementSettingsResourceModel{
		AutomatedDeviceEnrollmentServerUUID: types.StringValue("keep-me"),
	}
	assignAccessManagementSettingsResourceModel(&state, nil)
	if state.AutomatedDeviceEnrollmentServerUUID.ValueString() != "keep-me" {
		t.Errorf("nil response must not mutate state, got %q", state.AutomatedDeviceEnrollmentServerUUID.ValueString())
	}
}

// TestAssignDataSourceModel covers the data-source assigner (no prior-config nuance:
// empty wire value maps straight to null).
func TestAssignDataSourceModel(t *testing.T) {
	cases := []struct {
		name     string
		wire     *string
		wantNull bool
		want     string
	}{
		{"uuid", new("ABCD-1234"), false, "ABCD-1234"},
		{"empty", new(""), true, ""},
		{"nil", nil, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var state AccessManagementSettingsDataSourceModel
			assignAccessManagementSettingsDataSourceModel(&state, &pro.AccessManagementSetting{AutomatedDeviceEnrollmentServerUUID: c.wire})
			got := state.AutomatedDeviceEnrollmentServerUUID
			if c.wantNull {
				if !got.IsNull() {
					t.Errorf("expected null, got %#v", got)
				}
				return
			}
			if got.ValueString() != c.want {
				t.Errorf("got %q, want %q", got.ValueString(), c.want)
			}
		})
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched.
func TestAssign_DoesNotClobberID(t *testing.T) {
	state := AccessManagementSettingsResourceModel{ID: types.StringValue("singleton")}
	assignAccessManagementSettingsResourceModel(&state, &pro.AccessManagementSetting{AutomatedDeviceEnrollmentServerUUID: new("X")})
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := AccessManagementSettingsDataSourceModel{ID: types.StringValue("singleton")}
	assignAccessManagementSettingsDataSourceModel(&dsState, &pro.AccessManagementSetting{AutomatedDeviceEnrollmentServerUUID: new("X")})
	if dsState.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered on data source: got %q, want %q", dsState.ID.ValueString(), "singleton")
	}
}

// TestBuildInput covers the always-emit-concrete-string contract.
func TestBuildInput(t *testing.T) {
	t.Run("known value sent verbatim", func(t *testing.T) {
		body := buildAccessManagementSettingsInput(AccessManagementSettingsResourceModel{
			AutomatedDeviceEnrollmentServerUUID: types.StringValue("UUID-1"),
		}, nil)
		if body.AutomatedDeviceEnrollmentServerUUID == nil || *body.AutomatedDeviceEnrollmentServerUUID != "UUID-1" {
			t.Errorf("expected UUID-1, got %#v", body.AutomatedDeviceEnrollmentServerUUID)
		}
	})

	t.Run("explicit empty sent as empty string", func(t *testing.T) {
		body := buildAccessManagementSettingsInput(AccessManagementSettingsResourceModel{
			AutomatedDeviceEnrollmentServerUUID: types.StringValue(""),
		}, nil)
		if body.AutomatedDeviceEnrollmentServerUUID == nil || *body.AutomatedDeviceEnrollmentServerUUID != "" {
			t.Errorf("expected pointer to empty string, got %#v", body.AutomatedDeviceEnrollmentServerUUID)
		}
	})

	t.Run("unknown adopts current merge base", func(t *testing.T) {
		body := buildAccessManagementSettingsInput(AccessManagementSettingsResourceModel{
			AutomatedDeviceEnrollmentServerUUID: types.StringUnknown(),
		}, &pro.AccessManagementSetting{AutomatedDeviceEnrollmentServerUUID: new("LIVE-UUID")})
		if body.AutomatedDeviceEnrollmentServerUUID == nil || *body.AutomatedDeviceEnrollmentServerUUID != "LIVE-UUID" {
			t.Errorf("expected adopted LIVE-UUID, got %#v", body.AutomatedDeviceEnrollmentServerUUID)
		}
	})

	t.Run("null with no merge base emits empty string", func(t *testing.T) {
		body := buildAccessManagementSettingsInput(AccessManagementSettingsResourceModel{
			AutomatedDeviceEnrollmentServerUUID: types.StringNull(),
		}, nil)
		if body.AutomatedDeviceEnrollmentServerUUID == nil || *body.AutomatedDeviceEnrollmentServerUUID != "" {
			t.Errorf("expected pointer to empty string, got %#v", body.AutomatedDeviceEnrollmentServerUUID)
		}
	})
}

// TestSingletonIDConstant pins the import identifier.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
