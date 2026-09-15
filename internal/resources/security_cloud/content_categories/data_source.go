// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package content_categories implements the
// jamfplatform_security_cloud_content_categories data source, a read-only view of
// the Jamf-curated content category catalogue.
//
// Content categories are Jamf's own classification of web and app traffic —
// "Social", "Gambling", "Cloud & File Storage" and so on. They are the same for
// every entitled tenant and cannot be created, changed or deleted, so this package
// holds only a plural data source: a list resource over a fixed catalogue nobody
// manages would be ceremony without a payoff.
//
// Plural-only is a choice, not a constraint the API imposes. The catalogue is
// unpaginated and small, so one call serves every use; and the lookup a singular
// data source would exist for — by display name — already ships in the SDK
// client-side, as ResolveContentCategoryV1ByName and
// ResolveContentCategoryV1IDByName. A singular data source would add a Terraform
// construct that wraps a client-side filter over a list this one already returns.
// Add one if a configuration turns out to want the filtering server-side of the
// HCL, not because the wire forces it.
//
// The reason it exists is discovery, and one trap in particular. A category carries
// two names: `display_name` ("Social") and an internal `name`
// ("Category - Social"). Anything matching a category matches the **display name**,
// and the internal name is informational only — wire-confirmed by the SDK, whose
// generated resolver looks the catalogue up by `displayName`. Hard-coding either
// string into a configuration also risks drifting from a catalogue Jamf revises, so
// reading the ID or display name from here is the safer reference.
//
// Wire-probed 2026-08-29 in production EU: 36 categories, unpaginated, `totalCount`
// equal to the number of results, display names unique and returned in sorted
// order.
package content_categories

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultReadTimeout caps how long the content categories read will wait.
const defaultReadTimeout = 60 * time.Second

// dataSourceID is the fixed ID this data source reports. The catalogue is the same
// for every read, so there is nothing to derive an ID from.
const dataSourceID = "content_categories"

// ContentCategoriesDataSource implements the Terraform data source for the
// Jamf-curated content category catalogue.
type ContentCategoriesDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &ContentCategoriesDataSource{}

// NewContentCategoriesDataSource returns a new instance of ContentCategoriesDataSource.
func NewContentCategoriesDataSource() datasource.DataSource {
	return &ContentCategoriesDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ContentCategoriesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_content_categories"
}

// Schema returns the data source schema.
func (d *ContentCategoriesDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the content categories available in Jamf Security Cloud: the " +
			"classification of web and app traffic, such as `Social` or `Cloud & File Storage`. The catalogue " +
			"is centrally curated, identical for every entitled tenant, and cannot be changed.\n\n" +
			"Use this to reference a category without hard-coding a name that may be revised, in an output or " +
			"to pre-stage an identifier. A category has two names: reference `display_name`, not `name`.\n\n" +
			"A `jamfplatform_security_cloud_ztna_app` is what matches a category, so resolve the category " +
			"here and wire `display_name` into its `category` rather than writing the name out." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"content_categories": schema.ListNestedAttribute{
				MarkdownDescription: "The content categories available to this tenant, in the order Jamf " +
					"Security Cloud returns them.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Content category identifier.",
							Computed:            true,
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "The category name as shown in Jamf Security Cloud, for " +
								"example `Social`. **This is the name that identifies the category.** A " +
								"Zero Trust Network Access app's category matches on it, and it is the name " +
								"to reference. Wire it into a `jamfplatform_security_cloud_ztna_app`'s " +
								"`category`.",
							Computed: true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Jamf Security Cloud's internal label for the category, for " +
								"example `Category - Social`. Informational only; reference `display_name` " +
								"instead.",
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *ContentCategoriesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_content_categories")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the content category catalogue and populates Terraform state.
func (d *ContentCategoriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ContentCategoriesDataSourceModel
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

	categories, err := d.client.ListContentCategoriesV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud content categories", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(dataSourceID)
	data.ContentCategories = make([]ContentCategoryResultModel, 0, len(categories.Results))
	for _, c := range categories.Results {
		data.ContentCategories = append(data.ContentCategories, ContentCategoryResultModel{
			ID:          types.StringValue(c.ID),
			DisplayName: types.StringValue(c.DisplayName),
			Name:        types.StringValue(c.Name),
		})
	}

	tflog.Trace(ctx, "read Jamf Security Cloud content categories data source", map[string]any{"count": len(data.ContentCategories)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
