// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"context"
	"fmt"

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

// JSONWebTokenConfigurationDataSource implements the Terraform data source for
// Jamf Pro JSON Web Token configurations. Lookup is by ID or by exact name —
// exactly one must be supplied. The encryption key is not surfaced (Jamf Pro
// never returns it).
type JSONWebTokenConfigurationDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &JSONWebTokenConfigurationDataSource{}
	_ datasource.DataSourceWithConfigure        = &JSONWebTokenConfigurationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &JSONWebTokenConfigurationDataSource{}
)

// NewJSONWebTokenConfigurationDataSource returns a new instance of
// JSONWebTokenConfigurationDataSource.
func NewJSONWebTokenConfigurationDataSource() datasource.DataSource {
	return &JSONWebTokenConfigurationDataSource{}
}

// Metadata sets the data source type name.
func (d *JSONWebTokenConfigurationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_json_web_token_configuration"
}

// Schema returns the data source schema.
func (d *JSONWebTokenConfigurationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro JSON Web Token configuration by ID or by exact display name. Exactly one of `id` or `name` must be supplied. The encryption key is never surfaced." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "JSON Web Token configuration ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"token_expiry": schema.Int64Attribute{MarkdownDescription: "Number of minutes an issued token remains valid.", Computed: true},
			"enabled":      schema.BoolAttribute{MarkdownDescription: "Whether the JSON Web Token configuration is active.", Computed: true},
			"timeouts":     timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *JSONWebTokenConfigurationDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *JSONWebTokenConfigurationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_json_web_token_configuration")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a record by ID or by name and populates Terraform state.
func (d *JSONWebTokenConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data JSONWebTokenConfigurationDataSourceModel
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
		got *proclassic.JsonWebTokenConfiguration
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetJsonWebTokenConfigurationByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.findJSONWebTokenConfigurationByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing record selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
		return
	}
	assignJSONWebTokenConfigurationDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro JSON Web Token configuration data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// findJSONWebTokenConfigurationByName resolves an exact display name against
// the full list (the list returns full records, so no follow-up read is
// needed).
func (d *JSONWebTokenConfigurationDataSource) findJSONWebTokenConfigurationByName(ctx context.Context, name string) (*proclassic.JsonWebTokenConfiguration, error) {
	listed, err := d.client.ListJsonWebTokenConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	if listed != nil {
		for i := range listed.JsonWebTokenConfigurations {
			if helpers.DerefString(listed.JsonWebTokenConfigurations[i].Name) == name {
				return &listed.JsonWebTokenConfigurations[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no JSON Web Token configuration found with name %q", name)
}
