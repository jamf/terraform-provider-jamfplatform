// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PolicyDataSource reads one AI Governance policy by ID.
type PolicyDataSource struct {
	client *aigovernance.Client
}

var _ datasource.DataSource = &PolicyDataSource{}

// NewPolicyDataSource returns a new instance of PolicyDataSource.
func NewPolicyDataSource() datasource.DataSource {
	return &PolicyDataSource{}
}

// Metadata sets the data source type name.
func (d *PolicyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_governance_policy"
}

// Schema returns the Terraform schema for the AI policy data source.
//
// Lookup is by ID only. Policy names are not unique — two policies with the same name coexist
// happily — so a name lookup would resolve arbitrarily.
func (d *PolicyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads a single Jamf AI Governance policy by ID, including its current settings.\n\n" +
			"Lookup is by ID because policy names are not required to be unique." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "ID of the policy to read.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the policy.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "What the policy is for.",
				Computed:            true,
			},
			"tool_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the AI tool the policy configures, such as `com.anthropic.claudecode`.",
				Computed:            true,
			},
			"schema_version": schema.StringAttribute{
				MarkdownDescription: "Version of the tool's settings format the policy is written against.",
				Computed:            true,
			},
			"settings_json": schema.StringAttribute{
				MarkdownDescription: "The policy's current settings as a JSON object string. Reflects the draft when " +
					"one is unpublished, which `has_draft` reports.",
				Computed: true,
			},
			"published_version": schema.Int64Attribute{
				MarkdownDescription: "Number of the most recently published version, or null when the policy has never " +
					"been published. This is the value a blueprint's AI Governance component pins.",
				Computed: true,
			},
			"has_draft": schema.BoolAttribute{
				MarkdownDescription: "Whether the policy holds changes that have not been published.",
				Computed:            true,
			},
			"schema_drift": schema.BoolAttribute{
				MarkdownDescription: "Whether the policy's schema version is behind the one the platform now offers for the tool.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the policy was created, in RFC 3339 format.",
				Computed:            true,
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "When the policy was last changed, in RFC 3339 format.",
				Computed:            true,
			},
		},
	}
}

// Configure wires the AI Governance client into the data source.
func (d *PolicyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_ai_governance_policy", providerdata.AIGovernanceScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = aigovernance.New(pd.Client)
}

// policyDataSourceModel is the Terraform model for the singular policy data source.
type policyDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	ToolID           types.String `tfsdk:"tool_id"`
	SchemaVersion    types.String `tfsdk:"schema_version"`
	SettingsJSON     types.String `tfsdk:"settings_json"`
	PublishedVersion types.Int64  `tfsdk:"published_version"`
	HasDraft         types.Bool   `tfsdk:"has_draft"`
	SchemaDrift      types.Bool   `tfsdk:"schema_drift"`
	CreatedAt        types.String `tfsdk:"created_at"`
	UpdatedAt        types.String `tfsdk:"updated_at"`
}

// Read fetches the policy.
func (d *PolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config policyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	detail, err := d.client.GetPolicy(ctx, config.ID.ValueString())
	if err != nil {
		if isNotFound(err) {
			resp.Diagnostics.AddError(
				"AI policy not found",
				"No AI Governance policy exists with the ID "+config.ID.ValueString()+". An archived policy reads as "+
					"absent, so a policy that has been deleted reports the same way as one that never existed.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to read AI policy", helpers.APIErrorDetail(err))
		return
	}

	var model policyModel
	if err := applyPolicyToState(&model, detail); err != nil {
		resp.Diagnostics.AddError("Unable to read AI policy settings", helpers.APIErrorDetail(err))
		return
	}

	config.Name = model.Name
	config.Description = model.Description
	config.ToolID = model.ToolID
	config.SchemaVersion = model.SchemaVersion
	config.SettingsJSON = types.StringValue(model.SettingsJSON.ValueString())
	config.PublishedVersion = model.PublishedVersion
	config.HasDraft = model.HasDraft
	config.SchemaDrift = model.SchemaDrift
	config.CreatedAt = model.CreatedAt
	config.UpdatedAt = model.UpdatedAt

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
