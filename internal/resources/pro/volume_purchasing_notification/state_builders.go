// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignVolumePurchasingNotificationResourceModel populates a resource model from
// a GET response. All four collections flatten an empty wire array to an empty set
// (never null): the endpoint is full-replace, so null and [] both mean "none" and
// state always mirrors the server echo. internal_recipients drops the per-recipient
// frequency (always "DAILY") and keeps only the account id.
func assignVolumePurchasingNotificationResourceModel(ctx context.Context, state *VolumePurchasingNotificationResourceModel, d *pro.VolumePurchasingSubscription) diag.Diagnostics {
	var diags diag.Diagnostics
	if d == nil {
		return diags
	}

	state.ID = types.StringValue(d.ID)
	state.Name = types.StringValue(d.Name)
	state.Enabled = types.BoolValue(d.Enabled)
	state.SiteID = types.StringValue(d.SiteID)

	triggers, td := types.SetValueFrom(ctx, types.StringType, emptyStrings(d.Triggers))
	diags.Append(td...)
	state.Triggers = triggers

	locations, ld := types.SetValueFrom(ctx, types.StringType, emptyStrings(d.LocationIds))
	diags.Append(ld...)
	state.LocationIDs = locations

	accountIDs := make([]string, 0, len(d.InternalRecipients))
	for _, ir := range d.InternalRecipients {
		accountIDs = append(accountIDs, ir.AccountID)
	}
	internal, id := types.SetValueFrom(ctx, types.StringType, accountIDs)
	diags.Append(id...)
	state.InternalRecipients = internal

	external, ed := externalRecipientsToSet(ctx, d.ExternalRecipients)
	diags.Append(ed...)
	state.ExternalRecipients = external

	return diags
}

// externalRecipientsToSet maps the SDK external recipients into a TF Set of
// objects (empty, not null, when there are none).
func externalRecipientsToSet(ctx context.Context, in []pro.ExternalRecipient) (types.Set, diag.Diagnostics) {
	models := make([]ExternalRecipientModel, 0, len(in))
	for _, e := range in {
		models = append(models, ExternalRecipientModel{
			Email: types.StringValue(e.Email),
			Name:  types.StringValue(e.Name),
		})
	}
	return types.SetValueFrom(ctx, externalRecipientObjectType, models)
}

// assignVolumePurchasingNotificationDataSourceModel projects a GET response into
// the singular data source model. Collections are read-only Lists.
func assignVolumePurchasingNotificationDataSourceModel(ctx context.Context, state *VolumePurchasingNotificationDataSourceModel, d *pro.VolumePurchasingSubscription) diag.Diagnostics {
	var diags diag.Diagnostics
	if d == nil {
		return diags
	}

	state.ID = types.StringValue(d.ID)
	state.Name = types.StringValue(d.Name)
	state.Enabled = types.BoolValue(d.Enabled)
	state.SiteID = types.StringValue(d.SiteID)

	triggers, td := types.ListValueFrom(ctx, types.StringType, emptyStrings(d.Triggers))
	diags.Append(td...)
	state.Triggers = triggers

	locations, ld := types.ListValueFrom(ctx, types.StringType, emptyStrings(d.LocationIds))
	diags.Append(ld...)
	state.LocationIDs = locations

	accountIDs := make([]string, 0, len(d.InternalRecipients))
	for _, ir := range d.InternalRecipients {
		accountIDs = append(accountIDs, ir.AccountID)
	}
	internal, id := types.ListValueFrom(ctx, types.StringType, accountIDs)
	diags.Append(id...)
	state.InternalRecipients = internal

	models := make([]ExternalRecipientModel, 0, len(d.ExternalRecipients))
	for _, e := range d.ExternalRecipients {
		models = append(models, ExternalRecipientModel{
			Email: types.StringValue(e.Email),
			Name:  types.StringValue(e.Name),
		})
	}
	external, ed := types.ListValueFrom(ctx, externalRecipientObjectType, models)
	diags.Append(ed...)
	state.ExternalRecipients = external

	return diags
}

// emptyStrings normalises a nil slice to an empty slice so SetValueFrom /
// ListValueFrom produce a known empty collection rather than a typed nil.
func emptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
