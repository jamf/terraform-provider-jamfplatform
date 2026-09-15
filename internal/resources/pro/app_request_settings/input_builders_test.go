// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_settings

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func emailSet(t *testing.T, vals ...string) types.Set {
	t.Helper()
	set, diags := types.SetValueFrom(context.Background(), types.StringType, vals)
	if diags.HasError() {
		t.Fatalf("building email set: %v", diags)
	}
	return set
}

func TestBuildAppRequestSettingsInput(t *testing.T) {
	ctx := context.Background()

	t.Run("known plan, nil merge base (update)", func(t *testing.T) {
		in, diags := buildAppRequestSettingsInput(ctx, AppRequestSettingsResourceModel{
			Enabled:              types.BoolValue(true),
			AppStoreLocale:       types.StringValue("US"),
			ApproverEmails:       emailSet(t, "a@example.com"),
			RequesterUserGroupID: types.Int64Value(3),
		}, nil)
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if in.IsEnabled == nil || !*in.IsEnabled {
			t.Errorf("enabled = %v", in.IsEnabled)
		}
		if in.AppStoreLocale == nil || *in.AppStoreLocale != "US" {
			t.Errorf("locale = %v", in.AppStoreLocale)
		}
		if in.RequesterUserGroupID == nil || *in.RequesterUserGroupID != 3 {
			t.Errorf("group = %v", in.RequesterUserGroupID)
		}
		if in.ApproverEmails == nil || len(*in.ApproverEmails) != 1 {
			t.Errorf("emails = %v", in.ApproverEmails)
		}
	})

	t.Run("omitted fields fall back to merge base (create adopt)", func(t *testing.T) {
		current := &pro.AppRequestSettings{
			IsEnabled:            new(true),
			AppStoreLocale:       new("GB"),
			RequesterUserGroupID: new(7),
		}
		in, diags := buildAppRequestSettingsInput(ctx, AppRequestSettingsResourceModel{
			Enabled:              types.BoolNull(),
			AppStoreLocale:       types.StringNull(),
			ApproverEmails:       emailSet(t, "b@example.com"),
			RequesterUserGroupID: types.Int64Null(),
		}, current)
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if in.IsEnabled == nil || !*in.IsEnabled {
			t.Errorf("enabled must come from merge base, got %v", in.IsEnabled)
		}
		if in.AppStoreLocale == nil || *in.AppStoreLocale != "GB" {
			t.Errorf("locale must come from merge base, got %v", in.AppStoreLocale)
		}
		if in.RequesterUserGroupID == nil || *in.RequesterUserGroupID != 7 {
			t.Errorf("group must come from merge base, got %v", in.RequesterUserGroupID)
		}
	})

	t.Run("disabled clears requester group even when set in plan", func(t *testing.T) {
		// A disabled write must never carry a (possibly stale/dangling) group id.
		in, diags := buildAppRequestSettingsInput(ctx, AppRequestSettingsResourceModel{
			Enabled:              types.BoolValue(false),
			ApproverEmails:       emailSet(t, "a@example.com"),
			RequesterUserGroupID: types.Int64Value(5),
		}, nil)
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if in.RequesterUserGroupID != nil {
			t.Errorf("disabled write must send nil requester, got %v", *in.RequesterUserGroupID)
		}
	})

	t.Run("disabled clears requester carried from merge base", func(t *testing.T) {
		current := &pro.AppRequestSettings{IsEnabled: new(false), RequesterUserGroupID: new(445)}
		in, diags := buildAppRequestSettingsInput(ctx, AppRequestSettingsResourceModel{
			Enabled:              types.BoolNull(),
			ApproverEmails:       emailSet(t, "a@example.com"),
			RequesterUserGroupID: types.Int64Null(),
		}, current)
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if in.RequesterUserGroupID != nil {
			t.Errorf("disabled merge base must not resurrect a stale group, got %v", *in.RequesterUserGroupID)
		}
	})

	t.Run("omitted fields with nil merge base yield nil pointers", func(t *testing.T) {
		in, diags := buildAppRequestSettingsInput(ctx, AppRequestSettingsResourceModel{
			Enabled:              types.BoolNull(),
			AppStoreLocale:       types.StringNull(),
			ApproverEmails:       emailSet(t, "c@example.com"),
			RequesterUserGroupID: types.Int64Null(),
		}, nil)
		if diags.HasError() {
			t.Fatalf("diags: %v", diags)
		}
		if in.IsEnabled != nil || in.AppStoreLocale != nil || in.RequesterUserGroupID != nil {
			t.Errorf("nil merge base must yield nil pointers, got enabled=%v locale=%v group=%v", in.IsEnabled, in.AppStoreLocale, in.RequesterUserGroupID)
		}
	})
}
