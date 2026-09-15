// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"context"
	"errors"
	"testing"
	"time"

	resourcetimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

func TestStringValueOrNull(t *testing.T) {
	tests := []struct {
		input    string
		wantNull bool
	}{
		{"", true},
		{"hello", false},
	}
	for _, tc := range tests {
		result := StringValueOrNull(tc.input)
		if tc.wantNull && !result.IsNull() {
			t.Errorf("StringValueOrNull(%q) should be null", tc.input)
		}
		if !tc.wantNull && result.ValueString() != tc.input {
			t.Errorf("StringValueOrNull(%q) = %q", tc.input, result.ValueString())
		}
	}
}

func TestStringPointerValueOrNull(t *testing.T) {
	s := "hello"
	empty := ""

	if result := StringPointerValueOrNull(nil); !result.IsNull() {
		t.Error("nil pointer should produce null")
	}
	if result := StringPointerValueOrNull(&empty); !result.IsNull() {
		t.Error("empty string pointer should produce null")
	}
	if result := StringPointerValueOrNull(&s); result.ValueString() != "hello" {
		t.Errorf("expected 'hello', got %q", result.ValueString())
	}
}

func TestDerivedRefName(t *testing.T) {
	hq := "HQ"
	none := "NONE"
	id5 := 5
	idZero := 0
	idNeg := -1

	// Real positive id: trust the echoed name.
	if got := DerivedRefName(&id5, &hq); got.ValueString() != "HQ" {
		t.Errorf("positive id should yield echoed name, got %q", got.ValueString())
	}
	// Sentinel ids (nil, 0, -1): name must null regardless of the echo, because
	// the classic GET nondeterministically echoes or omits "NONE".
	if got := DerivedRefName(nil, &none); !got.IsNull() {
		t.Error("nil id should yield null name")
	}
	if got := DerivedRefName(&idZero, &none); !got.IsNull() {
		t.Error("id 0 should yield null name")
	}
	if got := DerivedRefName(&idNeg, &none); !got.IsNull() {
		t.Error("id -1 should yield null name")
	}
}

func TestOptionalStringPointer(t *testing.T) {
	if got := OptionalStringPointer(types.StringNull()); got != nil {
		t.Errorf("null should yield nil pointer, got %v", got)
	}
	if got := OptionalStringPointer(types.StringUnknown()); got != nil {
		t.Errorf("unknown should yield nil pointer (not pointer to empty string), got %v", got)
	}
	if got := OptionalStringPointer(types.StringValue("hello")); got == nil || *got != "hello" {
		t.Errorf("expected pointer to 'hello', got %v", got)
	}
	if got := OptionalStringPointer(types.StringValue("")); got == nil || *got != "" {
		t.Errorf("explicit empty string must be forwarded, got %v", got)
	}
}

func TestAlwaysEmitStringPointer(t *testing.T) {
	if got := AlwaysEmitStringPointer(types.StringNull()); got == nil || *got != "" {
		t.Errorf("null must yield a pointer to the empty string so the element is emitted and clears, got %v", got)
	}
	if got := AlwaysEmitStringPointer(types.StringUnknown()); got != nil {
		t.Errorf("unknown must yield nil (server-owned), got %q", *got)
	}
	if got := AlwaysEmitStringPointer(types.StringValue("hello")); got == nil || *got != "hello" {
		t.Errorf("expected pointer to 'hello', got %v", got)
	}
	if got := AlwaysEmitStringPointer(types.StringValue("")); got == nil || *got != "" {
		t.Errorf("explicit empty string must be forwarded, got %v", got)
	}
}

func TestAlwaysEmitBoolPointer(t *testing.T) {
	if got := AlwaysEmitBoolPointer(types.BoolNull()); got == nil || *got {
		t.Errorf("null must yield a pointer to false so the flag is emitted and cleared, got %v", got)
	}
	if got := AlwaysEmitBoolPointer(types.BoolUnknown()); got != nil {
		t.Errorf("unknown must yield nil (server-owned), got %v", *got)
	}
	if got := AlwaysEmitBoolPointer(types.BoolValue(true)); got == nil || !*got {
		t.Errorf("expected pointer to true, got %v", got)
	}
	if got := AlwaysEmitBoolPointer(types.BoolValue(false)); got == nil || *got {
		t.Errorf("explicit false must be forwarded, got %v", got)
	}
}

func TestBoolPointerValueOrNull(t *testing.T) {
	b := true
	f := false

	if result := BoolPointerValueOrNull(nil); !result.IsNull() {
		t.Error("nil pointer should produce null")
	}
	if result := BoolPointerValueOrNull(&b); result.ValueBool() != true {
		t.Error("expected true")
	}
	if result := BoolPointerValueOrNull(&f); result.ValueBool() != false {
		t.Error("expected false")
	}
}

func TestIsConfiguredValue(t *testing.T) {
	if IsConfiguredValue(types.StringNull()) {
		t.Error("null should not be configured")
	}
	if IsConfiguredValue(types.StringUnknown()) {
		t.Error("unknown should not be configured")
	}
	if !IsConfiguredValue(types.StringValue("x")) {
		t.Error("concrete value should be configured")
	}
	if !IsConfiguredValue(types.StringValue("")) {
		t.Error("empty string should be configured")
	}
}

func TestReconcileOptionalBool(t *testing.T) {
	if result := ReconcileOptionalBool(true, types.BoolNull()); !result.IsNull() {
		t.Error("unmanaged field should stay null")
	}
	if result := ReconcileOptionalBool(true, types.BoolValue(false)); result.ValueBool() != true {
		t.Error("managed field should take API value")
	}
}

func TestReconcileOptionalInt(t *testing.T) {
	if result := ReconcileOptionalInt(42, types.Int64Null()); !result.IsNull() {
		t.Error("unmanaged field should stay null")
	}
	if result := ReconcileOptionalInt(42, types.Int64Value(0)); result.ValueInt64() != 42 {
		t.Error("managed field should take API value")
	}
}

func TestReconcileOptionalString(t *testing.T) {
	if result := ReconcileOptionalString("", types.StringNull()); !result.IsNull() {
		t.Error("empty API + null current should produce null")
	}
	if result := ReconcileOptionalString("", types.StringValue("")); result.ValueString() != "" {
		t.Error("empty API + explicit empty should preserve empty")
	}
	if result := ReconcileOptionalString("hello", types.StringNull()); result.ValueString() != "hello" {
		t.Error("non-empty API should produce value")
	}
}

func TestPreserveStringWhenWireEmpty(t *testing.T) {
	cases := []struct {
		name    string
		wire    *string
		current types.String
		want    types.String
	}{
		{
			name:    "wire empty + configured non-empty current preserves current (the masked-error-path bug class)",
			wire:    new(""),
			current: types.StringValue("user-authored"),
			want:    types.StringValue("user-authored"),
		},
		{
			name:    "wire nil + configured current preserves current",
			wire:    nil,
			current: types.StringValue("user-authored"),
			want:    types.StringValue("user-authored"),
		},
		{
			name:    "wire non-empty replaces current",
			wire:    new("server-value"),
			current: types.StringValue("old"),
			want:    types.StringValue("server-value"),
		},
		{
			name:    "wire nil + null current stays null",
			wire:    nil,
			current: types.StringNull(),
			want:    types.StringNull(),
		},
		{
			name:    "wire empty + null current stays null",
			wire:    new(""),
			current: types.StringNull(),
			want:    types.StringNull(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PreserveStringWhenWireEmpty(tc.wire, tc.current)
			if !got.Equal(tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("some error"), false},
		{"404 response", &jamfplatform.APIResponseError{StatusCode: 404}, true},
		// The 400 + INVALID_ID arm is the Pro-ism half of this function and had no
		// fixture at all: removing it left the suite green. The 404 case above carries
		// no Details, so it can only pass via the status arm — the two are pinned
		// independently.
		{"400 with INVALID_ID", &jamfplatform.APIResponseError{
			StatusCode: 400,
			Errors:     []jamfplatform.ErrorDetail{{Code: "INVALID_ID"}},
		}, true},
		{"400 with another code", &jamfplatform.APIResponseError{
			StatusCode: 400,
			Errors:     []jamfplatform.ErrorDetail{{Code: "INVALID_FIELD"}},
		}, false},
		{"400 with no details", &jamfplatform.APIResponseError{StatusCode: 400}, false},
		{"500 response", &jamfplatform.APIResponseError{StatusCode: 500}, false},
		{"200 response", &jamfplatform.APIResponseError{StatusCode: 200}, false},
		// The regression this function exists to avoid: a 404 the gateway
		// produced because it routes nothing there is not the object being gone,
		// and treating it as such deletes every managed object from state on one
		// refresh. Every fixture below carries a body a real not-found was
		// observed to carry on 2026-09-08, so the discrimination is by body text
		// and not by "the body is not JSON" — two of them are not JSON either.
		{"404 from an unrouted gateway path", &jamfplatform.APIResponseError{
			StatusCode: 404,
			Body:       "404 page not found\n",
		}, false},
		{"404 carrying a Jamf JSON body", &jamfplatform.APIResponseError{
			StatusCode: 404,
			Body:       `{"httpStatus":404,"errors":[{"code":"NOT_FOUND"}]}`,
		}, true},
		{"404 carrying the classic HTML status page", &jamfplatform.APIResponseError{
			StatusCode: 404,
			Body:       "<html>\n<head>\n   <title>Status page</title>\n</head>\n<body>Not Found</body>\n</html>",
		}, true},
		{"404 carrying an empty body", &jamfplatform.APIResponseError{
			StatusCode: 404,
			Body:       "",
		}, true},
		{"404 mentioning the gateway text inside a longer body", &jamfplatform.APIResponseError{
			StatusCode: 404,
			Body:       `{"httpStatus":404,"errors":[{"description":"404 page not found"}]}`,
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := IsNotFoundError(tc.err); result != tc.expected {
				t.Errorf("IsNotFoundError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

// TestIsGatewayUnrouted pins the discrimination on its own, rather than only
// through IsNotFoundError, because three other call sites act on it: two
// Security Cloud not-found checks and the app-installer retry check, whose
// "nothing to retry" tell is a 404 with no error details — which a bare gateway
// 404 satisfies.
func TestIsGatewayUnrouted(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("404 page not found"), false},
		{"the gateway text", &jamfplatform.APIResponseError{StatusCode: 404, Body: "404 page not found"}, true},
		{"the gateway text with trailing newline", &jamfplatform.APIResponseError{StatusCode: 404, Body: "404 page not found\n"}, true},
		{"the gateway text on another status", &jamfplatform.APIResponseError{StatusCode: 403, Body: "404 page not found"}, false},
		{"a Jamf JSON 404", &jamfplatform.APIResponseError{StatusCode: 404, Body: `{"httpStatus":404}`}, false},
		{"an empty 404", &jamfplatform.APIResponseError{StatusCode: 404}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := IsGatewayUnrouted(tc.err); result != tc.expected {
				t.Errorf("IsGatewayUnrouted(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("some error"), false},
		{"500 response", &jamfplatform.APIResponseError{StatusCode: 500}, true},
		{"404 response", &jamfplatform.APIResponseError{StatusCode: 404}, false},
		{"502 response", &jamfplatform.APIResponseError{StatusCode: 502}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := IsServerError(tc.err); result != tc.expected {
				t.Errorf("IsServerError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestIsForbiddenError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("some error"), false},
		{"403 response", &jamfplatform.APIResponseError{StatusCode: 403}, true},
		{"401 response", &jamfplatform.APIResponseError{StatusCode: 401}, false},
		{"404 response", &jamfplatform.APIResponseError{StatusCode: 404}, false},
		{"500 response", &jamfplatform.APIResponseError{StatusCode: 500}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := IsForbiddenError(tc.err); result != tc.expected {
				t.Errorf("IsForbiddenError(%v) = %v, want %v", tc.err, result, tc.expected)
			}
		})
	}
}

func TestNormalizedFilterString(t *testing.T) {
	tests := []struct {
		input    types.String
		wantStr  string
		wantBool bool
	}{
		{types.StringNull(), "", false},
		{types.StringUnknown(), "", false},
		{types.StringValue(""), "", false},
		{types.StringValue("  "), "", false},
		{types.StringValue(" hello "), "hello", true},
		{types.StringValue("test"), "test", true},
	}
	for _, tc := range tests {
		str, ok := NormalizedFilterString(tc.input)
		if str != tc.wantStr || ok != tc.wantBool {
			t.Errorf("NormalizedFilterString(%v) = (%q, %v), want (%q, %v)", tc.input, str, ok, tc.wantStr, tc.wantBool)
		}
	}
}

func TestSetToStringSlice_Values(t *testing.T) {
	ctx := context.Background()
	set, _ := types.SetValueFrom(ctx, types.StringType, []string{"a", "b", "c"})
	result, diags := SetToStringSlice(ctx, set)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
}

func TestSetToStringSlice_Null(t *testing.T) {
	result, diags := SetToStringSlice(context.Background(), types.SetNull(types.StringType))
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if result != nil {
		t.Errorf("expected nil for null set, got %v", result)
	}
}

func TestSetToStringSlice_Unknown(t *testing.T) {
	result, diags := SetToStringSlice(context.Background(), types.SetUnknown(types.StringType))
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if result != nil {
		t.Errorf("expected nil for unknown set, got %v", result)
	}
}

func TestResolveTimeout_NullReturnsDefault(t *testing.T) {
	duration, diags := ResolveTimeout(context.Background(), true, false, 5*time.Minute, nil)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if duration != 5*time.Minute {
		t.Errorf("expected 5m, got %v", duration)
	}
}

func TestResolveTimeout_UnknownReturnsDefault(t *testing.T) {
	duration, diags := ResolveTimeout(context.Background(), false, true, 10*time.Minute, nil)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if duration != 10*time.Minute {
		t.Errorf("expected 10m, got %v", duration)
	}
}

func TestResolveTimeout_NilResolverReturnsDefault(t *testing.T) {
	duration, diags := ResolveTimeout(context.Background(), false, false, 3*time.Minute, nil)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if duration != 3*time.Minute {
		t.Errorf("expected 3m, got %v", duration)
	}
}

func TestResolveTimeout_CallsResolver(t *testing.T) {
	called := false
	resolver := func(ctx context.Context, def time.Duration) (time.Duration, diag.Diagnostics) {
		called = true
		return 7 * time.Minute, nil
	}
	duration, diags := ResolveTimeout(context.Background(), false, false, 3*time.Minute, resolver)
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if !called {
		t.Error("expected resolver to be called")
	}
	if duration != 7*time.Minute {
		t.Errorf("expected 7m, got %v", duration)
	}
}

func TestSetIdentity_NilTarget(t *testing.T) {
	diags := SetIdentity(context.Background(), nil, "test")
	if diags != nil {
		t.Errorf("expected nil diags for nil target, got %v", diags)
	}
}

// TestSetIdentity_TypedNilTarget covers what call sites actually pass. A
// resource hands SetIdentity its `resp.Identity`, a `*tfsdk.ResourceIdentity`
// that the framework leaves nil when the resource declares no identity schema
// or the client does not support identity. Boxing that pointer into
// IdentitySetter produces a non-nil interface, so TestSetIdentity_NilTarget
// above — which passes an untyped nil, and therefore a nil interface — proves
// nothing about the reachable case. Before the guard looked through the
// interface, this panicked in (*ResourceIdentity).Set.
func TestSetIdentity_TypedNilTarget(t *testing.T) {
	var target *tfsdk.ResourceIdentity

	diags := SetIdentity(context.Background(), target, "test")
	if diags != nil {
		t.Errorf("expected nil diags for a typed-nil target, got %v", diags)
	}
}

// TestSetIdentity_TypedNilOfAnyImplementation pins that the guard is not
// written against the one concrete type the framework happens to use, so a
// future identity setter gets the same treatment.
func TestSetIdentity_TypedNilOfAnyImplementation(t *testing.T) {
	var target *mockIdentitySetter

	diags := SetIdentity(context.Background(), target, "test")
	if diags != nil {
		t.Errorf("expected nil diags for a typed-nil target, got %v", diags)
	}
}

func TestSetIdentity_WithTarget(t *testing.T) {
	mock := &mockIdentitySetter{}
	diags := SetIdentity(context.Background(), mock, "test-identity")
	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if mock.value != "test-identity" {
		t.Errorf("expected identity 'test-identity', got %v", mock.value)
	}
}

type mockIdentitySetter struct {
	value any
}

func (m *mockIdentitySetter) Set(ctx context.Context, v any) diag.Diagnostics {
	m.value = v
	return nil
}

func TestEnsureResourceTimeouts_NullValue(t *testing.T) {
	attrTypes := map[string]attr.Type{
		"create": types.StringType,
		"read":   types.StringType,
	}
	result := EnsureResourceTimeouts(resourcetimeouts.Value{}, attrTypes)
	if result.IsUnknown() {
		t.Error("expected non-unknown result")
	}
}

func TestNewResourceTimeoutsNullValue(t *testing.T) {
	attrTypes := map[string]attr.Type{
		"create": types.StringType,
	}
	result := NewResourceTimeoutsNullValue(attrTypes)
	if result.IsUnknown() {
		t.Error("expected non-unknown result")
	}
}

// TestSelfServiceCategoryDisplayIn pins the default that makes a Self Service
// category store at all. An unset display_in used to travel as omitted, and
// Jamf Pro discards a <category> without it — so `categories = [{ id = "64" }]`
// reported success and stored nothing, on all six classic resources carrying
// the block. See SelfServiceCategoryDisplayIn for the wire record.
func TestSelfServiceCategoryDisplayIn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   types.Bool
		want bool
	}{
		{"null defaults to true, because an omitted display_in is discarded", types.BoolNull(), true},
		{"unknown defaults to true for the same reason", types.BoolUnknown(), true},
		{"an explicit true is sent as true", types.BoolValue(true), true},
		{"an explicit false is preserved — it is how a category is removed", types.BoolValue(false), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SelfServiceCategoryDisplayIn(tc.in)
			if got == nil {
				t.Fatal("display_in must never be omitted: the server discards a category without it")
			}
			if *got != tc.want {
				t.Errorf("got %v, want %v", *got, tc.want)
			}
		})
	}
}
