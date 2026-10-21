// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_ComputerExtensionAttributeResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_ComputerExtensionAttributeResourceModel(t *testing.T) {
	w := &pro.ComputerExtensionAttributes{}
	listhydration.Fill(w)
	w.InputType = inputTypePopup
	state := &ComputerExtensionAttributeResourceModel{}
	ctx := context.Background()
	assignComputerExtensionAttributeResourceModel(ctx, state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
