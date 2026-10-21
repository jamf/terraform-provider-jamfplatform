// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package automated_device_enrollment

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_AutomatedDeviceEnrollmentResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_AutomatedDeviceEnrollmentResourceModel(t *testing.T) {
	w := &pro.DeviceEnrollmentInstance{}
	listhydration.Fill(w)
	state := &AutomatedDeviceEnrollmentResourceModel{}

	assignAutomatedDeviceEnrollmentResourceModel(state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
