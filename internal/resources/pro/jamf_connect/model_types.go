// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_connect

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// Jamf Connect auto-deployment types. The wire tokens are cryptic; the
// schema descriptions translate them into the admin-UI "Update Type" labels
// (Manual / Maintenance / Minor & Maintenance) and the deploy toggle.
const (
	// NONE is the toggle off — the server manages nothing and the version is ignored.
	autoDeploymentNone = pro.LinkedConnectProfileAutoDeploymentTypeNone
	// INITIAL_INSTALLATION_ONLY is the UI's "Manual".
	autoDeploymentInitialOnly = pro.LinkedConnectProfileAutoDeploymentTypeInitialInstallationOnly
	// PATCH_UPDATES is the UI's "Maintenance".
	autoDeploymentPatch = pro.LinkedConnectProfileAutoDeploymentTypePatchUpdates
	// MINOR_AND_PATCH_UPDATES is the UI's "Minor & Maintenance".
	autoDeploymentMinorAndPatch = pro.LinkedConnectProfileAutoDeploymentTypeMinorAndPatchUpdates
)

// JamfConnectResourceModel is the Terraform model for an adopted Jamf Connect
// deployment-and-update configuration.
//
// The resource adopts a pre-existing macOS configuration profile that already
// carries a Jamf Connect payload (the profile auto-links into the Jamf Connect
// settings the moment it contains such a payload). Terraform manages only how
// Jamf Connect is auto-deployed/updated on that profile — it never creates or
// removes the profile itself.
//
//   - ProfileID is the adoption key — the Jamf Pro configuration profile ID
//     (a jamfplatform_pro_macos_configuration_profile's id). RequiresReplace.
//     The resource resolves it to the Jamf Connect profile UUID internally.
//   - ConfigProfileUUID is the resolved Jamf Connect UUID (server-minted,
//     distinct from the config profile's own PayloadUUID) — Computed; used as
//     the PUT path key.
//   - AutoDeploymentType + Version are the only writable fields.
//   - ProfileName / ScopeDescription / SiteID are server-derived display
//     fields, plain Computed.
type JamfConnectResourceModel struct {
	ID                types.String `tfsdk:"id"`
	ConfigProfileUUID types.String `tfsdk:"config_profile_uuid"`

	AutoDeploymentType types.String `tfsdk:"auto_deployment_type"`
	Version            types.String `tfsdk:"version"`

	ProfileID        types.Int64  `tfsdk:"profile_id"`
	ProfileName      types.String `tfsdk:"profile_name"`
	ScopeDescription types.String `tfsdk:"scope_description"`
	SiteID           types.String `tfsdk:"site_id"`

	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// jamfConnectIdentityModel is the import/identity object — keyed by the Jamf
// Connect profile UUID (the adoption key).
type jamfConnectIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// JamfConnectDataSourceModel is the model for the singular data source. Lookup
// is by exactly one of config_profile_uuid, profile_id, or profile_name; the
// remaining fields are populated from the matched row.
type JamfConnectDataSourceModel struct {
	ConfigProfileUUID types.String `tfsdk:"config_profile_uuid"`
	ProfileID         types.Int64  `tfsdk:"profile_id"`
	ProfileName       types.String `tfsdk:"profile_name"`

	AutoDeploymentType types.String `tfsdk:"auto_deployment_type"`
	Version            types.String `tfsdk:"version"`
	ScopeDescription   types.String `tfsdk:"scope_description"`
	SiteID             types.String `tfsdk:"site_id"`

	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}
