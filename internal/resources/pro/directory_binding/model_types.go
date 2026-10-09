// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DirectoryBindingResourceModel is the Terraform resource model for a Jamf
// Pro directory binding. The Classic /directorybindings endpoint returns a
// flat envelope (id, name, priority, type, domain, username, password,
// computer_ou) plus exactly one of five per-type nested blocks selected by
// `type`. The wire encoding is described in
// `local-testing/directorybindings/AUDIT_FINDINGS.md`.
//
// `Password` is `WriteOnly`: the user-supplied plaintext is sent on writes
// but never persisted in Terraform state. `PasswordWoVersion` is the
// rotation trigger — bumping it forces the next Update to re-send the
// current `Password` value to Jamf Pro.
type DirectoryBindingResourceModel struct {
	ID                types.String                          `tfsdk:"id"`
	Name              types.String                          `tfsdk:"name"`
	Priority          types.Int64                           `tfsdk:"priority"`
	Type              types.String                          `tfsdk:"type"`
	Domain            types.String                          `tfsdk:"domain"`
	Username          types.String                          `tfsdk:"username"`
	Password          types.String                          `tfsdk:"password"`
	PasswordWoVersion types.Int64                           `tfsdk:"password_wo_version"`
	ComputerOU        types.String                          `tfsdk:"computer_ou"`
	ActiveDirectory   *directoryBindingActiveDirectoryModel `tfsdk:"active_directory"`
	OpenDirectory     *directoryBindingOpenDirectoryModel   `tfsdk:"open_directory"`
	Admitmac          *directoryBindingAdmitmacModel        `tfsdk:"admitmac"`
	Centrify          *directoryBindingCentrifyModel        `tfsdk:"centrify"`
	Timeouts          resourceTimeouts.Value                `tfsdk:"timeouts"`
}

// DirectoryBindingDataSourceModel is the Terraform data source model. Mirrors
// the resource shape; either `id` or `name` selects the record
// (ExactlyOneOf).
type DirectoryBindingDataSourceModel struct {
	ID              types.String                          `tfsdk:"id"`
	Name            types.String                          `tfsdk:"name"`
	Priority        types.Int64                           `tfsdk:"priority"`
	Type            types.String                          `tfsdk:"type"`
	Domain          types.String                          `tfsdk:"domain"`
	Username        types.String                          `tfsdk:"username"`
	ComputerOU      types.String                          `tfsdk:"computer_ou"`
	ActiveDirectory *directoryBindingActiveDirectoryModel `tfsdk:"active_directory"`
	OpenDirectory   *directoryBindingOpenDirectoryModel   `tfsdk:"open_directory"`
	Admitmac        *directoryBindingAdmitmacModel        `tfsdk:"admitmac"`
	Centrify        *directoryBindingCentrifyModel        `tfsdk:"centrify"`
	Timeouts        datasourceTimeouts.Value              `tfsdk:"timeouts"`
}

// directoryBindingActiveDirectoryModel is the nested model for type =
// "Active Directory". TF attribute names lean on the Jamf Pro admin UI
// labels where the wire names are cryptic (e.g. `cache_last_user` →
// `create_mobile_account`); the input/state builders translate at the
// boundary. See model_types.go in this package's history for the
// wire-name list.
type directoryBindingActiveDirectoryModel struct {
	Forest                  types.String `tfsdk:"forest"`
	CreateMobileAccount     types.Bool   `tfsdk:"create_mobile_account"`
	RequireConfirmation     types.Bool   `tfsdk:"require_confirmation"`
	ForceLocalHomeDirectory types.Bool   `tfsdk:"force_local_home_directory"`
	UseUncPath              types.Bool   `tfsdk:"use_unc_path"`
	NetworkProtocol         types.String `tfsdk:"network_protocol"`
	DefaultShell            types.String `tfsdk:"default_shell"`
	UIDAttributeMapping     types.String `tfsdk:"uid_attribute_mapping"`
	UserGIDAttributeMapping types.String `tfsdk:"user_gid_attribute_mapping"`
	GIDAttributeMapping     types.String `tfsdk:"gid_attribute_mapping"`
	MultipleDomains         types.Bool   `tfsdk:"multiple_domains"`
	PreferredDomain         types.String `tfsdk:"preferred_domain"`
	AdminGroups             types.String `tfsdk:"admin_groups"`
}

// directoryBindingOpenDirectoryModel is the nested model for type =
// "Open Directory" (UI label "Apple Open Directory").
type directoryBindingOpenDirectoryModel struct {
	EncryptUsingSSL      types.Bool `tfsdk:"encrypt_using_ssl"`
	PerformSecureBind    types.Bool `tfsdk:"perform_secure_bind"`
	UseForAuthentication types.Bool `tfsdk:"use_for_authentication"`
	UseForContacts       types.Bool `tfsdk:"use_for_contacts"`
}

// directoryBindingAdmitmacModel is the nested model for type = "ADmitMac".
// Mirrors the AD model's UI-aligned renames; ADmitMac's `local_home` is
// renamed to `home_location` to disambiguate from the AD field of the
// same wire name, which is a bool whereas ADmitMac's is a string.
type directoryBindingAdmitmacModel struct {
	RequireConfirmation     types.Bool   `tfsdk:"require_confirmation"`
	HomeLocation            types.String `tfsdk:"home_location"`
	NetworkProtocol         types.String `tfsdk:"network_protocol"`
	DefaultShell            types.String `tfsdk:"default_shell"`
	MountNetworkHome        types.Bool   `tfsdk:"mount_network_home"`
	PlaceHomeFolders        types.String `tfsdk:"place_home_folders"`
	UIDAttributeMapping     types.String `tfsdk:"uid_attribute_mapping"`
	UserGIDAttributeMapping types.String `tfsdk:"user_gid_attribute_mapping"`
	GIDAttributeMapping     types.String `tfsdk:"gid_attribute_mapping"`
	AdminGroup              types.String `tfsdk:"admin_group"`
	CachedCredentials       types.Int64  `tfsdk:"cached_credentials"`
	AddUserToLocal          types.Bool   `tfsdk:"add_user_to_local"`
	UsersOU                 types.String `tfsdk:"users_ou"`
	GroupsOU                types.String `tfsdk:"groups_ou"`
	PrintersOU              types.String `tfsdk:"printers_ou"`
	SharedFoldersOU         types.String `tfsdk:"shared_folders_ou"`
}

// directoryBindingCentrifyModel is the nested model for type = "Centrify".
// `update_pam` round-trips through the wire element `<update_PAM>` —
// uppercase preserved in the SDK xml tag; the TF schema uses snake_case.
type directoryBindingCentrifyModel struct {
	WorkstationMode       types.Bool   `tfsdk:"workstation_mode"`
	OverwriteExisting     types.Bool   `tfsdk:"overwrite_existing"`
	UpdatePAM             types.Bool   `tfsdk:"update_pam"`
	Zone                  types.String `tfsdk:"zone"`
	PreferredDomainServer types.String `tfsdk:"preferred_domain_server"`
}

// directoryBindingIdentityModel is the identity object for resource imports
// and list-resource identities.
type directoryBindingIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DirectoryBindingListResourceModel is the config model for the list
// resource. Classic /directorybindings has no RSQL, so the filter shape
// reuses the shared client-side substring block.
type DirectoryBindingListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
