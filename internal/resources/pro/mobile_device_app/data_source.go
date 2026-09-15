// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

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

// MobileAppDataSource implements the Terraform data source for Jamf Pro mobile
// device apps. Lookup is by ID or by exact name — exactly one must be supplied.
// The data source surfaces a flat Computed projection of the most-frequently
// looked-up fields. For full detail, manage the app as a resource or import it.
type MobileAppDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &MobileAppDataSource{}
	_ datasource.DataSourceWithConfigure        = &MobileAppDataSource{}
	_ datasource.DataSourceWithConfigValidators = &MobileAppDataSource{}
)

// NewMobileAppDataSource returns a new instance of MobileAppDataSource.
func NewMobileAppDataSource() datasource.DataSource {
	return &MobileAppDataSource{}
}

// Metadata sets the data source type name.
func (d *MobileAppDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_app"
}

// Schema returns the data source schema.
func (d *MobileAppDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro mobile device app by ID or by exact name. Exactly one of `id` or `name` must be supplied. Surfaces a flat read-only projection; manage the app as a resource for full detail." + dataSourcePrivileges,
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
			"version":   schema.StringAttribute{MarkdownDescription: "App version string.", Computed: true},
			"bundle_id": schema.StringAttribute{MarkdownDescription: "App bundle identifier.", Computed: true},
			// os_type is populated for in-house apps once it has been set (the
			// server echoes it on GET after a PUT); null for never-set or
			// externally-hosted apps.
			"os_type":         schema.StringAttribute{MarkdownDescription: "Operating system the app targets (`iOS` or `tvOS`). Populated for in-house apps once set; null otherwise.", Computed: true},
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
func (d *MobileAppDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *MobileAppDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an app by ID or by name and populates Terraform state.
func (d *MobileAppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data MobileAppDataSourceModel
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
		got *proclassic.MobileDeviceApplication
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetMobileDeviceApplicationByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetMobileDeviceApplicationByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing app selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro mobile device app", helpers.APIErrorDetail(err))
		return
	}
	assignMobileAppFlatDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro mobile device app data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignMobileAppFlatDataSource projects a *proclassic.MobileDeviceApplication
// into the flat data source model.
func assignMobileAppFlatDataSource(state *MobileAppDataSourceModel, a *proclassic.MobileDeviceApplication) {
	if a == nil {
		return
	}
	if id := extractMobileAppID(a); id != "" {
		state.ID = helpers.StringPointerValueOrNull(&id)
	}
	if a.General != nil {
		state.Name = helpers.StringPointerValueOrNull(a.General.Name)
		state.Version = helpers.StringPointerValueOrNull(a.General.Version)
		state.BundleID = helpers.StringPointerValueOrNull(a.General.BundleID)
		state.OsType = helpers.StringPointerValueOrNull(a.General.OsType)
		state.IsFree = helpers.BoolPointerValueOrNull(a.General.Free)
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
