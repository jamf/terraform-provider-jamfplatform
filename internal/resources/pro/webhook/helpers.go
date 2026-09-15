// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// extractWebhookID returns the assigned ID as a string from a Create/GET
// response. The classic /webhooks endpoint echoes the integer ID at the top
// level (<webhook><id>).
func extractWebhookID(w *proclassic.Webhook) string {
	if w == nil || w.ID == nil {
		return ""
	}
	return strconv.Itoa(*w.ID)
}

// smartGroupIDToState normalises the wire <smart_group_id> into state. The
// server returns the sentinel -1 for a smart-group event with no group
// selected, and omits the element entirely for non-smart events; both map to
// null so config-null == state-null (WEBHOOK_SPIKE.md §5 invariant 5).
func smartGroupIDToState(p *int) types.Int64 {
	if p == nil || *p == -1 {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

// emptyStringToNull maps a server-echoed empty string to null. Used for
// `username` / `header`, which the server returns as "" when the active auth
// type does not use them, so config-null == state-null.
func emptyStringToNull(p *string) types.String {
	if p == nil || *p == "" {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// displayFieldNames extracts the <display_field><name> values from the server
// response in order.
func displayFieldNames(df *proclassic.WebhookDisplayFields) []string {
	if df == nil || df.DisplayField == nil {
		return nil
	}
	names := make([]string, 0, len(*df.DisplayField))
	for _, item := range *df.DisplayField {
		if item.Name != nil {
			names = append(names, *item.Name)
		}
	}
	return names
}

// flattenStringSet builds a types.Set of strings, never null (the
// display_fields attribute is Computed and must hold a known value).
func flattenStringSet(ctx context.Context, names []string) types.Set {
	if names == nil {
		names = []string{}
	}
	out, _ := types.SetValueFrom(ctx, types.StringType, names)
	return out
}
