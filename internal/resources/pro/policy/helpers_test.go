// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestBuildNotificationEnabled_Null(t *testing.T) {
	t.Parallel()
	if got := buildNotificationEnabled(types.BoolNull()); got != nil {
		t.Fatalf("expected nil for null input, got %+v", got)
	}
}

func TestBuildNotificationEnabled_True(t *testing.T) {
	t.Parallel()
	got := buildNotificationEnabled(types.BoolValue(true))
	if got == nil || got.Enabled == nil || !*got.Enabled {
		t.Fatalf("expected Enabled=true, got %+v", got)
	}
	if got.Method != nil {
		t.Fatalf("Method must remain nil — method travels via NotificationType field, got %+v", got.Method)
	}
}

func TestFlattenNotificationEnabled_NilSDKValue_PreservesCurrent(t *testing.T) {
	t.Parallel()
	if got := flattenNotificationEnabled(nil, types.BoolValue(true)); got.IsNull() || !got.ValueBool() {
		t.Fatalf("expected preserved current=true, got %v", got)
	}
}

func TestFlattenNotificationEnabled_NilSDKValue_NullCurrent(t *testing.T) {
	t.Parallel()
	if got := flattenNotificationEnabled(nil, types.BoolNull()); !got.IsNull() {
		t.Fatalf("expected null when current is null and SDK absent, got %v", got)
	}
}

func TestFlattenNotificationEnabled_ImportPath(t *testing.T) {
	t.Parallel()
	v := true
	got := flattenNotificationEnabled(&proclassic.NotificationValue{Enabled: &v}, types.BoolNull())
	if got.IsNull() || !got.ValueBool() {
		t.Fatalf("expected import to adopt API value, got %v", got)
	}
}

func TestExtractPolicyID_TopLevel(t *testing.T) {
	t.Parallel()
	id := 42
	p := &proclassic.Policy{ID: &id}
	got := extractPolicyID(p)
	if got != "42" {
		t.Fatalf("expected 42, got %q", got)
	}
}

func TestExtractPolicyID_FallbackToGeneral(t *testing.T) {
	t.Parallel()
	id := 7
	p := &proclassic.Policy{General: &proclassic.PolicyGeneral{ID: &id}}
	got := extractPolicyID(p)
	if got != "7" {
		t.Fatalf("expected 7, got %q", got)
	}
}

func TestExtractPolicyID_Nil(t *testing.T) {
	t.Parallel()
	if got := extractPolicyID(nil); got != "" {
		t.Fatalf("expected empty for nil policy, got %q", got)
	}
	p := &proclassic.Policy{}
	if got := extractPolicyID(p); got != "" {
		t.Fatalf("expected empty for policy without ID, got %q", got)
	}
}

func TestStringIDPtr_ValidInteger(t *testing.T) {
	t.Parallel()
	got := helpers.StringIDPtr(types.StringValue("123"))
	if got == nil || *got != 123 {
		t.Fatalf("expected 123, got %v", got)
	}
}

func TestStringIDPtr_NullOrEmpty(t *testing.T) {
	t.Parallel()
	if helpers.StringIDPtr(types.StringNull()) != nil {
		t.Fatal("expected nil for null input")
	}
	if helpers.StringIDPtr(types.StringValue("")) != nil {
		t.Fatal("expected nil for empty string")
	}
	if helpers.StringIDPtr(types.StringValue("not-an-int")) != nil {
		t.Fatal("expected nil for non-integer input")
	}
}
