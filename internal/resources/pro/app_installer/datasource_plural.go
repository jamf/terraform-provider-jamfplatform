// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// AppInstallersDataSource implements the Terraform plural data source.
type AppInstallersDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AppInstallersDataSource{}

// NewAppInstallersDataSource returns a new instance of AppInstallersDataSource.
func NewAppInstallersDataSource() datasource.DataSource {
	return &AppInstallersDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AppInstallersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_installers"
}

// Schema returns the data source schema.
func (d *AppInstallersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns every App Installer deployment for the tenant in expanded form, including the resolved app, site, category, and smart-group references and per-deployment computer status counts. Use the optional `name_substring` to narrow the result; matching is case-insensitive and applied after the full list is fetched." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"name_substring": schema.StringAttribute{
				MarkdownDescription: "Optional case-insensitive substring matched against each deployment's name. When omitted, all deployments are returned.",
				Optional:            true,
			},
			"deployments": schema.ListNestedAttribute{
				MarkdownDescription: "App Installer deployments, optionally narrowed by `name_substring`.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{MarkdownDescription: "Deployment ID.", Computed: true},
						"name":            schema.StringAttribute{MarkdownDescription: "Deployment display name.", Computed: true},
						"enabled":         schema.BoolAttribute{MarkdownDescription: "Whether the deployment is enabled.", Computed: true},
						"deployment_type": schema.StringAttribute{MarkdownDescription: "Delivery method.", Computed: true},
						"update_behavior": schema.StringAttribute{MarkdownDescription: "Update behavior.", Computed: true},
						"app": schema.SingleNestedAttribute{
							MarkdownDescription: "Resolved catalog app reference. Null when the deployment has no app.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"id":                     schema.StringAttribute{MarkdownDescription: "Catalog title ID.", Computed: true},
								"bundle_id":              schema.StringAttribute{MarkdownDescription: "Primary bundle identifier.", Computed: true},
								"icon_url":               schema.StringAttribute{MarkdownDescription: "App icon URL.", Computed: true},
								"latest_version":         schema.StringAttribute{MarkdownDescription: "Latest catalog version.", Computed: true},
								"selected_version":       schema.StringAttribute{MarkdownDescription: "Selected version. Empty under AUTOMATIC.", Computed: true},
								"deployed_version":       schema.StringAttribute{MarkdownDescription: "Currently deployed version.", Computed: true},
								"media_source_type":      schema.StringAttribute{MarkdownDescription: "Media source type (e.g. JAMF_SERVER).", Computed: true},
								"title_available_in_ais": schema.BoolAttribute{MarkdownDescription: "Whether the title is available in the App Installers catalog.", Computed: true},
								"version_removed":        schema.BoolAttribute{MarkdownDescription: "Whether the selected version has been removed.", Computed: true},
							},
						},
						"site":        namedRefAttribute("Resolved site reference. Null when no site is set."),
						"category":    namedRefAttribute("Resolved category reference. Null when no category is set."),
						"smart_group": namedRefAttribute("Resolved smart computer group reference. Null when no smart group is set."),
						"computer_statuses": schema.SingleNestedAttribute{
							MarkdownDescription: "Per-deployment computer status counts. Null when the server does not report them.",
							Computed:            true,
							Attributes: map[string]schema.Attribute{
								"available":   schema.Int64Attribute{MarkdownDescription: "Computers where the app is available.", Computed: true},
								"failed":      schema.Int64Attribute{MarkdownDescription: "Computers where the install failed.", Computed: true},
								"in_progress": schema.Int64Attribute{MarkdownDescription: "Computers where the install is in progress.", Computed: true},
								"installed":   schema.Int64Attribute{MarkdownDescription: "Computers where the app is installed.", Computed: true},
								"unqualified": schema.Int64Attribute{MarkdownDescription: "Computers that do not qualify for the deployment.", Computed: true},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// namedRefAttribute returns the Computed SingleNested schema for a resolved
// {id, name} reference (site / category / smart group).
func namedRefAttribute(desc string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: desc,
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{MarkdownDescription: "Reference ID.", Computed: true},
			"name": schema.StringAttribute{MarkdownDescription: "Reference display name.", Computed: true},
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *AppInstallersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_installers")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches all deployments (optionally narrowed) and populates state.
func (d *AppInstallersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AppInstallersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	entries, err := d.client.ListAppInstallerDeploymentsV1(readCtx, nil, "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to list App Installer deployments", helpers.APIErrorDetail(err))
		return
	}

	data.Deployments = FilterAndMapDeployments(entries, data.NameSubstring)
	data.ID = types.StringValue("app_installers")

	tflog.Trace(ctx, "read App Installer deployments data source", map[string]any{"returned": len(data.Deployments)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// FilterAndMapDeployments maps every expanded list entry into the data source
// model, keeping only deployments whose name contains nameSubstring
// (case-insensitive) when set. The slice is always non-nil so an empty result
// serialises as an empty list.
func FilterAndMapDeployments(entries []pro.AppTitleDeploymentSummary, nameSubstring types.String) []DeploymentModel {
	out := make([]DeploymentModel, 0, len(entries))
	substr := ""
	if helpers.IsConfiguredValue(nameSubstring) {
		substr = strings.ToLower(nameSubstring.ValueString())
	}
	for i := range entries {
		if substr != "" && !strings.Contains(strings.ToLower(entries[i].Name), substr) {
			continue
		}
		out = append(out, mapDeployment(entries[i]))
	}
	return out
}

// mapDeployment projects one expanded list entry into the model.
func mapDeployment(e pro.AppTitleDeploymentSummary) DeploymentModel {
	m := DeploymentModel{
		ID:             types.StringValue(e.ID),
		Name:           types.StringValue(e.Name),
		Enabled:        types.BoolValue(e.Enabled),
		DeploymentType: types.StringValue(e.DeploymentType),
		UpdateBehavior: types.StringValue(e.UpdateBehavior),
	}
	if e.App != nil {
		m.App = &AppModel{
			ID:                  types.StringValue(e.App.ID),
			BundleID:            helpers.StringPointerValueOrNull(e.App.BundleID),
			IconURL:             helpers.StringPointerValueOrNull(e.App.IconURL),
			LatestVersion:       helpers.StringPointerValueOrNull(e.App.LatestVersion),
			SelectedVersion:     types.StringValue(e.App.SelectedVersion),
			DeployedVersion:     types.StringValue(e.App.DeployedVersion),
			MediaSourceType:     types.StringValue(e.App.MediaSourceType),
			TitleAvailableInAis: types.BoolValue(e.App.TitleAvailableInAis),
			VersionRemoved:      types.BoolValue(e.App.VersionRemoved),
		}
	}
	if e.Site != nil {
		m.Site = namedRef(e.Site.ID, e.Site.Name)
	}
	if e.Category != nil {
		m.Category = namedRef(e.Category.ID, e.Category.Name)
	}
	if e.SmartGroup != nil {
		m.SmartGroup = namedRef(e.SmartGroup.ID, e.SmartGroup.Name)
	}
	if e.ComputerStatuses != nil {
		m.ComputerStatuses = &ComputerStatusesModel{
			Available:   types.Int64Value(int64(e.ComputerStatuses.Available)),
			Failed:      types.Int64Value(int64(e.ComputerStatuses.Failed)),
			InProgress:  types.Int64Value(int64(e.ComputerStatuses.InProgress)),
			Installed:   types.Int64Value(int64(e.ComputerStatuses.Installed)),
			Unqualified: types.Int64Value(int64(e.ComputerStatuses.Unqualified)),
		}
	}
	return m
}

// namedRef projects one of the list entry's site/category/smart-group
// references into the model. Jamf Pro declares each reference's ID as always
// present and its name as nullable, and documents null as either "no assignment"
// or "no permission to read that object" — indistinguishable from here — so a
// missing name stays null in state rather than becoming an empty string.
func namedRef(id string, name *string) *NamedRefModel {
	return &NamedRefModel{
		ID:   types.StringValue(id),
		Name: helpers.StringPointerValueOrNull(name),
	}
}
