// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// PrinterResourceModel represents the Terraform resource model for a Jamf Pro
// printer. The classic /printers endpoint exposes a flat 16-field shape with
// two non-trivial behaviours: the `category` field round-trips through a
// server sentinel ("No category assigned" == null), and `use_generic` ⇒
// PPD-trio cross-field rules are enforced server-side and mirrored here at
// plan time by a ConfigValidator.
//
// `ppd_contents` uses a custom string type with trailing-whitespace semantic
// equality (see custom_types.go) so the server's trailing-whitespace strip
// does not surface as drift.
type PrinterResourceModel struct {
	ID             types.String           `tfsdk:"id"`
	Name           types.String           `tfsdk:"name"`
	Category       types.String           `tfsdk:"category"`
	URI            types.String           `tfsdk:"uri"`
	CUPSName       types.String           `tfsdk:"cups_name"`
	Location       types.String           `tfsdk:"location"`
	Model          types.String           `tfsdk:"model"`
	Info           types.String           `tfsdk:"info"`
	Notes          types.String           `tfsdk:"notes"`
	MakeDefault    types.Bool             `tfsdk:"make_default"`
	UseGeneric     types.Bool             `tfsdk:"use_generic"`
	PPD            types.String           `tfsdk:"ppd"`
	PPDPath        types.String           `tfsdk:"ppd_path"`
	PPDContents    trimmedStringValue     `tfsdk:"ppd_contents"`
	Shared         types.Bool             `tfsdk:"shared"`
	OSRequirements types.String           `tfsdk:"os_requirements"`
	Timeouts       resourceTimeouts.Value `tfsdk:"timeouts"`
}

// PrinterDataSourceModel represents the Terraform data source model for a
// Jamf Pro printer. Either id or name must be supplied (enforced by
// ExactlyOneOf at config validation).
type PrinterDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	Name           types.String             `tfsdk:"name"`
	Category       types.String             `tfsdk:"category"`
	URI            types.String             `tfsdk:"uri"`
	CUPSName       types.String             `tfsdk:"cups_name"`
	Location       types.String             `tfsdk:"location"`
	Model          types.String             `tfsdk:"model"`
	Info           types.String             `tfsdk:"info"`
	Notes          types.String             `tfsdk:"notes"`
	MakeDefault    types.Bool               `tfsdk:"make_default"`
	UseGeneric     types.Bool               `tfsdk:"use_generic"`
	PPD            types.String             `tfsdk:"ppd"`
	PPDPath        types.String             `tfsdk:"ppd_path"`
	PPDContents    trimmedStringValue       `tfsdk:"ppd_contents"`
	Shared         types.Bool               `tfsdk:"shared"`
	OSRequirements types.String             `tfsdk:"os_requirements"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// printerIdentityModel represents the identity object for printer resources
// and list results.
type printerIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// PrinterListResourceModel represents the config model for printer list
// queries. Classic has no RSQL — the filter shape is the shared client-side
// substring block.
type PrinterListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
