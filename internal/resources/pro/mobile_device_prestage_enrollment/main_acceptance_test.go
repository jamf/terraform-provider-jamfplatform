//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment_test

import (
	"os"
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestMain deletes the ADE instance the package's tests share once they have
// all finished (see requireSharedADE).
func TestMain(m *testing.M) {
	code := m.Run()
	testhelpers.ReleaseSharedFixtures()
	os.Exit(code)
}
