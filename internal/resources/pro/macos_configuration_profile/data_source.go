// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DataSource looks up a macOS configuration profile by ID or by name.
type DataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &DataSource{}
	_ datasource.DataSourceWithConfigure        = &DataSource{}
	_ datasource.DataSourceWithConfigValidators = &DataSource{}
)

// FlatDataSourceModel is the flat read-only projection — the resource's
// nested model would be heavy for a lookup-by-name pattern.
type FlatDataSourceModel struct {
	ID                 types.String   `tfsdk:"id"`
	Name               types.String   `tfsdk:"name"`
	Description        types.String   `tfsdk:"description"`
	Level              types.String   `tfsdk:"level"`
	DistributionMethod types.String   `tfsdk:"distribution_method"`
	UserRemovable      types.Bool     `tfsdk:"user_removable"`
	RedeployOnUpdate   types.String   `tfsdk:"redeploy_on_update"`
	UUID               types.String   `tfsdk:"uuid"`
	CategoryID         types.String   `tfsdk:"category_id"`
	CategoryName       types.String   `tfsdk:"category_name"`
	SiteID             types.String   `tfsdk:"site_id"`
	SiteName           types.String   `tfsdk:"site_name"`
	Timeouts           timeouts.Value `tfsdk:"timeouts"`
}

// NewDataSource returns a new DataSource instance.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata sets the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_macos_configuration_profile"
}

// Schema returns the data source schema.
func (d *DataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a macOS configuration profile by ID or by exact name. Exactly one of `id` or `name` must be supplied. Returns a flat Computed projection of the most-frequently looked-up fields. To manage the full payload, use the `jamfplatform_pro_macos_configuration_profile` resource." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":                  schema.StringAttribute{MarkdownDescription: "Profile ID. Mutually exclusive with `name`.", Optional: true, Computed: true},
			"name":                schema.StringAttribute{MarkdownDescription: "Profile display name (exact match). Mutually exclusive with `id`.", Optional: true, Computed: true},
			"description":         schema.StringAttribute{MarkdownDescription: "Profile description.", Computed: true},
			"level":               schema.StringAttribute{MarkdownDescription: "Delivery level. Either `Computer Level` or `User Level`.", Computed: true},
			"distribution_method": schema.StringAttribute{MarkdownDescription: "How the profile reaches devices.", Computed: true},
			"user_removable":      schema.BoolAttribute{MarkdownDescription: "Whether the device user can remove the profile.", Computed: true},
			"redeploy_on_update":  schema.StringAttribute{MarkdownDescription: "Re-deploy policy on update.", Computed: true},
			"uuid":                schema.StringAttribute{MarkdownDescription: "Profile UUID assigned by Jamf Pro.", Computed: true},
			"category_id":         schema.StringAttribute{MarkdownDescription: "Category ID. `-1` means no category.", Computed: true},
			"category_name":       schema.StringAttribute{MarkdownDescription: "Category display name.", Computed: true},
			"site_id":             schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means no site.", Computed: true},
			"site_name":           schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
			"timeouts":            timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces id-xor-name.
func (d *DataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the SDK client.
func (d *DataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_macos_configuration_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read resolves the profile by ID or by name.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", providerNotConfigured)
		return
	}
	var data FlatDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	readTimeout, td := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(td...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var (
		got *proclassic.OsXConfigurationProfile
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetOSXConfigurationProfileByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetOSXConfigurationProfileByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing profile selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find macOS configuration profile", helpers.APIErrorDetail(err))
		return
	}
	assignFlatDataSource(&data, got)
	tflog.Trace(ctx, "read jamfplatform_pro_macos_configuration_profile data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func assignFlatDataSource(state *FlatDataSourceModel, p *proclassic.OsXConfigurationProfile) {
	if p == nil {
		return
	}
	if p.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(p.ID)
	}
	if p.General != nil {
		if p.General.Name != nil {
			state.Name = helpers.StringPointerValueOrNull(p.General.Name)
		}
		state.Description = helpers.StringPointerValueOrNull(p.General.Description)
		if p.General.Level != nil {
			state.Level = types.StringValue(levelFromWireRead(*p.General.Level))
		}
		state.DistributionMethod = helpers.StringPointerValueOrNull(p.General.DistributionMethod)
		state.UserRemovable = helpers.BoolPointerValueOrNull(p.General.UserRemovable)
		state.RedeployOnUpdate = helpers.StringPointerValueOrNull(p.General.RedeployOnUpdate)
		state.UUID = helpers.StringPointerValueOrNull(p.General.UUID)
		if p.General.Category != nil {
			state.CategoryID = helpers.StringValueFromIntPtr(p.General.Category.ID)
			state.CategoryName = helpers.DerivedRefName(p.General.Category.ID, p.General.Category.Name)
		}
		if p.General.Site != nil {
			state.SiteID = helpers.StringValueFromIntPtr(p.General.Site.ID)
			state.SiteName = helpers.DerivedRefName(p.General.Site.ID, p.General.Site.Name)
		}
	}
}
