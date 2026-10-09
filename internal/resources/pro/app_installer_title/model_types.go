// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_title

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// AppInstallerTitleDataSourceModel is the read-only data source model for a
// single App Installer catalog title. Every attribute other than `id` and
// `version` is returned by Jamf Pro; the catalog is server-managed and not
// user-creatable.
//
// `version` is both a lookup argument and an echoed field: omit it and Jamf Pro
// answers with the title's current version, set it and Jamf Pro answers with
// that historical version's metadata — a different package hash, minimum OS
// version and signing identity, all of which move between versions.
type AppInstallerTitleDataSourceModel struct {
	ID                         types.String               `tfsdk:"id"`
	Version                    types.String               `tfsdk:"version"`
	TitleName                  types.String               `tfsdk:"title_name"`
	Publisher                  types.String               `tfsdk:"publisher"`
	BundleID                   types.String               `tfsdk:"bundle_id"`
	ShortVersion               types.String               `tfsdk:"short_version"`
	Architecture               types.String               `tfsdk:"architecture"`
	MinimumOsVersion           types.String               `tfsdk:"minimum_os_version"`
	Language                   types.String               `tfsdk:"language"`
	AvailabilityDate           types.String               `tfsdk:"availability_date"`
	IconURL                    types.String               `tfsdk:"icon_url"`
	MediaSourceType            types.String               `tfsdk:"media_source_type"`
	InstallationPathShared     types.Bool                 `tfsdk:"installation_path_shared"`
	SizeInBytes                types.Int64                `tfsdk:"size_in_bytes"`
	InstallerPackageHash       types.String               `tfsdk:"installer_package_hash"`
	InstallerPackageHashType   types.String               `tfsdk:"installer_package_hash_type"`
	LaunchDaemonIncluded       types.Bool                 `tfsdk:"launch_daemon_included"`
	NotificationAvailable      types.Bool                 `tfsdk:"notification_available"`
	PackageSigningIdentity     types.String               `tfsdk:"package_signing_identity"`
	SuppressAutoUpdate         types.Bool                 `tfsdk:"suppress_auto_update"`
	OriginalMediaSources       []OriginalMediaSourceModel `tfsdk:"original_media_sources"`
	OriginalTermsAndConditions types.List                 `tfsdk:"original_terms_and_conditions"`
	Versions                   []TitleVersionModel        `tfsdk:"versions"`
	Timeouts                   timeouts.Value             `tfsdk:"timeouts"`
}

// TitleVersionModel models one version Jamf Pro still publishes for a title.
// Most titles publish none: the catalog keeps a version history only for titles
// whose older builds remain installable, so an empty list is the common case and
// not a sign of a failed read.
type TitleVersionModel struct {
	Version         types.String `tfsdk:"version"`
	MediaSourceType types.String `tfsdk:"media_source_type"`
}

// AppInstallerTitleSummaryModel is one element of the plural data source's
// `titles` list. The catalog list endpoint returns a strict subset of the
// per-title fields, so the sparse shape is modelled explicitly rather than
// reusing the singular model and leaving two thirds of it null — see
// STYLE_GUIDE §"Plural (list) data sources". Read the singular data source for
// a title's package metadata.
type AppInstallerTitleSummaryModel struct {
	ID                     types.String `tfsdk:"id"`
	TitleName              types.String `tfsdk:"title_name"`
	Publisher              types.String `tfsdk:"publisher"`
	BundleID               types.String `tfsdk:"bundle_id"`
	Version                types.String `tfsdk:"version"`
	IconURL                types.String `tfsdk:"icon_url"`
	InstallationPathShared types.Bool   `tfsdk:"installation_path_shared"`
}

// OriginalMediaSourceModel models one original media source entry on a title.
type OriginalMediaSourceModel struct {
	Hash     types.String `tfsdk:"hash"`
	HashType types.String `tfsdk:"hash_type"`
	URL      types.String `tfsdk:"url"`
}
