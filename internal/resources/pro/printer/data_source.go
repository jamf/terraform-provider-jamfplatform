// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PrinterDataSource implements the Terraform data source for Jamf Pro printers.
// The singular data source supports lookup by ID OR by name — exactly one of
// the two must be supplied.
type PrinterDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &PrinterDataSource{}
	_ datasource.DataSourceWithConfigure        = &PrinterDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PrinterDataSource{}
)

// NewPrinterDataSource returns a new instance of PrinterDataSource.
func NewPrinterDataSource() datasource.DataSource {
	return &PrinterDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PrinterDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_printer"
}

// Schema returns the data source schema.
func (d *PrinterDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro printer by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Printer ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Printer display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"category":        schema.StringAttribute{MarkdownDescription: "Assigned Jamf Pro category name, or null if no category is assigned.", Computed: true},
			"uri":             schema.StringAttribute{MarkdownDescription: "Device URI of the printer.", Computed: true},
			"cups_name":       schema.StringAttribute{MarkdownDescription: "CUPS queue name.", Computed: true},
			"location":        schema.StringAttribute{MarkdownDescription: "Physical location of the printer.", Computed: true},
			"model":           schema.StringAttribute{MarkdownDescription: "Printer model.", Computed: true},
			"info":            schema.StringAttribute{MarkdownDescription: "Free-text information shown to administrators.", Computed: true},
			"notes":           schema.StringAttribute{MarkdownDescription: "Free-text notes.", Computed: true},
			"make_default":    schema.BoolAttribute{MarkdownDescription: "Whether this printer is set as default when mapped.", Computed: true},
			"use_generic":     schema.BoolAttribute{MarkdownDescription: "Whether the bundled macOS Generic.ppd is used.", Computed: true},
			"ppd":             schema.StringAttribute{MarkdownDescription: "Short name of the PPD file.", Computed: true},
			"ppd_path":        schema.StringAttribute{MarkdownDescription: "Filesystem path to the PPD file.", Computed: true},
			"ppd_contents":    schema.StringAttribute{MarkdownDescription: "Inline contents of the PPD file. Trailing whitespace is semantically ignored, because Jamf Pro strips it on every round-trip.", CustomType: trimmedStringType{}, Computed: true},
			"shared":          schema.BoolAttribute{MarkdownDescription: "Whether the printer is shared.", Computed: true},
			"os_requirements": schema.StringAttribute{MarkdownDescription: "Operating-system version requirement.", Computed: true},
			"timeouts":        timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *PrinterDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the
// shared providerdata.ConfigureProClassic helper.
func (d *PrinterDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_printer")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a printer by ID or by name and populates Terraform state.
func (d *PrinterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PrinterDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var (
		got *proclassic.Printer
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetPrinterByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetPrinterByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing printer selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro printer", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignPrinterDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro printer data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
