// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_connect

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignJamfConnectResourceModel_EmptyVersionToNull(t *testing.T) {
	var state JamfConnectResourceModel
	assignJamfConnectResourceModel(&state, &pro.LinkedConnectProfile{
		UUID:               new("abc-123"),
		ProfileID:          new(47),
		ProfileName:        new("Connect Login"),
		ScopeDescription:   new("All Managed"),
		SiteID:             new("-1"),
		Version:            new(""),
		AutoDeploymentType: new(autoDeploymentNone),
	})

	if state.ProfileID.ValueInt64() != 47 {
		t.Errorf("profile_id = %d, want 47", state.ProfileID.ValueInt64())
	}
	if state.ID.ValueString() != "47" {
		t.Errorf("id = %q, want \"47\"", state.ID.ValueString())
	}
	if state.ConfigProfileUUID.ValueString() != "abc-123" {
		t.Errorf("config_profile_uuid = %q", state.ConfigProfileUUID.ValueString())
	}
	if !state.Version.IsNull() {
		t.Errorf("empty wire version must map to null, got %q", state.Version.ValueString())
	}
	if state.AutoDeploymentType.ValueString() != autoDeploymentNone {
		t.Errorf("auto_deployment_type = %q, want NONE", state.AutoDeploymentType.ValueString())
	}
	if state.ScopeDescription.ValueString() != "All Managed" {
		t.Errorf("scope_description = %q", state.ScopeDescription.ValueString())
	}
}

func TestAssignJamfConnectResourceModel_VersionPreserved(t *testing.T) {
	var state JamfConnectResourceModel
	assignJamfConnectResourceModel(&state, &pro.LinkedConnectProfile{
		UUID:               new("def-456"),
		ProfileID:          new(59),
		Version:            new("2.45.1"),
		AutoDeploymentType: new(autoDeploymentPatch),
	})
	if state.Version.ValueString() != "2.45.1" {
		t.Errorf("version = %q, want 2.45.1", state.Version.ValueString())
	}
	// Nil display fields normalise to null.
	if !state.ProfileName.IsNull() || !state.SiteID.IsNull() {
		t.Errorf("nil display fields must be null")
	}
}

func TestResolveStateProfileID(t *testing.T) {
	// From profile_id.
	if id, ok := resolveStateProfileID(JamfConnectResourceModel{ProfileID: types.Int64Value(47)}); !ok || id != 47 {
		t.Errorf("from profile_id: got (%d,%v), want (47,true)", id, ok)
	}
	// From id string (import path), profile_id null.
	if id, ok := resolveStateProfileID(JamfConnectResourceModel{ID: types.StringValue("59")}); !ok || id != 59 {
		t.Errorf("from id: got (%d,%v), want (59,true)", id, ok)
	}
	// Neither usable.
	if _, ok := resolveStateProfileID(JamfConnectResourceModel{ID: types.StringValue("not-a-number")}); ok {
		t.Errorf("non-numeric id must not resolve")
	}
}
