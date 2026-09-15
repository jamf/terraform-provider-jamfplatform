// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildLicensedSoftwareInput projects a plan model into an SDK
// *proclassic.LicensedSoftware for Create / Update.
//
// software_definitions and licenses follow the wire's natural three-way
// (opt-out) semantics — the classic PUT merges, so:
//   - attribute null (omitted in config) → the wrapper is NOT emitted → Jamf Pro
//     retains the stored collection (the user opts out of managing it).
//   - attribute set to [] → an empty wrapper is emitted → Jamf Pro clears it.
//   - attribute set to [items] → the wrapper is emitted → Jamf Pro replaces it.
//
// A null list decodes to a nil Go slice and [] to a non-nil zero-length slice,
// so the nil check below distinguishes "don't manage" from "clear" (verified by
// the framework reflect behaviour). computers and per-licence attachments are
// read-only and never sent.
func buildLicensedSoftwareInput(plan LicensedSoftwareResourceModel) (*proclassic.LicensedSoftware, diag.Diagnostics) {
	var diags diag.Diagnostics

	out := &proclassic.LicensedSoftware{General: buildGeneral(plan)}

	if plan.SoftwareDefinitions != nil {
		out.SoftwareDefinitions = &proclassic.LicensedSoftwareSoftwareDefinitions{
			Definition: buildDefinitions(plan.SoftwareDefinitions),
		}
	}
	if plan.Licenses != nil {
		out.Licenses = &proclassic.LicensedSoftwareLicenses{
			License: buildLicenses(plan.Licenses),
		}
	}

	return out, diags
}

// buildGeneral maps the UI-aligned header fields into <general>.
func buildGeneral(plan LicensedSoftwareResourceModel) *proclassic.LicensedSoftwareGeneral {
	g := &proclassic.LicensedSoftwareGeneral{
		Name:                               helpers.OptionalStringPointer(plan.Name),
		Publisher:                          helpers.OptionalStringPointer(plan.Publisher),
		Platform:                           helpers.OptionalStringPointer(plan.Platform),
		Notes:                              helpers.OptionalStringPointer(plan.Notes),
		SendEmailOnViolation:               helpers.OptionalBoolPointer(plan.SendEmailOnViolation),
		RemoveTitlesFromInventoryReports:   helpers.OptionalBoolPointer(plan.RemoveTitlesFromInventoryReports),
		ExcludeTitlesPurchasedFromAppStore: helpers.OptionalBoolPointer(plan.ExcludeTitlesPurchasedFromAppStore),
	}
	if siteID := helpers.StringIDPtr(plan.SiteID); siteID != nil {
		g.Site = &proclassic.SiteObject{ID: siteID}
	}
	return g
}

// buildDefinitions maps the ordered software_definitions list. Returns nil for
// an empty list so the wrapper serialises as an empty <software_definitions>
// element (clears the server's copy on PUT).
func buildDefinitions(models []LicensedSoftwareDefinitionModel) *[]proclassic.LicensedSoftwareDefintion {
	if len(models) == 0 {
		return nil
	}
	out := make([]proclassic.LicensedSoftwareDefintion, 0, len(models))
	for _, m := range models {
		out = append(out, proclassic.LicensedSoftwareDefintion{
			Name:        helpers.OptionalStringPointer(m.Name),
			Version:     helpers.OptionalStringPointer(m.Version),
			CompareType: helpers.OptionalStringPointer(m.CompareType),
		})
	}
	return &out
}

// buildLicenses maps the ordered licenses list. Returns nil for an empty list so
// the wrapper serialises as an empty <licenses> element (clears on PUT).
func buildLicenses(models []LicensedSoftwareLicenseModel) *[]proclassic.LicensedSoftwareLicensesLicenseItem {
	if len(models) == 0 {
		return nil
	}
	out := make([]proclassic.LicensedSoftwareLicensesLicenseItem, 0, len(models))
	for _, m := range models {
		item := proclassic.LicensedSoftwareLicensesLicenseItem{
			SerialNumber1:    helpers.OptionalStringPointer(m.SerialNumber1),
			SerialNumber2:    helpers.OptionalStringPointer(m.SerialNumber2),
			OrganizationName: helpers.OptionalStringPointer(m.OrganizationName),
			RegisteredTo:     helpers.OptionalStringPointer(m.RegisteredTo),
			LicenseType:      helpers.OptionalStringPointer(m.LicenseType),
			LicenseCount:     helpers.OptionalInt64Pointer(m.LicenseCount),
			Notes:            helpers.OptionalStringPointer(m.Notes),
		}
		if m.Purchasing != nil {
			item.Purchasing = buildPurchasing(m.Purchasing)
		}
		out = append(out, item)
	}
	return &out
}

// buildPurchasing maps a license's purchasing block. license_term expands to the
// mutually-exclusive is_perpetual / is_annual wire pair. The server-derived
// *_epoch / *_utc echoes are never sent.
func buildPurchasing(p *LicensedSoftwarePurchasingModel) *proclassic.LicensedSoftwareLicensesLicenseItemPurchasing {
	perpetual := p.LicenseTerm.ValueString() == licenseTermPerpetual
	return &proclassic.LicensedSoftwareLicensesLicenseItemPurchasing{
		IsPerpetual:       new(perpetual),
		IsAnnual:          new(!perpetual),
		PoNumber:          helpers.OptionalStringPointer(p.PoNumber),
		PoDate:            helpers.OptionalStringPointer(p.PoDate),
		Vendor:            helpers.OptionalStringPointer(p.Vendor),
		LicenseExpires:    helpers.OptionalStringPointer(p.LicenseExpires),
		PurchasePrice:     helpers.OptionalStringPointer(p.PurchasePrice),
		LifeExpectancy:    helpers.OptionalInt64Pointer(p.LifeExpectancy),
		PurchasingAccount: helpers.OptionalStringPointer(p.PurchasingAccount),
		PurchasingContact: helpers.OptionalStringPointer(p.PurchasingContact),
	}
}
