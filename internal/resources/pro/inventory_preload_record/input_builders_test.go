// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// knownEAList builds a known extension_attributes list value from element models.
func knownEAList(t *testing.T, models []extensionAttributeModel) types.List {
	t.Helper()
	list, diags := types.ListValueFrom(context.Background(), extensionAttributeObjectType, models)
	if diags.HasError() {
		t.Fatalf("building test list: %v", diags)
	}
	return list
}

func TestBuildInventoryPreloadRecordInput_MinimalNullsBecomeNilPointers(t *testing.T) {
	plan := InventoryPreloadRecordResourceModel{
		SerialNumber:        types.StringValue("ZTFACC0001"),
		DeviceType:          types.StringValue("Computer"),
		ExtensionAttributes: types.ListNull(extensionAttributeObjectType),
	}

	got, diags := buildInventoryPreloadRecordInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.SerialNumber != "ZTFACC0001" {
		t.Errorf("expected SerialNumber %q, got %q", "ZTFACC0001", got.SerialNumber)
	}
	if got.DeviceType != "Computer" {
		t.Errorf("expected DeviceType %q, got %q", "Computer", got.DeviceType)
	}
	for name, ptr := range map[string]*string{
		"Username": got.Username, "FullName": got.FullName, "EmailAddress": got.EmailAddress,
		"PhoneNumber": got.PhoneNumber, "Position": got.Position, "Department": got.Department,
		"Building": got.Building, "Room": got.Room, "PoNumber": got.PoNumber, "PoDate": got.PoDate,
		"WarrantyExpiration": got.WarrantyExpiration, "LeaseExpiration": got.LeaseExpiration,
		"AppleCareID": got.AppleCareID, "LifeExpectancy": got.LifeExpectancy,
		"PurchasePrice": got.PurchasePrice, "PurchasingContact": got.PurchasingContact,
		"PurchasingAccount": got.PurchasingAccount, "BarCode1": got.BarCode1,
		"BarCode2": got.BarCode2, "AssetTag": got.AssetTag, "Vendor": got.Vendor,
	} {
		if ptr != nil {
			t.Errorf("%s: expected nil pointer for null input, got %q", name, *ptr)
		}
	}
	if got.ExtensionAttributes != nil {
		t.Errorf("expected ExtensionAttributes nil (omitted) for null list, got %v", got.ExtensionAttributes)
	}
}

func TestBuildInventoryPreloadRecordInput_UnknownsBecomeNilPointers(t *testing.T) {
	plan := InventoryPreloadRecordResourceModel{
		SerialNumber:        types.StringValue("ZTFACC0002"),
		DeviceType:          types.StringValue("Mobile Device"),
		Username:            types.StringUnknown(),
		AssetTag:            types.StringUnknown(),
		Vendor:              types.StringUnknown(),
		ExtensionAttributes: types.ListUnknown(extensionAttributeObjectType),
	}

	got, diags := buildInventoryPreloadRecordInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.Username != nil || got.AssetTag != nil || got.Vendor != nil {
		t.Errorf("expected nil pointers for Unknown scalars, got Username=%v AssetTag=%v Vendor=%v", got.Username, got.AssetTag, got.Vendor)
	}
	if got.ExtensionAttributes != nil {
		t.Errorf("expected ExtensionAttributes nil (omitted) for unknown list, got %v", got.ExtensionAttributes)
	}
}

func TestBuildInventoryPreloadRecordInput_AllScalarsPopulated(t *testing.T) {
	plan := InventoryPreloadRecordResourceModel{
		SerialNumber:        types.StringValue("ZTFACC0003"),
		DeviceType:          types.StringValue("Computer"),
		Username:            types.StringValue("preload.user"),
		FullName:            types.StringValue("Preload User"),
		EmailAddress:        types.StringValue("preload.user@example.com"),
		PhoneNumber:         types.StringValue("555-0100"),
		Position:            types.StringValue("Technician"),
		Department:          types.StringValue("IT"),
		Building:            types.StringValue("HQ"),
		Room:                types.StringValue("101"),
		PoNumber:            types.StringValue("PO-1234"),
		PoDate:              types.StringValue("2026-01-15"),
		WarrantyExpiration:  types.StringValue("2029-01-15"),
		LeaseExpiration:     types.StringValue("2028-01-15"),
		AppleCareID:         types.StringValue("AC-1"),
		LifeExpectancy:      types.StringValue("4"),
		PurchasePrice:       types.StringValue("1999.00"),
		PurchasingContact:   types.StringValue("Purchasing Contact"),
		PurchasingAccount:   types.StringValue("ACCT-1"),
		BarCode1:            types.StringValue("BC1"),
		BarCode2:            types.StringValue("BC2"),
		AssetTag:            types.StringValue("ASSET-1"),
		Vendor:              types.StringValue("Vendor Inc"),
		ExtensionAttributes: types.ListNull(extensionAttributeObjectType),
	}

	got, diags := buildInventoryPreloadRecordInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	cases := []struct {
		field    string
		got      *string
		expected string
	}{
		{"Username", got.Username, "preload.user"},
		{"FullName", got.FullName, "Preload User"},
		{"EmailAddress", got.EmailAddress, "preload.user@example.com"},
		{"PhoneNumber", got.PhoneNumber, "555-0100"},
		{"Position", got.Position, "Technician"},
		{"Department", got.Department, "IT"},
		{"Building", got.Building, "HQ"},
		{"Room", got.Room, "101"},
		{"PoNumber", got.PoNumber, "PO-1234"},
		{"PoDate", got.PoDate, "2026-01-15"},
		{"WarrantyExpiration", got.WarrantyExpiration, "2029-01-15"},
		{"LeaseExpiration", got.LeaseExpiration, "2028-01-15"},
		{"AppleCareID", got.AppleCareID, "AC-1"},
		{"LifeExpectancy", got.LifeExpectancy, "4"},
		{"PurchasePrice", got.PurchasePrice, "1999.00"},
		{"PurchasingContact", got.PurchasingContact, "Purchasing Contact"},
		{"PurchasingAccount", got.PurchasingAccount, "ACCT-1"},
		{"BarCode1", got.BarCode1, "BC1"},
		{"BarCode2", got.BarCode2, "BC2"},
		{"AssetTag", got.AssetTag, "ASSET-1"},
		{"Vendor", got.Vendor, "Vendor Inc"},
	}
	for _, c := range cases {
		if c.got == nil {
			t.Errorf("%s: expected non-nil pointer, got nil", c.field)
			continue
		}
		if *c.got != c.expected {
			t.Errorf("%s: expected %q, got %q", c.field, c.expected, *c.got)
		}
	}
}

func TestBuildInventoryPreloadRecordInput_EmptyStringScalarSentVerbatim(t *testing.T) {
	plan := InventoryPreloadRecordResourceModel{
		SerialNumber:        types.StringValue("ZTFACC0004"),
		DeviceType:          types.StringValue("Computer"),
		AssetTag:            types.StringValue(""),
		ExtensionAttributes: types.ListNull(extensionAttributeObjectType),
	}

	got, diags := buildInventoryPreloadRecordInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.AssetTag == nil {
		t.Fatalf("expected non-nil AssetTag pointer for explicit empty string")
	}
	if *got.AssetTag != "" {
		t.Errorf("expected AssetTag %q, got %q", "", *got.AssetTag)
	}
}

func TestBuildInventoryPreloadRecordInput_EmptyEAListEmitsEmptySlice(t *testing.T) {
	plan := InventoryPreloadRecordResourceModel{
		SerialNumber:        types.StringValue("ZTFACC0005"),
		DeviceType:          types.StringValue("Computer"),
		ExtensionAttributes: knownEAList(t, []extensionAttributeModel{}),
	}

	got, diags := buildInventoryPreloadRecordInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.ExtensionAttributes == nil {
		t.Fatalf("expected non-nil empty slice for known-empty set (a [] clears the collection on the full-replace PUT), got nil")
	}
	if len(*got.ExtensionAttributes) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(*got.ExtensionAttributes))
	}
}

func TestBuildInventoryPreloadRecordInput_EAValueNullVsEmptyDistinct(t *testing.T) {
	plan := InventoryPreloadRecordResourceModel{
		SerialNumber: types.StringValue("ZTFACC0006"),
		DeviceType:   types.StringValue("Computer"),
		ExtensionAttributes: knownEAList(t, []extensionAttributeModel{
			{Name: types.StringValue("NullVal"), Value: types.StringNull()},
			{Name: types.StringValue("EmptyVal"), Value: types.StringValue("")},
			{Name: types.StringValue("SetVal"), Value: types.StringValue("populated")},
		}),
	}

	got, diags := buildInventoryPreloadRecordInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got.ExtensionAttributes == nil {
		t.Fatalf("expected non-nil ExtensionAttributes")
	}
	byName := map[string]*string{}
	for _, ea := range *got.ExtensionAttributes {
		byName[ea.Name] = ea.Value
	}
	if len(byName) != 3 {
		t.Fatalf("expected 3 extension attributes, got %d", len(byName))
	}
	if byName["NullVal"] != nil {
		t.Errorf("NullVal: expected nil Value pointer (omitted, stores null), got %q", *byName["NullVal"])
	}
	if byName["EmptyVal"] == nil || *byName["EmptyVal"] != "" {
		t.Errorf("EmptyVal: expected pointer to empty string, got %v", byName["EmptyVal"])
	}
	if byName["SetVal"] == nil || *byName["SetVal"] != "populated" {
		t.Errorf("SetVal: expected %q, got %v", "populated", byName["SetVal"])
	}
}

// TestInventoryPreloadRecordResourceModel_ExtensionAttributesIsTypesList guards the
// model field against regressing to a Go typed slice (the Computed nested collection
// rule: a slice cannot carry Unknown) or back to types.Set, which cannot hold the
// repeats Jamf Pro stores (issue #568).
func TestInventoryPreloadRecordResourceModel_ExtensionAttributesIsTypesList(t *testing.T) {
	var model InventoryPreloadRecordResourceModel
	var _ attr.Value = model.ExtensionAttributes
	func(types.List) {}(model.ExtensionAttributes)
}
