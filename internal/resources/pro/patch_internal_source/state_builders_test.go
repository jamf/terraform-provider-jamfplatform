// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_internal_source

import (
	"encoding/xml"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestPatchInternalSource_LiveWireUnmarshal pins the decode contract for the
// real /patchinternalsources wire shape.
func TestPatchInternalSource_LiveWireUnmarshal(t *testing.T) {
	wire := `<patch_internal_source><name>Jamf</name><endpoint>https://jamf-patch.jamfcloud.com/v1/</endpoint><enabled>true</enabled><id>1</id></patch_internal_source>`

	var p proclassic.PatchInternalSource
	if err := xml.Unmarshal([]byte(wire), &p); err != nil {
		t.Fatalf("unmarshal live wire: %v", err)
	}
	if p.Name == nil || *p.Name != "Jamf" {
		t.Errorf("Name mismatch: %v", p.Name)
	}
	if p.Endpoint == nil || *p.Endpoint != "https://jamf-patch.jamfcloud.com/v1/" {
		t.Errorf("Endpoint mismatch: %v", p.Endpoint)
	}
	if p.Enabled == nil || *p.Enabled != true {
		t.Errorf("Enabled mismatch: %v", p.Enabled)
	}
	if p.ID == nil || *p.ID != 1 {
		t.Errorf("ID mismatch: %v", p.ID)
	}
}

func TestAssignPatchInternalSourceDataSourceModel_PopulatesAll(t *testing.T) {
	id, name, endpoint := 1, "Jamf", "https://jamf-patch.jamfcloud.com/v1/"
	enabled := true
	state := PatchInternalSourceDataSourceModel{}
	api := &proclassic.PatchInternalSource{
		ID:       &id,
		Name:     &name,
		Endpoint: &endpoint,
		Enabled:  &enabled,
	}

	assignPatchInternalSourceDataSourceModel(&state, api)

	if state.ID.ValueString() != "1" {
		t.Errorf("ID: got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Jamf" {
		t.Errorf("Name: got %q", state.Name.ValueString())
	}
	if state.Endpoint.ValueString() != "https://jamf-patch.jamfcloud.com/v1/" {
		t.Errorf("Endpoint: got %q", state.Endpoint.ValueString())
	}
	if state.Enabled.ValueBool() != true {
		t.Errorf("Enabled: got %v", state.Enabled.ValueBool())
	}
}

func TestAssignPatchInternalSourceDataSourceModel_PreservesSelectorOnNilAPIFields(t *testing.T) {
	id := 1
	state := PatchInternalSourceDataSourceModel{
		ID:   types.StringNull(),
		Name: types.StringValue("Jamf"),
	}
	api := &proclassic.PatchInternalSource{ID: &id, Name: nil}

	assignPatchInternalSourceDataSourceModel(&state, api)

	if state.ID.ValueString() != "1" {
		t.Errorf("expected ID written, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Jamf" {
		t.Errorf("expected Name preserved as Jamf, got %q", state.Name.ValueString())
	}
}

func TestAssignPatchInternalSourceDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := PatchInternalSourceDataSourceModel{
		ID:   types.StringValue("1"),
		Name: types.StringValue("Jamf"),
	}
	assignPatchInternalSourceDataSourceModel(&state, nil)
	if state.ID.ValueString() != "1" || state.Name.ValueString() != "Jamf" {
		t.Errorf("expected state unchanged, got id=%q name=%q", state.ID.ValueString(), state.Name.ValueString())
	}
}
