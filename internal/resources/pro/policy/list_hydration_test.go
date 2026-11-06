// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/listhydration"
)

// TestListHydration_PolicyResourceModel guards the list resource's config-generation path
// (#520): every block and collection the wire carries must come back populated
// from an empty model, or the generated configuration silently drops it.
//
// The deprecated blocks stay null on purpose: config generation emits their
// flat replacements, so a generated configuration never carries a deprecated
// attribute. The replacements are not in the allowed list, which is what
// proves they hydrate. See deprecations.go.
func TestListHydration_PolicyResourceModel(t *testing.T) {
	w := &proclassic.Policy{}
	listhydration.Fill(w)
	state := &PolicyResourceModel{}
	assignPolicyResourceModel(context.Background(), state, w, true)
	if got := listhydration.NilBlocks(state, deprecatedBlocksLeftNullByConfigGeneration...); len(got) != 0 {
		t.Errorf("list path left blocks null: %v", got)
	}
}

var deprecatedBlocksLeftNullByConfigGeneration = []string{
	"Scripts",
	"Printers",
	"DockItems",
	"General.DateTimeLimitations",
	"General.NetworkLimitations",
	"General.OverrideDefaultSettings",
}
