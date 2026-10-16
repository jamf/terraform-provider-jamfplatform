// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package app_store_app implements the jamfplatform_apple_app_store_app data source: an App Store
// app's public listing, read so that jamfplatform_pro_mac_app_store_app and
// jamfplatform_pro_mobile_device_app can take their name, version, bundle identifier, store URL,
// description and icon from the store instead of from hand-copied values.
//
// The upstream is Apple, not Jamf. The lookup goes through the SDK's standalone appstore package
// (github.com/jamf/jamfplatform-go-sdk/appstore), whose godoc carries the wire law probed
// 2026-10-16. Three consequences shape this package:
//
//   - No Jamf call is made, so Configure takes no SDK client, applies no integration-scope gate
//     and no Jamf Pro version floor, and the description carries no permissions table. The data
//     source still needs a configured provider, because provider Configure validates the Jamf
//     credentials before any construct runs.
//   - Lookups go through providerdata.AppStoreCache: one per configured provider instance, a
//     listing or a definitive miss stored for the rest of the run, at most
//     providerdata.AppStoreMaxConcurrent requests in flight.
//   - The App Store matches a bundle identifier case-insensitively but Jamf Pro stores it verbatim,
//     so a configured bundle_id differing from the listing's only in case is an error naming the
//     right spelling rather than a value passed through to an app record.
//   - icon_url is the listing's artwork image, never iconArtwork, for both targets: artwork is what
//     Jamf Pro pulls in by default when it adds an App Store app itself, so the icon matches an app
//     added in Jamf Pro. On iPhone and iPad that is the opaque full-bleed square rather than the
//     rounded-square masked image (see iconURL).
//
// The resources are deliberately untouched: no lookup happens inside them, and values are wired
// across explicitly in configuration. An App Store release therefore appears as a planned change
// on whichever app record reads version, and an artwork change replaces the
// jamfplatform_pro_icon built from icon_url.
//
// Attribute ↔ listing field:
//
//	id                          results.<key>.id
//	bundle_id                   results.<key>.bundleId
//	store_url                   parsed client-side for the id and storefront
//	target                      platform: mac → macappstore, mobile_device → enterprisestore
//	country_region              cc, checked against meta.storefront.cc
//	language                    l (negotiated by Apple)
//	name                        name
//	version                     offers[first without quantity].version.display
//	url                         url
//	description                 description.standard
//	icon_url                    artwork.url filled at 512x512bb.png (see iconURL)
//	is_free                     offers[first without quantity].price == 0
//	genres                      genreNames
//	vpp_device_based_licensing  isVppDeviceBasedLicensingEnabled
package app_store_app

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// AppStoreAppDataSource implements jamfplatform_apple_app_store_app.
type AppStoreAppDataSource struct {
	lookups *providerdata.AppStoreCache
}

var (
	_ datasource.DataSource                     = &AppStoreAppDataSource{}
	_ datasource.DataSourceWithConfigure        = &AppStoreAppDataSource{}
	_ datasource.DataSourceWithConfigValidators = &AppStoreAppDataSource{}
)

// NewAppStoreAppDataSource returns a new instance of AppStoreAppDataSource.
func NewAppStoreAppDataSource() datasource.DataSource {
	return &AppStoreAppDataSource{}
}

// Metadata sets the data source type name.
func (d *AppStoreAppDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_apple_app_store_app"
}

// selectorPaths are the three mutually exclusive ways to name the app.
var selectorPaths = []path.Expression{
	path.MatchRoot("id"),
	path.MatchRoot("bundle_id"),
	path.MatchRoot("store_url"),
}

// Schema returns the data source schema.
func (d *AppStoreAppDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up an app's App Store listing, so a `jamfplatform_pro_mac_app_store_app` or " +
			"`jamfplatform_pro_mobile_device_app` can take its name, version, bundle identifier, store URL, description " +
			"and icon from the App Store instead of having them copied by hand. Set exactly one of `id`, `bundle_id` or " +
			"`store_url`, and set `target` to the kind of app record the values feed.\n\n" +
			"The listing is read again on every plan. When the developer releases a new version, `version` changes and " +
			"an app record that uses it shows the new version as a planned change. Pin `general.version` or list it in " +
			"`lifecycle.ignore_changes` to keep releases out of plans.\n\n" +
			"The icon follows the listing the same way. When the developer changes the app's artwork, `icon_url` " +
			"changes, the next plan replaces the `jamfplatform_pro_icon` built from it, and the app record picks up the " +
			"new icon. That replacement is intended and is not drift.\n\n" +
			"Custom apps distributed to a single organization are not listed publicly, so they cannot be looked up. " +
			"No Jamf Pro permission is needed, because the lookup reads the App Store directly.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App Store ID: the number after `id` in the app's App Store link (`497799835` for " +
					"Xcode). Set exactly one of `id`, `bundle_id` or `store_url`. Filled from the listing when the lookup " +
					"uses one of the other two.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9]+$`), "must be the App Store ID's digits")},
			},
			"bundle_id": schema.StringAttribute{
				MarkdownDescription: "App bundle identifier, such as `com.apple.dt.Xcode`. Set exactly one of `id`, " +
					"`bundle_id` or `store_url`. Jamf Pro stores a bundle identifier exactly as written, so a value whose " +
					"letter case differs from the App Store listing is rejected with the listed spelling. Filled from the " +
					"listing when the lookup uses `id` or `store_url`. Use it for `general.bundle_id`.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.RegexMatches(regexp.MustCompile(`^[^,\s]+$`), "must be a single bundle identifier")},
			},
			"store_url": schema.StringAttribute{
				MarkdownDescription: "The app's App Store link, such as " +
					"`https://apps.apple.com/gb/app/pages-create-documents/id361309726`. Set exactly one of `id`, " +
					"`bundle_id` or `store_url`. The country or region in the link is used when `country_region` is not set.",
				Optional:   true,
				Validators: []validator.String{storeURLValidator{}},
			},
			"target": schema.StringAttribute{
				MarkdownDescription: "The kind of app record the values feed: `mac` for `jamfplatform_pro_mac_app_store_app` " +
					"(Mac Apps) or `mobile_device` for `jamfplatform_pro_mobile_device_app` (Mobile Device Apps). An app " +
					"sold for both has a separate Mac listing with its own icon, description and version, so the target " +
					"must match the record. An iPhone and iPad app is not found with `mac`. One of " + valueList(targetValues()) + ".",
				Required:   true,
				Validators: []validator.String{stringvalidator.OneOf(targetValues()...)},
			},
			"country_region": schema.StringAttribute{
				MarkdownDescription: "Two-letter App Store country or region code in upper case, such as `US` or `GB`. " +
					"Defaults to the one in `store_url`, then `US`. A code the App Store does not recognise is rejected " +
					"rather than answered from another country. Use it for `general.itunes_country_region` on a mobile " +
					"device app.",
				Optional:   true,
				Computed:   true,
				Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Z]{2}$`), "must be two upper-case letters, such as US")},
			},
			"language": schema.StringAttribute{
				MarkdownDescription: "Language for the listing's name and description, such as `en` or `fr`. When the " +
					"country or region does not offer it, the App Store answers in that country's default language.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "App name as listed. Use it for `general.name`.",
				Computed:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Version currently on sale. Use it for `general.version`.",
				Computed:            true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "App Store URL of the listing in `country_region`. Use it for `general.url` on a Mac " +
					"app or `general.itunes_store_url` on a mobile device app.",
				Computed: true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "App description as listed. Use it for `self_service.self_service_description`.",
				Computed:            true,
			},
			"icon_url": schema.StringAttribute{
				MarkdownDescription: "URL of the app's icon as a 512 × 512 PNG. Use it as `icon_file_source` on a " +
					"`jamfplatform_pro_icon`, and set the app record's `self_service.self_service_icon.id` to that icon. " +
					"Null when the listing has no icon.",
				Computed: true,
			},
			"is_free": schema.BoolAttribute{
				MarkdownDescription: "Whether the app is free in `country_region`. Use it for `general.is_free`.",
				Computed:            true,
			},
			"genres": schema.ListAttribute{
				MarkdownDescription: "App Store categories, primary category first.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"vpp_device_based_licensing": schema.BoolAttribute{
				MarkdownDescription: "Whether licenses bought through Apple School Manager or Apple Business Manager can " +
					"be assigned to devices rather than to users. The app record's " +
					"`vpp.assign_vpp_device_based_licenses = true` depends on it.",
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// valueList renders accepted values as a Markdown list of code spans.
func valueList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "`" + v + "`"
	}
	return strings.Join(quoted, ", ")
}

// ConfigValidators enforces that exactly one of id, bundle_id and store_url is set.
func (d *AppStoreAppDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(selectorPaths...)}
}

// Configure takes the App Store lookup cache from the provider data. No Jamf client, scope gate or
// Jamf Pro version check applies: this data source makes no Jamf call.
func (d *AppStoreAppDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData))
		return
	}
	d.lookups = pd.AppStoreCache()
}

// Read looks the app up and populates state.
func (d *AppStoreAppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AppStoreAppDataSourceModel
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

	query, diags := buildQuery(&data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	app, err := d.lookups.Lookup(readCtx, query)
	if err != nil {
		resp.Diagnostics.Append(lookupErrorDiagnostic(query, data.Target.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(assignApp(ctx, &data, query, app)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read App Store listing", map[string]any{"id": data.ID.ValueString(), "target": data.Target.ValueString(), "country_region": data.CountryRegion.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
