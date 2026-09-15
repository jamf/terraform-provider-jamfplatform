// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

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

// UserExtensionAttributeDataSource implements the Terraform data source for Jamf
// Pro user extension attributes. Lookup is by ID or by exact name — exactly one
// must be supplied.
type UserExtensionAttributeDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &UserExtensionAttributeDataSource{}
	_ datasource.DataSourceWithConfigure        = &UserExtensionAttributeDataSource{}
	_ datasource.DataSourceWithConfigValidators = &UserExtensionAttributeDataSource{}
)

// NewUserExtensionAttributeDataSource returns a new instance of the data source.
func NewUserExtensionAttributeDataSource() datasource.DataSource {
	return &UserExtensionAttributeDataSource{}
}

// Metadata sets the data source type name.
func (d *UserExtensionAttributeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_extension_attribute"
}

// Schema returns the data source schema.
func (d *UserExtensionAttributeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro user extension attribute by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "User extension attribute ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "User extension attribute display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{MarkdownDescription: "Description of the extension attribute.", Computed: true},
			"data_type":   schema.StringAttribute{MarkdownDescription: "Data type collected (`String`, `Integer`, `Date`).", Computed: true},
			"input_type":  schema.StringAttribute{MarkdownDescription: "Input type (`Text Field`, `Pop-up Menu`).", Computed: true},
			"popup_menu_choices": schema.ListAttribute{
				MarkdownDescription: "Ordered pop-up menu choices (Pop-up Menu input type).",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *UserExtensionAttributeDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro Classic client into the data source.
func (d *UserExtensionAttributeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a user extension attribute by ID or by name.
func (d *UserExtensionAttributeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UserExtensionAttributeDataSourceModel
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
		got *proclassic.UserExtensionAttribute
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetUserExtensionAttributeByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.ResolveUserExtensionAttributeByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing user extension attribute selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro user extension attribute", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignUserExtensionAttributeDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro user extension attribute data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
