// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestAssignWebhookResourceModel_Mapping verifies the server response maps into
// the resource model, including the sentinel normalisations.
func TestAssignWebhookResourceModel_Mapping(t *testing.T) {
	ctx := context.Background()
	w := &proclassic.Webhook{
		ID:                 new(66),
		Name:               new("hook"),
		Enabled:            new(true),
		URL:                new("https://e.com/x"),
		AuthenticationType: new(authTypeBasic),
		ConnectionTimeout:  new(5),
		ReadTimeout:        new(2),
		ContentType:        new("text/xml"),
		Event:              new("ComputerCheckIn"),
		Username:           new("bob"),
		Password:           new("********************"), // redaction sentinel
		Header:             new(""),
		HashAlgorithm:      new("SHA256"),
	}
	var state WebhookResourceModel
	// Seed a wo_version to confirm it is preserved (server has no such field).
	state.PasswordWoVersion = types.Int64Value(3)

	if diags := assignWebhookResourceModel(ctx, &state, w); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	if state.ID.ValueString() != "66" {
		t.Errorf("id = %q, want 66", state.ID.ValueString())
	}
	if state.Event.ValueString() != "ComputerCheckIn" {
		t.Errorf("event not mapped")
	}
	if state.Username.ValueString() != "bob" {
		t.Errorf("username not mapped")
	}
	// password is WriteOnly — must never be written into state, even though the
	// server returned the redaction sentinel.
	if !state.Password.IsNull() {
		t.Errorf("password must stay null in state, got %q", state.Password.ValueString())
	}
	// header was "" → null.
	if !state.Header.IsNull() {
		t.Errorf("empty header must map to null, got %q", state.Header.ValueString())
	}
	// wo_version preserved.
	if state.PasswordWoVersion.ValueInt64() != 3 {
		t.Errorf("password_wo_version must be preserved, got %v", state.PasswordWoVersion)
	}
	if state.ConnectionTimeout.ValueInt64() != 5 || state.ReadTimeout.ValueInt64() != 2 {
		t.Errorf("timeouts not mapped")
	}
}

// TestSmartGroupIDToState_Normalisation covers the -1 / absent → null mapping.
func TestSmartGroupIDToState_Normalisation(t *testing.T) {
	if got := smartGroupIDToState(nil); !got.IsNull() {
		t.Errorf("nil smart_group_id must be null")
	}
	if got := smartGroupIDToState(new(-1)); !got.IsNull() {
		t.Errorf("-1 sentinel must map to null")
	}
	if got := smartGroupIDToState(new(29)); got.IsNull() || got.ValueInt64() != 29 {
		t.Errorf("real smart_group_id must map through, got %v", got)
	}
}

// TestDisplayFieldNames extracts names and ignores nil entries.
func TestDisplayFieldNames(t *testing.T) {
	df := &proclassic.WebhookDisplayFields{
		DisplayField: &[]proclassic.WebhookDisplayFieldsDisplayFieldItem{
			{Name: new("Serial Number")},
			{Name: nil},
			{Name: new("Computer Name")},
		},
	}
	got := displayFieldNames(df)
	if len(got) != 2 || got[0] != "Serial Number" || got[1] != "Computer Name" {
		t.Errorf("unexpected display field names: %v", got)
	}
	if displayFieldNames(nil) != nil {
		t.Errorf("nil display_fields must yield nil")
	}
}

// TestAssignWebhookResourceModel_DisplayFieldsNeverNull confirms the
// Computed-only set is never null after a read.
func TestAssignWebhookResourceModel_DisplayFieldsNeverNull(t *testing.T) {
	ctx := context.Background()
	w := &proclassic.Webhook{ID: new(1), Name: new("x")}
	var state WebhookResourceModel
	if diags := assignWebhookResourceModel(ctx, &state, w); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if state.DisplayFields.IsNull() {
		t.Errorf("display_fields must be a known (empty) set, not null")
	}
}
