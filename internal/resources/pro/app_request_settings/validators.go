// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_settings

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// deviceLocaleSentinel is the literal app_store_locale value that means "follow each
// device's locale". It is not an entry in the country-code list, so it is allowed
// unconditionally.
const deviceLocaleSentinel = "deviceLocale"

// canonicalLocale returns the form Jamf Pro stores for a locale value: the exact
// deviceLocale sentinel, or an upper-cased country code (wire-probed 2026-06-13 — the
// server stores/echoes "us" and "Us" as "US"). app_store_locale is Optional+Computed, and
// Terraform forbids changing a user-supplied value during plan, so a plan modifier cannot
// canonicalise it — instead the validator requires the user to supply the canonical form.
func canonicalLocale(v string) string {
	if strings.EqualFold(v, deviceLocaleSentinel) {
		return deviceLocaleSentinel
	}
	return strings.ToUpper(v)
}

// appStoreLocaleLister is the subset of *pro.Client the locale preflight needs. Declaring
// it as an interface keeps the validator unit-testable without a live client.
type appStoreLocaleLister interface {
	ListAppStoreCountryCodesV1(ctx context.Context) (*pro.CountryCodes, error)
}

// validateAppStoreLocale is a plan-time preflight for app_store_locale. It enforces two
// things:
//
//  1. Canonical form. Jamf Pro stores a country code upper-cased and the sentinel as the
//     exact "deviceLocale" spelling. Because the attribute is Optional+Computed, Terraform
//     rejects any plan that changes a user-supplied value, so the provider cannot
//     canonicalise it automatically — a non-canonical value (e.g. "us") would otherwise
//     fail apply with "inconsistent result after apply". This surfaces it at plan time with
//     the exact value to use.
//  2. Membership. Jamf Pro rejects an unknown code at apply time with a 400 ("Invalid
//     country code provided"). The valid set varies by tenant/version and is fetched live
//     from the same list the server validates against rather than baked into a static
//     enum.
//
// Behaviour:
//   - null/unknown value: no-op (deferred).
//   - a non-canonical value: an error naming the canonical form.
//   - the deviceLocale sentinel: allowed (no membership check).
//   - nil lister: canonical-form check still runs; membership is skipped.
//   - a code absent from the live list: an error diagnostic at attrPath.
//   - a fetch error: a WARNING (not an error). The membership preflight is best-effort and
//     must not block plans when the endpoint is unreachable; the server still enforces it.
func validateAppStoreLocale(ctx context.Context, lister appStoreLocaleLister, value types.String, attrPath path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return diags
	}

	v := value.ValueString()
	if canonical := canonicalLocale(v); v != canonical {
		diags.AddAttributeError(
			attrPath,
			"App Store locale must be in canonical form",
			fmt.Sprintf("Jamf Pro stores this value as %q. Set app_store_locale to %q (country codes are upper-case; the sentinel is exactly \"deviceLocale\") to avoid a post-apply inconsistency.", canonical, canonical),
		)
		return diags
	}

	if v == deviceLocaleSentinel {
		return diags
	}

	if lister == nil {
		return diags
	}

	live, err := lister.ListAppStoreCountryCodesV1(ctx)
	if err != nil {
		diags.AddAttributeWarning(
			attrPath,
			"Could not verify App Store country code",
			fmt.Sprintf("Skipping plan-time validation of app_store_locale: %s. The Jamf Pro server will still enforce a valid country code on apply.", err),
		)
		return diags
	}

	if live != nil {
		for _, c := range live.CountryCodes {
			if strings.EqualFold(c.Code, v) {
				return diags
			}
		}
	}

	diags.AddAttributeError(
		attrPath,
		"Invalid App Store country code",
		fmt.Sprintf("%q is not a valid App Store country or region for this tenant. Use the literal \"deviceLocale\" or an ISO 3166-1 alpha-2 country code. Jamf Pro would reject this on apply with \"Invalid country code provided\". The full list is available from the jamfplatform_pro_app_store_country_codes data source.", v),
	)
	return diags
}

// validateEnabledRequiresRequesterGroup enforces, at plan time, the dependency Jamf Pro
// enforces on the wire: enabling App Requests requires a (static) requester user group.
// Wire-probed 2026-06-13 — enabling with a null/unknown/smart group returns HTTP 400
// ("Requester static group id was not found").
//
// It runs in ModifyPlan against the resolved plan (after UseStateForUnknown), so a group
// preserved from prior state is a known value and passes — only a known enabled=true with a
// known-null requester group errors. The brand-new "enabled but never set a group" case
// leaves the group Unknown at plan time and is deferred to the server 400. Static-vs-smart
// and existence are left to the server (no plan-time preflight).
func validateEnabledRequiresRequesterGroup(enabled types.Bool, requesterGroupID types.Int64, attrPath path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	if enabled.IsNull() || enabled.IsUnknown() || !enabled.ValueBool() {
		return diags
	}
	if requesterGroupID.IsUnknown() {
		return diags
	}
	if requesterGroupID.IsNull() {
		diags.AddAttributeError(
			attrPath,
			"Requester user group required when App Requests are enabled",
			"requester_user_group_id must reference a static Jamf Pro user group when enabled is true. Jamf Pro rejects an enabled configuration without a requester group.",
		)
	}
	return diags
}

// validateRequesterRequiresEnabled is the converse cross-field rule: a requester user group
// is meaningless when App Requests are disabled, and the provider clears it on a disabled
// write, so setting one in config while enabled is false would produce a post-apply
// inconsistency. Surface it as a clear plan error instead. Operates on the *config* value
// (an explicit user choice), not the UseStateForUnknown-resolved plan value, so a group
// carried from prior state during a disable transition does not falsely trip it.
func validateRequesterRequiresEnabled(enabled types.Bool, configRequesterGroupID types.Int64, attrPath path.Path) diag.Diagnostics {
	var diags diag.Diagnostics
	if configRequesterGroupID.IsNull() || configRequesterGroupID.IsUnknown() {
		return diags
	}
	if enabled.IsNull() || enabled.IsUnknown() || enabled.ValueBool() {
		return diags
	}
	diags.AddAttributeError(
		attrPath,
		"Requester user group requires App Requests to be enabled",
		"requester_user_group_id can only be set when enabled is true. A disabled App Request has no requester group.",
	)
	return diags
}
