// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildAppRequestSettingsInput converts the Terraform plan model into an SDK payload. The
// App Request settings API is a full-replace PUT, so every field is emitted on every write
// (wire-probed 2026-06-13: omitting appStoreLocale returns HTTP 500, omitting approverEmails
// clears it). enabled / app_store_locale / requester_user_group_id are Optional+Computed
// with UseStateForUnknown: on update an omitted field is a known prior value (preserved);
// on first create there is no prior state, so the `current` merge base — the live settings
// read in Create — supplies the value for any field the user omitted, so the singleton is
// adopted rather than reset. On update `current` is nil (USFU already filled the plan).
// approver_emails is Required, so it is always taken from the plan.
func buildAppRequestSettingsInput(ctx context.Context, plan AppRequestSettingsResourceModel, current *pro.AppRequestSettings) (*pro.AppRequestSettings, diag.Diagnostics) {
	emails, diags := helpers.SetToStringSlice(ctx, plan.ApproverEmails)
	if diags.HasError() {
		return nil, diags
	}

	enabled := boolPtrOrCurrent(plan.Enabled, currentBoolPtr(current))
	requester := intPtrOrCurrent(plan.RequesterUserGroupID, currentIntPtr(current))
	// A disabled App Request has no requester group. Never send a (possibly stale or
	// dangling) group id when disabling: Jamf Pro validates requesterUserGroupId on every
	// write and rejects an unknown id with a 400 even when isEnabled is false (wire-probed
	// 2026-06-13). ModifyPlan nulls the planned value in the same case, so plan and wire
	// agree.
	if enabled != nil && !*enabled {
		requester = nil
	}

	return &pro.AppRequestSettings{
		IsEnabled:            enabled,
		AppStoreLocale:       stringPtrOrCurrent(plan.AppStoreLocale, currentStringPtr(current)),
		ApproverEmails:       &emails,
		RequesterUserGroupID: requester,
	}, diags
}

// boolPtrOrCurrent emits the plan value when known (declared, or USFU-carried on update),
// else falls back to the live value read from the server (preserve undeclared fields on
// first create).
func boolPtrOrCurrent(v types.Bool, current *bool) *bool {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueBoolPointer()
	}
	return current
}

// stringPtrOrCurrent mirrors boolPtrOrCurrent for the locale string.
func stringPtrOrCurrent(v types.String, current *string) *string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueStringPointer()
	}
	return current
}

// intPtrOrCurrent mirrors boolPtrOrCurrent for the requester user group ID. A null/unknown
// plan with a nil current yields a nil pointer (the field is omitted / cleared on the wire).
func intPtrOrCurrent(v types.Int64, current *int) *int {
	if !v.IsNull() && !v.IsUnknown() {
		return helpers.OptionalInt64Pointer(v)
	}
	return current
}

// currentBoolPtr safely extracts IsEnabled from a possibly-nil merge base.
func currentBoolPtr(current *pro.AppRequestSettings) *bool {
	if current == nil {
		return nil
	}
	return current.IsEnabled
}

// currentStringPtr safely extracts AppStoreLocale from a possibly-nil merge base.
func currentStringPtr(current *pro.AppRequestSettings) *string {
	if current == nil {
		return nil
	}
	return current.AppStoreLocale
}

// currentIntPtr safely extracts RequesterUserGroupID from a possibly-nil merge base.
func currentIntPtr(current *pro.AppRequestSettings) *int {
	if current == nil {
		return nil
	}
	return current.RequesterUserGroupID
}
