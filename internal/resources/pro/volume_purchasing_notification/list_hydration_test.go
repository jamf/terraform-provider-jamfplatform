// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_VolumePurchasingNotificationResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
func TestListHydration_VolumePurchasingNotificationResourceModel(t *testing.T) {
	w := &pro.VolumePurchasingSubscription{}
	listhydration.Fill(w)
	state := &VolumePurchasingNotificationResourceModel{}
	ctx := context.Background()
	assignVolumePurchasingNotificationResourceModel(ctx, state, w)
	if got := listhydration.NilBlocks(state); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}
