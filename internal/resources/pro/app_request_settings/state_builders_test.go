// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_settings

import (
	"context"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignAppRequestSettingsResourceModel(t *testing.T) {
	ctx := context.Background()

	t.Run("full echo", func(t *testing.T) {
		var state AppRequestSettingsResourceModel
		emails := []string{"a@example.com", "b@example.com"}
		diags := assignAppRequestSettingsResourceModel(ctx, &state, &pro.AppRequestSettings{
			IsEnabled:            new(true),
			AppStoreLocale:       new("US"),
			ApproverEmails:       &emails,
			RequesterUserGroupID: new(3),
		})
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if !state.Enabled.ValueBool() {
			t.Errorf("enabled = %v", state.Enabled)
		}
		if state.AppStoreLocale.ValueString() != "US" {
			t.Errorf("locale = %q", state.AppStoreLocale.ValueString())
		}
		if state.RequesterUserGroupID.ValueInt64() != 3 {
			t.Errorf("group = %d", state.RequesterUserGroupID.ValueInt64())
		}
		if l := len(state.ApproverEmails.Elements()); l != 2 {
			t.Errorf("emails count = %d", l)
		}
	})

	t.Run("no approvers reported", func(t *testing.T) {
		var state AppRequestSettingsResourceModel
		diags := assignAppRequestSettingsResourceModel(ctx, &state, &pro.AppRequestSettings{
			IsEnabled: new(false),
		})
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if state.ApproverEmails.IsNull() || len(state.ApproverEmails.Elements()) != 0 {
			t.Errorf("an omitted approver list must read as an empty (non-null) set, got %v", state.ApproverEmails)
		}
	})

	t.Run("baseline unconfigured", func(t *testing.T) {
		var state AppRequestSettingsResourceModel
		empty := []string{}
		diags := assignAppRequestSettingsResourceModel(ctx, &state, &pro.AppRequestSettings{
			IsEnabled:      new(false),
			AppStoreLocale: new("deviceLocale"),
			ApproverEmails: &empty,
		})
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if state.Enabled.ValueBool() {
			t.Errorf("enabled must be false")
		}
		if !state.RequesterUserGroupID.IsNull() {
			t.Errorf("nil group must map to null, got %v", state.RequesterUserGroupID)
		}
		if state.ApproverEmails.IsNull() || len(state.ApproverEmails.Elements()) != 0 {
			t.Errorf("empty emails must be an empty (non-null) set, got %v", state.ApproverEmails)
		}
	})
}
