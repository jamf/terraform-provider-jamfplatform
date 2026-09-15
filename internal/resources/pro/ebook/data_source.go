// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// EbookDataSource implements the Terraform data source for Jamf Pro ebooks.
// Lookup is by ID or by exact name — exactly one must be supplied. The data
// source surfaces a flat Computed projection of the most-frequently looked-up
// fields. For full detail, manage the ebook as a resource or import it.
type EbookDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &EbookDataSource{}
	_ datasource.DataSourceWithConfigure        = &EbookDataSource{}
	_ datasource.DataSourceWithConfigValidators = &EbookDataSource{}
)

// NewEbookDataSource returns a new instance of EbookDataSource.
func NewEbookDataSource() datasource.DataSource {
	return &EbookDataSource{}
}

// Metadata sets the data source type name.
func (d *EbookDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ebook"
}

// Schema returns the data source schema.
func (d *EbookDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro ebook by ID or by exact name. Exactly one of `id` or `name` must be supplied. Surfaces a flat read-only projection; manage the ebook as a resource for full detail." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Ebook ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Ebook display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"author":            schema.StringAttribute{MarkdownDescription: "Ebook author.", Computed: true},
			"url":               schema.StringAttribute{MarkdownDescription: "Ebook URL.", Computed: true},
			"deployment_type":   schema.StringAttribute{MarkdownDescription: "Distribution Method.", Computed: true},
			"file_type":         schema.StringAttribute{MarkdownDescription: "File Type.", Computed: true},
			"version":           schema.StringAttribute{MarkdownDescription: "Ebook version.", Computed: true},
			"free":              schema.BoolAttribute{MarkdownDescription: "Whether the ebook is free.", Computed: true},
			"deploy_as_managed": schema.BoolAttribute{MarkdownDescription: "Whether the ebook is managed when possible.", Computed: true},
			"category_id":       schema.StringAttribute{MarkdownDescription: "Category ID.", Computed: true},
			"category_name":     schema.StringAttribute{MarkdownDescription: "Category display name.", Computed: true},
			"site_id":           schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means no site.", Computed: true},
			"site_name":         schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
			"timeouts":          timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *EbookDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *EbookDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ebook")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an ebook by ID or by name and populates Terraform state.
func (d *EbookDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data EbookDataSourceModel
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

	var (
		got *proclassic.Ebook
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetEbookByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetEbookByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing ebook selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro ebook", helpers.APIErrorDetail(err))
		return
	}
	assignEbookFlatDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro ebook data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignEbookFlatDataSource projects a *proclassic.Ebook into the flat data
// source model.
func assignEbookFlatDataSource(state *EbookDataSourceModel, e *proclassic.Ebook) {
	if e == nil {
		return
	}
	if id := extractEbookID(e); id != "" {
		state.ID = helpers.StringPointerValueOrNull(&id)
	}
	if e.General != nil {
		state.Name = helpers.StringPointerValueOrNull(e.General.Name)
		state.Author = helpers.StringPointerValueOrNull(e.General.Author)
		state.URL = helpers.StringPointerValueOrNull(e.General.URL)
		state.DeploymentType = helpers.StringPointerValueOrNull(e.General.DeploymentType)
		state.FileType = helpers.StringPointerValueOrNull(e.General.FileType)
		state.Version = helpers.StringPointerValueOrNull(e.General.Version)
		state.Free = helpers.BoolPointerValueOrNull(e.General.Free)
		state.DeployAsManaged = helpers.BoolPointerValueOrNull(e.General.DeployAsManaged)
		if e.General.Category != nil {
			state.CategoryID = helpers.StringValueFromIntPtr(e.General.Category.ID)
			state.CategoryName = helpers.DerivedRefName(e.General.Category.ID, e.General.Category.Name)
		}
		if e.General.Site != nil {
			state.SiteID = helpers.StringValueFromIntPtr(e.General.Site.ID)
			state.SiteName = helpers.DerivedRefName(e.General.Site.ID, e.General.Site.Name)
		}
	}
}
