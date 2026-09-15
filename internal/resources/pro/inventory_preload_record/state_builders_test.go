// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignInventoryPreloadRecordResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := InventoryPreloadRecordResourceModel{
		ID: types.StringValue("42"),
	}
	api := &pro.InventoryPreloadRecordV2{
		SerialNumber: "ZTFACC0001",
		DeviceType:   "Computer",
	}

	diags := assignInventoryPreloadRecordResourceModel(context.Background(), &state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "42" {
		t.Errorf("expected state.ID preserved as %q, got %q", "42", state.ID.ValueString())
	}
	if state.SerialNumber.ValueString() != "ZTFACC0001" {
		t.Errorf("expected SerialNumber updated, got %q", state.SerialNumber.ValueString())
	}
	if state.DeviceType.ValueString() != "Computer" {
		t.Errorf("expected DeviceType updated, got %q", state.DeviceType.ValueString())
	}
}

func TestAssignInventoryPreloadRecordResourceModel_OverwritesIDWhenAPIPresent(t *testing.T) {
	state := InventoryPreloadRecordResourceModel{
		ID: types.StringValue("placeholder"),
	}
	api := &pro.InventoryPreloadRecordV2{
		ID:           new("99"),
		SerialNumber: "ZTFACC0002",
		DeviceType:   "Mobile Device",
	}

	diags := assignInventoryPreloadRecordResourceModel(context.Background(), &state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "99" {
		t.Errorf("expected state.ID overwritten to %q, got %q", "99", state.ID.ValueString())
	}
}

func TestAssignInventoryPreloadRecordResourceModel_NilOptionalPointersBecomeNull(t *testing.T) {
	state := InventoryPreloadRecordResourceModel{
		ID: types.StringValue("1"),
	}
	api := &pro.InventoryPreloadRecordV2{
		ID:           new("1"),
		SerialNumber: "ZTFACC0003",
		DeviceType:   "Computer",
	}

	diags := assignInventoryPreloadRecordResourceModel(context.Background(), &state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	for _, f := range []struct {
		name string
		got  types.String
	}{
		{"Username", state.Username},
		{"FullName", state.FullName},
		{"EmailAddress", state.EmailAddress},
		{"PoDate", state.PoDate},
		{"WarrantyExpiration", state.WarrantyExpiration},
		{"LeaseExpiration", state.LeaseExpiration},
		{"BarCode1", state.BarCode1},
		{"AssetTag", state.AssetTag},
		{"Vendor", state.Vendor},
	} {
		if !f.got.IsNull() {
			t.Errorf("%s: expected null for nil API pointer, got %q", f.name, f.got.ValueString())
		}
	}
}

func TestAssignInventoryPreloadRecordResourceModel_PopulatedScalarsRoundTrip(t *testing.T) {
	state := InventoryPreloadRecordResourceModel{}
	api := &pro.InventoryPreloadRecordV2{
		ID:                 new("7"),
		SerialNumber:       "ztfacc0004",
		DeviceType:         "Mobile Device",
		Username:           new("preload.user"),
		FullName:           new("Preload User"),
		EmailAddress:       new("preload.user@example.com"),
		Department:         new("IT"),
		Building:           new("HQ"),
		PoDate:             new("2026-01-15"),
		WarrantyExpiration: new("2029-01-15"),
		AssetTag:           new("ASSET-7"),
	}

	diags := assignInventoryPreloadRecordResourceModel(context.Background(), &state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.SerialNumber.ValueString() != "ztfacc0004" {
		t.Errorf("expected SerialNumber echoed verbatim (case preserved), got %q", state.SerialNumber.ValueString())
	}
	cases := []struct {
		name     string
		got      types.String
		expected string
	}{
		{"Username", state.Username, "preload.user"},
		{"FullName", state.FullName, "Preload User"},
		{"EmailAddress", state.EmailAddress, "preload.user@example.com"},
		{"Department", state.Department, "IT"},
		{"Building", state.Building, "HQ"},
		{"PoDate", state.PoDate, "2026-01-15"},
		{"WarrantyExpiration", state.WarrantyExpiration, "2029-01-15"},
		{"AssetTag", state.AssetTag, "ASSET-7"},
	}
	for _, c := range cases {
		if c.got.IsNull() || c.got.ValueString() != c.expected {
			t.Errorf("%s: expected %q, got %v", c.name, c.expected, c.got)
		}
	}
}

func TestAssignInventoryPreloadRecordResourceModel_ConfiguredEmptyStringPreserved(t *testing.T) {
	state := InventoryPreloadRecordResourceModel{
		ID:       types.StringValue("1"),
		AssetTag: types.StringValue(""),
	}
	api := &pro.InventoryPreloadRecordV2{
		ID:           new("1"),
		SerialNumber: "ZTFACC0005",
		DeviceType:   "Computer",
		AssetTag:     new(""),
	}

	diags := assignInventoryPreloadRecordResourceModel(context.Background(), &state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.AssetTag.IsNull() {
		t.Errorf("expected configured empty string preserved (not collapsed to null)")
	}
	if state.AssetTag.ValueString() != "" {
		t.Errorf("expected empty string, got %q", state.AssetTag.ValueString())
	}
}

func TestFlattenExtensionAttributesSet_NilBecomesKnownEmptySet(t *testing.T) {
	set, diags := flattenExtensionAttributesSet(context.Background(), nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if set.IsNull() || set.IsUnknown() {
		t.Fatalf("expected known set, got null=%v unknown=%v", set.IsNull(), set.IsUnknown())
	}
	if len(set.Elements()) != 0 {
		t.Errorf("expected empty set, got %d elements", len(set.Elements()))
	}
}

func TestFlattenExtensionAttributesSet_EmptySliceBecomesKnownEmptySet(t *testing.T) {
	eas := []pro.InventoryPreloadExtensionAttribute{}
	set, diags := flattenExtensionAttributesSet(context.Background(), &eas)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if set.IsNull() || set.IsUnknown() || len(set.Elements()) != 0 {
		t.Errorf("expected known empty set, got null=%v unknown=%v len=%d", set.IsNull(), set.IsUnknown(), len(set.Elements()))
	}
}

func TestFlattenExtensionAttributesSet_NullVsEmptyValueDistinct(t *testing.T) {
	eas := []pro.InventoryPreloadExtensionAttribute{
		{Name: "NullVal", Value: nil},
		{Name: "EmptyVal", Value: new("")},
		{Name: "SetVal", Value: new("populated")},
	}
	set, diags := flattenExtensionAttributesSet(context.Background(), &eas)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var models []extensionAttributeModel
	if d := set.ElementsAs(context.Background(), &models, false); d.HasError() {
		t.Fatalf("decoding set: %v", d)
	}
	byName := map[string]types.String{}
	for _, m := range models {
		byName[m.Name.ValueString()] = m.Value
	}
	if len(byName) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(byName))
	}
	if !byName["NullVal"].IsNull() {
		t.Errorf("NullVal: expected null value, got %q", byName["NullVal"].ValueString())
	}
	if byName["EmptyVal"].IsNull() || byName["EmptyVal"].ValueString() != "" {
		t.Errorf("EmptyVal: expected empty string, got %v", byName["EmptyVal"])
	}
	if byName["SetVal"].ValueString() != "populated" {
		t.Errorf("SetVal: expected %q, got %v", "populated", byName["SetVal"])
	}
}

func TestAssignInventoryPreloadRecordDataSourceModel_PopulatedRoundTrip(t *testing.T) {
	state := InventoryPreloadRecordDataSourceModel{}
	eas := []pro.InventoryPreloadExtensionAttribute{
		{Name: "EA One", Value: new("v1")},
	}
	api := &pro.InventoryPreloadRecordV2{
		ID:                  new("12"),
		SerialNumber:        "ZTFACC0006",
		DeviceType:          "Computer",
		Username:            new("preload.user"),
		Department:          new("IT"),
		ExtensionAttributes: &eas,
	}

	diags := assignInventoryPreloadRecordDataSourceModel(context.Background(), &state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "12" {
		t.Errorf("expected ID %q, got %q", "12", state.ID.ValueString())
	}
	if state.SerialNumber.ValueString() != "ZTFACC0006" {
		t.Errorf("expected SerialNumber %q, got %q", "ZTFACC0006", state.SerialNumber.ValueString())
	}
	if state.Username.ValueString() != "preload.user" {
		t.Errorf("expected Username %q, got %q", "preload.user", state.Username.ValueString())
	}
	if state.FullName.IsNull() != true {
		t.Errorf("expected FullName null for nil API pointer")
	}
	if state.ExtensionAttributes.IsNull() || len(state.ExtensionAttributes.Elements()) != 1 {
		t.Errorf("expected 1 extension attribute, got %v", state.ExtensionAttributes)
	}
}
