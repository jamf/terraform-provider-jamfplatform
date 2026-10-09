// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"
	"time"
)

// The cap is the only bound on a stalled token exchange once the SDK stops enforcing its own, so a
// change to it should be a reviewable test diff rather than a silent one.
func TestCredentialValidationTimeout_IsSixtySeconds(t *testing.T) {
	if credentialValidationTimeout != 60*time.Second {
		t.Fatalf("credentialValidationTimeout = %s, want 60s", credentialValidationTimeout)
	}
}
