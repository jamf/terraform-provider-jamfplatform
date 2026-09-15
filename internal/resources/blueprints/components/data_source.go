// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ComponentsDataSource{}

// NewComponentsDataSource returns a new instance of ComponentsDataSource.
func NewComponentsDataSource() datasource.DataSource {
	return &ComponentsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ComponentsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blueprints_components"
}

// Schema sets the Terraform schema for the data source.
func (d *ComponentsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns all available blueprint components. Requires **Blueprints API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"components": schema.ListNestedAttribute{
				MarkdownDescription: "List of all blueprint components.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"identifier": schema.StringAttribute{
							MarkdownDescription: "Component identifier.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Component name.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Component description.",
							Computed:            true,
						},
						"supported_os": schema.MapAttribute{
							MarkdownDescription: "Supported operating systems with their versions.",
							ElementType: types.ListType{
								ElemType: types.StringType,
							},
							Computed: true,
						},
					},
				},
			},
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *ComponentsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_blueprints_components", providerdata.BlueprintsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = blueprints.New(pd.Client)
}

// Read fetches all components and populates the Terraform state.
func (d *ComponentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ComponentsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure provider block is set up correctly.",
		)
		return
	}

	components, err := d.client.ListBlueprintComponents(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to get components",
			helpers.APIErrorDetail(err),
		)
		return
	}

	var componentsList []ComponentListModel
	for _, comp := range components {
		supportedOsAttrType := types.ListType{
			ElemType: types.StringType,
		}

		var supportedOsMap attr.Value
		if len(comp.Meta.SupportedOs) == 0 {
			supportedOsMap = types.MapNull(supportedOsAttrType)
		} else {
			supportedOsMapVals := make(map[string]attr.Value)
			for osFamily, versions := range comp.Meta.SupportedOs {
				osVersionVals := make([]attr.Value, len(versions))
				for i, v := range versions {
					osVersionVals[i] = types.StringValue(v.Version)
				}
				supportedOsList, _ := types.ListValue(types.StringType, osVersionVals)
				supportedOsMapVals[osFamily] = supportedOsList
			}
			supportedOsMap, _ = types.MapValue(supportedOsAttrType, supportedOsMapVals)
		}

		componentsList = append(componentsList, ComponentListModel{
			Identifier:  types.StringValue(comp.Identifier),
			Name:        types.StringValue(comp.Name),
			Description: helpers.StringPointerValueOrNull(comp.Description),
			SupportedOs: supportedOsMap.(types.Map),
		})
	}

	data = ComponentsDataSourceModel{
		Components: componentsList,
	}

	tflog.Trace(ctx, "read a data source")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
