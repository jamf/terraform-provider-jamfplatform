// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package tool implements the jamfplatform_ai_governance_tool and jamfplatform_ai_governance_tools
// data sources, which read the catalogue of AI tools Jamf can govern.
//
// The catalogue is read-only server data: Jamf adds tools and publishes new settings schema versions
// for them, and nothing about either is configurable. Its purpose here is to supply the two values
// an AI policy needs — the tool identifier and a settings schema version — and, through
// settings_schema_json, the document that says what the settings for that pair may contain.
//
// The singular data source exposes the schema document; the plural one does not. A single tool's
// schema runs to 184 KB, so a catalogue that inlined every one would put close to half a megabyte
// into state to serve a lookup that is usually about identifiers.
package tool

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ToolDataSource reads one AI tool from the catalogue.
type ToolDataSource struct {
	client *aigovernance.Client
}

var _ datasource.DataSource = &ToolDataSource{}

// NewToolDataSource returns a new instance of ToolDataSource.
func NewToolDataSource() datasource.DataSource {
	return &ToolDataSource{}
}

// Metadata sets the data source type name.
func (d *ToolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_governance_tool"
}

// Schema returns the Terraform schema for the AI tool data source.
func (d *ToolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one AI tool the platform can govern, and the schema describing what its settings may " +
			"contain.\n\n" +
			"Use this to discover the `schema_version` values a `jamfplatform_ai_governance_policy` may be written " +
			"against, and `settings_schema_json` to see what settings that version accepts. The schema is the tool " +
			"vendor's own document. See the [AI Governance policies guide](../guides/ai-governance-policies) for " +
			"where each vendor documents its settings." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the tool, such as `com.anthropic.claudecode`. Read the available " +
					"identifiers from the `jamfplatform_ai_governance_tools` data source.",
				Required: true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "The tool's name as the Jamf Account admin UI shows it, such as `Claude Code`.",
				Computed:            true,
			},
			"schema_version": schema.StringAttribute{
				MarkdownDescription: "Which settings schema version `settings_schema_json` describes. Defaults to the " +
					"tool's current version; set it to read an older one that policies are still written against.",
				Optional: true,
				Computed: true,
			},
			"current_schema_version": schema.StringAttribute{
				MarkdownDescription: "The tool's current settings schema version. A policy written against an older " +
					"version keeps working and reports `schema_drift`.",
				Computed: true,
			},
			"schema_versions": schema.ListAttribute{
				MarkdownDescription: "Every settings schema version the tool offers, newest first.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"settings_schema_json": schema.StringAttribute{
				MarkdownDescription: "The JSON Schema document describing what a policy's `settings_json` may contain " +
					"for this tool at `schema_version`. This is the tool vendor's own document, and it is large: " +
					"tens to hundreds of kilobytes.",
				Computed: true,
			},
		},
	}
}

// Configure wires the AI Governance client into the data source.
func (d *ToolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := configure(req.ProviderData, "jamfplatform_ai_governance_tool")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// toolDataSourceModel is the Terraform model for the singular tool data source.
type toolDataSourceModel struct {
	ID                   types.String `tfsdk:"id"`
	DisplayName          types.String `tfsdk:"display_name"`
	SchemaVersion        types.String `tfsdk:"schema_version"`
	CurrentSchemaVersion types.String `tfsdk:"current_schema_version"`
	SchemaVersions       types.List   `tfsdk:"schema_versions"`
	SettingsSchemaJSON   types.String `tfsdk:"settings_schema_json"`
}

// Read fetches the tool and the schema document for the requested version.
//
// An absent tool and a failed read are reported separately: only the first is a configuration
// problem the reader can fix by correcting the identifier, and pointing a transient failure or an
// entitlement gap at the catalogue sends the reader down the wrong path.
func (d *ToolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config toolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	summary, err := d.client.GetTool(ctx, config.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddAttributeError(
				path.Root("id"),
				"AI tool not found",
				"Jamf governs no AI tool with the identifier "+config.ID.ValueString()+". Identifiers are matched "+
					"exactly. Read the available identifiers from the jamfplatform_ai_governance_tools data source.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to read AI tool", helpers.APIErrorDetail(err))
		return
	}

	version := summary.SchemaVersion
	if helpers.IsConfiguredValue(config.SchemaVersion) {
		version = config.SchemaVersion.ValueString()
		if !slices.Contains(summary.SchemaVersions, version) {
			resp.Diagnostics.AddError(
				"Unknown settings schema version",
				fmt.Sprintf("%s does not offer a settings schema version %q. Accepted versions: %s.",
					summary.DisplayName, version, strings.Join(summary.SchemaVersions, ", ")),
			)
			return
		}
	}

	document, err := d.client.GetToolSchema(ctx, summary.ID, version)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read AI tool settings schema", helpers.APIErrorDetail(err))
		return
	}

	versions, diags := types.ListValueFrom(ctx, types.StringType, summary.SchemaVersions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	config.DisplayName = types.StringValue(summary.DisplayName)
	config.SchemaVersion = types.StringValue(version)
	config.CurrentSchemaVersion = types.StringValue(summary.SchemaVersion)
	config.SchemaVersions = versions
	config.SettingsSchemaJSON = types.StringValue(string(document.Schema))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// configure resolves the shared provider data into an AI Governance client, applying the scope gate.
func configure(providerData any, construct string) (*aigovernance.Client, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	pd, ok := providerData.(*providerdata.Data)
	if !ok {
		diags.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, diags
	}
	diags.Append(pd.RequireScope(construct, providerdata.AIGovernanceScopes...)...)
	if diags.HasError() {
		return nil, diags
	}
	return aigovernance.New(pd.Client), diags
}
