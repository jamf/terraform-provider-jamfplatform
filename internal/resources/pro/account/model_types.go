// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// AccountResourceModel is the Terraform model for a Jamf Pro administrator login
// account (NOT the jamfplatform_pro_user inventory construct). The resource is a
// hybrid: created via the Pro v1 /accounts API (the only path that can set
// accountType, including FEDERATED), with the Custom privilege grid carried via
// the ProClassic /accounts/userid endpoint. Base-field updates route through Pro
// PUT; privilege updates route through classic.
type AccountResourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	Username            types.String             `tfsdk:"username"`
	FullName            types.String             `tfsdk:"full_name"`
	EmailAddress        types.String             `tfsdk:"email_address"`
	AccessLevel         types.String             `tfsdk:"access_level"`
	PrivilegeSet        types.String             `tfsdk:"privilege_set"`
	AccessStatus        types.String             `tfsdk:"access_status"`
	AccountType         types.String             `tfsdk:"account_type"`
	LdapServerID        types.Int64              `tfsdk:"ldap_server_id"`
	SiteID              types.Int64              `tfsdk:"site_id"`
	ForcePasswordChange types.Bool               `tfsdk:"force_password_change"`
	Password            types.String             `tfsdk:"password"`
	PasswordWOVersion   types.Int64              `tfsdk:"password_wo_version"`
	Privileges          *accountprivileges.Model `tfsdk:"privileges"`
	Timeouts            resourceTimeouts.Value   `tfsdk:"timeouts"`
}

// AccountDataSourceModel is the Terraform model for the account data source.
// Lookup is by id or username; base fields come from Pro v1, and the Custom
// privilege grid (when applicable) from classic.
type AccountDataSourceModel struct {
	ID           types.String             `tfsdk:"id"`
	Username     types.String             `tfsdk:"username"`
	FullName     types.String             `tfsdk:"full_name"`
	EmailAddress types.String             `tfsdk:"email_address"`
	AccessLevel  types.String             `tfsdk:"access_level"`
	PrivilegeSet types.String             `tfsdk:"privilege_set"`
	AccessStatus types.String             `tfsdk:"access_status"`
	AccountType  types.String             `tfsdk:"account_type"`
	LdapServerID types.Int64              `tfsdk:"ldap_server_id"`
	SiteID       types.Int64              `tfsdk:"site_id"`
	Timeouts     datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// accountIdentityModel is the identity object for import and list results.
type accountIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// AccountListResourceModel is the config model for list queries.
type AccountListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
