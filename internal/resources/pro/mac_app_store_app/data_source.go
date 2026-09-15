// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

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

// MacAppDataSource implements the Terraform data source for Jamf Pro App Store
// Mac apps. Lookup is by ID or by exact name — exactly one must be supplied.
// The data source surfaces a flat Computed projection of the most-frequently
// looked-up fields. For full detail, manage the app as a resource or import it.
type MacAppDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &MacAppDataSource{}
	_ datasource.DataSourceWithConfigure        = &MacAppDataSource{}
	_ datasource.DataSourceWithConfigValidators = &MacAppDataSource{}
)

// NewMacAppDataSource returns a new instance of MacAppDataSource.
func NewMacAppDataSource() datasource.DataSource {
	return &MacAppDataSource{}
}

// Metadata sets the data source type name.
func (d *MacAppDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mac_app_store_app"
}

// Schema returns the data source schema.
func (d *MacAppDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro App Store Mac app by ID or by exact name. Exactly one of `id` or `name` must be supplied. Surfaces a flat read-only projection; manage the app as a resource for full detail." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "App display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"version":         schema.StringAttribute{MarkdownDescription: "App version string.", Computed: true},
			"bundle_id":       schema.StringAttribute{MarkdownDescription: "App bundle identifier.", Computed: true},
			"is_free":         schema.BoolAttribute{MarkdownDescription: "Whether the app is free.", Computed: true},
			"deployment_type": schema.StringAttribute{MarkdownDescription: "Install method.", Computed: true},
			"category_id":     schema.StringAttribute{MarkdownDescription: "Category ID.", Computed: true},
			"category_name":   schema.StringAttribute{MarkdownDescription: "Category display name.", Computed: true},
			"site_id":         schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means no site.", Computed: true},
			"site_name":       schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
			"timeouts":        timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *MacAppDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *MacAppDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mac_app_store_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an app by ID or by name and populates Terraform state.
func (d *MacAppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data MacAppDataSourceModel
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
		got *proclassic.MacApplication
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetMacApplicationByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetMacApplicationByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing app selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro Mac App Store app", helpers.APIErrorDetail(err))
		return
	}
	assignMacAppFlatDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro Mac App Store app data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignMacAppFlatDataSource projects a *proclassic.MacApplication into the
// flat data source model.
func assignMacAppFlatDataSource(state *MacAppDataSourceModel, a *proclassic.MacApplication) {
	if a == nil {
		return
	}
	if id := extractMacAppID(a); id != "" {
		state.ID = helpers.StringPointerValueOrNull(&id)
	}
	if a.General != nil {
		state.Name = helpers.StringPointerValueOrNull(a.General.Name)
		state.Version = helpers.StringPointerValueOrNull(a.General.Version)
		state.BundleID = helpers.StringPointerValueOrNull(a.General.BundleID)
		state.IsFree = helpers.BoolPointerValueOrNull(a.General.IsFree)
		state.DeploymentType = helpers.StringPointerValueOrNull(a.General.DeploymentType)
		if a.General.Category != nil {
			state.CategoryID = helpers.StringValueFromIntPtr(a.General.Category.ID)
			state.CategoryName = helpers.DerivedRefName(a.General.Category.ID, a.General.Category.Name)
		}
		if a.General.Site != nil {
			state.SiteID = helpers.StringValueFromIntPtr(a.General.Site.ID)
			state.SiteName = helpers.DerivedRefName(a.General.Site.ID, a.General.Site.Name)
		}
	}
}
