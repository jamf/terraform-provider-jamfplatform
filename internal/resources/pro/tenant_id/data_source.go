// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package tenant_id implements the jamfplatform_pro_tenant_id data source, which
// reads the platform tenant identifier of the Jamf Pro tenant the provider is
// scoped to.
//
// It exists to be consumed rather than read for its own sake. Jamf Security
// Cloud's UEM Connect integration identifies the Jamf Pro instance it syncs with
// by that identifier, and it is the one value in that configuration an operator
// cannot copy off a Jamf Pro screen — so without this data source the identifier
// has to be pasted into the configuration by hand.
//
// SDK endpoints used:
//
//	pro.GetCsaTenantIdV1
//
// Status: current.
package tenant_id

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this data
// source. Empty string skips the check.
//
// No endpoint-specific floor could be sourced: the SDK records no "available
// since" for this operation and Jamf's release notes do not name it. Reading the
// tenant identifier is cloud-services plumbing that predates the versions this
// provider supports, and it was confirmed answering on 11.31.1, so a gate here
// would only be a guess dressed as a requirement. Revisit if a tenant on the
// provider's floor is ever seen to refuse it.
const minJamfProVersion = ""

const defaultReadTimeout = 60 * time.Second

var _ datasource.DataSource = &TenantIDDataSource{}

// NewTenantIDDataSource returns a new instance of TenantIDDataSource.
func NewTenantIDDataSource() datasource.DataSource {
	return &TenantIDDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *TenantIDDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_tenant_id"
}

// Schema returns the data source schema.
func (d *TenantIDDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the platform tenant identifier of the Jamf Pro tenant this provider is scoped " +
			"to. Takes no arguments. A provider instance is scoped to one tenant, and this returns that " +
			"tenant's identifier.\n\n" +
			"Use it wherever a configuration has to name a Jamf Pro tenant to another Jamf product, rather than " +
			"copying the identifier between consoles by hand. " +
			"`jamfplatform_security_cloud_uem_connect` is the case it was built for.\n\n" +
			"A provider scoped to a platform environment resolves the Jamf Pro tenant within that environment. " +
			"Only an environment holding a single Jamf Pro tenant has been observed, so treat the result as " +
			"unverified where an environment holds more than one." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "The Jamf Pro tenant identifier.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *TenantIDDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_tenant_id")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the Jamf Pro tenant identifier and populates Terraform state.
func (d *TenantIDDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data TenantIDDataSourceModel
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

	info, err := d.client.GetCsaTenantIdV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro tenant identifier", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignTenantIDDataSourceModel(&data, info)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro tenant identifier")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
