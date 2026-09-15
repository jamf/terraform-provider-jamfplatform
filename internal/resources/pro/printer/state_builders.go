// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignPrinterResourceModel populates a resource model from a Printer
// response. state.ID is only overwritten when the API ID is non-nil so a
// transient GET that drops the ID does not clobber the value persisted from
// Create.
//
// `category` round-trips through the server sentinel
// `categoryUnassignedSentinel`. The server emits the sentinel for every
// record that has no category assigned; we decode it back to null in state so
// the Jamf-internal magic string never surfaces in plans or state files.
//
// The plain Optional strings are reconciled rather than copied: the input
// builder always emits them, so a config that omits one sends an empty element
// and the server echoes "" back. helpers.ReconcileOptionalStringPointer folds
// that "" to null when the incoming model (plan on write, prior state on
// refresh) does not carry the attribute, and keeps an explicit "" the user
// configured, so an omitted attribute round-trips instead of tripping the
// framework's inconsistent-result check.
func assignPrinterResourceModel(state *PrinterResourceModel, p *proclassic.Printer) diag.Diagnostics {
	var diags diag.Diagnostics
	if p == nil {
		return diags
	}
	if p.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(p.ID)
	}
	if p.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(p.Name)
	}
	state.Category = decodeCategory(p.Category)
	state.URI = helpers.ReconcileOptionalStringPointer(p.URI, state.URI)
	state.CUPSName = helpers.ReconcileOptionalStringPointer(p.CUPSName, state.CUPSName)
	state.Location = helpers.ReconcileOptionalStringPointer(p.Location, state.Location)
	state.Model = helpers.ReconcileOptionalStringPointer(p.Model, state.Model)
	state.Info = helpers.ReconcileOptionalStringPointer(p.Info, state.Info)
	state.Notes = helpers.ReconcileOptionalStringPointer(p.Notes, state.Notes)
	state.MakeDefault = helpers.BoolPointerValueOrNull(p.MakeDefault)
	state.UseGeneric = helpers.BoolPointerValueOrNull(p.UseGeneric)
	state.PPD, state.PPDPath, state.PPDContents = ppdTrioValues(p, state.PPD)
	state.Shared = helpers.BoolPointerValueOrNull(p.Shared)
	state.OSRequirements = helpers.ReconcileOptionalStringPointer(p.OsRequirements, state.OSRequirements)
	return diags
}

// ppdTrioValues maps the server's PPD trio (ppd, ppd_path, ppd_contents) into
// state, collapsing all three to null when the printer is in generic mode
// (use_generic true or omitted). The Jamf Pro server echoes the bundled
// Generic.ppd path even for a generic printer, but the cross-field validator
// forbids the trio in that mode — so a faithful read must null it to round-trip.
// In custom mode `ppd` is reconciled against currentPPD rather than copied,
// because the input builder always emits it and an omitted `ppd` comes back
// as "" beside a populated `ppd_path`.
func ppdTrioValues(p *proclassic.Printer, currentPPD types.String) (ppd, ppdPath types.String, ppdContents trimmedStringValue) {
	if p.UseGeneric == nil || *p.UseGeneric {
		return types.StringNull(), types.StringNull(), trimmedStringValueFromPtr(nil)
	}
	return helpers.ReconcileOptionalStringPointer(p.Ppd, currentPPD),
		helpers.StringPointerValueOrNull(p.PpdPath),
		trimmedStringValueFromPtr(p.PpdContents)
}

// assignPrinterDataSourceModel populates a data source model from a Printer
// response. Symmetric with the resource builder but always copies the API
// value over the user's selector (the selector is just an input — output is
// Computed).
func assignPrinterDataSourceModel(state *PrinterDataSourceModel, p *proclassic.Printer) diag.Diagnostics {
	var diags diag.Diagnostics
	if p == nil {
		return diags
	}
	if p.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(p.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(p.Name)
	state.Category = decodeCategory(p.Category)
	state.URI = helpers.StringPointerValueOrNull(p.URI)
	state.CUPSName = helpers.StringPointerValueOrNull(p.CUPSName)
	state.Location = helpers.StringPointerValueOrNull(p.Location)
	state.Model = helpers.StringPointerValueOrNull(p.Model)
	state.Info = helpers.StringPointerValueOrNull(p.Info)
	state.Notes = helpers.StringPointerValueOrNull(p.Notes)
	state.MakeDefault = helpers.BoolPointerValueOrNull(p.MakeDefault)
	state.UseGeneric = helpers.BoolPointerValueOrNull(p.UseGeneric)
	state.PPD, state.PPDPath, state.PPDContents = ppdTrioValues(p, types.StringNull())
	state.Shared = helpers.BoolPointerValueOrNull(p.Shared)
	state.OSRequirements = helpers.StringPointerValueOrNull(p.OsRequirements)
	return diags
}

// decodeCategory maps a Printer.Category wire value into the TF
// representation. Nil, empty, and the server sentinel
// `categoryUnassignedSentinel` all map to null so the sentinel never leaks
// into plans or state.
func decodeCategory(p *string) types.String {
	if p == nil || *p == "" || *p == categoryUnassignedSentinel {
		return types.StringNull()
	}
	return types.StringValue(*p)
}
