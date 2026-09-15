// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignPrinterResourceModel_FullPayload(t *testing.T) {
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:             new(70),
		Name:           new("Lab Color"),
		Category:       new("Printers"),
		URI:            new("ipp://printer.lab.example.com/queue1"),
		CUPSName:       new("lab_color"),
		Location:       new("B5, F2"),
		Model:          new("HP DeskJet 2600 series"),
		Info:           new("info field"),
		Notes:          new("notes field"),
		MakeDefault:    new(true),
		UseGeneric:     new(false),
		Ppd:            new("HP DeskJet 2600 series.ppd"),
		PpdPath:        new("/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd"),
		PpdContents:    new("*PPD-Adobe: \"4.3\""),
		Shared:         new(true),
		OsRequirements: new("13.5, 16.0"),
	}

	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "70" {
		t.Errorf("expected ID=70, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Lab Color" {
		t.Errorf("expected Name=Lab Color, got %q", state.Name.ValueString())
	}
	if state.Category.ValueString() != "Printers" {
		t.Errorf("expected Category=Printers, got %q", state.Category.ValueString())
	}
	if !state.MakeDefault.ValueBool() {
		t.Errorf("expected MakeDefault=true, got %v", state.MakeDefault)
	}
	if state.UseGeneric.ValueBool() {
		t.Errorf("expected UseGeneric=false, got %v", state.UseGeneric)
	}
	if !state.Shared.ValueBool() {
		t.Errorf("expected Shared=true, got %v", state.Shared)
	}
	if state.OSRequirements.ValueString() != "13.5, 16.0" {
		t.Errorf("expected OSRequirements, got %q", state.OSRequirements.ValueString())
	}
}

// TestAssignPrinterResourceModel_GenericCollapsesPPDTrio asserts that when the
// server reports use_generic=true it still echoes the bundled Generic.ppd path,
// the state builder nulls the whole PPD trio. The cross-field validator forbids
// ppd/ppd_path/ppd_contents in generic mode, so a faithful read must collapse the
// echo to round-trip.
func TestAssignPrinterResourceModel_GenericCollapsesPPDTrio(t *testing.T) {
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:          new(67),
		Name:        new("Printer"),
		UseGeneric:  new(true),
		Ppd:         new("Generic.ppd"),
		PpdPath:     new("/System/Library/Frameworks/ApplicationServices.framework/Versions/A/Frameworks/PrintCore.framework/Resources/Generic.ppd"),
		PpdContents: new("*PPD-Adobe: \"4.3\""),
	}
	if diags := assignPrinterResourceModel(&state, api); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.PPD.IsNull() {
		t.Errorf("ppd must be null in generic mode, got %q", state.PPD.ValueString())
	}
	if !state.PPDPath.IsNull() {
		t.Errorf("ppd_path must be null in generic mode, got %q", state.PPDPath.ValueString())
	}
	if !state.PPDContents.IsNull() {
		t.Errorf("ppd_contents must be null in generic mode, got %q", state.PPDContents.ValueString())
	}
}

// TestAssignPrinterResourceModel_NonGenericKeepsPPDPath asserts the PPD trio is
// preserved when use_generic=false.
func TestAssignPrinterResourceModel_NonGenericKeepsPPDPath(t *testing.T) {
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:         new(70),
		Name:       new("HP"),
		UseGeneric: new(false),
		PpdPath:    new("/Library/Printers/PPDs/Contents/Resources/HP.ppd"),
	}
	if diags := assignPrinterResourceModel(&state, api); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.PPDPath.ValueString() != "/Library/Printers/PPDs/Contents/Resources/HP.ppd" {
		t.Errorf("ppd_path must be preserved in non-generic mode, got %q", state.PPDPath.ValueString())
	}
}

// TestAssignPrinterResourceModel_CategorySentinelDecodes verifies the wire
// sentinel `categoryUnassignedSentinel` rounds-trips to null in TF state.
func TestAssignPrinterResourceModel_CategorySentinelDecodes(t *testing.T) {
	sentinel := categoryUnassignedSentinel
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:       new(71),
		Name:     new("Minimal"),
		Category: &sentinel,
	}
	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Category.IsNull() {
		t.Errorf("expected Category to decode sentinel to null, got %q", state.Category.ValueString())
	}
}

func TestAssignPrinterResourceModel_CategoryEmptyDecodes(t *testing.T) {
	empty := ""
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:       new(71),
		Name:     new("Minimal"),
		Category: &empty,
	}
	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Category.IsNull() {
		t.Errorf("expected empty Category to decode to null, got %q", state.Category.ValueString())
	}
}

func TestAssignPrinterResourceModel_CategoryNilDecodes(t *testing.T) {
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:       new(71),
		Name:     new("Minimal"),
		Category: nil,
	}
	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Category.IsNull() {
		t.Errorf("expected nil Category to decode to null, got %q", state.Category.ValueString())
	}
}

// TestAssignPrinterResourceModel_SharedFalseDecodes ensures the literal
// "false" wire string decodes to a TF Bool false (and not null).
func TestAssignPrinterResourceModel_SharedFalseDecodes(t *testing.T) {
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:     new(71),
		Name:   new("X"),
		Shared: new(false),
	}
	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Shared.IsNull() {
		t.Errorf("expected Shared=false, got null")
	}
	if state.Shared.ValueBool() {
		t.Errorf("expected Shared=false, got true")
	}
}

func TestAssignPrinterResourceModel_SharedNilDecodes(t *testing.T) {
	state := PrinterResourceModel{}
	api := &proclassic.Printer{
		ID:     new(71),
		Name:   new("X"),
		Shared: nil,
	}
	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Shared.IsNull() {
		t.Errorf("expected nil Shared to decode to null, got %v", state.Shared)
	}
}

func TestAssignPrinterResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := PrinterResourceModel{ID: types.StringValue("17")}
	api := &proclassic.Printer{ID: nil}

	diags := assignPrinterResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "17" {
		t.Errorf("expected state.ID preserved as %q, got %q", "17", state.ID.ValueString())
	}
}

func TestAssignPrinterResourceModel_NilAPIIsNoop(t *testing.T) {
	state := PrinterResourceModel{
		ID:   types.StringValue("11"),
		Name: types.StringValue("Preset"),
	}
	diags := assignPrinterResourceModel(&state, nil)
	if diags.HasError() {
		t.Fatalf("nil API must not error, got %v", diags)
	}
	if state.ID.ValueString() != "11" || state.Name.ValueString() != "Preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}

func TestAssignPrinterDataSourceModel_FullPayload(t *testing.T) {
	state := PrinterDataSourceModel{}
	api := &proclassic.Printer{
		ID:       new(70),
		Name:     new("Lab"),
		Category: new("Printers"),
		Shared:   new(true),
	}

	diags := assignPrinterDataSourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "70" {
		t.Errorf("expected ID=70, got %q", state.ID.ValueString())
	}
	if state.Category.ValueString() != "Printers" {
		t.Errorf("expected Category=Printers, got %q", state.Category.ValueString())
	}
	if !state.Shared.ValueBool() {
		t.Errorf("expected Shared=true, got %v", state.Shared)
	}
}

func TestAssignPrinterDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := PrinterDataSourceModel{ID: types.StringValue("9")}
	diags := assignPrinterDataSourceModel(&state, nil)
	if diags.HasError() {
		t.Fatalf("nil API must not error, got %v", diags)
	}
	if state.ID.ValueString() != "9" {
		t.Errorf("expected state preserved, got %q", state.ID.ValueString())
	}
}
