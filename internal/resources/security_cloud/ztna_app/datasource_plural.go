// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPagedReadTimeout caps how long a read that walks the whole application
// collection will wait. Higher than defaultReadTimeout because the list endpoint is
// paged and a large tenant costs one request per hundred applications. It serves the
// plural read and the singular data source's `name` / `predefined_app_id` lookups,
// which make the same paged call.
const defaultPagedReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The application
// list endpoint takes no filter, so every read returns the same collection and there
// is nothing to derive an ID from.
const pluralDataSourceID = "ztna_apps"

// ZtnaAppsDataSource implements the Terraform data source for listing every Jamf
// Security Cloud ZTNA access policy application.
type ZtnaAppsDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &ZtnaAppsDataSource{}

// NewZtnaAppsDataSource returns a new instance of ZtnaAppsDataSource.
func NewZtnaAppsDataSource() datasource.DataSource {
	return &ZtnaAppsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ZtnaAppsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_apps"
}

// Schema returns the plural data source schema.
func (d *ZtnaAppsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every Jamf Security Cloud access policy application on the tenant. Jamf " +
			"Security Cloud exposes no filter parameters for applications, so this data source takes no " +
			"search arguments. Filter the result in Terraform." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"ztna_apps": schema.ListNestedAttribute{
				MarkdownDescription: "The access policy applications on the tenant, in the order Jamf " +
					"Security Cloud returns them. Jamf Security Cloud accepts no sort parameter, so the order " +
					"is its own and should not be relied on. Match on `id`, `name` or `predefined_app_id` " +
					"instead.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Application ID assigned by Jamf Security Cloud.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Application name. Null for a predefined application, whose " +
								"name belongs to the Jamf-maintained definition.",
							Computed: true,
						},
						"predefined_app_id": schema.StringAttribute{
							MarkdownDescription: "ID of the Jamf-maintained definition this application is " +
								"based on. Null for a custom application.",
							Computed: true,
						},
						"app_type": schema.StringAttribute{
							MarkdownDescription: "Whether the application is predefined or custom: " +
								markdownList(appTypeValues()) + ". Follows from whether `predefined_app_id` " +
								"is set.",
							Computed: true,
						},
						"category": schema.StringAttribute{
							MarkdownDescription: "Category the application is classified under.",
							Computed:            true,
						},
						"hostnames": schema.ListAttribute{
							MarkdownDescription: "Host names whose traffic belongs to this application. For a " +
								"predefined application these are the additions to the definition's own host " +
								"names, which are not reported here.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"direct_ips_and_subnets": schema.ListAttribute{
							MarkdownDescription: "Address ranges whose traffic belongs to this application.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"all_device_groups": schema.BoolAttribute{
							MarkdownDescription: "Whether every device group may reach this application.",
							Computed:            true,
						},
						"device_group_ids": schema.ListAttribute{
							MarkdownDescription: "IDs of the device groups that may reach this application.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"routing": schema.SingleNestedAttribute{
							MarkdownDescription: "How authorised devices reach this application's servers.",
							Computed:            true,
							Attributes:          dsRoutingSchemaAttributes(),
						},
						"routing_overrides": schema.ListNestedAttribute{
							MarkdownDescription: "Per-group routing that overrides the application's own.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"device_group_ids": schema.ListAttribute{
										MarkdownDescription: "IDs of the device groups this override applies to.",
										Computed:            true,
										ElementType:         types.StringType,
									},
									"routing": schema.SingleNestedAttribute{
										MarkdownDescription: "Routing applied to the groups named above.",
										Computed:            true,
										Attributes:          dsRoutingSchemaAttributes(),
									},
								},
							},
						},
						"security": schema.SingleNestedAttribute{
							MarkdownDescription: "What a device must prove before it may reach this application.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"managed_device": schema.SingleNestedAttribute{
									MarkdownDescription: "Whether access requires the device to be managed.",
									Computed:            true,
									Attributes:          dsSecurityControlSchemaAttributes(),
								},
								"device_risk": schema.SingleNestedAttribute{
									MarkdownDescription: "Whether access requires device risk validation.",
									Computed:            true,
									Attributes: map[string]schema.Attribute{
										"enabled": schema.BoolAttribute{
											MarkdownDescription: "Whether the requirement is enforced.",
											Computed:            true,
										},
										"deny_at_risk_level": schema.StringAttribute{
											MarkdownDescription: "Risk level at which access is denied, lowest " +
												"first: " + markdownList(riskLevelValues()) + ". Jamf Security " +
												"Cloud keeps this value even while the requirement is not enforced.",
											Computed: true,
										},
										"device_push_notifications": schema.BoolAttribute{
											MarkdownDescription: "Whether the user is told when access is denied.",
											Computed:            true,
										},
									},
								},
								"jamf_trust": schema.SingleNestedAttribute{
									MarkdownDescription: "Whether access requires Jamf Trust to be enabled.",
									Computed:            true,
									Attributes:          dsSecurityControlSchemaAttributes(),
								},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the shared
// providerdata.ConfigureSecurityCloud helper.
func (d *ZtnaAppsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_apps")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every application and populates Terraform state.
func (d *ZtnaAppsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ZtnaAppsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPagedReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	apps, err := d.client.ListZtnaAppsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud access policy applications", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	data.ZtnaApps = make([]ZtnaAppsDataSourceResultModel, 0, len(apps))
	for _, app := range apps {
		result, resultDiags := buildAppsResultModel(ctx, app)
		resp.Diagnostics.Append(resultDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.ZtnaApps = append(data.ZtnaApps, result)
	}

	tflog.Trace(ctx, "read Jamf Security Cloud access policy applications data source", map[string]any{"count": len(data.ZtnaApps)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
