// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_DockItemResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_DockItemResourceModel(t *testing.T) {
	w := &proclassic.DockItem{}
	listhydration.Fill(w)
	state := &DockItemResourceModel{}

	assignDockItemResourceModel(state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
