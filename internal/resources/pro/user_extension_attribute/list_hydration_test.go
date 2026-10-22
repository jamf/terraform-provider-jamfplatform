// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_UserExtensionAttributeResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_UserExtensionAttributeResourceModel(t *testing.T) {
	w := &proclassic.UserExtensionAttribute{}
	listhydration.Fill(w)
	state := &UserExtensionAttributeResourceModel{}
	ctx := context.Background()
	assignUserExtensionAttributeResourceModel(ctx, state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
