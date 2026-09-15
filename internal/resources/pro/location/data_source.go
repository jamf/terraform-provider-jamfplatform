// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetVolumePurchasingLocationV1
//   pro.ResolveVolumePurchasingLocationV1ByName
// Status: current. Last reviewed 2026-05-25.

package location

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// VolumePurchasingLocationDataSource implements the Terraform data source for
// Jamf Pro Volume Purchasing (VPP) locations. The singular data source
// supports lookup by ID OR by name — exactly one of the two must be supplied.
// WriteOnly resource attributes (`service_token`, `service_token_wo_version`)
// are omitted from the data source schema because the Jamf Pro GET response
// never echoes them back.
type VolumePurchasingLocationDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &VolumePurchasingLocationDataSource{}
	_ datasource.DataSourceWithConfigure        = &VolumePurchasingLocationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &VolumePurchasingLocationDataSource{}
)

// NewVolumePurchasingLocationDataSource returns a new instance of the data
// source.
func NewVolumePurchasingLocationDataSource() datasource.DataSource {
	return &VolumePurchasingLocationDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *VolumePurchasingLocationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_volume_purchasing_location"
}

// Schema returns the data source schema.
func (d *VolumePurchasingLocationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Volume Purchasing (VPP) location by ID or by exact name. Exactly one of `id` or `name` must be supplied. The uploaded service token is never returned, because Jamf Pro does not return it on reads. Use the `jamfplatform_pro_volume_purchasing_location` resource to manage the token." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Volume Purchasing location ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "VPP location display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"automatically_populate_purchased_content": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro automatically populates purchased content from Apple after every sync for this location.",
				Computed:            true,
			},
			"send_notification_when_no_longer_assigned": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro sends a notification when a previously-assigned content item is no longer assigned to the location.",
				Computed:            true,
			},
			"auto_register_managed_users": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro auto-registers managed users associated with this location.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site ID associated with this VPP location, or the sentinel `\"-1\"` when no site is set.",
				Computed:            true,
			},
			"site_name": schema.StringAttribute{
				MarkdownDescription: "Site display name for the associated `site_id`. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"apple_id": schema.StringAttribute{
				MarkdownDescription: "Apple ID associated with the uploaded service token.",
				Computed:            true,
			},
			"organization_name": schema.StringAttribute{
				MarkdownDescription: "Organization name parsed from the uploaded service token. Apple may return values containing trailing whitespace; the provider preserves the exact value Jamf Pro reports.",
				Computed:            true,
			},
			"location_name": schema.StringAttribute{
				MarkdownDescription: "Apple-returned location name (distinct from the user-supplied `name`). Apple may return values containing trailing whitespace; the provider preserves the exact value Jamf Pro reports.",
				Computed:            true,
			},
			"country_code": schema.StringAttribute{
				MarkdownDescription: "Apple-returned country code for the location.",
				Computed:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Apple-returned contact email for the location.",
				Computed:            true,
			},
			"token_expiration": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 expiration timestamp for the uploaded service token.",
				Computed:            true,
			},
			"total_purchased_licenses": schema.Int64Attribute{
				MarkdownDescription: "Total number of licenses purchased across all content items for this location.",
				Computed:            true,
			},
			"total_used_licenses": schema.Int64Attribute{
				MarkdownDescription: "Total number of licenses currently in use across all content items for this location.",
				Computed:            true,
			},
			"last_sync_time": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 timestamp of the most recent Apple content sync for this location.",
				Computed:            true,
			},
			"client_context_mismatch": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro detected a client-context mismatch for this location.",
				Computed:            true,
			},
			"content": schema.ListNestedAttribute{
				MarkdownDescription: "Apple-returned purchased-content catalog for this location, one row per " +
					"`adam_id`. Use this to look up `license_count_total` / `license_count_in_use` for a " +
					"specific App Store / iTunes adam_id before assigning a Mac or iOS app.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"adam_id": schema.StringAttribute{
							MarkdownDescription: "Apple App Store / iTunes adam_id for this content item.",
							Computed:            true,
						},
						"content_type": schema.StringAttribute{
							MarkdownDescription: "Apple-reported content type (e.g. `App`, `Book`).",
							Computed:            true,
						},
						"device_types": schema.ListAttribute{
							MarkdownDescription: "Device types this content item targets.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"icon_url": schema.StringAttribute{
							MarkdownDescription: "App Store / iTunes icon URL for this content item.",
							Computed:            true,
						},
						"license_count_in_use": schema.Int64Attribute{
							MarkdownDescription: "Number of licenses Jamf Pro reports as currently assigned for this content item.",
							Computed:            true,
						},
						"license_count_reported": schema.Int64Attribute{
							MarkdownDescription: "Number of licenses Apple last reported for this content item.",
							Computed:            true,
						},
						"license_count_total": schema.Int64Attribute{
							MarkdownDescription: "Total number of licenses purchased for this content item.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Apple-reported display name for this content item.",
							Computed:            true,
						},
						"pricing_param": schema.StringAttribute{
							MarkdownDescription: "Apple-reported pricing parameter for this content item.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *VolumePurchasingLocationDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *VolumePurchasingLocationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_volume_purchasing_location")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a VPP location by ID or by name and populates Terraform state.
func (d *VolumePurchasingLocationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data VolumePurchasingLocationDataSourceModel
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
		got *pro.VolumePurchasingLocation
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetVolumePurchasingLocationV1(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.ResolveVolumePurchasingLocationV1ByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing Volume Purchasing location selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro Volume Purchasing location", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignVolumePurchasingLocationDataSourceModel(ctx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro Volume Purchasing location data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
