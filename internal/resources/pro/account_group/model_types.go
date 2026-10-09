// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// AccountGroupResourceModel is the Terraform model for a Jamf Pro administrator
// account group (NOT the jamfplatform_pro_user_group inventory construct). The
// resource is backed by the ProClassic /accounts/groupid endpoint, which is the
// only writable path; the Pro v1 /account-groups endpoint is read-only and
// powers the data source.
type AccountGroupResourceModel struct {
	ID             types.String             `tfsdk:"id"`
	DisplayName    types.String             `tfsdk:"display_name"`
	AccessLevel    types.String             `tfsdk:"access_level"`
	PrivilegeSet   types.String             `tfsdk:"privilege_set"`
	SiteID         types.Int64              `tfsdk:"site_id"`
	SiteName       types.String             `tfsdk:"site_name"`
	LdapServerID   types.Int64              `tfsdk:"ldap_server_id"`
	LdapServerName types.String             `tfsdk:"ldap_server_name"`
	Members        types.Set                `tfsdk:"members"`
	Privileges     *accountprivileges.Model `tfsdk:"privileges"`
	Timeouts       resourceTimeouts.Value   `tfsdk:"timeouts"`
}

// AccountGroupDataSourceModel is the Terraform model for the account-group data
// source. It is sourced from the ProClassic /accounts/groupid endpoint (the Pro
// v1 /account-groups read endpoint is gateway-blocked today), so it uses the
// classic enum spellings — matching the resource — and surfaces the categorised
// privilege grid flattened into a single set. Lookup is by id or display_name.
type AccountGroupDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	DisplayName    types.String             `tfsdk:"display_name"`
	AccessLevel    types.String             `tfsdk:"access_level"`
	PrivilegeSet   types.String             `tfsdk:"privilege_set"`
	SiteID         types.Int64              `tfsdk:"site_id"`
	SiteName       types.String             `tfsdk:"site_name"`
	LdapServerID   types.Int64              `tfsdk:"ldap_server_id"`
	LdapServerName types.String             `tfsdk:"ldap_server_name"`
	Members        types.Set                `tfsdk:"members"`
	Privileges     types.Set                `tfsdk:"privileges"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// accountGroupIdentityModel is the identity object for import and list results.
type accountGroupIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// AccountGroupListResourceModel is the config model for list queries. The Pro v1
// list endpoint takes an RSQL filter; the shared classic substring filter is
// used here for parity with sibling classic resources whose list is client-side.
type AccountGroupListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
