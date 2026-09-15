// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package location

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestContentListValue_RoundTrip(t *testing.T) {
	ctx := context.Background()

	input := []pro.VolumePurchasingContent{
		{
			AdamID:               "123456789",
			ContentType:          "App",
			DeviceTypes:          nil,
			IconURL:              "https://example.test/icon-a.png",
			LicenseCountInUse:    1,
			LicenseCountReported: 5,
			LicenseCountTotal:    5,
			Name:                 "App Alpha",
			PricingParam:         "STDQ",
		},
		{
			AdamID:               "987654321",
			ContentType:          "App",
			DeviceTypes:          []string{"iphone", "ipad"},
			IconURL:              "https://example.test/icon-b.png",
			LicenseCountInUse:    3,
			LicenseCountReported: 10,
			LicenseCountTotal:    10,
			Name:                 "App Beta",
			PricingParam:         "STDQ",
		},
	}

	list, diags := contentListValue(ctx, input)
	if diags.HasError() {
		t.Fatalf("contentListValue returned diagnostics: %v", diags)
	}
	if list.IsNull() {
		t.Fatalf("contentListValue returned a null list for a non-empty input")
	}
	if got, want := len(list.Elements()), 2; got != want {
		t.Fatalf("expected %d elements, got %d", want, got)
	}

	first, ok := list.Elements()[0].(types.Object)
	if !ok {
		t.Fatalf("expected first element to be types.Object, got %T", list.Elements()[0])
	}
	attrs := first.Attributes()

	adamIDValue, ok := attrs["adam_id"].(types.String)
	if !ok {
		t.Fatalf("expected adam_id to be types.String, got %T", attrs["adam_id"])
	}
	if got, want := adamIDValue.ValueString(), input[0].AdamID; got != want {
		t.Errorf("adam_id round-trip mismatch: got %q want %q", got, want)
	}

	// Sanity-check the row with non-empty DeviceTypes — verify the list element
	// type carried through and the length matches the input slice.
	second, ok := list.Elements()[1].(types.Object)
	if !ok {
		t.Fatalf("expected second element to be types.Object, got %T", list.Elements()[1])
	}
	dt, ok := second.Attributes()["device_types"].(types.List)
	if !ok {
		t.Fatalf("expected device_types to be types.List, got %T", second.Attributes()["device_types"])
	}
	if got, want := len(dt.Elements()), len(input[1].DeviceTypes); got != want {
		t.Errorf("device_types element count mismatch: got %d want %d", got, want)
	}
}

func TestContentListValue_NilInput(t *testing.T) {
	ctx := context.Background()

	list, diags := contentListValue(ctx, nil)
	if diags.HasError() {
		t.Fatalf("contentListValue(nil) returned diagnostics: %v", diags)
	}
	if list.IsNull() {
		t.Errorf("contentListValue(nil) must return a non-null empty list, got null")
	}
	if got := len(list.Elements()); got != 0 {
		t.Errorf("contentListValue(nil) must return an empty list, got %d elements", got)
	}

	// Element type must match the schema's object shape so the empty list slots
	// into state without a type mismatch.
	want := types.ObjectType{AttrTypes: VolumePurchasingLocationContentObjectAttrTypes()}
	if got := list.ElementType(ctx); !got.Equal(want) {
		t.Errorf("contentListValue(nil) element type mismatch: got %v want %v", got, want)
	}
}

func TestAssignVolumePurchasingLocationResourceModel_NilContent(t *testing.T) {
	ctx := context.Background()

	loc := &pro.VolumePurchasingLocation{
		ID:           "loc-1",
		Name:         "x",
		LastSyncTime: "2026-05-25T10:00:00Z",
		Content:      nil,
	}

	// Seed the state with a non-null empty content list so the assigner
	// overwrite path is exercised end-to-end.
	state := &VolumePurchasingLocationResourceModel{
		Content: types.ListValueMust(
			types.ObjectType{AttrTypes: VolumePurchasingLocationContentObjectAttrTypes()},
			[]attr.Value{},
		),
	}

	diags := assignVolumePurchasingLocationResourceModel(ctx, state, loc)
	if diags.HasError() {
		t.Fatalf("assignVolumePurchasingLocationResourceModel returned diagnostics: %v", diags)
	}

	if got, want := state.Name.ValueString(), "x"; got != want {
		t.Errorf("Name mismatch: got %q want %q", got, want)
	}
	if got, want := state.LastSyncTime.ValueString(), "2026-05-25T10:00:00Z"; got != want {
		t.Errorf("LastSyncTime mismatch: got %q want %q", got, want)
	}
	if state.Content.IsNull() {
		t.Errorf("Content must be non-null after assigning nil server content")
	}
	if got := len(state.Content.Elements()); got != 0 {
		t.Errorf("Content must have zero elements for nil server content, got %d", got)
	}
}
