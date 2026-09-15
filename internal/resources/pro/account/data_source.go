// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	datasourcevalidator "github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// AccountDataSource implements the account data source (base fields, Pro v1).
// The Custom privilege grid is not surfaced here — use the resource or the
// jamfplatform_pro_account_privileges data source for privilege discovery.
type AccountDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AccountDataSource{}
var _ datasource.DataSourceWithConfigValidators = &AccountDataSource{}

// NewAccountDataSource returns a new instance of AccountDataSource.
func NewAccountDataSource() datasource.DataSource {
	return &AccountDataSource{}
}

// Metadata sets the data source type name.
func (d *AccountDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_account"
}

// ConfigValidators enforces exactly one of id / username.
func (d *AccountDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("username")),
	}
}

// Schema returns the data source schema. Enum values use the UI spellings,
// translated from the Pro wire values.
func (d *AccountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro administrator login account by `id` or `username`. Surfaces base account fields; the Custom privilege grid is not included (use the `jamfplatform_pro_account` resource or `jamfplatform_pro_account_privileges` data source)." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{MarkdownDescription: "Account ID. Provide this or `username`.", Optional: true, Computed: true},
			"username":       schema.StringAttribute{MarkdownDescription: "Account username. Provide this or `id`.", Optional: true, Computed: true},
			"full_name":      schema.StringAttribute{MarkdownDescription: "Full name.", Computed: true},
			"email_address":  schema.StringAttribute{MarkdownDescription: "Email address.", Computed: true},
			"access_level":   schema.StringAttribute{MarkdownDescription: "Access level (UI spelling).", Computed: true},
			"privilege_set":  schema.StringAttribute{MarkdownDescription: "Privilege set (UI spelling).", Computed: true},
			"access_status":  schema.StringAttribute{MarkdownDescription: "Account status.", Computed: true},
			"account_type":   schema.StringAttribute{MarkdownDescription: "Account type (DEFAULT or FEDERATED).", Computed: true},
			"ldap_server_id": schema.Int64Attribute{MarkdownDescription: "Backing LDAP / cloud-identity-provider server ID (-1 for local).", Computed: true},
			"site_id":        schema.Int64Attribute{MarkdownDescription: "Scoped site ID (-1 for none).", Computed: true},
			"timeouts":       timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *AccountDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an account by id or username and populates state.
func (d *AccountDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The provider client was not configured.")
		return
	}

	var data AccountDataSourceModel
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

	var got *pro.UserAccount
	var err error
	if !data.ID.IsNull() && data.ID.ValueString() != "" {
		got, err = d.client.GetAccountV1(readCtx, data.ID.ValueString())
	} else {
		got, err = d.client.ResolveAccountV1ByName(readCtx, data.Username.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro account", helpers.APIErrorDetail(err))
		return
	}

	assignAccountDataSourceModel(&data, got)
	tflog.Trace(ctx, "read Jamf Pro account data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func assignAccountDataSourceModel(data *AccountDataSourceModel, a *pro.UserAccount) {
	if a == nil {
		return
	}
	data.ID = helpers.StringPointerValueOrNull(a.ID)
	data.Username = helpers.StringPointerValueOrNull(a.Username)
	data.FullName = helpers.StringPointerValueOrNull(a.Realname)
	data.EmailAddress = helpers.StringPointerValueOrNull(a.Email)
	if a.AccessLevel != nil {
		data.AccessLevel = types.StringValue(translate(accessLevelFromWire, *a.AccessLevel))
	}
	if a.PrivilegeLevel != nil {
		data.PrivilegeSet = types.StringValue(translate(privilegeSetFromWire, *a.PrivilegeLevel))
	}
	data.AccessStatus = helpers.StringPointerValueOrNull(a.AccountStatus)
	data.AccountType = helpers.StringPointerValueOrNull(a.AccountType)
	data.LdapServerID = int64OrNull(a.LdapServerID)
	data.SiteID = int64OrNull(a.SiteID)
}
