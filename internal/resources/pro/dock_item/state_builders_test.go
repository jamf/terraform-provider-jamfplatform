// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignDockItemResourceModel_FullPayload(t *testing.T) {
	state := DockItemResourceModel{}
	plist := "<dict><key>tile-type</key><string>file-tile</string></dict>"
	api := &proclassic.DockItem{
		ID:       new(71),
		Name:     new("App"),
		Type:     new("App"),
		Path:     new("/My/App.app"),
		Contents: &plist,
	}

	diags := assignDockItemResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "71" {
		t.Errorf("expected ID=71, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "App" {
		t.Errorf("expected Name=App, got %q", state.Name.ValueString())
	}
	if state.Type.ValueString() != "App" {
		t.Errorf("expected Type=App, got %q", state.Type.ValueString())
	}
	if state.Path.ValueString() != "/My/App.app" {
		t.Errorf("expected Path=/My/App.app, got %q", state.Path.ValueString())
	}
	if state.Contents.ValueString() != plist {
		t.Errorf("expected Contents PLIST round-tripped, got %q", state.Contents.ValueString())
	}
}

func TestAssignDockItemResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := DockItemResourceModel{ID: types.StringValue("17")}
	api := &proclassic.DockItem{ID: nil}

	diags := assignDockItemResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "17" {
		t.Errorf("expected state.ID preserved as %q, got %q", "17", state.ID.ValueString())
	}
}

func TestAssignDockItemResourceModel_NilAPIIsNoop(t *testing.T) {
	state := DockItemResourceModel{
		ID:   types.StringValue("11"),
		Name: types.StringValue("Preset"),
	}
	diags := assignDockItemResourceModel(&state, nil)
	if diags.HasError() {
		t.Fatalf("nil API must not error, got %v", diags)
	}
	if state.ID.ValueString() != "11" || state.Name.ValueString() != "Preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}

func TestAssignDockItemResourceModel_NilContentsBecomesNull(t *testing.T) {
	// Server omitted <contents> on this read (defensive — UI says it is
	// always populated, but the SDK exposes Contents as *string so we
	// handle the nil case gracefully).
	state := DockItemResourceModel{}
	api := &proclassic.DockItem{
		ID:       new(42),
		Name:     new("Bare"),
		Type:     new("App"),
		Path:     new("/Applications/Bare.app"),
		Contents: nil,
	}

	diags := assignDockItemResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Contents.IsNull() {
		t.Errorf("expected Contents=null when API returns nil, got %q", state.Contents.ValueString())
	}
}

func TestAssignDockItemDataSourceModel_FullPayload(t *testing.T) {
	state := DockItemDataSourceModel{}
	api := &proclassic.DockItem{
		ID:   new(72),
		Name: new("Folder"),
		Type: new("Folder"),
		Path: new("~/Downloads"),
	}

	diags := assignDockItemDataSourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "72" {
		t.Errorf("expected ID=72, got %q", state.ID.ValueString())
	}
	if state.Type.ValueString() != "Folder" {
		t.Errorf("expected Type=Folder, got %q", state.Type.ValueString())
	}
	if state.Path.ValueString() != "~/Downloads" {
		t.Errorf("expected Path=~/Downloads, got %q", state.Path.ValueString())
	}
}

func TestAssignDockItemDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := DockItemDataSourceModel{ID: types.StringValue("9")}
	diags := assignDockItemDataSourceModel(&state, nil)
	if diags.HasError() {
		t.Fatalf("nil API must not error, got %v", diags)
	}
	if state.ID.ValueString() != "9" {
		t.Errorf("expected state preserved, got %q", state.ID.ValueString())
	}
}
