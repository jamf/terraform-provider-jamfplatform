// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// wireRecord builds a representative GET-by-id response mirroring the
// wire-probed shape: two licences (the first fully populated, the second a
// "bare" licence the server pads with empty strings, license_count 0, and a
// default <purchasing> block), one software definition, and the read-only
// computers/attachments echoes.
func wireRecord() *proclassic.LicensedSoftware {
	return &proclassic.LicensedSoftware{
		ID: new(66),
		General: &proclassic.LicensedSoftwareGeneral{
			ID:                                 new(66),
			Name:                               new("ZZ-Probe-LicSW"),
			Publisher:                          new("ProbeCo"),
			Platform:                           new("Mac"),
			Notes:                              new("probe notes"),
			SendEmailOnViolation:               new(true),
			RemoveTitlesFromInventoryReports:   new(false),
			ExcludeTitlesPurchasedFromAppStore: new(true),
			Site:                               &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		},
		SoftwareDefinitions: &proclassic.LicensedSoftwareSoftwareDefinitions{
			Definition: &[]proclassic.LicensedSoftwareDefintion{
				{CompareType: new("is"), Name: new("SoftA"), Version: new("1.0")},
			},
		},
		Licenses: &proclassic.LicensedSoftwareLicenses{
			License: &[]proclassic.LicensedSoftwareLicensesLicenseItem{
				{
					SerialNumber1:    new("SER-A1"),
					SerialNumber2:    new(""), // empty-echo
					OrganizationName: new("OrgA"),
					RegisteredTo:     new("RegA"),
					LicenseType:      new("Standard"),
					LicenseCount:     new(5),
					Notes:            new("licA"),
					Purchasing: &proclassic.LicensedSoftwareLicensesLicenseItemPurchasing{
						IsPerpetual:         new(true),
						IsAnnual:            new(false),
						PoNumber:            new("PO-A"),
						Vendor:              new("VendA"),
						PurchasePrice:       new("199.99"),
						PurchasingAccount:   new("AcctA"),
						PoDate:              new("2026-03-15"),
						PoDateEpoch:         new(1773532800000),
						PoDateUtc:           new("2026-03-15T00:00:00.000+0000"),
						LicenseExpires:      new("2027-03-15"),
						LicenseExpiresEpoch: new(1805068800000),
						LicenseExpiresUtc:   new("2027-03-15T00:00:00.000+0000"),
						LifeExpectancy:      new(3),
						PurchasingContact:   new("ContactA"),
					},
					Attachments: &proclassic.LicensedSoftwareLicensesLicenseItemAttachments{
						Attachment: &[]proclassic.Attachment{
							{ID: new(30), Filename: new("Signature.jpg"), URI: new("https://example.test/attachment?id=30")},
						},
					},
				},
				{
					// Bare licence: server pads unset optionals with "" / 0 and a
					// default perpetual <purchasing> block even when the user
					// declared none.
					SerialNumber1:    new("SER-B1"),
					SerialNumber2:    new(""),
					OrganizationName: new("OrgB"),
					LicenseType:      new("Concurrent"),
					LicenseCount:     new(0),
					Notes:            new(""),
					Purchasing: &proclassic.LicensedSoftwareLicensesLicenseItemPurchasing{
						IsPerpetual:         new(true),
						IsAnnual:            new(false),
						PoNumber:            new(""),
						PoDateEpoch:         new(0),
						LicenseExpiresEpoch: new(0),
						LifeExpectancy:      new(0),
					},
				},
			},
		},
		Computers: &proclassic.LicensedSoftwareComputers{
			Computer: &[]proclassic.LicensedSoftwareComputersComputerItem{
				{ID: new(101), Name: new("Mac-101"), UDID: new("UDID-101")},
			},
		},
	}
}

// TestAssign_CreatePath_RoundTrip exercises the create/update path: the model
// carries the plan, the first licence manages purchasing, the second does not.
func TestAssign_CreatePath_RoundTrip(t *testing.T) {
	plan := &LicensedSoftwareResourceModel{
		Name:      types.StringValue("ZZ-Probe-LicSW"),
		Publisher: types.StringValue("ProbeCo"),
		Platform:  types.StringValue("Mac"),
		SoftwareDefinitions: []LicensedSoftwareDefinitionModel{
			{Name: types.StringValue("SoftA"), Version: types.StringValue("1.0"), CompareType: types.StringValue("is")},
		},
		Licenses: []LicensedSoftwareLicenseModel{
			{
				SerialNumber1: types.StringValue("SER-A1"),
				LicenseType:   types.StringValue("Standard"),
				LicenseCount:  types.Int64Value(5),
				Purchasing: &LicensedSoftwarePurchasingModel{
					LicenseTerm:    types.StringValue("perpetual"),
					PoDate:         types.StringValue("2026-03-15"),
					LicenseExpires: types.StringValue("2027-03-15"),
					LifeExpectancy: types.Int64Value(3),
				},
			},
			{
				// User declared a licence with NO purchasing block.
				SerialNumber1: types.StringValue("SER-B1"),
				LicenseType:   types.StringValue("Concurrent"),
				LicenseCount:  types.Int64Value(0),
			},
		},
	}

	if diags := assignLicensedSoftwareResourceModel(context.Background(), plan, wireRecord(), false); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	if got := plan.ID.ValueString(); got != "66" {
		t.Errorf("ID = %q, want 66", got)
	}
	if !plan.SiteID.Equal(types.StringValue("-1")) {
		t.Errorf("SiteID = %v, want -1", plan.SiteID)
	}
	// Sentinel site (id -1): derived name nulls, not the flaky server echo.
	if !plan.SiteName.IsNull() {
		t.Errorf("SiteName = %v, want null on the sentinel", plan.SiteName)
	}

	if len(plan.Licenses) != 2 {
		t.Fatalf("licenses len = %d, want 2", len(plan.Licenses))
	}
	a, b := plan.Licenses[0], plan.Licenses[1]

	// Empty-echo serial_number_2 must collapse to null on the unconfigured field.
	if !a.SerialNumber2.IsNull() {
		t.Errorf("license[0].serial_number_2 = %v, want null", a.SerialNumber2)
	}
	// license_count 0 is meaningful and must be preserved, not nulled.
	if a.LicenseCount.ValueInt64() != 5 {
		t.Errorf("license[0].license_count = %v, want 5", a.LicenseCount)
	}
	if b.LicenseCount.ValueInt64() != 0 || b.LicenseCount.IsNull() {
		t.Errorf("license[1].license_count = %v, want 0 (not null)", b.LicenseCount)
	}

	// license[0] manages purchasing → surfaced; epochs/utc derived.
	if a.Purchasing == nil {
		t.Fatal("license[0].purchasing = nil, want populated")
	}
	if a.Purchasing.LicenseTerm.ValueString() != "perpetual" {
		t.Errorf("license[0].license_term = %v, want perpetual", a.Purchasing.LicenseTerm)
	}
	if a.Purchasing.PoDateEpoch.ValueInt64() != 1773532800000 {
		t.Errorf("license[0].po_date_epoch = %v", a.Purchasing.PoDateEpoch)
	}
	if a.Purchasing.PoDateUtc.IsNull() {
		t.Error("license[0].po_date_utc = null, want value")
	}
	if a.Purchasing.LifeExpectancy.ValueInt64() != 3 {
		t.Errorf("license[0].life_expectancy = %v, want 3", a.Purchasing.LifeExpectancy)
	}

	// license[1] declared NO purchasing → must stay nil despite the server echo.
	if b.Purchasing != nil {
		t.Errorf("license[1].purchasing = %+v, want nil (unmanaged echo suppressed)", b.Purchasing)
	}

	// Read-only attachments echo flattens into a known list (Computed types.List).
	if a.Attachments.IsNull() || a.Attachments.IsUnknown() {
		t.Error("license[0].attachments must be a known list")
	}
	if n := len(a.Attachments.Elements()); n != 1 {
		t.Errorf("license[0].attachments len = %d, want 1", n)
	}
	if b.Attachments.IsNull() || b.Attachments.IsUnknown() || len(b.Attachments.Elements()) != 0 {
		t.Errorf("license[1].attachments = %v, want known empty list", b.Attachments)
	}

	// computers is a read-only echo, always surfaced as a known list.
	if plan.Computers.IsNull() || plan.Computers.IsUnknown() {
		t.Error("computers must be a known list, not null/unknown")
	}
	if n := len(plan.Computers.Elements()); n != 1 {
		t.Errorf("computers len = %d, want 1", n)
	}
}

// TestAssign_ImportPath_HydratesLists exercises first-time import hydration (no
// prior model, firstHydration=true — see the import-hydration fix): with no
// prior list to correlate against, software_definitions and licenses must
// still populate from the wire (an empty, non-nil placeholder list is enough to
// flip the managed gate on; flattenDefinitions/flattenLicenses's `i < len(prior)`
// bound check naturally falls through to the wire value for every element).
// Before the fix, these stayed permanently null after any import regardless of
// config, matching issue #278's root cause for the other 12 resources.
func TestAssign_ImportPath_HydratesLists(t *testing.T) {
	state := &LicensedSoftwareResourceModel{} // no prior nested lists (import)

	if diags := assignLicensedSoftwareResourceModel(context.Background(), state, wireRecord(), true); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	if state.Name.ValueString() != "ZZ-Probe-LicSW" {
		t.Errorf("name = %v, want ZZ-Probe-LicSW", state.Name)
	}

	if len(state.SoftwareDefinitions) != 1 {
		t.Fatalf("software_definitions len = %d, want 1", len(state.SoftwareDefinitions))
	}
	if got := state.SoftwareDefinitions[0].Name.ValueString(); got != "SoftA" {
		t.Errorf("software_definitions[0].name = %q, want SoftA", got)
	}

	if len(state.Licenses) != 2 {
		t.Fatalf("licenses len = %d, want 2", len(state.Licenses))
	}
	a, b := state.Licenses[0], state.Licenses[1]
	if got := a.SerialNumber1.ValueString(); got != "SER-A1" {
		t.Errorf("license[0].serial_number_1 = %q, want SER-A1", got)
	}
	// No prior at all (import): the echoed <purchasing> on EVERY licence is
	// surfaced faithfully, including the bare licence's default-shaped block —
	// there is nothing declared yet to compare against, so there is no
	// "unmanaged echo" to suppress.
	if a.Purchasing == nil {
		t.Fatal("license[0].purchasing = nil, want populated on import")
	}
	if a.Purchasing.LicenseTerm.ValueString() != "perpetual" {
		t.Errorf("license[0].license_term = %v, want perpetual", a.Purchasing.LicenseTerm)
	}
	if b.Purchasing == nil {
		t.Fatal("license[1].purchasing = nil, want populated on import (default-shaped echo, faithfully surfaced)")
	}
	if b.Purchasing.LicenseTerm.ValueString() != "perpetual" {
		t.Errorf("license[1].license_term = %v, want perpetual", b.Purchasing.LicenseTerm)
	}

	if state.Computers.IsNull() || len(state.Computers.Elements()) != 1 {
		t.Errorf("computers = %v, want one known entry", state.Computers)
	}
}

// TestLicenseTermFromBools covers the exactly-one server contract.
func TestLicenseTermFromBools(t *testing.T) {
	cases := []struct {
		name       string
		perpetual  *bool
		annual     *bool
		wantAnnual bool
	}{
		{"perpetual", new(true), new(false), false},
		{"annual", new(false), new(true), true},
		{"both-false-defaults-perpetual", new(false), new(false), false},
		{"nil-defaults-perpetual", nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := licenseTermFromBools(c.perpetual, c.annual)
			want := licenseTermPerpetual
			if c.wantAnnual {
				want = licenseTermAnnual
			}
			if got != want {
				t.Errorf("licenseTermFromBools = %q, want %q", got, want)
			}
		})
	}
}
