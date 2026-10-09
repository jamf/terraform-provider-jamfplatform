// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package pkg (folder name `package`, Go identifier `pkg` to avoid the
// reserved-word collision) implements the jamfplatform_pro_package resource,
// data source, and list resource backed by the Jamf Pro /v1/packages API.
package pkg

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// PackageResourceModel represents the Terraform resource model for a Jamf
// Pro package. The flat envelope mirrors the UI-aligned attribute names;
// wire-side translation (TF attribute → SDK request/response field) lives
// in mappings.go, input_builders.go, and state_builders.go.
//
// Three operating modes are inferred at runtime, NOT modelled in the type:
//   - JCDS: PackageFileSource non-empty. Hash attrs Computed-only post-upload.
//   - FSDP-with-hashes: PackageFileSource empty, any hash attr set. Provider
//     PUTs user-supplied values; server stores verbatim.
//   - Pure metadata-only: PackageFileSource empty, no hashes.
type PackageResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	DisplayName               types.String `tfsdk:"display_name"`
	FileName                  types.String `tfsdk:"file_name"`
	CategoryID                types.String `tfsdk:"category_id"`
	Info                      types.String `tfsdk:"info"`
	Notes                     types.String `tfsdk:"notes"`
	Priority                  types.Int64  `tfsdk:"priority"`
	FillUserTemplate          types.Bool   `tfsdk:"fill_user_template"`
	FillExistingUsers         types.Bool   `tfsdk:"fill_existing_users"`
	RebootRequired            types.Bool   `tfsdk:"reboot_required"`
	OSRequirements            types.String `tfsdk:"os_requirements"`
	AvailableInSoftwareUpdate types.Bool   `tfsdk:"available_in_software_update"`

	// Upload-source inputs (never returned by the server; provider-internal).
	PackageFileSource         types.String `tfsdk:"package_file_source"`
	PackageFileSourceChecksum types.String `tfsdk:"package_file_source_checksum"`
	StreamURLDirectly         types.Bool   `tfsdk:"stream_url_directly"`
	ManifestFileSource        types.String `tfsdk:"manifest_file_source"`

	// Manifest body Computed: server echoes verbatim plist on read.
	Manifest         types.String `tfsdk:"manifest"`
	ManifestFileName types.String `tfsdk:"manifest_file_name"`

	// Hash attributes: Optional+Computed. FSDP user-settable, JCDS
	// server-populated. ConflictsWith(package_file_source) enforced in schema.
	Sha3512   types.String `tfsdk:"sha3_512"`
	Sha256    types.String `tfsdk:"sha256"`
	Md5       types.String `tfsdk:"md5"`
	HashType  types.String `tfsdk:"hash_type"`
	HashValue types.String `tfsdk:"hash_value"`

	// Computed-only server-derived fields. Captured to suppress drift.
	Size                types.String `tfsdk:"size"`
	InstallLanguage     types.String `tfsdk:"install_language"`
	ParentPackageID     types.String `tfsdk:"parent_package_id"`
	SelfHealingAction   types.String `tfsdk:"self_healing_action"`
	SelfHealNotify      types.Bool   `tfsdk:"self_heal_notify"`
	CloudTransferStatus types.String `tfsdk:"cloud_transfer_status"`
	Indexed             types.Bool   `tfsdk:"indexed"`
	Format              types.String `tfsdk:"format"`

	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// PackageDataSourceModel is the Computed-everywhere data source view of the
// resource. ID and DisplayName accept Optional+Computed selectors so users
// can look up by either; the rest mirrors the resource model.
type PackageDataSourceModel struct {
	ID                        types.String             `tfsdk:"id"`
	DisplayName               types.String             `tfsdk:"display_name"`
	FileName                  types.String             `tfsdk:"file_name"`
	CategoryID                types.String             `tfsdk:"category_id"`
	Info                      types.String             `tfsdk:"info"`
	Notes                     types.String             `tfsdk:"notes"`
	Priority                  types.Int64              `tfsdk:"priority"`
	FillUserTemplate          types.Bool               `tfsdk:"fill_user_template"`
	FillExistingUsers         types.Bool               `tfsdk:"fill_existing_users"`
	RebootRequired            types.Bool               `tfsdk:"reboot_required"`
	OSRequirements            types.String             `tfsdk:"os_requirements"`
	AvailableInSoftwareUpdate types.Bool               `tfsdk:"available_in_software_update"`
	Manifest                  types.String             `tfsdk:"manifest"`
	ManifestFileName          types.String             `tfsdk:"manifest_file_name"`
	Sha3512                   types.String             `tfsdk:"sha3_512"`
	Sha256                    types.String             `tfsdk:"sha256"`
	Md5                       types.String             `tfsdk:"md5"`
	HashType                  types.String             `tfsdk:"hash_type"`
	HashValue                 types.String             `tfsdk:"hash_value"`
	Size                      types.String             `tfsdk:"size"`
	InstallLanguage           types.String             `tfsdk:"install_language"`
	ParentPackageID           types.String             `tfsdk:"parent_package_id"`
	SelfHealingAction         types.String             `tfsdk:"self_healing_action"`
	SelfHealNotify            types.Bool               `tfsdk:"self_heal_notify"`
	CloudTransferStatus       types.String             `tfsdk:"cloud_transfer_status"`
	Indexed                   types.Bool               `tfsdk:"indexed"`
	Format                    types.String             `tfsdk:"format"`
	Timeouts                  datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// packageIdentityModel represents the identity object for package resources
// and list results.
type packageIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// PackageListResourceModel represents the config model for package list
// queries.
type PackageListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}
