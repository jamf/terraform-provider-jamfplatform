// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignResourceModel_EmptyWireFlattensToEmptySet verifies that empty wire
// arrays become empty (non-null) sets — null and [] both mean "none" under
// full-replace, so state must mirror the server echo, never collapse to null.
func TestAssignResourceModel_EmptyWireFlattensToEmptySet(t *testing.T) {
	d := &pro.VolumePurchasingSubscription{
		ID:                 "43",
		Name:               "Test",
		Enabled:            true,
		Triggers:           []string{},
		LocationIds:        []string{},
		InternalRecipients: []pro.InternalRecipient{},
		ExternalRecipients: []pro.ExternalRecipient{},
		SiteID:             "-1",
	}
	var state VolumePurchasingNotificationResourceModel
	if diags := assignVolumePurchasingNotificationResourceModel(context.Background(), &state, d); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "43" || state.Name.ValueString() != "Test" {
		t.Errorf("scalar mismatch: id=%q name=%q", state.ID.ValueString(), state.Name.ValueString())
	}
	if !state.Enabled.ValueBool() {
		t.Error("enabled should be true")
	}
	for name, set := range map[string]interface {
		IsNull() bool
		IsUnknown() bool
	}{
		"triggers":            state.Triggers,
		"location_ids":        state.LocationIDs,
		"internal_recipients": state.InternalRecipients,
		"external_recipients": state.ExternalRecipients,
	} {
		if set.IsNull() {
			t.Errorf("%s must be an empty set, not null", name)
		}
	}
	if state.Triggers.IsNull() || len(state.Triggers.Elements()) != 0 {
		t.Errorf("triggers must be empty set, got %v", state.Triggers)
	}
	if state.SiteID.ValueString() != "-1" {
		t.Errorf("site_id mismatch: %q", state.SiteID.ValueString())
	}
}

// TestAssignResourceModel_PopulatedDropsFrequency verifies internal_recipients
// keeps only the account id (the always-DAILY frequency is dropped) and external
// recipients round-trip both fields.
func TestAssignResourceModel_PopulatedDropsFrequency(t *testing.T) {
	freq := frequencyDaily
	d := &pro.VolumePurchasingSubscription{
		ID:          "43",
		Name:        "Test",
		Enabled:     false,
		Triggers:    []string{triggerRemovedFromAppStore, triggerNoMoreLicenses},
		LocationIds: []string{"3"},
		InternalRecipients: []pro.InternalRecipient{
			{AccountID: "66", Frequency: &freq},
			{AccountID: "67", Frequency: &freq},
		},
		ExternalRecipients: []pro.ExternalRecipient{{Email: "x@example.com", Name: "X Person"}},
		SiteID:             "12",
	}
	var state VolumePurchasingNotificationResourceModel
	if diags := assignVolumePurchasingNotificationResourceModel(context.Background(), &state, d); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if len(state.Triggers.Elements()) != 2 {
		t.Errorf("triggers count: %d", len(state.Triggers.Elements()))
	}
	var accountIDs []string
	if diags := state.InternalRecipients.ElementsAs(context.Background(), &accountIDs, false); diags.HasError() {
		t.Fatalf("internal recipients decode: %v", diags)
	}
	if len(accountIDs) != 2 {
		t.Fatalf("internal recipients count: %d", len(accountIDs))
	}
	if len(state.ExternalRecipients.Elements()) != 1 {
		t.Errorf("external recipients count: %d", len(state.ExternalRecipients.Elements()))
	}
}

// TestAssignDataSourceModel_Lists verifies the data source projects collections as
// (read-only) Lists.
func TestAssignDataSourceModel_Lists(t *testing.T) {
	freq := frequencyDaily
	d := &pro.VolumePurchasingSubscription{
		ID:                 "43",
		Name:               "Test",
		Enabled:            true,
		Triggers:           []string{triggerNoMoreLicenses},
		LocationIds:        []string{"3"},
		InternalRecipients: []pro.InternalRecipient{{AccountID: "66", Frequency: &freq}},
		ExternalRecipients: []pro.ExternalRecipient{{Email: "x@example.com", Name: "X Person"}},
		SiteID:             "-1",
	}
	var state VolumePurchasingNotificationDataSourceModel
	if diags := assignVolumePurchasingNotificationDataSourceModel(context.Background(), &state, d); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if len(state.Triggers.Elements()) != 1 || len(state.LocationIDs.Elements()) != 1 {
		t.Errorf("list counts: triggers=%d locations=%d", len(state.Triggers.Elements()), len(state.LocationIDs.Elements()))
	}
	if len(state.InternalRecipients.Elements()) != 1 || len(state.ExternalRecipients.Elements()) != 1 {
		t.Errorf("recipient list counts: internal=%d external=%d", len(state.InternalRecipients.Elements()), len(state.ExternalRecipients.Elements()))
	}
}
