// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package invitationcommon

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func bigInt(s string) *proclassic.BigInt {
	var b proclassic.BigInt
	b.SetString(s)
	return &b
}

// TestExpirationDatesEqual_MinuteBoundaryDrift is the load-bearing case: the
// server echoes a finite expiration ~1s lower than written, with a
// non-deterministic sub-second component, and the drift can cross a minute
// boundary (`23:59:00` written → `23:58:59.306` read). A naive
// truncate-to-minute string compare would treat `23:59` and `23:58` as
// different and surface a post-apply inconsistency. The time-delta tolerance
// treats them as equal.
func TestExpirationDatesEqual_MinuteBoundaryDrift(t *testing.T) {
	cases := []struct {
		name           string
		config, server string
		want           bool
	}{
		{"observed computer drift across minute boundary", "2026-12-31 23:59:00", "2026-12-31 23:58:59.306", true},
		{"observed mobile drift across minute boundary", "2026-12-31 23:59:00", "2026-12-31 23:58:59.918", true},
		{"same minute, sub-second drift", "2026-06-04 12:30:30", "2026-06-04 12:30:29.5", true},
		{"identical", "2026-06-04 12:30:00", "2026-06-04 12:30:00", true},
		{"unlimited verbatim equal", "Unlimited", "Unlimited", true},
		{"unlimited vs finite", "Unlimited", "2026-12-31 23:59:00", false},
		{"finite vs unlimited", "2026-12-31 23:59:00", "Unlimited", false},
		{"genuinely different day", "2026-12-31 23:59:00", "2026-12-30 23:59:00", false},
		{"beyond tolerance (5 min)", "2026-06-04 12:30:00", "2026-06-04 12:25:00", false},
		{"unparseable config", "not-a-date", "2026-06-04 12:30:00", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpirationDatesEqual(tc.config, tc.server); got != tc.want {
				t.Errorf("ExpirationDatesEqual(%q, %q) = %v, want %v", tc.config, tc.server, got, tc.want)
			}
		})
	}
}

func TestReconcileExpirationDate_PreservesConfigOnDrift(t *testing.T) {
	current := types.StringValue("2026-12-31 23:59:00")
	server := "2026-12-31 23:58:59.306"
	got := ReconcileExpirationDate(&server, current)
	if got.ValueString() != "2026-12-31 23:59:00" {
		t.Errorf("expected configured value preserved across drift, got %q", got.ValueString())
	}
}

func TestReconcileExpirationDate_AdoptsServerOnImport(t *testing.T) {
	// Null current (import) → adopt server value verbatim.
	server := "2026-12-31 23:58:59.306"
	got := ReconcileExpirationDate(&server, types.StringNull())
	if got.ValueString() != server {
		t.Errorf("expected server value adopted on import, got %q", got.ValueString())
	}
}

func TestReconcileExpirationDate_AdoptsServerOnRealChange(t *testing.T) {
	current := types.StringValue("2026-12-31 23:59:00")
	server := "2025-01-01 00:00:00"
	got := ReconcileExpirationDate(&server, current)
	if got.ValueString() != server {
		t.Errorf("expected server value adopted on real difference, got %q", got.ValueString())
	}
}

func TestReconcileExpirationDate_KeepsCurrentWhenServerEmpty(t *testing.T) {
	current := types.StringValue("2026-12-31 23:59:00")
	got := ReconcileExpirationDate(nil, current)
	if got.ValueString() != "2026-12-31 23:59:00" {
		t.Errorf("expected current preserved when server nil, got %q", got.ValueString())
	}
}

func TestBigIntStringOrNull(t *testing.T) {
	if got := BigIntStringOrNull(nil); !got.IsNull() {
		t.Errorf("nil BigInt must be null")
	}
	if got := BigIntStringOrNull(bigInt("308000000000000000000000000000000000001")); got.ValueString() != "308000000000000000000000000000000000001" {
		t.Errorf("big invitation code lost precision: %q", got.ValueString())
	}
}

func TestInt64ValueFromIntPtr(t *testing.T) {
	if got := Int64ValueFromIntPtr(nil); !got.IsNull() {
		t.Errorf("nil int must be null")
	}
	if got := Int64ValueFromIntPtr(new(42)); got.ValueInt64() != 42 {
		t.Errorf("expected 42, got %d", got.ValueInt64())
	}
}
