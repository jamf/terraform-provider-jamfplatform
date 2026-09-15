// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package invitationcommon holds the read/state-builder logic shared by the
// Jamf ProClassic enrollment-invitation resources (jamfplatform_pro_computer_invitation
// and jamfplatform_pro_mobile_device_invitation).
//
// Both endpoints are create + delete only (no update route on the wire) and
// share an identical expiration_date triple: a user-authored wall-clock string
// that the server echoes back ~1 second lower with a non-deterministic
// sub-second component, plus a server-derived `_epoch` (BigInt) and `_utc`. The
// drift-tolerant reconcile, BigInt→string conversion, and nil-safe int
// conversion are byte-identical between the two resources, so they live here
// rather than being duplicated.
package invitationcommon

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// Unlimited is the sentinel accepted by Jamf Pro for an invitation that never
// expires. It round-trips verbatim (no drift) and is compared literally; finite
// dates drift by ~1s with non-deterministic sub-seconds and are reconciled at a
// parsed time-delta tolerance instead.
const Unlimited = "Unlimited"

// Layout is the wire format Jamf Pro requires for a finite expiration_date on
// write: `yyyy-MM-dd HH:mm:ss`.
const Layout = "2006-01-02 15:04:05"

// expirationDriftTolerance bounds how far a server-echoed finite expiration may
// differ from the user's configured value before the provider treats it as a
// genuine change rather than the known ~1s server-side drift. Jamf Pro echoes a
// finite expiration_date back ~1 second lower than written, with a
// non-deterministic sub-second component (observed `23:59:00` written →
// `23:58:59.306` / `23:58:59.918` read). A naive minute-string truncation
// fails when the drift crosses a minute boundary (`23:59` vs `23:58`), so we
// compare parsed timestamps with a small wall-clock tolerance instead. The
// field is RequiresReplace, so we only ever compare one logical value against
// its own echo — a generous tolerance cannot mask a real change.
const expirationDriftTolerance = 90 * time.Second

// ExpirationDatesEqual reports whether a server-echoed expiration_date should be
// considered equal to the user's configured value. `Unlimited` compares
// verbatim. Two finite values are equal when both parse and their wall-clock
// difference is within expirationDriftTolerance. Anything that does not parse
// (or a mismatch between Unlimited and a finite value) is not equal.
func ExpirationDatesEqual(config, server string) bool {
	if config == Unlimited || server == Unlimited {
		return config == server
	}
	ct, cerr := parseExpiration(config)
	st, serr := parseExpiration(server)
	if cerr != nil || serr != nil {
		return false
	}
	d := ct.Sub(st)
	if d < 0 {
		d = -d
	}
	return d <= expirationDriftTolerance
}

// parseExpiration parses a Jamf Pro expiration_date string. It accepts the
// canonical `yyyy-MM-dd HH:mm:ss` write format as well as the drifted read form
// that carries fractional seconds (`yyyy-MM-dd HH:mm:ss.SSS`). Parsed in UTC;
// only the delta between two values matters, not the absolute zone.
func parseExpiration(s string) (time.Time, error) {
	if t, err := time.Parse(Layout, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05.999999999", s)
}

// ReconcileExpirationDate returns the value to persist for expiration_date.
// When the server echo is within drift tolerance of the user's existing
// configured value, the configured value is preserved (so an immutable,
// RequiresReplace, user-authored string does not surface a post-apply
// inconsistency). Otherwise the server value is adopted (covers import, drift
// beyond tolerance, and a server-defaulted value when the user omitted the
// field).
func ReconcileExpirationDate(server *string, current types.String) types.String {
	if server == nil || *server == "" {
		// User omitted expiration_date and the server returned nothing useful;
		// keep whatever is in state (null on create, prior value on refresh).
		return current
	}
	if !current.IsNull() && !current.IsUnknown() && ExpirationDatesEqual(current.ValueString(), *server) {
		return current
	}
	return types.StringValue(*server)
}

// BigIntStringOrNull converts a nil-safe *BigInt to a TF String, mapping nil to
// null. The SDK decodes the `Unlimited` sentinel in numeric fields to 0, so a
// "0" here for expiration_date_epoch corresponds to an unlimited expiration.
func BigIntStringOrNull(b *proclassic.BigInt) types.String {
	if b == nil {
		return types.StringNull()
	}
	return types.StringValue(b.String())
}

// Int64ValueFromIntPtr converts a nil-safe *int into a TF Int64, mapping nil to
// null.
func Int64ValueFromIntPtr(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}
