// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPluralReadTimeout caps how long the plural SSO connections read will
// wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed identifier the plural data source reports. The
// connection collection takes no search arguments, so every read returns the same
// set and there is nothing to derive an identifier from.
const pluralDataSourceID = "sso_connections"

// ConnectionsDataSource implements the Terraform data source for listing every
// SSO connection a Jamf Account organization holds.
type ConnectionsDataSource struct {
	client *account.Client
}

var _ datasource.DataSource = &ConnectionsDataSource{}

// NewConnectionsDataSource returns a new instance of ConnectionsDataSource.
func NewConnectionsDataSource() datasource.DataSource {
	return &ConnectionsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ConnectionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_connections"
}

// Schema returns the plural data source schema.
func (d *ConnectionsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every SSO connection your Jamf Account organization holds. Jamf Account " +
			"exposes no search arguments for connections, so this data source takes none. Filter the result in " +
			"Terraform.\n\nEach entry carries only what the organization's connection list reports. The " +
			"provider-specific settings (the addresses, the domains of an Entra tenant, the group and profile " +
			"options) are reported one connection at a time, so use the singular " +
			"`jamfplatform_account_sso_connection` data source for those rather than paying an extra read per " +
			"connection here.\n\nThis is the construct for finding the name Jamf Account actually holds for a " +
			"connection, which may be a uniquified form of the one it was created with." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"sso_connections": schema.ListNestedAttribute{
				MarkdownDescription: "The connections the organization holds, in the order Jamf Account returns " +
					"them.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier Jamf Account assigned to the connection.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name Jamf Account holds for the connection, which is what the " +
								"console lists.",
							Computed: true,
						},
						"connection_type": schema.StringAttribute{
							MarkdownDescription: "The identity provider family: one of " +
								markdownValueList(connectionTypeValues()) + ".",
							Computed: true,
						},
						"hosting_region": schema.StringAttribute{
							MarkdownDescription: "The region the connection's provider details are held in and its " +
								"sign-in traffic is routed through.",
							Computed: true,
						},
						"auth_method": schema.StringAttribute{
							MarkdownDescription: "How Jamf Account proves itself to the provider when it redeems " +
								"an authorization code.",
							Computed: true,
						},
						"sync_attributes_at_login": schema.BoolAttribute{
							MarkdownDescription: "Whether a person's profile details are refreshed from the " +
								"provider every time they sign in.",
							Computed: true,
						},
						"domains": schema.SetAttribute{
							MarkdownDescription: "The domain names this connection signs people in for. Jamf " +
								"Account holds a small number of connections with none.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"enabled_product_names": schema.SetAttribute{
							MarkdownDescription: "The Jamf products Jamf Account reports this connection as " +
								"enabled for. The tenants of each product are never returned.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"ticket_url": schema.StringAttribute{
							MarkdownDescription: "Address of the Google Workspace administrator consent request " +
								"for this connection, where one is outstanding.",
							Computed: true,
						},
						"easy_config": schema.BoolAttribute{
							MarkdownDescription: "Whether the connection was built by Jamf Account's guided " +
								"setup rather than configured directly.",
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *ConnectionsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_connections")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every connection and populates Terraform state.
//
// Nothing is filtered out. A connection this provider could not manage — one
// using Microsoft admin consent, or one Jamf lists but cannot read on its own
// identifier — is still part of the organization's configuration and is reported
// here, which is the opposite of the list resource's job: that one offers
// connections for import and so has to leave out anything an import could not
// reconcile.
func (d *ConnectionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ConnectionsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	summaries, err := d.client.ListConnections(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Account SSO connections", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	data.SSOConnections = make([]ConnectionsDataSourceResultModel, 0, len(summaries))
	for _, summary := range summaries {
		result, resultDiags := buildConnectionsResultModel(summary)
		resp.Diagnostics.Append(resultDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.SSOConnections = append(data.SSOConnections, result)
	}

	tflog.Trace(ctx, "read Jamf Account SSO connections data source", map[string]any{"count": len(data.SSOConnections)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
