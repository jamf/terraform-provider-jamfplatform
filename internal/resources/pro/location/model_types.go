// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package location

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// VolumePurchasingLocationResourceModel represents the Terraform resource
// model for a Jamf Pro Volume Purchasing (VPP) location.
//
// `ServiceToken` and `ServiceTokenWoVersion` together form the WriteOnly token
// rotation pair: the base64-encoded `.vpptoken` payload is sent to Jamf Pro on
// the wire but never persisted in Terraform state; bumping
// `ServiceTokenWoVersion` triggers an `UpdateVolumePurchasingLocationV1` call
// that re-submits the current `service_token`.
//
// `Content` projects the Apple-returned purchased-content catalog (one row per
// adam_id) into state so downstream resources (mobile-device-app /
// mac-app assignments) can look up available licenses by `adam_id` without a
// separate data source. The slice mirrors Apple's catalog at the last sync —
// it can drift on `terraform refresh` whenever new licenses are purchased or
// counts change. `UseStateForUnknown` on the outer list prevents the
// Computed-list-becomes-Unknown shake on no-op plans; real catalog drift
// still appears as a normal Read-time refresh diff.
type VolumePurchasingLocationResourceModel struct {
	ID                                    types.String           `tfsdk:"id"`
	Name                                  types.String           `tfsdk:"name"`
	ServiceToken                          types.String           `tfsdk:"service_token"`
	ServiceTokenWoVersion                 types.Int64            `tfsdk:"service_token_wo_version"`
	AutomaticallyPopulatePurchasedContent types.Bool             `tfsdk:"automatically_populate_purchased_content"`
	SendNotificationWhenNoLongerAssigned  types.Bool             `tfsdk:"send_notification_when_no_longer_assigned"`
	AutoRegisterManagedUsers              types.Bool             `tfsdk:"auto_register_managed_users"`
	SiteID                                types.String           `tfsdk:"site_id"`
	SiteName                              types.String           `tfsdk:"site_name"`
	AppleID                               types.String           `tfsdk:"apple_id"`
	OrganizationName                      types.String           `tfsdk:"organization_name"`
	LocationName                          types.String           `tfsdk:"location_name"`
	CountryCode                           types.String           `tfsdk:"country_code"`
	Email                                 types.String           `tfsdk:"email"`
	TokenExpiration                       types.String           `tfsdk:"token_expiration"`
	TotalPurchasedLicenses                types.Int64            `tfsdk:"total_purchased_licenses"`
	TotalUsedLicenses                     types.Int64            `tfsdk:"total_used_licenses"`
	LastSyncTime                          types.String           `tfsdk:"last_sync_time"`
	ClientContextMismatch                 types.Bool             `tfsdk:"client_context_mismatch"`
	Content                               types.List             `tfsdk:"content"`
	Timeouts                              resourceTimeouts.Value `tfsdk:"timeouts"`
}

// VolumePurchasingLocationContentObjectAttrTypes returns the attribute-type map
// for one row of the `content` ListNestedAttribute. Used by the state builders
// to construct `types.List` values from `pro.VolumePurchasingContent` slices.
func VolumePurchasingLocationContentObjectAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"adam_id":                types.StringType,
		"content_type":           types.StringType,
		"device_types":           types.ListType{ElemType: types.StringType},
		"icon_url":               types.StringType,
		"license_count_in_use":   types.Int64Type,
		"license_count_reported": types.Int64Type,
		"license_count_total":    types.Int64Type,
		"name":                   types.StringType,
		"pricing_param":          types.StringType,
	}
}

// volumePurchasingLocationIdentityModel represents the identity object for the
// VPP location resource. Used for import and list results.
type volumePurchasingLocationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// VolumePurchasingLocationDataSourceModel is the Terraform data source model
// for the singular `jamfplatform_pro_volume_purchasing_location` data source.
// Lookup is by `id` OR `name` (ExactlyOneOf). Mirrors the resource shape
// minus the WriteOnly attributes (`service_token`, `service_token_wo_version`)
// which the Jamf Pro GET response never echoes back.
type VolumePurchasingLocationDataSourceModel struct {
	ID                                    types.String             `tfsdk:"id"`
	Name                                  types.String             `tfsdk:"name"`
	AutomaticallyPopulatePurchasedContent types.Bool               `tfsdk:"automatically_populate_purchased_content"`
	SendNotificationWhenNoLongerAssigned  types.Bool               `tfsdk:"send_notification_when_no_longer_assigned"`
	AutoRegisterManagedUsers              types.Bool               `tfsdk:"auto_register_managed_users"`
	SiteID                                types.String             `tfsdk:"site_id"`
	SiteName                              types.String             `tfsdk:"site_name"`
	AppleID                               types.String             `tfsdk:"apple_id"`
	OrganizationName                      types.String             `tfsdk:"organization_name"`
	LocationName                          types.String             `tfsdk:"location_name"`
	CountryCode                           types.String             `tfsdk:"country_code"`
	Email                                 types.String             `tfsdk:"email"`
	TokenExpiration                       types.String             `tfsdk:"token_expiration"`
	TotalPurchasedLicenses                types.Int64              `tfsdk:"total_purchased_licenses"`
	TotalUsedLicenses                     types.Int64              `tfsdk:"total_used_licenses"`
	LastSyncTime                          types.String             `tfsdk:"last_sync_time"`
	ClientContextMismatch                 types.Bool               `tfsdk:"client_context_mismatch"`
	Content                               types.List               `tfsdk:"content"`
	Timeouts                              datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// VolumePurchasingLocationListResourceModel is the config model for the list
// resource. The Pro `/v1/volume-purchasing-locations` list endpoint accepts an
// RSQL filter, but the provider intentionally uses the shared client-side
// substring matcher to keep the surface symmetric with the other simple Pro
// list resources.
type VolumePurchasingLocationListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
