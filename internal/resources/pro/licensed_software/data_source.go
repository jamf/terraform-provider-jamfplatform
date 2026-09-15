// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

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

// LicensedSoftwareDataSource implements the Terraform data source for Jamf Pro
// licensed software records. Lookup is by ID or by exact name — exactly one must
// be supplied. The data source surfaces a flat Computed projection of the
// general header; manage the record as a resource or import it for full detail.
type LicensedSoftwareDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &LicensedSoftwareDataSource{}
	_ datasource.DataSourceWithConfigure        = &LicensedSoftwareDataSource{}
	_ datasource.DataSourceWithConfigValidators = &LicensedSoftwareDataSource{}
)

// NewLicensedSoftwareDataSource returns a new instance of LicensedSoftwareDataSource.
func NewLicensedSoftwareDataSource() datasource.DataSource {
	return &LicensedSoftwareDataSource{}
}

// Metadata sets the data source type name.
func (d *LicensedSoftwareDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_licensed_software"
}

// Schema returns the data source schema.
func (d *LicensedSoftwareDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro licensed software record by ID or by exact name. Exactly one of `id` or `name` must be supplied. Surfaces a flat read-only projection of the general header; manage the record as a resource for the nested software definitions and licences." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Record ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"publisher":                            schema.StringAttribute{MarkdownDescription: "Name of the licensed software publisher.", Computed: true},
			"platform":                             schema.StringAttribute{MarkdownDescription: "Platform the software is for.", Computed: true},
			"notes":                                schema.StringAttribute{MarkdownDescription: "Notes about the record.", Computed: true},
			"send_email_on_violation":              schema.BoolAttribute{MarkdownDescription: "Whether an email notification is sent when the licence count is exceeded.", Computed: true},
			"remove_titles_from_inventory_reports": schema.BoolAttribute{MarkdownDescription: "Whether matched titles are excluded from inventory reports.", Computed: true},
			"exclude_titles_purchased_from_app_store": schema.BoolAttribute{MarkdownDescription: "Whether Mac App Store copies are excluded from the licence count.", Computed: true},
			"site_id":   schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means no site.", Computed: true},
			"site_name": schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
			"timeouts":  timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *LicensedSoftwareDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *LicensedSoftwareDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_licensed_software")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a record by ID or by name and populates Terraform state.
func (d *LicensedSoftwareDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data LicensedSoftwareDataSourceModel
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
		got *proclassic.LicensedSoftware
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetLicensedSoftwareByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetLicensedSoftwareByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing record selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro licensed software", helpers.APIErrorDetail(err))
		return
	}
	assignLicensedSoftwareFlatDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro licensed software data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignLicensedSoftwareFlatDataSource projects a *proclassic.LicensedSoftware
// into the flat data source model.
func assignLicensedSoftwareFlatDataSource(state *LicensedSoftwareDataSourceModel, ls *proclassic.LicensedSoftware) {
	if ls == nil {
		return
	}
	if id := extractLicensedSoftwareID(ls); id != "" {
		state.ID = helpers.StringPointerValueOrNull(&id)
	}
	if ls.General != nil {
		g := ls.General
		state.Name = helpers.StringPointerValueOrNull(g.Name)
		state.Publisher = stringValueOrNullEmpty(g.Publisher)
		state.Platform = stringValueOrNullEmpty(g.Platform)
		state.Notes = stringValueOrNullEmpty(g.Notes)
		state.SendEmailOnViolation = helpers.BoolPointerValueOrNull(g.SendEmailOnViolation)
		state.RemoveTitlesFromInventoryReports = helpers.BoolPointerValueOrNull(g.RemoveTitlesFromInventoryReports)
		state.ExcludeTitlesPurchasedFromAppStore = helpers.BoolPointerValueOrNull(g.ExcludeTitlesPurchasedFromAppStore)
		if g.Site != nil {
			state.SiteID = helpers.StringValueFromIntPtr(g.Site.ID)
			state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
		}
	}
}
