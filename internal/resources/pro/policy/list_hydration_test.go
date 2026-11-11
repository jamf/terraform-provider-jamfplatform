// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
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

// TestListHydration_RepeatedEntriesSetCleanly is the exact path issue #568
// broke: config generation hydrates a policy that lists the same script,
// package, printer, dock item and directory binding more than once, and the
// hydrated model must set into the resource schema without a "Duplicate Set
// Element" diagnostic, keeping every repeat.
func TestListHydration_RepeatedEntriesSetCleanly(t *testing.T) {
	state := &PolicyResourceModel{Timeouts: helpers.NewResourceTimeoutsNullValue(policyTimeoutAttributeTypes)}
	if diags := assignPolicyResourceModel(context.Background(), state, repeatedEntriesPolicy(), true); diags.HasError() {
		t.Fatalf("assign: %v", diags)
	}
	if state.Scripts != nil || state.Printers != nil || state.DockItems != nil {
		t.Error("config generation must populate the flat attributes, not the deprecated wrappers")
	}
	assertEveryRepeatKept(t, state.ScriptAssignments, state.Packages.Packages, state.PrinterAssignments, state.DockItemAssignments, state.DirectoryBindings)
	setPolicyState(t, state)
}

var deprecatedBlocksLeftNullByConfigGeneration = []string{
	"Scripts",
	"Printers",
	"DockItems",
	"General.DateTimeLimitations",
	"General.NetworkLimitations",
	"General.OverrideDefaultSettings",
}
