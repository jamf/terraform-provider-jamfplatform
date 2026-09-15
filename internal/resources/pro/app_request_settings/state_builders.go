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

// assignAppRequestSettingsResourceModel populates a resource model from an SDK response.
// The App Request settings API echoes every field on a successful GET (full-replace PUT),
// so enabled / app_store_locale are read straight through. approver_emails becomes an
// empty set when the server returns none; requester_user_group_id is null when unset.
func assignAppRequestSettingsResourceModel(ctx context.Context, state *AppRequestSettingsResourceModel, s *pro.AppRequestSettings) diag.Diagnostics {
	var diags diag.Diagnostics
	if s == nil {
		return diags
	}

	state.Enabled = helpers.BoolPointerValueOrNull(s.IsEnabled)
	state.AppStoreLocale = helpers.StringPointerValueOrNull(s.AppStoreLocale)

	emails := []string{}
	if s.ApproverEmails != nil {
		emails = *s.ApproverEmails
	}
	set, setDiags := types.SetValueFrom(ctx, types.StringType, emails)
	diags.Append(setDiags...)
	if diags.HasError() {
		return diags
	}
	state.ApproverEmails = set

	if s.RequesterUserGroupID != nil {
		state.RequesterUserGroupID = types.Int64Value(int64(*s.RequesterUserGroupID))
	} else {
		state.RequesterUserGroupID = types.Int64Null()
	}

	return diags
}
