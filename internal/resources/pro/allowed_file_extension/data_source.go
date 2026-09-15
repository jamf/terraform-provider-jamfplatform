// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

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

// AllowedFileExtensionDataSource implements the Terraform data source for Jamf Pro
// allowed file extensions. The singular data source supports lookup by ID OR by
// extension — exactly one of the two must be supplied.
type AllowedFileExtensionDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &AllowedFileExtensionDataSource{}
	_ datasource.DataSourceWithConfigure        = &AllowedFileExtensionDataSource{}
	_ datasource.DataSourceWithConfigValidators = &AllowedFileExtensionDataSource{}
)

// NewAllowedFileExtensionDataSource returns a new instance of AllowedFileExtensionDataSource.
func NewAllowedFileExtensionDataSource() datasource.DataSource {
	return &AllowedFileExtensionDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AllowedFileExtensionDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_allowed_file_extension"
}

// Schema returns the data source schema.
func (d *AllowedFileExtensionDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro allowed file extension by ID or by extension. Exactly one of `id` or `extension` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Allowed file extension ID. Mutually exclusive with `extension`.",
				Optional:            true,
				Computed:            true,
			},
			// Lookup by extension matches case-insensitively, so the returned record
			// reflects Jamf Pro's stored spelling (which may differ in case from the value
			// supplied here).
			"extension": schema.StringAttribute{
				MarkdownDescription: "File extension (matched case-insensitively). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / extension is supplied.
func (d *AllowedFileExtensionDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("extension"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *AllowedFileExtensionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_allowed_file_extension")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an allowed file extension by ID or by extension and populates state.
func (d *AllowedFileExtensionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AllowedFileExtensionDataSourceModel
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
		got *proclassic.AllowedFileExtension
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetAllowedFileExtensionByID(readCtx, data.ID.ValueString())
	case !data.Extension.IsNull() && data.Extension.ValueString() != "":
		got, err = d.client.GetAllowedFileExtensionByExtension(readCtx, data.Extension.ValueString())
	default:
		resp.Diagnostics.AddError("Missing allowed file extension selector", "Exactly one of id or extension must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro allowed file extension", helpers.APIErrorDetail(err))
		return
	}
	assignAllowedFileExtensionDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro allowed file extension data source", map[string]any{"id": data.ID.ValueString(), "extension": data.Extension.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
