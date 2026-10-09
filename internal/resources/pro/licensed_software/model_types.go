// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// LicensedSoftwareResourceModel is the Terraform resource model for a Jamf Pro
// licensed software record (the classic /licensedsoftware endpoint). User-facing
// attribute names mirror the Jamf Pro admin UI; the differing wire names are
// recorded in input_builders.go / state_builders.go.
//
// The user-managed collections (software_definitions, licenses) are Optional Go
// typed slices reconciled POSITIONALLY — neither carries a server-readable id on
// this endpoint (wire-probed: GET-by-id returns idless <license> and
// <definition> elements; the server preserves send-order). The Computed-only
// collections (computers, licenses[].attachments) are types.List instead,
// because a Computed attribute is Unknown at plan time and a Go typed slice
// cannot carry an unknown value. See LICENSED_SOFTWARE_SPIKE.md.
type LicensedSoftwareResourceModel struct {
	ID                                 types.String                      `tfsdk:"id"`
	Name                               types.String                      `tfsdk:"name"`
	Publisher                          types.String                      `tfsdk:"publisher"`
	Platform                           types.String                      `tfsdk:"platform"`
	Notes                              types.String                      `tfsdk:"notes"`
	SendEmailOnViolation               types.Bool                        `tfsdk:"send_email_on_violation"`
	RemoveTitlesFromInventoryReports   types.Bool                        `tfsdk:"remove_titles_from_inventory_reports"`
	ExcludeTitlesPurchasedFromAppStore types.Bool                        `tfsdk:"exclude_titles_purchased_from_app_store"`
	SiteID                             types.String                      `tfsdk:"site_id"`
	SiteName                           types.String                      `tfsdk:"site_name"`
	SoftwareDefinitions                []LicensedSoftwareDefinitionModel `tfsdk:"software_definitions"`
	Licenses                           []LicensedSoftwareLicenseModel    `tfsdk:"licenses"`
	// Computers is Computed-only, hence types.List (Unknown at plan time).
	Computers types.List             `tfsdk:"computers"`
	Timeouts  resourceTimeouts.Value `tfsdk:"timeouts"`
}

// LicensedSoftwareDefinitionModel models one <software_definitions><definition>.
// font_definitions and plugin_definitions are NOT modeled: the server silently
// drops them on write on the supported Jamf Pro versions (wire-probed on both
// create and update — never echoed back). The modern admin UI exposes only the
// "Software Definitions" tab. Do not re-add those buckets without re-probing.
type LicensedSoftwareDefinitionModel struct {
	Name        types.String `tfsdk:"name"`
	Version     types.String `tfsdk:"version"`
	CompareType types.String `tfsdk:"compare_type"`
}

// LicensedSoftwareLicenseModel models one <licenses><license>. No id field: the
// endpoint never returns a per-license id, so licenses reconcile positionally.
type LicensedSoftwareLicenseModel struct {
	SerialNumber1    types.String                     `tfsdk:"serial_number_1"`
	SerialNumber2    types.String                     `tfsdk:"serial_number_2"`
	OrganizationName types.String                     `tfsdk:"organization_name"`
	RegisteredTo     types.String                     `tfsdk:"registered_to"`
	LicenseType      types.String                     `tfsdk:"license_type"`
	LicenseCount     types.Int64                      `tfsdk:"license_count"`
	Notes            types.String                     `tfsdk:"notes"`
	Purchasing       *LicensedSoftwarePurchasingModel `tfsdk:"purchasing"`
	// Attachments is Computed-only, hence types.List (Unknown at plan time).
	Attachments types.List `tfsdk:"attachments"`
}

// LicensedSoftwarePurchasingModel models <license><purchasing>. The UI radio
// "License Term" (is_perpetual / is_annual, server-enforced exactly-one) is
// collapsed into the license_term enum {perpetual, annual}. The *_epoch / *_utc
// siblings are server-derived from po_date / license_expires and are
// Computed-only (see feedback_server_derived_echo_attrs).
type LicensedSoftwarePurchasingModel struct {
	LicenseTerm         types.String `tfsdk:"license_term"`
	PoNumber            types.String `tfsdk:"po_number"`
	PoDate              types.String `tfsdk:"po_date"`
	PoDateEpoch         types.Int64  `tfsdk:"po_date_epoch"`
	PoDateUtc           types.String `tfsdk:"po_date_utc"`
	Vendor              types.String `tfsdk:"vendor"`
	LicenseExpires      types.String `tfsdk:"license_expires"`
	LicenseExpiresEpoch types.Int64  `tfsdk:"license_expires_epoch"`
	LicenseExpiresUtc   types.String `tfsdk:"license_expires_utc"`
	PurchasePrice       types.String `tfsdk:"purchase_price"`
	LifeExpectancy      types.Int64  `tfsdk:"life_expectancy"`
	PurchasingAccount   types.String `tfsdk:"purchasing_account"`
	PurchasingContact   types.String `tfsdk:"purchasing_contact"`
}

// LicensedSoftwareAttachmentModel models a read-only <license><attachments>
// entry. Attachments are uploaded via a separate endpoint and only echoed back
// here; the block is Computed-only and never sent on write.
type LicensedSoftwareAttachmentModel struct {
	ID       types.String `tfsdk:"id"`
	Filename types.String `tfsdk:"filename"`
	URI      types.String `tfsdk:"uri"`
}

// LicensedSoftwareComputerModel models a read-only <computers><computer> entry —
// the set of machines the server matched against the definitions (admin UI
// usage view). Computed-only.
type LicensedSoftwareComputerModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	UDID types.String `tfsdk:"udid"`
}

// LicensedSoftwareDataSourceModel is the flat data source model — a read-only
// projection of the general header fields so users can resolve IDs by name.
// Manage the record as a resource for the full nested detail.
type LicensedSoftwareDataSourceModel struct {
	ID                                 types.String             `tfsdk:"id"`
	Name                               types.String             `tfsdk:"name"`
	Publisher                          types.String             `tfsdk:"publisher"`
	Platform                           types.String             `tfsdk:"platform"`
	Notes                              types.String             `tfsdk:"notes"`
	SendEmailOnViolation               types.Bool               `tfsdk:"send_email_on_violation"`
	RemoveTitlesFromInventoryReports   types.Bool               `tfsdk:"remove_titles_from_inventory_reports"`
	ExcludeTitlesPurchasedFromAppStore types.Bool               `tfsdk:"exclude_titles_purchased_from_app_store"`
	SiteID                             types.String             `tfsdk:"site_id"`
	SiteName                           types.String             `tfsdk:"site_name"`
	Timeouts                           datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// licensedSoftwareIdentityModel represents the identity object for the resource
// and list results.
type licensedSoftwareIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// LicensedSoftwareListResourceModel represents the config model for list
// queries. Classic has no RSQL — the filter shape is the shared client-side
// substring block.
type LicensedSoftwareListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
