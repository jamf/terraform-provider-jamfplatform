// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestFlattenGeneral_ReportsDrift pins the wire-authoritative read on the
// general block: an echoed value that differs from state must land in state so
// `terraform plan` reports the change. Every field on this resource is echoed
// faithfully by the classic /licensedsoftware GET, on both the POST and the PUT
// path (Jamf Pro 11.31.1, wire-probed 2026-09-06), so none of them keeps a
// sticky read. See issue #387.
func TestFlattenGeneral_ReportsDrift(t *testing.T) {
	t.Parallel()
	state := &LicensedSoftwareResourceModel{
		Publisher:                          types.StringValue("state publisher"),
		Platform:                           types.StringValue("Windows"),
		Notes:                              types.StringValue("state notes"),
		SendEmailOnViolation:               types.BoolValue(false),
		RemoveTitlesFromInventoryReports:   types.BoolValue(false),
		ExcludeTitlesPurchasedFromAppStore: types.BoolValue(false),
		SiteID:                             types.StringValue("-1"),
	}
	flattenGeneral(&proclassic.LicensedSoftwareGeneral{
		Name:                               new("title"),
		Publisher:                          new("wire publisher"),
		Platform:                           new("Mac"),
		Notes:                              new("wire notes"),
		SendEmailOnViolation:               new(true),
		RemoveTitlesFromInventoryReports:   new(true),
		ExcludeTitlesPurchasedFromAppStore: new(true),
		Site:                               &proclassic.SiteObject{ID: new(1), Name: new("AGATA")},
	}, state)

	for _, tc := range []struct{ name, want, got string }{
		{"publisher", "wire publisher", state.Publisher.ValueString()},
		{"platform", "Mac", state.Platform.ValueString()},
		{"notes", "wire notes", state.Notes.ValueString()},
		{"site_id", "1", state.SiteID.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
	for _, tc := range []struct {
		name string
		got  types.Bool
	}{
		{"send_email_on_violation", state.SendEmailOnViolation},
		{"remove_titles_from_inventory_reports", state.RemoveTitlesFromInventoryReports},
		{"exclude_titles_purchased_from_app_store", state.ExcludeTitlesPurchasedFromAppStore},
	} {
		if !tc.got.ValueBool() {
			t.Errorf("%s: wire value must win, got false", tc.name)
		}
	}
}

// TestFlattenNestedLists_ReportDrift pins the same rule inside the
// positionally-reconciled nested lists: a definition, a license and its
// purchasing block all take the wire value over a divergent prior state, while
// the opt-out gate (a nil incoming list stays unmanaged) is untouched.
func TestFlattenNestedLists_ReportDrift(t *testing.T) {
	t.Parallel()
	defs := flattenDefinitions(
		[]proclassic.LicensedSoftwareDefintion{{
			Name:        new("wire.app"),
			Version:     new("2.0"),
			CompareType: new("like"),
		}},
		[]LicensedSoftwareDefinitionModel{{
			Name:        types.StringValue("state.app"),
			Version:     types.StringValue("1.0"),
			CompareType: types.StringValue("is"),
		}},
	)
	if len(defs) != 1 {
		t.Fatalf("expected one definition, got %d", len(defs))
	}
	for _, tc := range []struct{ name, want, got string }{
		{"name", "wire.app", defs[0].Name.ValueString()},
		{"version", "2.0", defs[0].Version.ValueString()},
		{"compare_type", "like", defs[0].CompareType.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("software_definitions[0].%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}

	var diags diag.Diagnostics
	licenses := flattenLicenses(context.Background(),
		[]proclassic.LicensedSoftwareLicensesLicenseItem{{
			SerialNumber1:    new("wire-sn1"),
			OrganizationName: new("wire org"),
			LicenseType:      new("Site"),
			Purchasing: &proclassic.LicensedSoftwareLicensesLicenseItemPurchasing{
				PoNumber:          new("wire-po"),
				Vendor:            new("wire vendor"),
				PurchasingContact: new("wire contact"),
			},
		}},
		[]LicensedSoftwareLicenseModel{{
			SerialNumber1:    types.StringValue("state-sn1"),
			OrganizationName: types.StringValue("state org"),
			LicenseType:      types.StringValue("Standard"),
			Purchasing: &LicensedSoftwarePurchasingModel{
				PoNumber:          types.StringValue("state-po"),
				Vendor:            types.StringValue("state vendor"),
				PurchasingContact: types.StringValue("state contact"),
			},
		}},
		&diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(licenses) != 1 {
		t.Fatalf("expected one license, got %d", len(licenses))
	}
	l := licenses[0]
	for _, tc := range []struct{ name, want, got string }{
		{"serial_number_1", "wire-sn1", l.SerialNumber1.ValueString()},
		{"organization_name", "wire org", l.OrganizationName.ValueString()},
		{"license_type", "Site", l.LicenseType.ValueString()},
		{"purchasing.po_number", "wire-po", l.Purchasing.PoNumber.ValueString()},
		{"purchasing.vendor", "wire vendor", l.Purchasing.Vendor.ValueString()},
		{"purchasing.purchasing_contact", "wire contact", l.Purchasing.PurchasingContact.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("licenses[0].%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
}
