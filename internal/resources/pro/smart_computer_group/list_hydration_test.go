// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_SmartComputerGroupResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_SmartComputerGroupResourceModel(t *testing.T) {
	w := &pro.SmartComputerGroupV3{}
	listhydration.Fill(w)
	state := &SmartComputerGroupResourceModel{}

	assignSmartComputerGroupResourceModel(state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
