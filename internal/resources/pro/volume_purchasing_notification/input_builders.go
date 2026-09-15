// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildVolumePurchasingNotificationInput projects a plan model into the SDK
// request type for both create and update. The endpoint is full-replace: every
// collection is always emitted (an empty slice clears the field, never nil), and
// every internal recipient is sent with frequency:"DAILY" (the only accepted
// value). `enabled` is omitted when null/unknown so the server applies its default
// (true) on create; on update the plan value is always known (config or carried
// forward) and is sent verbatim. `site_id` is sent as a pointer, defaulting to the
// "-1" no-site sentinel when null/unknown.
func buildVolumePurchasingNotificationInput(ctx context.Context, plan VolumePurchasingNotificationResourceModel) (*pro.VolumePurchasingSubscriptionBase, diag.Diagnostics) {
	var diags diag.Diagnostics

	triggers, d := helpers.SetToStringSlice(ctx, plan.Triggers)
	diags.Append(d...)
	triggers = emptyIfNil(triggers)

	locationIDs, d := helpers.SetToStringSlice(ctx, plan.LocationIDs)
	diags.Append(d...)
	locationIDs = emptyIfNil(locationIDs)

	accountIDs, d := helpers.SetToStringSlice(ctx, plan.InternalRecipients)
	diags.Append(d...)
	internal := make([]pro.InternalRecipient, 0, len(accountIDs))
	for _, id := range accountIDs {
		freq := frequencyDaily
		internal = append(internal, pro.InternalRecipient{AccountID: id, Frequency: &freq})
	}

	external, d := expandExternalRecipients(ctx, plan.ExternalRecipients)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	out := &pro.VolumePurchasingSubscriptionBase{
		Name:               plan.Name.ValueString(),
		Enabled:            helpers.OptionalBoolPointer(plan.Enabled),
		Triggers:           &triggers,
		LocationIds:        &locationIDs,
		InternalRecipients: &internal,
		ExternalRecipients: &external,
		SiteID:             siteIDPointer(plan.SiteID),
	}
	return out, diags
}

// expandExternalRecipients converts the external_recipients Set into the SDK
// slice. A null/unknown set yields an empty (non-nil) slice so the full-replace
// write clears the field.
func expandExternalRecipients(ctx context.Context, set types.Set) ([]pro.ExternalRecipient, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := []pro.ExternalRecipient{}
	if set.IsNull() || set.IsUnknown() {
		return out, diags
	}
	var models []ExternalRecipientModel
	diags.Append(set.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return out, diags
	}
	for _, m := range models {
		out = append(out, pro.ExternalRecipient{
			Email: m.Email.ValueString(),
			Name:  m.Name.ValueString(),
		})
	}
	return out, diags
}

// siteIDPointer returns the configured site_id, defaulting to the "-1" no-site
// sentinel when null/unknown so the full-replace write is explicit.
func siteIDPointer(value types.String) *string {
	v := siteNone
	if helpers.IsConfiguredValue(value) {
		v = value.ValueString()
	}
	return &v
}

// emptyIfNil normalises a nil slice (null/unknown set) to an empty slice so the
// full-replace payload always emits the collection.
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
