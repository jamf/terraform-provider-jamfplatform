// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// VolumePurchasingNotificationDataSource implements the Terraform data source for a
// single Volume Purchasing notification. Lookup is by ID or by exact name —
// exactly one must be supplied.
type VolumePurchasingNotificationDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &VolumePurchasingNotificationDataSource{}
	_ datasource.DataSourceWithConfigure        = &VolumePurchasingNotificationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &VolumePurchasingNotificationDataSource{}
)

// NewVolumePurchasingNotificationDataSource returns a new instance.
func NewVolumePurchasingNotificationDataSource() datasource.DataSource {
	return &VolumePurchasingNotificationDataSource{}
}

// Metadata sets the data source type name.
func (d *VolumePurchasingNotificationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_volume_purchasing_notification"
}

// Schema returns the data source schema.
func (d *VolumePurchasingNotificationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a single Volume Purchasing notification by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Notification ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Notification display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{MarkdownDescription: "Whether the notification is active.", Computed: true},
			"triggers": schema.ListAttribute{
				MarkdownDescription: "Events that send the notification.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"location_ids": schema.ListAttribute{
				MarkdownDescription: "Volume Purchasing location IDs the notification covers.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"internal_recipients": schema.ListAttribute{
				MarkdownDescription: "Jamf Pro account IDs that receive the daily summary email.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"external_recipients": schema.ListNestedAttribute{
				MarkdownDescription: "External email recipients of the daily summary.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"email": schema.StringAttribute{MarkdownDescription: "Email address.", Computed: true},
						"name":  schema.StringAttribute{MarkdownDescription: "Full name.", Computed: true},
					},
				},
			},
			"site_id":  schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means none.", Computed: true},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *VolumePurchasingNotificationDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *VolumePurchasingNotificationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_volume_purchasing_notification")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a notification by ID or by name and populates Terraform state.
func (d *VolumePurchasingNotificationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data VolumePurchasingNotificationDataSourceModel
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
		got *pro.VolumePurchasingSubscription
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetVolumePurchasingSubscriptionV1(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		var id string
		id, err = d.client.ResolveVolumePurchasingSubscriptionV1IDByName(readCtx, data.Name.ValueString())
		if err == nil {
			got, err = d.client.GetVolumePurchasingSubscriptionV1(readCtx, id)
		}
	default:
		resp.Diagnostics.AddError("Missing notification selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Volume Purchasing notification", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignVolumePurchasingNotificationDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Volume Purchasing notification data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
