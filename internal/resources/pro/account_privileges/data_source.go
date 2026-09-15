// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package account_privileges implements the jamfplatform_pro_account_privileges
// data source: a discovery surface for the privilege strings grantable on the
// tenant, categorised into the seven Jamf Pro privilege buckets. There is no
// privilege-catalog API endpoint; the catalog is read from a privilege_set =
// Administrator account group (preferred) or account, which enumerates the full
// grantable set. Use this to look up exact privilege strings for the
// `privileges` block of jamfplatform_pro_account / jamfplatform_pro_account_group.
package account_privileges

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const minJamfProVersion = ""

const defaultReadTimeout = 60 * time.Second

// AccountPrivilegesDataSourceModel is the data source model.
type AccountPrivilegesDataSourceModel struct {
	JamfProServerObjects  types.Set      `tfsdk:"jamf_pro_server_objects"`
	JamfProServerSettings types.Set      `tfsdk:"jamf_pro_server_settings"`
	JamfProServerActions  types.Set      `tfsdk:"jamf_pro_server_actions"`
	CasperAdmin           types.Set      `tfsdk:"casper_admin"`
	CasperRemote          types.Set      `tfsdk:"casper_remote"`
	CasperImaging         types.Set      `tfsdk:"casper_imaging"`
	Recon                 types.Set      `tfsdk:"recon"`
	All                   types.Set      `tfsdk:"all"`
	Timeouts              timeouts.Value `tfsdk:"timeouts"`
}

// AccountPrivilegesDataSource implements the privilege-catalog discovery DS.
type AccountPrivilegesDataSource struct {
	client *proclassic.Client
}

var _ datasource.DataSource = &AccountPrivilegesDataSource{}

// NewAccountPrivilegesDataSource returns a new instance.
func NewAccountPrivilegesDataSource() datasource.DataSource {
	return &AccountPrivilegesDataSource{}
}

// Metadata sets the data source type name.
func (d *AccountPrivilegesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_account_privileges"
}

// Schema returns the data source schema: the seven categorised privilege lists
// plus a flat `all` union.
func (d *AccountPrivilegesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	catAttrs := map[string]schema.Attribute{}
	for _, c := range accountprivileges.Categories {
		catAttrs[c.AttrName] = schema.SetAttribute{
			MarkdownDescription: "Grantable privileges in the " + c.AttrName + " category. " + c.Desc,
			Computed:            true,
			ElementType:         types.StringType,
		}
	}
	catAttrs["all"] = schema.SetAttribute{
		MarkdownDescription: "Flat union of all grantable privilege strings across every category.",
		Computed:            true,
		ElementType:         types.StringType,
	}
	catAttrs["timeouts"] = timeouts.Attributes(ctx)

	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the privilege strings grantable on this Jamf Pro tenant, categorised into the seven privilege buckets. The catalog is read from an existing account group or account whose privilege set is `Administrator` (which holds every grantable privilege). Use it to look up exact privilege strings for the `privileges` block of `jamfplatform_pro_account` and `jamfplatform_pro_account_group`." + dataSourcePrivileges,
		Attributes:          catAttrs,
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *AccountPrivilegesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account_privileges")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read discovers and returns the categorised privilege catalog.
func (d *AccountPrivilegesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The provider client was not configured.")
		return
	}

	var data AccountPrivilegesDataSourceModel
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

	catalog, err := accountprivileges.DiscoverCategorized(readCtx, d.client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to discover Jamf Pro privilege catalog", helpers.APIErrorDetail(err))
		return
	}

	// CategorizedSets sorts and de-duplicates every category and the union. The
	// dedup is load-bearing: the classic Administrator grid echoes some privilege
	// strings twice within a category (e.g. Create/Read/Update Cloud Distribution
	// Point), which types.SetValue rejects as a "Duplicate Set Element" (#290).
	sets, allSet, diags := accountprivileges.CategorizedSets(catalog)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.JamfProServerObjects = sets["jss_objects"]
	data.JamfProServerSettings = sets["jss_settings"]
	data.JamfProServerActions = sets["jss_actions"]
	data.CasperAdmin = sets["casper_admin"]
	data.CasperRemote = sets["casper_remote"]
	data.CasperImaging = sets["casper_imaging"]
	data.Recon = sets["recon"]
	data.All = allSet

	tflog.Trace(ctx, "read Jamf Pro account privileges catalog", map[string]any{"count": len(allSet.Elements())})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
