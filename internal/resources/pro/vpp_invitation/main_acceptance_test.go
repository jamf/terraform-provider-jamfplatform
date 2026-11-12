//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package vpp_invitation_test

import (
	"os"
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestMain deletes the Volume Purchasing location the package's tests share once
// they have all finished (see requireSharedVPP).
func TestMain(m *testing.M) {
	code := m.Run()
	testhelpers.ReleaseSharedFixtures()
	os.Exit(code)
}
