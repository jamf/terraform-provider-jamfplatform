// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_volume_purchasing_content_search

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_AdvancedVolumePurchasingContentSearchResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_AdvancedVolumePurchasingContentSearchResourceModel(t *testing.T) {
	w := &pro.AdvancedUserContentSearch{}
	listhydration.Fill(w)
	state := &AdvancedVolumePurchasingContentSearchResourceModel{}
	ctx := context.Background()
	assignAdvancedVolumePurchasingContentSearchResourceModel(ctx, state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
