// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_title

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// AppInstallerTitlesDataSourceModel is the plural data source model. `titles`
// carries the full catalog; the optional `name_substring` narrows the result
// client-side (the catalog endpoint has no server-side filter).
type AppInstallerTitlesDataSourceModel struct {
	ID            types.String                    `tfsdk:"id"`
	NameSubstring types.String                    `tfsdk:"name_substring"`
	Titles        []AppInstallerTitleSummaryModel `tfsdk:"titles"`
}

// AppInstallerTitlesDataSource implements the Terraform plural data source.
type AppInstallerTitlesDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AppInstallerTitlesDataSource{}

// NewAppInstallerTitlesDataSource returns a new instance of AppInstallerTitlesDataSource.
func NewAppInstallerTitlesDataSource() datasource.DataSource {
	return &AppInstallerTitlesDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AppInstallerTitlesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_installer_titles"
}

// Schema returns the data source schema.
func (d *AppInstallerTitlesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns the App Installer catalog — every title published to the tenant. Titles are managed by Jamf and cannot be created or modified. Use the optional `name_substring` to narrow the result; matching is case-insensitive and applied after the full catalog is fetched." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"name_substring": schema.StringAttribute{
				MarkdownDescription: "Optional case-insensitive substring matched against each title's name. When omitted, the whole catalog is returned.",
				Optional:            true,
			},
			"titles": schema.ListNestedAttribute{
				MarkdownDescription: "Catalog titles, optionally narrowed by `name_substring`. The catalog endpoint returns a summary of each title; read `jamfplatform_pro_app_installer_title` for a title's package metadata.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: TitleSummaryAttributes(),
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *AppInstallerTitlesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_installer_titles")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the full catalog (optionally narrowed) and populates state.
func (d *AppInstallerTitlesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AppInstallerTitlesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, defaultPluralReadTimeout)
	defer cancel()

	titles, err := d.client.ListAppInstallerTitlesV1(readCtx, nil, "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to list App Installer titles", helpers.APIErrorDetail(err))
		return
	}

	data.Titles = FilterAndMapTitles(titles, data.NameSubstring)
	data.ID = types.StringValue("app_installer_titles")

	tflog.Trace(ctx, "read App Installer titles data source", map[string]any{"returned": len(data.Titles)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// TitleSummaryAttributes returns the Computed attribute map describing one
// title as the catalog list endpoint returns it — a seven-field summary, not
// the full per-title shape.
func TitleSummaryAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id":                       computedString("Catalog title ID."),
		"title_name":               computedString("Title display name."),
		"publisher":                computedString("Title publisher."),
		"bundle_id":                computedString("Primary application bundle identifier."),
		"version":                  computedString("Current published version."),
		"icon_url":                 computedString("URL of the title icon."),
		"installation_path_shared": schema.BoolAttribute{MarkdownDescription: "Whether another title may install to the same path as this one.", Computed: true},
	}
}

// FilterAndMapTitles maps every SDK title summary into the plural model, keeping
// only titles whose name contains nameSubstring (case-insensitive) when that
// filter is set. The slice is always non-nil so an empty result serialises as
// an empty list, not null.
//
// The narrowing stays client-side deliberately. Jamf Pro's own `titleName`
// filter is a case-insensitive glob, so a substring would have to be sent as
// `*x*` — which silently changes meaning for a substring containing `*`. The
// whole catalog is a few hundred summaries.
func FilterAndMapTitles(titles []pro.AppTitle, nameSubstring types.String) []AppInstallerTitleSummaryModel {
	out := make([]AppInstallerTitleSummaryModel, 0, len(titles))
	substr := ""
	if helpers.IsConfiguredValue(nameSubstring) {
		substr = strings.ToLower(nameSubstring.ValueString())
	}
	for i := range titles {
		if substr != "" && !strings.Contains(strings.ToLower(titles[i].TitleName), substr) {
			continue
		}
		out = append(out, assignTitleSummary(titles[i]))
	}
	return out
}

// assignTitleSummary maps one SDK catalog summary into the plural model.
func assignTitleSummary(t pro.AppTitle) AppInstallerTitleSummaryModel {
	return AppInstallerTitleSummaryModel{
		ID:                     types.StringValue(t.ID),
		TitleName:              types.StringValue(t.TitleName),
		Publisher:              types.StringValue(t.Publisher),
		BundleID:               types.StringValue(t.BundleID),
		Version:                types.StringValue(t.Version),
		IconURL:                types.StringValue(t.IconURL),
		InstallationPathShared: types.BoolValue(t.InstallationPathShared),
	}
}
