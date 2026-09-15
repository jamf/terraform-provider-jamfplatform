// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package app_store_country_codes implements the read-only
// jamfplatform_pro_app_store_country_codes data source, which surfaces the set of valid
// App Store country/region codes for the tenant (the values accepted by
// jamfplatform_pro_app_request_settings.app_store_locale). The valid set varies by
// tenant/version, so this is sourced live rather than from a static list.
package app_store_country_codes

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
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

// minJamfProVersion is the minimum Jamf Pro tenant version required by this data source.
// Empty: no per-resource floor.
const minJamfProVersion = ""

// countryCodeObjectType is the element type of the country_codes list.
var countryCodeObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"code": types.StringType,
		"name": types.StringType,
	},
}

// AppStoreCountryCodesDataSourceModel is the data source model for the country-code list.
type AppStoreCountryCodesDataSourceModel struct {
	ID           types.String   `tfsdk:"id"`
	Search       types.String   `tfsdk:"search"`
	CountryCodes types.List     `tfsdk:"country_codes"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

// countryCodeModel is the element model of the country_codes list.
type countryCodeModel struct {
	Code types.String `tfsdk:"code"`
	Name types.String `tfsdk:"name"`
}

// AppStoreCountryCodesDataSource implements the Terraform data source for Jamf Pro App
// Store country codes.
type AppStoreCountryCodesDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &AppStoreCountryCodesDataSource{}

// NewAppStoreCountryCodesDataSource returns a new instance of AppStoreCountryCodesDataSource.
func NewAppStoreCountryCodesDataSource() datasource.DataSource {
	return &AppStoreCountryCodesDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AppStoreCountryCodesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_store_country_codes"
}

// Schema returns the data source schema.
func (d *AppStoreCountryCodesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the set of valid App Store country and region codes for the tenant: the values accepted by `jamfplatform_pro_app_request_settings.app_store_locale`, alongside the literal `deviceLocale`. The valid set varies by tenant and by Jamf Pro version. Use `search` to narrow the result to entries whose code or name contains a substring." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"search": schema.StringAttribute{
				MarkdownDescription: "Optional case-insensitive substring to narrow the returned entries (matched against both the code and the country name). When omitted, the full list is returned.",
				Optional:            true,
			},
			"country_codes": schema.ListNestedAttribute{
				MarkdownDescription: "The list of valid App Store country/region codes.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"code": schema.StringAttribute{
							MarkdownDescription: "ISO 3166-1 alpha-2 country code (for example, `US`).",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Country or region display name.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *AppStoreCountryCodesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_store_country_codes")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the country-code list (optionally filtered by search) and populates state.
func (d *AppStoreCountryCodesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AppStoreCountryCodesDataSourceModel
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

	codes, err := d.client.ListAppStoreCountryCodesV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro App Store country codes", helpers.APIErrorDetail(err))
		return
	}

	search := strings.ToLower(strings.TrimSpace(data.Search.ValueString()))
	models := make([]countryCodeModel, 0)
	if codes != nil {
		for _, c := range codes.CountryCodes {
			if search != "" && !strings.Contains(strings.ToLower(c.Code), search) && !strings.Contains(strings.ToLower(c.Name), search) {
				continue
			}
			models = append(models, countryCodeModel{
				Code: types.StringValue(c.Code),
				Name: types.StringValue(c.Name),
			})
		}
	}

	list, listDiags := types.ListValueFrom(ctx, countryCodeObjectType, models)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.CountryCodes = list
	data.ID = types.StringValue("app_store_country_codes")

	tflog.Trace(ctx, "read Jamf Pro App Store country codes data source", map[string]any{"count": len(models)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
