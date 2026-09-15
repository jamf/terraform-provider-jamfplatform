// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// RequireMinJamfProVersion skips the test unless the tenant's Jamf Pro version is
// at least min (e.g. "11.29"). Use it for features the server rejects on older
// releases — a hard skip keeps mixed-version CI green instead of failing on a
// tenant that legitimately predates the feature.
func RequireMinJamfProVersion(t *testing.T, min string) {
	t.Helper()
	c := pro.New(NewAcceptanceClient(t))
	v, err := c.GetJamfProVersionV1(context.Background())
	if err != nil {
		t.Fatalf("fetching Jamf Pro version for the >= %s gate: %v", min, err)
	}
	if !helpers.AtLeastJamfProVersion(v.Version, min) {
		t.Skipf("tenant Jamf Pro %s is below %s; skipping (feature unavailable)", v.Version, min)
	}
}
