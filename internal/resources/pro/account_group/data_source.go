// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	datasourcevalidator "github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// AccountGroupDataSource implements the account-group data source, sourced from
// the ProClassic API (the Pro v1 /account-groups read endpoint is currently
// gateway-blocked).
type AccountGroupDataSource struct {
	client *proclassic.Client
}

var _ datasource.DataSource = &AccountGroupDataSource{}
var _ datasource.DataSourceWithConfigValidators = &AccountGroupDataSource{}

// NewAccountGroupDataSource returns a new instance of AccountGroupDataSource.
func NewAccountGroupDataSource() datasource.DataSource {
	return &AccountGroupDataSource{}
}

// Metadata sets the data source type name.
func (d *AccountGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_account_group"
}

// ConfigValidators enforces exactly one of id / display_name.
func (d *AccountGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("display_name")),
	}
}

// Schema returns the data source schema.
func (d *AccountGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro administrator account group by `id` or `display_name`. Values use the same spellings as the `jamfplatform_pro_account_group` resource; the `privileges` attribute is the flattened union of the group's privilege grid." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{MarkdownDescription: "Account group ID. Provide this or `display_name`.", Optional: true, Computed: true},
			"display_name":     schema.StringAttribute{MarkdownDescription: "Group display name. Provide this or `id`.", Optional: true, Computed: true},
			"access_level":     schema.StringAttribute{MarkdownDescription: "Access level (`Full Access` or `Site Access`).", Computed: true},
			"privilege_set":    schema.StringAttribute{MarkdownDescription: "Privilege set (`Administrator`, `Auditor`, `Enrollment Only`, or `Custom`).", Computed: true},
			"site_id":          schema.Int64Attribute{MarkdownDescription: "Scoped site ID (`-1` for none).", Computed: true},
			"site_name":        schema.StringAttribute{MarkdownDescription: "Scoped site name.", Computed: true},
			"ldap_server_id":   schema.Int64Attribute{MarkdownDescription: "Backing LDAP / cloud-identity-provider server ID (null for a local group).", Computed: true},
			"ldap_server_name": schema.StringAttribute{MarkdownDescription: "Backing directory server name.", Computed: true},
			"members":          schema.SetAttribute{MarkdownDescription: "Account IDs that are explicit members of the group.", Computed: true, ElementType: types.Int64Type},
			"privileges":       schema.SetAttribute{MarkdownDescription: "Flattened union of all privilege strings granted to the group across every category.", Computed: true, ElementType: types.StringType},
			"timeouts":         timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *AccountGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an account group by id or display_name and populates state.
func (d *AccountGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "The provider client was not configured.")
		return
	}

	var data AccountGroupDataSourceModel
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

	var got *proclassic.Group
	var err error
	if !data.ID.IsNull() && data.ID.ValueString() != "" {
		got, err = d.client.GetAccountGroupByID(readCtx, data.ID.ValueString())
	} else {
		got, err = d.client.GetAccountGroupByName(readCtx, data.DisplayName.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro account group", helpers.APIErrorDetail(err))
		return
	}

	assignAccountGroupDataSourceModel(&data, got)
	tflog.Trace(ctx, "read Jamf Pro account group data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func assignAccountGroupDataSourceModel(data *AccountGroupDataSourceModel, g *proclassic.Group) {
	if g == nil {
		return
	}
	data.ID = helpers.StringValueFromIntPtr(g.ID)
	data.DisplayName = helpers.StringPointerValueOrNull(g.Name)
	data.AccessLevel = helpers.StringPointerValueOrNull(g.AccessLevel)
	data.PrivilegeSet = helpers.StringPointerValueOrNull(g.PrivilegeSet)

	if g.Site != nil && g.Site.ID != nil {
		data.SiteID = types.Int64Value(int64(*g.Site.ID))
		data.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	} else {
		data.SiteID = types.Int64Value(-1)
		data.SiteName = types.StringNull()
	}
	if g.LdapServer != nil && g.LdapServer.ID != nil && *g.LdapServer.ID > 0 {
		data.LdapServerID = types.Int64Value(int64(*g.LdapServer.ID))
		data.LdapServerName = helpers.StringPointerValueOrNull(g.LdapServer.Name)
	} else {
		data.LdapServerID = types.Int64Null()
		data.LdapServerName = types.StringNull()
	}

	data.Members = memberSet(g.Members)

	// Flatten the categorised grid into a deduplicated union — the same
	// privilege string can appear in more than one category, and a Set must
	// not contain duplicates.
	seen := map[string]struct{}{}
	var privElems []attr.Value
	for _, privs := range accountprivileges.FromGroupPrivileges(g.Privileges) {
		for _, p := range privs {
			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}
			privElems = append(privElems, types.StringValue(p))
		}
	}
	data.Privileges, _ = types.SetValue(types.StringType, privElems)
}
