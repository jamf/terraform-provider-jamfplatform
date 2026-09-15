// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// sortableProperties are the properties the policies list can be ordered by. Sorting by anything
// else is refused with VALIDATION_FAILED naming the sort parameter, so the set is checked at plan
// time. It is a server capability rather than a value vocabulary, so no SDK constant exists for it.
var sortableProperties = []string{"name", "createdAt", "updatedAt"}

// PoliciesDataSource reads every AI Governance policy.
type PoliciesDataSource struct {
	client *aigovernance.Client
}

var _ datasource.DataSource = &PoliciesDataSource{}

// NewPoliciesDataSource returns a new instance of PoliciesDataSource.
func NewPoliciesDataSource() datasource.DataSource {
	return &PoliciesDataSource{}
}

// Metadata sets the data source type name.
func (d *PoliciesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_governance_policies"
}

// Schema returns the Terraform schema for the AI policies data source.
//
// The list carries no settings: the platform's list items omit them, and fetching each policy in
// full to fill the gap would turn one call into one per policy. Read a policy's settings from the
// singular jamfplatform_ai_governance_policy data source.
func (d *PoliciesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads every Jamf AI Governance policy. Archived policies are not included.\n\n" +
			"Settings are not part of the listing. Read a policy's settings from the " +
			"`jamfplatform_ai_governance_policy` data source." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"sort": schema.ListAttribute{
				MarkdownDescription: "How to order the results, as `property:asc` or `property:desc` entries " +
					"applied in order. Sortable properties: `name`, `createdAt`, `updatedAt`. Unset leaves the " +
					"order to the platform.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(sortExpressionValidator()),
				},
			},
			"schema_drift_only": schema.BoolAttribute{
				MarkdownDescription: "When `true`, return only policies whose settings schema version is behind " +
					"the one the platform now offers for their tool. These are the policies worth reviewing " +
					"after a tool publishes a new schema.",
				Optional: true,
			},
			"policies": schema.ListNestedAttribute{
				MarkdownDescription: "The policies found.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "ID of the policy.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the policy.",
							Computed:            true,
						},
						"tool_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the AI tool the policy configures.",
							Computed:            true,
						},
						"published_version": schema.Int64Attribute{
							MarkdownDescription: "Number of the most recently published version, or null when the " +
								"policy has never been published.",
							Computed: true,
						},
						"has_draft": schema.BoolAttribute{
							MarkdownDescription: "Whether the policy holds changes that have not been published.",
							Computed:            true,
						},
						"schema_drift": schema.BoolAttribute{
							MarkdownDescription: "Whether the policy's schema version is behind the one the " +
								"platform now offers for the tool.",
							Computed: true,
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
				},
			},
		},
	}
}

// Configure wires the AI Governance client into the data source.
func (d *PoliciesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_ai_governance_policies", providerdata.AIGovernanceScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = aigovernance.New(pd.Client)
}

// policiesDataSourceModel is the Terraform model for the plural policies data source.
type policiesDataSourceModel struct {
	Sort            types.List          `tfsdk:"sort"`
	SchemaDriftOnly types.Bool          `tfsdk:"schema_drift_only"`
	Policies        []policySummaryItem `tfsdk:"policies"`
}

// policySummaryItem is one entry in the plural data source's result list.
type policySummaryItem struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ToolID           types.String `tfsdk:"tool_id"`
	PublishedVersion types.Int64  `tfsdk:"published_version"`
	HasDraft         types.Bool   `tfsdk:"has_draft"`
	SchemaDrift      types.Bool   `tfsdk:"schema_drift"`
	CreatedAt        types.String `tfsdk:"created_at"`
	UpdatedAt        types.String `tfsdk:"updated_at"`
}

// Read fetches every policy.
func (d *PoliciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config policiesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var sort []string
	if helpers.IsConfiguredValue(config.Sort) {
		resp.Diagnostics.Append(config.Sort.ElementsAs(ctx, &sort, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	summaries, err := d.client.ListPolicies(ctx, sort, config.SchemaDriftOnly.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list AI policies", helpers.APIErrorDetail(err))
		return
	}

	config.Policies = make([]policySummaryItem, 0, len(summaries))
	for _, summary := range summaries {
		config.Policies = append(config.Policies, policySummaryItem{
			ID:               types.StringValue(summary.ID),
			Name:             types.StringValue(summary.Name),
			ToolID:           types.StringValue(summary.ToolID),
			PublishedVersion: optionalInt64(summary.CurrentVersionNumber),
			HasDraft:         types.BoolValue(summary.HasDraft),
			SchemaDrift:      types.BoolValue(summary.SchemaDrift),
			CreatedAt:        types.StringValue(summary.CreatedAt.UTC().Format(time.RFC3339)),
			UpdatedAt:        types.StringValue(summary.UpdatedAt.UTC().Format(time.RFC3339)),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// sortExpressionValidator checks one sort entry's shape and property name.
func sortExpressionValidator() validator.String {
	pattern := "^(" + joinAlternatives(sortableProperties) + "):(asc|desc)$"
	return stringvalidator.RegexMatches(
		mustCompile(pattern),
		"must be one of "+joinCommas(sortableProperties)+" followed by :asc or :desc, for example \"name:asc\"",
	)
}
