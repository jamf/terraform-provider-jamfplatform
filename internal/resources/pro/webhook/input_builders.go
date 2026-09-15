// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildWebhookInput projects a plan model into an SDK *proclassic.Webhook for
// Create / Update. The classic PUT is a partial-merge, but the provider always
// sends the full plan so in-place edits converge.
//
// Three deliberate behaviours:
//
//   - `password` is sourced separately (it is a WriteOnly attribute, so the
//     plan exposes it as null). The caller threads a non-nil plaintext only
//     when `password_wo_version` changed; otherwise nil omits <password> and
//     Classic's merge retains the stored value (mirrors directory_binding).
//   - `smart_group_id` is ALWAYS emitted: the configured value, or the -1
//     sentinel ("none") when unset. This is load-bearing on transitions —
//     Classic's PUT merges, so omitting the element retains the stored group;
//     a smart→non-smart event change must therefore send -1 to clear the old
//     group, otherwise the server 409s (non-smart event + a real group is
//     illegal). The cross-field validator already forbids a configured
//     smart_group_id on a non-smart event, so the only value reaching a
//     non-smart event here is -1, which the server accepts and treats as
//     "none" (wire-probed — WEBHOOK_SPIKE.md §5.5). `display_fields` is never
//     written (Computed-only; the server 409s).
//   - `username` and `header` are emitted empty when null under every
//     authentication type except the one that requires them (see
//     authScopedField). Classic's merge retains an omitted element, so a
//     dropped attribute would otherwise survive on the server and Read would
//     echo it back as an inconsistent result (issue #384).
func buildWebhookInput(plan WebhookResourceModel, password *string) *proclassic.Webhook {
	in := &proclassic.Webhook{
		Name:                              helpers.OptionalStringPointer(plan.Name),
		Enabled:                           helpers.OptionalBoolPointer(plan.Enabled),
		URL:                               helpers.OptionalStringPointer(plan.URL),
		Event:                             helpers.OptionalStringPointer(plan.Event),
		ContentType:                       helpers.OptionalStringPointer(plan.ContentType),
		AuthenticationType:                helpers.OptionalStringPointer(plan.AuthenticationType),
		ConnectionTimeout:                 helpers.OptionalInt64Pointer(plan.ConnectionTimeout),
		ReadTimeout:                       helpers.OptionalInt64Pointer(plan.ReadTimeout),
		Username:                          authScopedField(plan.Username, plan.AuthenticationType, authTypeBasic),
		Password:                          password,
		Header:                            authScopedField(plan.Header, plan.AuthenticationType, authTypeHeader),
		HashAlgorithm:                     helpers.OptionalStringPointer(plan.HashAlgorithm),
		EnableDisplayFieldsForGroupObject: helpers.OptionalBoolPointer(plan.EnableDisplayFieldsForGroupObject),
	}

	if gid := helpers.OptionalInt64Pointer(plan.SmartGroupID); gid != nil {
		in.SmartGroupID = gid
	} else {
		none := -1
		in.SmartGroupID = &none
	}

	return in
}

// authScopedField encodes `username` or `header` for the wire. The server
// requires the field under exactly one authentication type (username under
// BASIC, header under HEADER) and refuses an empty element there — wire-probed
// 2026-09-06 on Jamf Pro 11.31.1: PUT with an empty <header> under HEADER
// answers `409 INVALID_REQUIRED`, PUT with an empty <username> under BASIC
// answers `409 Username is required`. Under every other type an empty element
// is accepted and clears the stored value (probed on the BASIC→HASH_SIGNATURE
// and →NONE transitions), which is what a config that dropped the attribute
// needs, because Classic's merge would retain an omitted element.
//
// So: when authType is the requiring type, the field is sent as configured
// (helpers.OptionalStringPointer — a null there is a plan-time error from the
// paired validator, never a silent omission); under any other known type it is
// always emitted (helpers.AlwaysEmitStringPointer); and when authType is
// unknown at build time the field is sent only if configured, since the server
// will resolve the type and clear inactive fields itself.
func authScopedField(field, authType types.String, requiringType string) *string {
	if !helpers.IsConfiguredValue(authType) || authType.ValueString() == requiringType {
		return helpers.OptionalStringPointer(field)
	}
	return helpers.AlwaysEmitStringPointer(field)
}
