// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignWebhookResourceModel populates a resource model from the SDK Webhook
// response. The server faithfully echoes every managed field (and the
// Optional+Computed defaults), so they are read straight back; `username` /
// `header` map server-empty → null, and `smart_group_id` normalises the -1 /
// absent sentinels → null. `password` is never written into state (WriteOnly),
// and `password_wo_version` is preserved from the caller's plan/state.
func assignWebhookResourceModel(ctx context.Context, state *WebhookResourceModel, w *proclassic.Webhook) diag.Diagnostics {
	var diags diag.Diagnostics
	if w == nil {
		return diags
	}

	if id := extractWebhookID(w); id != "" {
		state.ID = types.StringValue(id)
	}
	state.Name = helpers.StringPointerValueOrNull(w.Name)
	state.Enabled = helpers.BoolPointerValueOrNull(w.Enabled)
	state.URL = helpers.StringPointerValueOrNull(w.URL)
	state.AuthenticationType = helpers.StringPointerValueOrNull(w.AuthenticationType)
	state.ConnectionTimeout = helpers.Int64FromIntPtr(w.ConnectionTimeout)
	state.ReadTimeout = helpers.Int64FromIntPtr(w.ReadTimeout)
	state.ContentType = helpers.StringPointerValueOrNull(w.ContentType)
	state.Event = helpers.StringPointerValueOrNull(w.Event)
	state.Username = emptyStringToNull(w.Username)
	state.Header = emptyStringToNull(w.Header)
	state.HashAlgorithm = helpers.StringPointerValueOrNull(w.HashAlgorithm)
	state.SmartGroupID = smartGroupIDToState(w.SmartGroupID)
	state.EnableDisplayFieldsForGroupObject = helpers.BoolPointerValueOrNull(w.EnableDisplayFieldsForGroupObject)
	state.DisplayFields = flattenStringSet(ctx, displayFieldNames(w.DisplayFields))

	return diags
}
