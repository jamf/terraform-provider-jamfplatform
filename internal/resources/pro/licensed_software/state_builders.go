// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignLicensedSoftwareResourceModel populates a resource model from the SDK
// LicensedSoftware response. The same flatten runs on all four paths
// (create / update / read / import); the model passed in carries the plan
// (create / update), the prior state (read), or nothing (import). Nested lists
// reconcile positionally against that incoming model.
//
// software_definitions and licenses are opt-out lists: a list is only refreshed
// when the incoming model already manages it (non-nil — plan on create/update,
// prior state on read). When the user did not author the list (nil), it is left
// null and the server's echo is ignored, so an unmanaged collection does not
// surface as drift. This mirrors the scope-omission pattern.
//
// firstHydration is true only on a fresh import (see the Read caller): the
// incoming model has no prior list to correlate against, but flattenDefinitions
// / flattenLicenses already treat a positional index beyond the prior list's
// length as "nothing to prefer, take the wire value" (see their `i < len(prior)`
// bound checks) — so seeding an EMPTY (not nil) placeholder list is sufficient
// to flip the managed gate on and let every element hydrate from the wire, with
// no need to know the list's length up front.
func assignLicensedSoftwareResourceModel(ctx context.Context, state *LicensedSoftwareResourceModel, ls *proclassic.LicensedSoftware, firstHydration bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if ls == nil {
		return diags
	}

	// Capture the incoming nested lists before they are overwritten — they are
	// both the managed/unmanaged signal and the positional correlation source.
	priorDefs := state.SoftwareDefinitions
	priorLicenses := state.Licenses
	if firstHydration {
		if priorDefs == nil && ls.SoftwareDefinitions != nil {
			priorDefs = []LicensedSoftwareDefinitionModel{}
		}
		if priorLicenses == nil && ls.Licenses != nil {
			priorLicenses = []LicensedSoftwareLicenseModel{}
		}
	}

	if id := extractLicensedSoftwareID(ls); id != "" {
		state.ID = types.StringValue(id)
	}

	flattenGeneral(ls.General, state)
	if priorDefs != nil {
		state.SoftwareDefinitions = flattenDefinitions(definitionSlice(ls.SoftwareDefinitions), priorDefs)
	}
	if priorLicenses != nil {
		state.Licenses = flattenLicenses(ctx, licenseSlice(ls.Licenses), priorLicenses, &diags)
	}
	state.Computers = flattenComputers(ctx, computerSlice(ls.Computers), &diags)

	return diags
}

func flattenGeneral(g *proclassic.LicensedSoftwareGeneral, state *LicensedSoftwareResourceModel) {
	if g == nil {
		return
	}
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.Publisher = helpers.ReconcileOptionalStringPointer(g.Publisher, state.Publisher)
	state.Platform = helpers.ReconcileOptionalStringPointer(g.Platform, state.Platform)
	state.Notes = helpers.ReconcileOptionalStringPointer(g.Notes, state.Notes)
	state.SendEmailOnViolation = helpers.BoolPointerValueOrNull(g.SendEmailOnViolation)
	state.RemoveTitlesFromInventoryReports = helpers.BoolPointerValueOrNull(g.RemoveTitlesFromInventoryReports)
	state.ExcludeTitlesPurchasedFromAppStore = helpers.BoolPointerValueOrNull(g.ExcludeTitlesPurchasedFromAppStore)

	if g.Site != nil {
		state.SiteID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(g.Site.ID), state.SiteID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	} else {
		state.SiteID = helpers.ReconcileOptionalStringPointer(nil, state.SiteID)
		state.SiteName = types.StringNull()
	}
}

// flattenDefinitions is only called for a MANAGED list (prior non-nil). An empty
// wire collection therefore means the user set `[]` to clear it, which must
// round-trip as a non-nil empty list (not null) to match the plan.
func flattenDefinitions(wire []proclassic.LicensedSoftwareDefintion, prior []LicensedSoftwareDefinitionModel) []LicensedSoftwareDefinitionModel {
	if len(wire) == 0 {
		return []LicensedSoftwareDefinitionModel{}
	}
	out := make([]LicensedSoftwareDefinitionModel, 0, len(wire))
	for i, d := range wire {
		var cur LicensedSoftwareDefinitionModel
		if i < len(prior) {
			cur = prior[i]
		}
		out = append(out, LicensedSoftwareDefinitionModel{
			Name:        helpers.ReconcileOptionalStringPointer(d.Name, cur.Name),
			Version:     helpers.ReconcileOptionalStringPointer(d.Version, cur.Version),
			CompareType: helpers.ReconcileOptionalStringPointer(d.CompareType, cur.CompareType),
		})
	}
	return out
}

// flattenLicenses is only called for a MANAGED list (prior non-nil). An empty
// wire collection therefore means the user set `[]` to clear it, which must
// round-trip as a non-nil empty list (not null) to match the plan.
func flattenLicenses(ctx context.Context, wire []proclassic.LicensedSoftwareLicensesLicenseItem, prior []LicensedSoftwareLicenseModel, diags *diag.Diagnostics) []LicensedSoftwareLicenseModel {
	if len(wire) == 0 {
		return []LicensedSoftwareLicenseModel{}
	}
	out := make([]LicensedSoftwareLicenseModel, 0, len(wire))
	for i, w := range wire {
		var cur LicensedSoftwareLicenseModel
		hasCur := i < len(prior)
		if hasCur {
			cur = prior[i]
		}

		lm := LicensedSoftwareLicenseModel{
			SerialNumber1:    helpers.ReconcileOptionalStringPointer(w.SerialNumber1, cur.SerialNumber1),
			SerialNumber2:    helpers.ReconcileOptionalStringPointer(w.SerialNumber2, cur.SerialNumber2),
			OrganizationName: helpers.ReconcileOptionalStringPointer(w.OrganizationName, cur.OrganizationName),
			RegisteredTo:     helpers.ReconcileOptionalStringPointer(w.RegisteredTo, cur.RegisteredTo),
			LicenseType:      helpers.ReconcileOptionalStringPointer(w.LicenseType, cur.LicenseType),
			LicenseCount:     int64ValueOrZero(w.LicenseCount),
			Notes:            helpers.ReconcileOptionalStringPointer(w.Notes, cur.Notes),
			Attachments:      flattenAttachments(ctx, attachmentSlice(w.Attachments), diags),
		}

		// Surface the echoed <purchasing> only when the prior element already
		// managed it (lifecycle), or when there is no prior at all (import).
		if hasCur {
			if cur.Purchasing != nil {
				lm.Purchasing = flattenPurchasing(w.Purchasing, cur.Purchasing)
			}
		} else {
			lm.Purchasing = flattenPurchasing(w.Purchasing, nil)
		}

		out = append(out, lm)
	}
	return out
}

func flattenPurchasing(w *proclassic.LicensedSoftwareLicensesLicenseItemPurchasing, cur *LicensedSoftwarePurchasingModel) *LicensedSoftwarePurchasingModel {
	if w == nil {
		return cur
	}
	var c LicensedSoftwarePurchasingModel
	if cur != nil {
		c = *cur
	}
	return &LicensedSoftwarePurchasingModel{
		LicenseTerm:         licenseTermValue(licenseTermFromBools(w.IsPerpetual, w.IsAnnual), c.LicenseTerm),
		PoNumber:            helpers.ReconcileOptionalStringPointer(w.PoNumber, c.PoNumber),
		PoDate:              helpers.ReconcileOptionalStringPointer(w.PoDate, c.PoDate),
		PoDateEpoch:         int64ValueOrNullZero(w.PoDateEpoch),
		PoDateUtc:           stringValueOrNullEmpty(w.PoDateUtc),
		Vendor:              helpers.ReconcileOptionalStringPointer(w.Vendor, c.Vendor),
		LicenseExpires:      helpers.ReconcileOptionalStringPointer(w.LicenseExpires, c.LicenseExpires),
		LicenseExpiresEpoch: int64ValueOrNullZero(w.LicenseExpiresEpoch),
		LicenseExpiresUtc:   stringValueOrNullEmpty(w.LicenseExpiresUtc),
		PurchasePrice:       helpers.ReconcileOptionalStringPointer(w.PurchasePrice, c.PurchasePrice),
		LifeExpectancy:      int64ValueOrNullZero(w.LifeExpectancy),
		PurchasingAccount:   helpers.ReconcileOptionalStringPointer(w.PurchasingAccount, c.PurchasingAccount),
		PurchasingContact:   helpers.ReconcileOptionalStringPointer(w.PurchasingContact, c.PurchasingContact),
	}
}

// flattenAttachments builds the Computed attachments list. Returns a known
// (possibly empty) list so the Computed attribute resolves from Unknown at apply.
func flattenAttachments(ctx context.Context, wire []proclassic.Attachment, diags *diag.Diagnostics) types.List {
	out := make([]LicensedSoftwareAttachmentModel, 0, len(wire))
	for _, a := range wire {
		out = append(out, LicensedSoftwareAttachmentModel{
			ID:       helpers.StringValueFromIntPtr(a.ID),
			Filename: stringValueOrNullEmpty(a.Filename),
			URI:      stringValueOrNullEmpty(a.URI),
		})
	}
	list, d := types.ListValueFrom(ctx, attachmentObjectType, out)
	diags.Append(d...)
	return list
}

// flattenComputers builds the Computed computers list. Returns a known
// (possibly empty) list so the Computed attribute resolves from Unknown at apply.
func flattenComputers(ctx context.Context, wire []proclassic.LicensedSoftwareComputersComputerItem, diags *diag.Diagnostics) types.List {
	out := make([]LicensedSoftwareComputerModel, 0, len(wire))
	for _, c := range wire {
		out = append(out, LicensedSoftwareComputerModel{
			ID:   helpers.StringValueFromIntPtr(c.ID),
			Name: stringValueOrNullEmpty(c.Name),
			UDID: stringValueOrNullEmpty(c.UDID),
		})
	}
	list, d := types.ListValueFrom(ctx, computerObjectType, out)
	diags.Append(d...)
	return list
}

// licenseTermFromBools derives the license_term enum from the wire's
// mutually-exclusive bool pair. The server enforces exactly-one and defaults to
// perpetual, so anything that is not explicitly annual reads as perpetual.
func licenseTermFromBools(isPerpetual, isAnnual *bool) string {
	if isAnnual != nil && *isAnnual && (isPerpetual == nil || !*isPerpetual) {
		return licenseTermAnnual
	}
	return licenseTermPerpetual
}

// licenseTermValue prefers the configured value (license_term is Required when
// purchasing is set) and otherwise adopts the wire-derived term — the latter
// only on import, where there is no configured value.
func licenseTermValue(api string, current types.String) types.String {
	if helpers.IsConfiguredValue(current) {
		return current
	}
	return types.StringValue(api)
}

// ---- wire sub-slice accessors -------------------------------------------------

func definitionSlice(d *proclassic.LicensedSoftwareSoftwareDefinitions) []proclassic.LicensedSoftwareDefintion {
	if d == nil || d.Definition == nil {
		return nil
	}
	return *d.Definition
}

func licenseSlice(l *proclassic.LicensedSoftwareLicenses) []proclassic.LicensedSoftwareLicensesLicenseItem {
	if l == nil || l.License == nil {
		return nil
	}
	return *l.License
}

func attachmentSlice(a *proclassic.LicensedSoftwareLicensesLicenseItemAttachments) []proclassic.Attachment {
	if a == nil || a.Attachment == nil {
		return nil
	}
	return *a.Attachment
}

func computerSlice(c *proclassic.LicensedSoftwareComputers) []proclassic.LicensedSoftwareComputersComputerItem {
	if c == nil || c.Computer == nil {
		return nil
	}
	return *c.Computer
}
