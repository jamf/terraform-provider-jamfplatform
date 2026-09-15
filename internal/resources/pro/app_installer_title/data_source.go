// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package app_installer_title implements the read-only
// jamfplatform_pro_app_installer_title data source, which looks up a single
// App Installer catalog title by ID. The App Installer catalog is managed by
// Jamf Pro; titles are not user-creatable. Use this data source to discover a
// title's ID for jamfplatform_pro_app_installer.app_title_id.
package app_installer_title

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultReadTimeout = 60 * time.Second

// minJamfProVersion is the minimum Jamf Pro tenant version required by this data
// source. Empty: the App Installer endpoints predate the provider's overall
// floor (the App Catalog shipped well before the 11.0.0 minimum).
const minJamfProVersion = ""

// AppInstallerTitleDataSource implements the Terraform data source for a single
// App Installer catalog title.
type AppInstallerTitleDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AppInstallerTitleDataSource{}

// NewAppInstallerTitleDataSource returns a new instance of AppInstallerTitleDataSource.
func NewAppInstallerTitleDataSource() datasource.DataSource {
	return &AppInstallerTitleDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AppInstallerTitleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_installer_title"
}

// Schema returns the data source schema.
func (d *AppInstallerTitleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := TitleDataSourceAttributes()
	attrs["id"] = schema.StringAttribute{
		MarkdownDescription: "Catalog title ID to look up.",
		Required:            true,
	}
	attrs["version"] = schema.StringAttribute{
		MarkdownDescription: "Title version to look up. Omit to read the title's current version, which is then returned here. " +
			"Set it to read a historical version instead — the package hash, minimum OS version, availability date and signing identity all move between versions. " +
			"Use the `jamfplatform_pro_app_installer_titles` data source to discover available titles; a version Jamf Pro no longer publishes is a not-found error.",
		Optional: true,
		Computed: true,
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a single App Installer catalog title by ID. Titles are published by Jamf and cannot be created or modified; this data source surfaces a title's metadata so you can reference its `id` from `jamfplatform_pro_app_installer.app_title_id`." + dataSourcePrivileges,
		Attributes:          attrs,
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *AppInstallerTitleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_installer_title")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a title by ID and populates Terraform state.
func (d *AppInstallerTitleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AppInstallerTitleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "The id attribute must be provided to read an App Installer title.")
		return
	}

	version := ""
	if helpers.IsConfiguredValue(data.Version) {
		version = data.Version.ValueString()
	}

	readCtx, cancel := context.WithTimeout(ctx, defaultReadTimeout)
	defer cancel()

	got, err := d.client.GetAppInstallerTitleV1(readCtx, data.ID.ValueString(), version)
	if err != nil {
		resp.Diagnostics.AddError("Unable to find App Installer title", helpers.APIErrorDetail(err))
		return
	}

	data = AssignTitleDataSource(got)

	// The version list is a second endpoint; the per-title GET does not carry it.
	// A failure here is surfaced rather than swallowed: the attribute is Computed,
	// so returning an empty list on error would be indistinguishable from a title
	// that genuinely publishes no earlier versions, which is the common case.
	versions, err := d.client.ListAppInstallerTitleVersionsV1(readCtx, data.ID.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to list App Installer title versions", helpers.APIErrorDetail(err))
		return
	}
	data.Versions = assignTitleVersions(versions)

	tflog.Trace(ctx, "read App Installer title data source", map[string]any{"id": data.ID.ValueString(), "versions": len(data.Versions)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// TitleDataSourceAttributes returns the Computed attribute map describing one
// App Installer catalog title in full, as the per-title endpoint returns it.
// The caller adds the lookup `id` and `version` shapes. The plural data source
// does not reuse this map: the catalog list endpoint returns only a seven-field
// summary, which `TitleSummaryAttributes` describes.
func TitleDataSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":                          computedString("Catalog title ID."),
		"version":                     computedString("Published version this title's metadata describes."),
		"title_name":                  computedString("Title display name."),
		"publisher":                   computedString("Title publisher."),
		"bundle_id":                   computedString("Primary application bundle identifier."),
		"short_version":               computedString("Published short version string."),
		"architecture":                computedString("Supported CPU architecture (e.g. `x86_64`, `arm64`, `universal`)."),
		"minimum_os_version":          computedString("Minimum macOS version required to install the title."),
		"language":                    computedString("Title language."),
		"availability_date":           computedString("Date this version became available in the catalog."),
		"icon_url":                    computedString("URL of the title icon."),
		"media_source_type":           computedString("Where Jamf Pro downloads the installer from: `JAMF_SERVER` for a Jamf-hosted package, `EXTERNAL_URL` for the software vendor's own site."),
		"installation_path_shared":    schema.BoolAttribute{MarkdownDescription: "Whether another title may install to the same path as this one.", Computed: true},
		"size_in_bytes":               schema.Int64Attribute{MarkdownDescription: "Installer package size in bytes.", Computed: true},
		"installer_package_hash":      computedString("Hash of the installer package."),
		"installer_package_hash_type": computedString("Algorithm used for the installer package hash (e.g. `SHA_256`)."),
		"launch_daemon_included":      schema.BoolAttribute{MarkdownDescription: "Whether the title bundles a launch daemon.", Computed: true},
		"notification_available":      schema.BoolAttribute{MarkdownDescription: "Whether the title supports end-user install notifications.", Computed: true},
		"package_signing_identity":    computedString("Signing identity of the installer package."),
		"suppress_auto_update":        schema.BoolAttribute{MarkdownDescription: "Whether the title suppresses its built-in auto-update mechanism when managed by Jamf.", Computed: true},
		"versions": schema.ListNestedAttribute{
			MarkdownDescription: "Versions of this title Jamf Pro still publishes, oldest first, each usable as the `version` argument. " +
				"Empty for a title whose older builds are no longer installable, which is most of them — an empty list is not a failed read.",
			Computed: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"version":           computedString("Published version string."),
					"media_source_type": computedString("Where Jamf Pro downloads this version from: `JAMF_SERVER` or `EXTERNAL_URL`."),
				},
			},
		},
		"original_terms_and_conditions": schema.ListAttribute{
			MarkdownDescription: "URLs of the terms and conditions the software vendor publishes for this title. Empty when the vendor publishes none.",
			Computed:            true,
			ElementType:         types.StringType,
		},
		"original_media_sources": schema.ListNestedAttribute{
			MarkdownDescription: "Original media sources Jamf used to build the title's installer package.",
			Computed:            true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"hash":      computedString("Media source hash."),
					"hash_type": computedString("Media source hash algorithm."),
					"url":       computedString("Media source URL."),
				},
			},
		},
	}
}

// AssignTitleDataSource maps the per-title SDK response into the data source model.
func AssignTitleDataSource(t *pro.AppTitleDetails) AppInstallerTitleDataSourceModel {
	out := AppInstallerTitleDataSourceModel{}
	if t == nil {
		return out
	}
	out.ID = types.StringValue(t.ID)
	out.TitleName = types.StringValue(t.TitleName)
	out.Publisher = types.StringValue(t.Publisher)
	out.BundleID = types.StringValue(t.BundleID)
	out.Version = types.StringValue(t.Version)
	out.ShortVersion = types.StringValue(t.ShortVersion)
	out.Architecture = types.StringValue(t.Architecture)
	out.MinimumOsVersion = types.StringValue(t.MinimumOsVersion)
	out.Language = types.StringValue(t.Language)
	out.AvailabilityDate = types.StringValue(t.AvailabilityDate)
	out.IconURL = types.StringValue(t.IconURL)
	out.MediaSourceType = types.StringValue(t.MediaSourceType)
	out.InstallationPathShared = types.BoolValue(t.InstallationPathShared)
	out.SizeInBytes = types.Int64Value(int64(t.SizeInBytes))
	out.InstallerPackageHash = types.StringValue(t.InstallerPackageHash)
	out.InstallerPackageHashType = types.StringValue(t.InstallerPackageHashType)
	out.LaunchDaemonIncluded = types.BoolValue(t.LaunchDaemonIncluded)
	out.NotificationAvailable = types.BoolValue(t.NotificationAvailable)
	out.PackageSigningIdentity = types.StringValue(t.PackageSigningIdentity)
	out.SuppressAutoUpdate = types.BoolValue(t.SuppressAutoUpdate)
	out.OriginalTermsAndConditions = stringList(t.OriginalTermsAndConditions)
	out.OriginalMediaSources = make([]OriginalMediaSourceModel, 0, len(t.OriginalMediaSources))
	for _, m := range t.OriginalMediaSources {
		out.OriginalMediaSources = append(out.OriginalMediaSources, OriginalMediaSourceModel{
			Hash:     types.StringValue(m.Hash),
			HashType: types.StringValue(m.HashType),
			URL:      types.StringValue(m.URL),
		})
	}
	return out
}

// assignTitleVersions maps the version-list response into the model. The slice is
// always non-nil so a title publishing no earlier versions serialises as an empty
// list rather than null.
func assignTitleVersions(r *pro.AppTitleVersionsResult) []TitleVersionModel {
	out := []TitleVersionModel{}
	if r == nil {
		return out
	}
	for _, v := range r.Results {
		out = append(out, TitleVersionModel{
			Version:         helpers.StringPointerValueOrNull(v.Version),
			MediaSourceType: types.StringValue(v.MediaSourceType),
		})
	}
	return out
}

// stringList renders a server string slice as a types.List, always non-null so
// a vendor that publishes no terms and conditions yields an empty list rather
// than null.
func stringList(in []string) types.List {
	elems := make([]attr.Value, 0, len(in))
	for _, v := range in {
		elems = append(elems, types.StringValue(v))
	}
	return types.ListValueMust(types.StringType, elems)
}

// computedString returns a Computed-only StringAttribute for a server-derived
// title field.
func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
}
