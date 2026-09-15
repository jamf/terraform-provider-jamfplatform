// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package location

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignVolumePurchasingLocationResourceModel populates a resource model from
// a `pro.VolumePurchasingLocation` GET response.
//
// Apple-returned strings (`OrganizationName`, `LocationName`, etc.) may
// contain trailing whitespace; the assignment intentionally does not
// TrimSpace them so plan output mirrors the byte-exact server representation.
//
// `ServiceToken` is `WriteOnly` and is never overwritten from the wire
// response — the field is omitted from this function entirely so the
// plan-supplied (or null) value carries through unchanged.
//
// `Content` is projected as a Computed `types.List` of nested objects so
// downstream resources (mobile-device app / Mac app assignments) can look up
// licence counts by `adam_id` without reaching for a separate data source.
func assignVolumePurchasingLocationResourceModel(ctx context.Context, state *VolumePurchasingLocationResourceModel, loc *pro.VolumePurchasingLocation) diag.Diagnostics {
	if loc == nil {
		return nil
	}
	state.ID = types.StringValue(loc.ID)
	state.Name = types.StringValue(loc.Name)
	state.AutomaticallyPopulatePurchasedContent = types.BoolValue(loc.AutomaticallyPopulatePurchasedContent)
	state.SendNotificationWhenNoLongerAssigned = types.BoolValue(loc.SendNotificationWhenNoLongerAssigned)
	state.AutoRegisterManagedUsers = types.BoolValue(loc.AutoRegisterManagedUsers)
	state.SiteID = types.StringValue(loc.SiteID)
	state.SiteName = types.StringValue(loc.SiteName)
	state.AppleID = types.StringValue(loc.AppleID)
	state.OrganizationName = types.StringValue(loc.OrganizationName)
	state.LocationName = types.StringValue(loc.LocationName)
	state.CountryCode = types.StringValue(loc.CountryCode)
	state.Email = types.StringValue(loc.Email)
	state.TokenExpiration = types.StringValue(loc.TokenExpiration)
	state.TotalPurchasedLicenses = types.Int64Value(int64(loc.TotalPurchasedLicenses))
	state.TotalUsedLicenses = types.Int64Value(int64(loc.TotalUsedLicenses))
	state.LastSyncTime = types.StringValue(loc.LastSyncTime)
	state.ClientContextMismatch = types.BoolValue(loc.ClientContextMismatch)

	contentList, diags := contentListValue(ctx, loc.Content)
	if diags.HasError() {
		return diags
	}
	state.Content = contentList
	return diags
}

// assignVolumePurchasingLocationDataSourceModel populates the data source
// model from a `pro.VolumePurchasingLocation` GET response. Mirrors the
// resource state builder byte-for-byte minus the WriteOnly token fields.
func assignVolumePurchasingLocationDataSourceModel(ctx context.Context, state *VolumePurchasingLocationDataSourceModel, loc *pro.VolumePurchasingLocation) diag.Diagnostics {
	if loc == nil {
		return nil
	}
	state.ID = types.StringValue(loc.ID)
	state.Name = types.StringValue(loc.Name)
	state.AutomaticallyPopulatePurchasedContent = types.BoolValue(loc.AutomaticallyPopulatePurchasedContent)
	state.SendNotificationWhenNoLongerAssigned = types.BoolValue(loc.SendNotificationWhenNoLongerAssigned)
	state.AutoRegisterManagedUsers = types.BoolValue(loc.AutoRegisterManagedUsers)
	state.SiteID = types.StringValue(loc.SiteID)
	state.SiteName = types.StringValue(loc.SiteName)
	state.AppleID = types.StringValue(loc.AppleID)
	state.OrganizationName = types.StringValue(loc.OrganizationName)
	state.LocationName = types.StringValue(loc.LocationName)
	state.CountryCode = types.StringValue(loc.CountryCode)
	state.Email = types.StringValue(loc.Email)
	state.TokenExpiration = types.StringValue(loc.TokenExpiration)
	state.TotalPurchasedLicenses = types.Int64Value(int64(loc.TotalPurchasedLicenses))
	state.TotalUsedLicenses = types.Int64Value(int64(loc.TotalUsedLicenses))
	state.LastSyncTime = types.StringValue(loc.LastSyncTime)
	state.ClientContextMismatch = types.BoolValue(loc.ClientContextMismatch)

	contentList, diags := contentListValue(ctx, loc.Content)
	if diags.HasError() {
		return diags
	}
	state.Content = contentList
	return diags
}

// contentListValue projects `[]pro.VolumePurchasingContent` into a
// `types.List` of nested objects matching the schema's `content` attribute.
// A nil or empty input yields an empty (non-null) List value so a location
// with no purchased content distinguishes correctly from a location whose
// content was never read.
func contentListValue(ctx context.Context, content []pro.VolumePurchasingContent) (types.List, diag.Diagnostics) {
	objType := types.ObjectType{AttrTypes: VolumePurchasingLocationContentObjectAttrTypes()}
	if content == nil {
		return types.ListValueMust(objType, []attr.Value{}), nil
	}
	values := make([]attr.Value, 0, len(content))
	for _, item := range content {
		deviceTypes, diags := types.ListValueFrom(ctx, types.StringType, item.DeviceTypes)
		if diags.HasError() {
			return types.ListNull(objType), diags
		}
		obj, diags := types.ObjectValue(VolumePurchasingLocationContentObjectAttrTypes(), map[string]attr.Value{
			"adam_id":                types.StringValue(item.AdamID),
			"content_type":           types.StringValue(item.ContentType),
			"device_types":           deviceTypes,
			"icon_url":               types.StringValue(item.IconURL),
			"license_count_in_use":   types.Int64Value(int64(item.LicenseCountInUse)),
			"license_count_reported": types.Int64Value(int64(item.LicenseCountReported)),
			"license_count_total":    types.Int64Value(int64(item.LicenseCountTotal)),
			"name":                   types.StringValue(item.Name),
			"pricing_param":          types.StringValue(item.PricingParam),
		})
		if diags.HasError() {
			return types.ListNull(objType), diags
		}
		values = append(values, obj)
	}
	return types.ListValue(objType, values)
}
