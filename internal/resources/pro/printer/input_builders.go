// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildPrinterInput converts the Terraform plan model into the SDK Printer
// payload used for both Create and Update. ID is omitted on write — Create
// uses path id="0" and Update derives it from state.
//
// The classic /printers PUT merges field by field: an omitted element keeps
// the stored value and an empty element clears it (wire-probed 2026-09-06 on
// Jamf Pro 11.31.1 for every string below, issue #384). Two emission rules
// follow:
//
//   - Every plain Optional string — `category`, `uri`, `cups_name`,
//     `location`, `model`, `info`, `notes`, `os_requirements` and `ppd` — is
//     emitted on every write through helpers.AlwaysEmitStringPointer, empty
//     when null, so a value the user removes from config is cleared rather
//     than retained and echoed back as an inconsistent result. `category`
//     clears to the server sentinel categoryUnassignedSentinel; the state
//     builder folds both the sentinel and an echoed "" back to null. `ppd` is
//     safe to send empty in either mode: under `use_generic = false` the
//     gate is `ppd_path`, and an empty `<ppd>` beside a populated
//     `<ppd_path>` leaves `use_generic` false (probed); under
//     `use_generic = true` the server discards the trio anyway.
//
//   - `ppd_path` and `ppd_contents` are emitted only when set. `ppd_path` is
//     Optional+Computed: when `use_generic = true` the server forces it to the
//     bundled Generic.ppd path regardless of input, and the ConfigValidator
//     blocks the user from setting the trio in that combination, so a null
//     model value means "server-owned" and must omit the tag.
func buildPrinterInput(plan PrinterResourceModel) *proclassic.Printer {
	return &proclassic.Printer{
		Name:           helpers.OptionalStringPointer(plan.Name),
		Category:       helpers.AlwaysEmitStringPointer(plan.Category),
		URI:            helpers.AlwaysEmitStringPointer(plan.URI),
		CUPSName:       helpers.AlwaysEmitStringPointer(plan.CUPSName),
		Location:       helpers.AlwaysEmitStringPointer(plan.Location),
		Model:          helpers.AlwaysEmitStringPointer(plan.Model),
		Info:           helpers.AlwaysEmitStringPointer(plan.Info),
		Notes:          helpers.AlwaysEmitStringPointer(plan.Notes),
		MakeDefault:    helpers.OptionalBoolPointer(plan.MakeDefault),
		UseGeneric:     helpers.OptionalBoolPointer(plan.UseGeneric),
		Ppd:            helpers.AlwaysEmitStringPointer(plan.PPD),
		PpdPath:        helpers.OptionalStringPointer(plan.PPDPath),
		PpdContents:    helpers.OptionalStringPointer(plan.PPDContents.StringValue),
		Shared:         helpers.OptionalBoolPointer(plan.Shared),
		OsRequirements: helpers.AlwaysEmitStringPointer(plan.OSRequirements),
	}
}
