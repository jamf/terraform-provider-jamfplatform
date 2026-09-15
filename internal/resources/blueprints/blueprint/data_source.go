// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// BlueprintDataSource implements the Terraform data source for Jamf Blueprint.
type BlueprintDataSource struct {
	client *blueprints.Client
}

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &BlueprintDataSource{}

// NewBlueprintDataSource returns a new instance of BlueprintDataSource.
func NewBlueprintDataSource() datasource.DataSource {
	return &BlueprintDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *BlueprintDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blueprints_blueprint"
}

// Schema sets the Terraform schema for the data source.
func (d *BlueprintDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns a blueprint by ID or name. Requires **Blueprints API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The blueprint ID to fetch. Optional if name is set.",
				Optional:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"name": schema.StringAttribute{
				MarkdownDescription: "The blueprint name to fetch. Optional if id is set.",
				Optional:            true,
			},
			"blueprint_id": schema.StringAttribute{
				MarkdownDescription: "Blueprint ID.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description.",
				Computed:            true,
			},
			"created": schema.StringAttribute{
				MarkdownDescription: "Created at (RFC3339).",
				Computed:            true,
			},
			"updated": schema.StringAttribute{
				MarkdownDescription: "Updated at (RFC3339).",
				Computed:            true,
			},
			"deployment_state": schema.StringAttribute{
				MarkdownDescription: "Deployment state.",
				Computed:            true,
			},
			"device_groups": schema.ListAttribute{
				MarkdownDescription: "Device groups in scope.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"activation_conditions": schema.StringAttribute{
				MarkdownDescription: "Activation condition expression that further restricts which scoped devices the blueprint applies to. Reflects the first component block only; read `component_blocks` for per-block conditions.",
				Computed:            true,
				DeprecationMessage:  componentAttrDeprecation,
			},
			"component": schema.ListNestedAttribute{
				MarkdownDescription: "Components of the first component block. Read `component_blocks` for the components of every block.",
				Computed:            true,
				DeprecationMessage:  componentAttrDeprecation,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"identifier": schema.StringAttribute{
							MarkdownDescription: "Component identifier.",
							Computed:            true,
						},
						"configuration": schema.MapAttribute{
							MarkdownDescription: "Component configuration as a map of key-value pairs.",
							ElementType:         types.StringType,
							Computed:            true,
						},
					},
				},
			},
			"component_blocks": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered component blocks of the blueprint. Each block has a name, an optional activation condition, and its own components.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Component block name.",
							Computed:            true,
						},
						"activation_conditions": schema.StringAttribute{
							MarkdownDescription: "Activation condition applied to this block. Empty when the block applies to all devices in scope.",
							Computed:            true,
						},
						"component": schema.ListNestedAttribute{
							MarkdownDescription: "Components in this block.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"identifier": schema.StringAttribute{
										MarkdownDescription: "Component identifier.",
										Computed:            true,
									},
									"configuration": schema.MapAttribute{
										MarkdownDescription: "Component configuration as a map of key-value pairs.",
										ElementType:         types.StringType,
										Computed:            true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *BlueprintDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_blueprints_blueprint", providerdata.BlueprintsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = blueprints.New(pd.Client)
}

// Read fetches a blueprint by ID or title and populates the Terraform state.
func (d *BlueprintDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data BlueprintDataSourceModel

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

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure provider block is set up correctly.",
		)
		return
	}

	var bp *blueprints.BlueprintDetail
	var err error
	if helpers.IsConfiguredValue(data.ID) && data.ID.ValueString() != "" {
		bp, err = d.client.GetBlueprint(readCtx, data.ID.ValueString())
	} else if helpers.IsConfiguredValue(data.Name) && data.Name.ValueString() != "" {
		var id string
		if id, err = d.client.ResolveBlueprintIDByName(readCtx, data.Name.ValueString()); err == nil {
			bp, err = d.client.GetBlueprint(readCtx, id)
		}
	} else {
		resp.Diagnostics.AddError(
			"Missing Required Attribute",
			"Either 'id' or 'name' must be set to look up a blueprint.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get blueprint",
			helpers.APIErrorDetail(err),
		)
		return
	}

	deviceGroupsList, _ := types.ListValueFrom(context.Background(), types.StringType, scopeDeviceGroups(bp.Scope))

	var activationConditions types.String
	if len(bp.Steps) > 0 {
		activationConditions = types.StringValue(helpers.DerefString(bp.Steps[0].ActivationPredicate))
	}

	var components []ComponentModel
	if len(bp.Steps) > 0 {
		components = flattenStepComponents(bp.Steps[0])
	}

	componentBlocks := make([]ComponentBlockDataSourceModel, 0, len(bp.Steps))
	for _, step := range bp.Steps {
		componentBlocks = append(componentBlocks, ComponentBlockDataSourceModel{
			Name:                 types.StringValue(helpers.DerefString(step.Name)),
			ActivationConditions: types.StringValue(helpers.DerefString(step.ActivationPredicate)),
			Components:           flattenStepComponents(step),
		})
	}

	deployState := ""
	if bp.DeploymentState != nil {
		deployState = bp.DeploymentState.State
	}

	timeoutsValue := data.Timeouts
	data = BlueprintDataSourceModel{
		ID:                   data.ID,
		Name:                 types.StringValue(bp.Name),
		BlueprintID:          types.StringValue(bp.ID),
		Description:          types.StringValue(helpers.DerefString(bp.Description)),
		Created:              types.StringValue(bp.Created.Format(time.RFC3339)),
		Updated:              types.StringValue(bp.Updated.Format(time.RFC3339)),
		DeploymentState:      types.StringValue(deployState),
		DeviceGroups:         deviceGroupsList,
		ActivationConditions: activationConditions,
		Components:           components,
		ComponentBlocks:      componentBlocks,
		Timeouts:             timeoutsValue,
	}

	tflog.Trace(ctx, "read a data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// flattenStepComponents converts a wire step's components into the data source's read-only
// component list (identifier + flattened key/value configuration map).
func flattenStepComponents(step blueprints.BlueprintStep) []ComponentModel {
	components := make([]ComponentModel, len(step.Components))
	for i, comp := range step.Components {
		configMap := make(map[string]string)
		if comp.Configuration != nil {
			var jsonObj map[string]any
			if err := json.Unmarshal(comp.Configuration, &jsonObj); err == nil {
				flattenJSON(jsonObj, "", configMap)
			}
		}

		configMapValue, _ := types.MapValueFrom(context.Background(), types.StringType, configMap)
		components[i] = ComponentModel{
			Identifier:    types.StringValue(comp.Identifier),
			Configuration: configMapValue,
		}
	}
	return components
}
