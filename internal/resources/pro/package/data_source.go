// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PackageDataSource implements the Terraform data source for Jamf Pro
// packages. Supports lookup by either `id` or `display_name`; exactly one
// must be supplied.
type PackageDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &PackageDataSource{}
	_ datasource.DataSourceWithConfigure        = &PackageDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PackageDataSource{}
)

// NewPackageDataSource returns a new instance of the data source.
func NewPackageDataSource() datasource.DataSource {
	return &PackageDataSource{}
}

// Metadata sets the data source type name.
func (d *PackageDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_package"
}

// Schema returns the data source schema. Every attribute is Computed
// (apart from the id/display_name selectors, which are Optional+Computed).
func (d *PackageDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro package by ID or by exact display name. Exactly one of `id` or `display_name` must be supplied. Returns the full record including manifest body, every hash populated by Jamf Pro, and cloud distribution point transfer status." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Package ID. Mutually exclusive with `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Package display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"file_name":                    schema.StringAttribute{MarkdownDescription: "On-disk filename Jamf Pro associates with the binary.", Computed: true},
			"category_id":                  schema.StringAttribute{MarkdownDescription: "Category ID (`\"-1\"` sentinel for None).", Computed: true},
			"info":                         schema.StringAttribute{MarkdownDescription: "Free-form info field.", Computed: true},
			"notes":                        schema.StringAttribute{MarkdownDescription: "Free-form notes field.", Computed: true},
			"priority":                     schema.Int64Attribute{MarkdownDescription: "Install priority (1–20).", Computed: true},
			"fill_user_template":           schema.BoolAttribute{MarkdownDescription: "Fill User Template (FUT) flag.", Computed: true},
			"fill_existing_users":          schema.BoolAttribute{MarkdownDescription: "Fill Existing User home directories (FEU) flag.", Computed: true},
			"reboot_required":              schema.BoolAttribute{MarkdownDescription: "Requires restart flag.", Computed: true},
			"os_requirements":              schema.StringAttribute{MarkdownDescription: "Operating system requirements (comma-separated).", Computed: true},
			"available_in_software_update": schema.BoolAttribute{MarkdownDescription: "Available in Software Update flag.", Computed: true},
			"manifest":                     schema.StringAttribute{MarkdownDescription: "Raw plist manifest body.", Computed: true},
			"manifest_file_name":           schema.StringAttribute{MarkdownDescription: "Manifest upload filename.", Computed: true},
			"sha3_512":                     schema.StringAttribute{MarkdownDescription: "SHA-3-512 hex digest of the package binary.", Computed: true},
			"sha256":                       schema.StringAttribute{MarkdownDescription: "SHA-256 hex digest.", Computed: true},
			"md5":                          schema.StringAttribute{MarkdownDescription: "MD5 hex digest.", Computed: true},
			"hash_type":                    schema.StringAttribute{MarkdownDescription: "Hash algorithm tag.", Computed: true},
			"hash_value":                   schema.StringAttribute{MarkdownDescription: "Primary hash value.", Computed: true},
			"size":                         schema.StringAttribute{MarkdownDescription: "Package binary size in bytes. Returned by Jamf Pro; not user-settable.", Computed: true},
			"install_language":             schema.StringAttribute{MarkdownDescription: "Locale tag (default `\"en_US\"`).", Computed: true},
			"parent_package_id":            schema.StringAttribute{MarkdownDescription: "Parent package ID (`\"-1\"` for no parent).", Computed: true},
			"self_healing_action":          schema.StringAttribute{MarkdownDescription: "Self-healing action.", Computed: true},
			"self_heal_notify":             schema.BoolAttribute{MarkdownDescription: "Self-healing notify flag.", Computed: true},
			"cloud_transfer_status":        schema.StringAttribute{MarkdownDescription: "Cloud distribution point transfer status.", Computed: true},
			"indexed":                      schema.BoolAttribute{MarkdownDescription: "Indexing telemetry.", Computed: true},
			"format":                       schema.StringAttribute{MarkdownDescription: "Distribution-point format.", Computed: true},
			"timeouts":                     timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / display_name is supplied.
func (d *PackageDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("display_name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *PackageDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_package")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a package by ID or by display name.
func (d *PackageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PackageDataSourceModel
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
		got *pro.Package
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetPackageV1(readCtx, data.ID.ValueString())
	case !data.DisplayName.IsNull() && data.DisplayName.ValueString() != "":
		got, err = d.client.ResolvePackageV1ByName(readCtx, data.DisplayName.ValueString())
	default:
		resp.Diagnostics.AddError("Missing package selector", "Exactly one of id or display_name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro package", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignPackageDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro package data source", map[string]any{"id": data.ID.ValueString(), "display_name": data.DisplayName.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
