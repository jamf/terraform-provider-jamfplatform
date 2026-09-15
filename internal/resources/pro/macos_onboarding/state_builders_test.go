// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// unsortedConfig returns a configuration whose items are in arbitrary (non-priority)
// wire order — mirroring the live GET, where array order != priority order.
func unsortedConfig() *pro.OnboardingConfiguration {
	return &pro.OnboardingConfiguration{
		Enabled: true,
		ID:      new("1"),
		OnboardingItems: []pro.OnboardingItem{
			{EntityID: "87", SelfServiceEntityType: entityTypeMacApp, Priority: 3, ID: new("26"), EntityName: new("Numbers"), ScopeDescription: new("All Managed"), SiteDescription: new("None")},
			{EntityID: "4", SelfServiceEntityType: entityTypePolicy, Priority: 1, ID: new("21"), EntityName: new("Submit inventory")},
			{EntityID: "11", SelfServiceEntityType: entityTypeConfigProfile, Priority: 2, ID: new("24"), EntityName: new("Energy Saver")},
		},
	}
}

// TestFlattenOnboardingItems_SortsByPriority verifies the wire's arbitrary array order
// is normalised to priority-ascending order so the list index is canonical.
func TestFlattenOnboardingItems_SortsByPriority(t *testing.T) {
	got := flattenOnboardingItems(unsortedConfig().OnboardingItems)
	if len(got) != 3 {
		t.Fatalf("got %d items, want 3", len(got))
	}
	wantEntityByIndex := []string{"4", "11", "87"} // priority 1, 2, 3
	for i, want := range wantEntityByIndex {
		if got[i].EntityID.ValueString() != want {
			t.Errorf("index %d entity_id = %q, want %q (priority-sorted)", i, got[i].EntityID.ValueString(), want)
		}
		if got[i].Priority.ValueInt64() != int64(i+1) {
			t.Errorf("index %d priority = %d, want %d", i, got[i].Priority.ValueInt64(), i+1)
		}
	}
	// Echoes carried through.
	if got[0].EntityName.ValueString() != "Submit inventory" {
		t.Errorf("index 0 entity_name = %q, want %q", got[0].EntityName.ValueString(), "Submit inventory")
	}
	// A nil echo becomes null (not "").
	if !got[0].ScopeDescription.IsNull() {
		t.Errorf("index 0 scope_description should be null when the wire omits it, got %q", got[0].ScopeDescription.ValueString())
	}
}

// TestFlattenOnboardingItems_Empty verifies nil/empty input yields nil.
func TestFlattenOnboardingItems_Empty(t *testing.T) {
	if got := flattenOnboardingItems(nil); got != nil {
		t.Errorf("flatten(nil) = %v, want nil", got)
	}
	if got := flattenOnboardingItems([]pro.OnboardingItem{}); got != nil {
		t.Errorf("flatten(empty) = %v, want nil", got)
	}
}

// TestAssignOnboardingResourceModel_RoundTrip verifies enabled + a known, priority-sorted
// list are assigned, and an empty config yields a known empty list (not null).
func TestAssignOnboardingResourceModel_RoundTrip(t *testing.T) {
	ctx := context.Background()

	var state OnboardingResourceModel
	if diags := assignOnboardingResourceModel(ctx, &state, unsortedConfig()); diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}
	if !state.Enabled.ValueBool() {
		t.Errorf("enabled = false, want true")
	}
	if state.OnboardingItems.IsNull() || state.OnboardingItems.IsUnknown() {
		t.Fatalf("onboarding_items must be a known list")
	}
	if len(state.OnboardingItems.Elements()) != 3 {
		t.Errorf("got %d elements, want 3", len(state.OnboardingItems.Elements()))
	}

	var empty OnboardingResourceModel
	if diags := assignOnboardingResourceModel(ctx, &empty, &pro.OnboardingConfiguration{Enabled: false}); diags.HasError() {
		t.Fatalf("assign(empty) diags: %v", diags)
	}
	if empty.OnboardingItems.IsNull() {
		t.Errorf("empty config must yield a known empty list, got null")
	}
	if len(empty.OnboardingItems.Elements()) != 0 {
		t.Errorf("empty config got %d elements, want 0", len(empty.OnboardingItems.Elements()))
	}
}

// TestAssign_DoesNotClobberID verifies the assigner leaves a pre-existing state.ID
// untouched (the CRUD handler is responsible for stamping helpers.SingletonID).
func TestAssign_DoesNotClobberID(t *testing.T) {
	ctx := context.Background()
	state := OnboardingResourceModel{ID: types.StringValue("preexisting")}
	if diags := assignOnboardingResourceModel(ctx, &state, unsortedConfig()); diags.HasError() {
		t.Fatalf("assign diags: %v", diags)
	}
	if state.ID.ValueString() != "preexisting" {
		t.Errorf("assigner clobbered state.ID = %q, want preexisting", state.ID.ValueString())
	}
}

// TestSingletonIDConstant pins the shared singleton id constant.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID = %q, want \"singleton\"", helpers.SingletonID)
	}
}

// TestMapOnboardingEligibleItems verifies eligible items map directly and an empty
// input yields a non-nil empty slice.
func TestMapOnboardingEligibleItems(t *testing.T) {
	got := mapOnboardingEligibleItems([]pro.OnboardingEligibleItem{
		{ID: "84", Name: "Keynote", ScopeDescription: "All Laptops", SiteDescription: "None"},
	})
	if len(got) != 1 || got[0].ID.ValueString() != "84" || got[0].Name.ValueString() != "Keynote" {
		t.Errorf("unexpected mapping: %+v", got)
	}
	if empty := mapOnboardingEligibleItems(nil); empty == nil {
		t.Errorf("map(nil) = nil, want non-nil empty slice")
	}
}
