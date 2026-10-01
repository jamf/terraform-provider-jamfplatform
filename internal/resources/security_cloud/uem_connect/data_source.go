// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var _ datasource.DataSource = &UEMConnectDataSource{}

// NewUEMConnectDataSource returns a new instance of UEMConnectDataSource.
func NewUEMConnectDataSource() datasource.DataSource {
	return &UEMConnectDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *UEMConnectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_uem_connect"
}

// Schema returns the data source schema.
//
// It takes no arguments: a tenant holds at most one UEM Connect integration, so
// there is nothing to select between.
func (d *UEMConnectDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the Jamf Security Cloud **UEM Connect** integration. It takes no arguments, " +
			"because a tenant holds at most one.\n\n" +
			"Alongside the configuration, this reports what the resource deliberately leaves out: whether the " +
			"integration is currently connected, the Jamf Pro version behind it, and the most recent sync. Those " +
			"change on their own, so they belong to a read rather than to managed state." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Integration ID assigned by Jamf Security Cloud.",
				Computed:            true,
			},
			"uem_vendor": schema.StringAttribute{
				MarkdownDescription: "**\"UEM vendor\"** in the Jamf Security Cloud admin UI.",
				Computed:            true,
			},
			"uem_server_url": schema.StringAttribute{
				MarkdownDescription: "**\"UEM server URL\"** in the Jamf Security Cloud admin UI.",
				Computed:            true,
			},
			"platform_tenant_id": schema.StringAttribute{
				MarkdownDescription: "The Jamf Pro tenant this integration syncs with, when it was set up by naming " +
					"one. Null where credentials were supplied instead.",
				Computed: true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "**\"Client ID\"** in the Jamf Security Cloud admin UI: the credential the " +
					"integration authenticates with. Where the integration was set up by naming a tenant, this is " +
					"the credential Jamf Security Cloud provisioned for itself. The secret is never readable.",
				Computed: true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the integration is set to sync.",
				Computed:            true,
			},
			"connected": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Security Cloud currently reaches the Jamf Pro instance.",
				Computed:            true,
			},
			"jamf_pro_version": schema.StringAttribute{
				MarkdownDescription: "Version of the Jamf Pro instance the integration is connected to, as Jamf " +
					"Security Cloud last observed it.",
				Computed: true,
			},
			"scheduled_sync_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Security Cloud syncs on a schedule.",
				Computed:            true,
			},
			"sync_refresh_interval_minutes": schema.Int64Attribute{
				MarkdownDescription: "**\"Sync refresh interval\"** in the Jamf Security Cloud admin UI, in minutes.",
				Computed:            true,
			},
			"uem_auto_delete_behavior": schema.StringAttribute{
				MarkdownDescription: "**\"Configure UEM auto-delete behavior\"** in the Jamf Security Cloud admin UI.",
				Computed:            true,
			},
			"unmanaged_sync_threshold": schema.Int64Attribute{
				MarkdownDescription: "Days since a device last checked in before Jamf Security Cloud treats it as " +
					"unmanaged. Reported as `0` for a Jamf Pro connection, which does not use this setting.",
				Computed: true,
			},
			"device_risk_uem_signaling_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable device risk UEM signaling\"** in the Jamf Security Cloud admin UI.",
				Computed:            true,
			},
			"disable_sync_on_auth_error": schema.BoolAttribute{
				MarkdownDescription: "Whether syncing stops after repeated credential failures.",
				Computed:            true,
			},
			"concurrent_device_sync_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Sync multiple devices simultaneously for faster inventory updates\"** in " +
					"the Jamf Security Cloud admin UI.",
				Computed: true,
			},
			"user_data_field_mapping": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"User data field mapping\"** in the Jamf Security Cloud admin UI.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"device_name":  schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Device name\"** in the Jamf Security Cloud admin UI."},
					"user_name":    schema.StringAttribute{Computed: true, MarkdownDescription: "**\"User name\"** in the Jamf Security Cloud admin UI."},
					"user_id":      schema.StringAttribute{Computed: true, MarkdownDescription: "**\"User ID\"** in the Jamf Security Cloud admin UI."},
					"phone_number": schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Phone number\"** in the Jamf Security Cloud admin UI."},
					"email": schema.SingleNestedAttribute{
						MarkdownDescription: "**\"Email\"** in the Jamf Security Cloud admin UI.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"source":                schema.StringAttribute{Computed: true, MarkdownDescription: "The Jamf Pro attribute the address is read from."},
							"prefix":                schema.StringAttribute{Computed: true, MarkdownDescription: "Prepended to the value read from Jamf Pro."},
							"suffix":                schema.StringAttribute{Computed: true, MarkdownDescription: "Appended to the value read from Jamf Pro."},
							"only_if_email_missing": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rule applies only where Jamf Pro's own email attribute is empty."},
						},
					},
				},
			},
			"group_membership_mapping": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Group membership mapping\"** in the Jamf Security Cloud admin UI.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"enabled":                         schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Enable group membership mapping\"** in the Jamf Security Cloud admin UI."},
					"default_security_cloud_group_id": schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Default mapping\"** in the Jamf Security Cloud admin UI. Null means the built-in Default Group."},
					"mappings": schema.ListNestedAttribute{
						MarkdownDescription: "Group assignments, in the order membership is evaluated.",
						Computed:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"uem_group_id":            schema.StringAttribute{Computed: true, MarkdownDescription: "The Jamf Pro group."},
								"security_cloud_group_id": schema.StringAttribute{Computed: true, MarkdownDescription: "The Jamf Security Cloud device group its members are assigned to."},
							},
						},
					},
				},
			},
			"latest_sync": schema.SingleNestedAttribute{
				MarkdownDescription: "The most recent sync. Null until the integration has run one. Device counts " +
					"are not part of this: the integration record keeps the current sync's state, not its tallies.",
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"transaction_id":    schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Transaction ID\"** in the Jamf Security Cloud admin UI's sync logs."},
					"status":            schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Sync result\"** in the Jamf Security Cloud admin UI."},
					"trigger":           schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Type\"** in the Jamf Security Cloud admin UI: what started the sync."},
					"started":           schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Sync start\"** in the Jamf Security Cloud admin UI, in RFC 3339."},
					"finished":          schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Sync completed\"** in the Jamf Security Cloud admin UI, in RFC 3339. Null while a sync is running."},
					"error_reason":      schema.StringAttribute{Computed: true, MarkdownDescription: "Why the sync failed, as a category. Null when it did not."},
					"error_description": schema.StringAttribute{Computed: true, MarkdownDescription: "Why the sync failed, in words. Null when it did not."},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source.
func (d *UEMConnectDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_uem_connect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the tenant's UEM Connect integration.
//
// The list read is what locates it. There is no by-ID read to use instead without
// asking the caller for an ID they have no way to know, and the list is at most
// one element.
//
// An absent integration is an error rather than an empty result: a data source
// reference cannot be satisfied, and returning null attributes would push the
// failure downstream into whatever consumed them.
func (d *UEMConnectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UEMConnectDataSourceModel
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

	page, err := d.client.ListUemConnectorsV1(readCtx)
	if err != nil {
		if !appendCreateDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error reading Jamf Security Cloud UEM Connect integration", helpers.APIErrorDetail(err))
		}
		return
	}
	connectors := jamfProConnectors(page)
	resp.Diagnostics.Append(appendMissingIntegrationDiagnostics(connectors)...)
	if resp.Diagnostics.HasError() {
		return
	}

	connector := connectors[0]
	resp.Diagnostics.Append(assignUEMConnectDataSourceModel(ctx, &data, &connector)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Security Cloud UEM Connect integration", map[string]any{"id": connector.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
