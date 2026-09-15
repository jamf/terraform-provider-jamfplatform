// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package class

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

// ClassDataSource implements the Terraform data source for Jamf Pro classes.
// Lookup is by ID or by exact name — exactly one must be supplied.
type ClassDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &ClassDataSource{}
	_ datasource.DataSourceWithConfigure        = &ClassDataSource{}
	_ datasource.DataSourceWithConfigValidators = &ClassDataSource{}
)

// NewClassDataSource returns a new instance of the data source.
func NewClassDataSource() datasource.DataSource {
	return &ClassDataSource{}
}

// Metadata sets the data source type name.
func (d *ClassDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_class"
}

// Schema returns the data source schema.
func (d *ClassDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro class by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Class ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Class display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"description": schema.StringAttribute{MarkdownDescription: "Class description.", Computed: true},
			"site_id":     schema.StringAttribute{MarkdownDescription: "Jamf Pro site ID scoping the class. `-1` means no site (`NONE`).", Computed: true},
			"site_name":   schema.StringAttribute{MarkdownDescription: "Jamf Pro site display name.", Computed: true},
			"source":      schema.StringAttribute{MarkdownDescription: "How the class was created in Jamf Pro.", Computed: true},
			"students": schema.SetAttribute{
				MarkdownDescription: "Usernames of the students assigned to the class.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"teachers": schema.SetAttribute{
				MarkdownDescription: "Usernames of the teachers assigned to the class.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"student_group_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro user group IDs (as strings) assigned as student groups.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"teacher_group_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro user group IDs (as strings) assigned as teacher groups.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"mobile_device_group_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro mobile device group IDs (as strings) assigned to the class.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"student_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro user IDs (as strings) for the students, resolved by Jamf Pro.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"teacher_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro user IDs (as strings) for the teachers, resolved by Jamf Pro.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *ClassDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *ClassDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_class")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a class by ID or by name.
func (d *ClassDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ClassDataSourceModel
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
		got *proclassic.Class
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetClassByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetClassByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing class selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro class", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignClassDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro class data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
