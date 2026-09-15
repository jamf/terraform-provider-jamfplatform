// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// extractPolicyID returns the top-level ID as a string from a Create response.
// Classic returns the assigned ID at <policy><id> (Policy.ID); General.ID is
// the same value echoed inside the general block. We prefer the top-level
// reading first and fall back to General.
func extractPolicyID(p *proclassic.Policy) string {
	if p == nil {
		return ""
	}
	if p.ID != nil {
		return strconv.Itoa(*p.ID)
	}
	if p.General != nil && p.General.ID != nil {
		return strconv.Itoa(*p.General.ID)
	}
	return ""
}

// buildNotificationEnabled projects the schema's display_notifications bool
// into proclassic.NotificationValue carrying only the bool leg. The classic
// wire uses a single <notification>true|false</notification> element for the
// boolean; the method string travels as a sibling <notification_type>
// element on the same parent (PolicySelfService.NotificationType), modelled
// by the provider as notification_location — not as a second <notification>
// tag. Returns nil when the attribute is null/unknown.
func buildNotificationEnabled(enabled types.Bool) *proclassic.NotificationValue {
	if !helpers.IsConfiguredValue(enabled) {
		return nil
	}
	v := enabled.ValueBool()
	return &proclassic.NotificationValue{Enabled: &v}
}

// flattenNotificationEnabled extracts the bool leg from the SDK
// NotificationValue (the boolean <notification> wire element).
//
// It keeps a sticky read, so a configured caller value survives every refresh
// and server-side drift on it is never reported. The justification is that
// there is no wire value to read: Jamf Pro stores the whole <notification>
// family on a policy and omits every element of it from the GET, verified on a
// POST that set all four fields and on a later PUT (Jamf Pro 11.31.1,
// wire-probed 2026-09-06). See flattenPolicySelfService and issue #387.
func flattenNotificationEnabled(n *proclassic.NotificationValue, current types.Bool) types.Bool {
	var apiEnabled *bool
	if n != nil {
		apiEnabled = n.Enabled
	}
	return helpers.WireWhenPresentBool(apiEnabled, current)
}

// stickyIgnoringDriftInt64 is the Int64 sibling of helpers.StickyIgnoringDriftString
// and carries the same caveat: a set value is never re-read from the wire, so
// server-side drift on it is never reported. API is *int (SDK convention).
// Prefer helpers.Int64FromIntPtr unless the field is wire-probed as never
// echoed or never persisted, and name that evidence at the call site.
func stickyIgnoringDriftInt64(api *int, current types.Int64) types.Int64 {
	if helpers.IsConfiguredValue(current) {
		return current
	}
	if api == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*api))
}

// invertOptionalBoolPointer is optionalBoolPointer with the value negated.
// Used to translate a user-facing boolean attribute into its inverse on the
// wire (e.g. permanently_delete_home_directory ↔ archive_home_directory).
// Null/unknown still collapses to nil so the wire emits no element.
func invertOptionalBoolPointer(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := !value.ValueBool()
	return &v
}

// invertBoolPointerValueOrNull is the state-side inverse of
// invertOptionalBoolPointer. A nil server pointer becomes null state; a
// non-nil pointer becomes its negated TF Bool value.
func invertBoolPointerValueOrNull(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(!*b)
}

// optionalInt64ToInt projects a TF Int64 into a *int suitable for SDK
// payloads. Returns nil for null/unknown.
func optionalInt64ToInt(value types.Int64) *int {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	v := int(value.ValueInt64())
	return &v
}

// accountMaintenanceSecretsForCreate builds the WriteOnly password carrier
// for Create. There is no prior state, so every plaintext sourced from cfg
// is treated as fresh and threaded to the wire. Accounts are keyed by
// username (the wire identity used by Jamf to match Create/Reset/Delete
// actions). Null cfg values produce nil emit-pointers so the SDK omits
// the corresponding XML element.
func accountMaintenanceSecretsForCreate(cfg *PolicyResourceModel) *policyAccountMaintenanceSecrets {
	out := &policyAccountMaintenanceSecrets{accountPasswords: map[string]*string{}}
	if cfg == nil {
		return out
	}
	for _, a := range cfg.LocalAccounts {
		if a.Username.IsNull() || a.Username.IsUnknown() {
			continue
		}
		out.accountPasswords[a.Username.ValueString()] = helpers.OptionalStringPointer(a.Password)
	}
	if cfg.ManagementAccount != nil {
		out.managedPassword = helpers.OptionalStringPointer(cfg.ManagementAccount.ManagedPassword)
	}
	if cfg.EfiPassword != nil {
		out.ofPassword = helpers.OptionalStringPointer(cfg.EfiPassword.OfPassword)
	}
	return out
}

// accountMaintenanceSecretsForUpdate builds the WriteOnly password carrier
// for Update. Unlike directory_binding / disk_encryption (where the
// password is a one-shot bind credential and Jamf retains its stored
// value when the wire field is omitted), Jamf classic policy actions
// embed the password in the stored policy and clients re-execute the
// action on every policy run. Omitting `<password>` on Update would
// erase the stored value and break the next Create/Reset run — the
// server enforces this and returns HTTP 409 "Problem with reset
// account fields" when a Reset entry lacks a password.
//
// Consequence: Update must always include the cfg-sourced plaintext on
// the wire (when the user has one in their config). The `*_wo_version`
// companion still exists as the rotation trigger — WriteOnly attribute
// changes alone are invisible to Terraform's plan diff, so the user must
// bump wo_version (or change another non-WriteOnly attribute) to make
// Update fire at all. But the wire payload itself does not gate on
// wo_version — that gate would break the policy at the next client run.
//
// Accounts are matched by username (the wire identity used by Jamf for
// Create/Reset/Delete/DisableFileVault).
func accountMaintenanceSecretsForUpdate(plan, state, cfg *PolicyResourceModel) *policyAccountMaintenanceSecrets {
	_ = state // intentionally unused; see doc comment for the reasoning.
	return accountMaintenanceSecretsForCreate(cfg)
}
