// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tool

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ToolsDataSource reads the whole catalogue of AI tools Jamf can govern.
type ToolsDataSource struct {
	client *aigovernance.Client
}

var _ datasource.DataSource = &ToolsDataSource{}

// NewToolsDataSource returns a new instance of ToolsDataSource.
func NewToolsDataSource() datasource.DataSource {
	return &ToolsDataSource{}
}

// Metadata sets the data source type name.
func (d *ToolsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_governance_tools"
}

// Schema returns the Terraform schema for the AI tools data source.
func (d *ToolsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads every AI tool the platform can govern, with the settings schema versions each one offers. " +
			"Use it to discover the `tool_id` and `schema_version` values a `jamfplatform_ai_governance_policy` " +
			"accepts.\n\n" +
			"The settings schema documents themselves are not included. Read one from the " +
			"`jamfplatform_ai_governance_tool` data source." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"tools": schema.ListNestedAttribute{
				MarkdownDescription: "The AI tools the platform can govern.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the tool, such as `com.anthropic.claudecode`. This is " +
								"a policy's `tool_id`.",
							Computed: true,
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "The tool's name as the Jamf Account admin UI shows it.",
							Computed:            true,
						},
						"current_schema_version": schema.StringAttribute{
							MarkdownDescription: "The tool's current settings schema version.",
							Computed:            true,
						},
						"schema_versions": schema.ListAttribute{
							MarkdownDescription: "Every settings schema version the tool offers, newest first.",
							Computed:            true,
							ElementType:         types.StringType,
						},
					},
				},
			},
		},
	}
}

// Configure wires the AI Governance client into the data source.
func (d *ToolsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := configure(req.ProviderData, "jamfplatform_ai_governance_tools")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// toolsDataSourceModel is the Terraform model for the plural tools data source.
type toolsDataSourceModel struct {
	Tools []toolSummaryItem `tfsdk:"tools"`
}

// toolSummaryItem is one entry in the catalogue listing.
type toolSummaryItem struct {
	ID                   types.String `tfsdk:"id"`
	DisplayName          types.String `tfsdk:"display_name"`
	CurrentSchemaVersion types.String `tfsdk:"current_schema_version"`
	SchemaVersions       types.List   `tfsdk:"schema_versions"`
}

// Read fetches the catalogue.
func (d *ToolsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	response, err := d.client.ListTools(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list AI tools", helpers.APIErrorDetail(err))
		return
	}

	model := toolsDataSourceModel{Tools: make([]toolSummaryItem, 0, len(response.Results))}
	for _, summary := range response.Results {
		versions, diags := types.ListValueFrom(ctx, types.StringType, summary.SchemaVersions)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		model.Tools = append(model.Tools, toolSummaryItem{
			ID:                   types.StringValue(summary.ID),
			DisplayName:          types.StringValue(summary.DisplayName),
			CurrentSchemaVersion: types.StringValue(summary.SchemaVersion),
			SchemaVersions:       versions,
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
