// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

const testPayloadIdentifierNamespace = "41d36a3f-3e44-4a07-9db4-0efdb086743f"

type privateStateStub map[string][]byte

func (s privateStateStub) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return s[key], nil
}

func (s privateStateStub) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	s[key] = value
	return nil
}

func TestEnsurePayloadIdentifierNamespace(t *testing.T) {
	t.Parallel()

	state := privateStateStub{}
	first, diags := ensurePayloadIdentifierNamespace(context.Background(), nil, state)
	if diags.HasError() {
		t.Fatalf("generate namespace: %v", diags)
	}
	second, diags := ensurePayloadIdentifierNamespace(context.Background(), state, state)
	if diags.HasError() {
		t.Fatalf("read namespace: %v", diags)
	}
	if first != second {
		t.Fatalf("namespace changed from %q to %q", first, second)
	}
}

func TestEnsurePayloadIdentifierNamespaceRequiresPrivateState(t *testing.T) {
	t.Parallel()

	_, diags := ensurePayloadIdentifierNamespace(context.Background(), nil, nil)
	if !diags.HasError() {
		t.Fatal("expected an error when private state is unavailable")
	}
}

func TestGeneratePayloadIdentifier(t *testing.T) {
	t.Parallel()

	first := generatePayloadIdentifier(testPayloadIdentifierNamespace, 0, "com.apple.ManagedClient.preferences")
	if want := "1e484f1d-0a38-2d4e-4205-9fffb02d53a8"; first != want {
		t.Fatalf("identifier = %q, want %q", first, want)
	}
	cases := map[string]string{
		"same input":          generatePayloadIdentifier(testPayloadIdentifierNamespace, 0, "com.apple.ManagedClient.preferences"),
		"different namespace": generatePayloadIdentifier("92e88a09-5c2c-4124-a288-22f27ff45952", 0, "com.apple.ManagedClient.preferences"),
		"different step":      generatePayloadIdentifier(testPayloadIdentifierNamespace, 1, "com.apple.ManagedClient.preferences"),
		"different type":      generatePayloadIdentifier(testPayloadIdentifierNamespace, 0, "com.apple.applicationaccess"),
	}

	if cases["same input"] != first {
		t.Fatal("same input produced a different identifier")
	}
	for name, got := range cases {
		if name != "same input" && got == first {
			t.Errorf("%s produced the same identifier", name)
		}
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(first) {
		t.Errorf("identifier %q is not UUID-formatted", first)
	}
}

func TestDescribeBlueprintBlocks(t *testing.T) {
	named := "Passcode"
	empty := ""

	got := describeBlueprintBlocks([]blueprints.BlueprintStep{
		{
			Name: &named,
			Components: []blueprints.Component{
				{Identifier: "com.jamf.ddm.passcode-settings"},
				{Identifier: "com.jamf.ddm.math-settings"},
			},
		},
		{Name: nil, Components: []blueprints.Component{{Identifier: "com.jamf.ddm.safari-settings"}}},
		{Name: &empty, Components: nil},
	})

	want := strings.Join([]string{
		"  1. Passcode — com.jamf.ddm.passcode-settings, com.jamf.ddm.math-settings",
		"  2. (unnamed) — com.jamf.ddm.safari-settings",
		"  3. (unnamed) — no components",
		"",
	}, "\n")

	if got != want {
		t.Errorf("describeBlueprintBlocks() =\n%q\nwant\n%q", got, want)
	}
}

func TestDescribeBlueprintBlocks_NoSteps(t *testing.T) {
	if got := describeBlueprintBlocks(nil); got != "" {
		t.Errorf("describeBlueprintBlocks(nil) = %q, want empty", got)
	}
}

func TestSetNestedValue_SimpleString(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "name", "test")
	if obj["name"] != "test" {
		t.Errorf("expected 'test', got %v", obj["name"])
	}
}

func TestSetNestedValue_BoolTrue(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "enabled", "true")
	if obj["enabled"] != true {
		t.Errorf("expected true, got %v", obj["enabled"])
	}
}

func TestSetNestedValue_BoolFalse(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "disabled", "false")
	if obj["disabled"] != false {
		t.Errorf("expected false, got %v", obj["disabled"])
	}
}

func TestSetNestedValue_Integer(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "count", "42")
	if obj["count"] != 42 {
		t.Errorf("expected 42, got %v", obj["count"])
	}
}

func TestSetNestedValue_EmptyValue(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "field", "")
	if obj["field"] != nil {
		t.Errorf("expected nil, got %v", obj["field"])
	}
}

func TestSetNestedValue_NestedKey(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "parent_child", "value")
	parent, ok := obj["parent"].(map[string]any)
	if !ok {
		t.Fatal("expected nested map at 'parent'")
	}
	if parent["child"] != "value" {
		t.Errorf("expected 'value' at parent.child, got %v", parent["child"])
	}
}

func TestSetNestedValue_DeepNested(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "a_b_c", "deep")
	a, ok := obj["a"].(map[string]any)
	if !ok {
		t.Fatal("expected nested map at 'a'")
	}
	b, ok := a["b"].(map[string]any)
	if !ok {
		t.Fatal("expected nested map at 'a.b'")
	}
	if b["c"] != "deep" {
		t.Errorf("expected 'deep' at a.b.c, got %v", b["c"])
	}
}

func TestSetNestedValue_JSONArray(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "items", `[1,2,3]`)
	arr, ok := obj["items"].([]any)
	if !ok {
		t.Fatalf("expected array, got %T", obj["items"])
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr))
	}
}

func TestSetNestedValue_JSONObject(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "config", `{"key":"val"}`)
	m, ok := obj["config"].(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", obj["config"])
	}
	if m["key"] != "val" {
		t.Errorf("expected 'val', got %v", m["key"])
	}
}

func TestSetNestedValue_OverwritesExistingNonMap(t *testing.T) {
	obj := map[string]any{"parent": "not-a-map"}
	setNestedValue(obj, "parent_child", "value")
	parent, ok := obj["parent"].(map[string]any)
	if !ok {
		t.Fatal("expected parent to be overwritten to a map")
	}
	if parent["child"] != "value" {
		t.Errorf("expected 'value', got %v", parent["child"])
	}
}

func TestFlattenJSON_SimpleValues(t *testing.T) {
	obj := map[string]any{
		"name":    "test",
		"enabled": true,
		"count":   float64(42),
	}
	result := make(map[string]string)
	flattenJSON(obj, "", result)

	if result["name"] != "test" {
		t.Errorf("expected 'test', got %q", result["name"])
	}
	if result["enabled"] != "true" {
		t.Errorf("expected 'true', got %q", result["enabled"])
	}
	if result["count"] != "42" {
		t.Errorf("expected '42', got %q", result["count"])
	}
}

func TestFlattenJSON_BoolFalse(t *testing.T) {
	obj := map[string]any{"disabled": false}
	result := make(map[string]string)
	flattenJSON(obj, "", result)
	if result["disabled"] != "false" {
		t.Errorf("expected 'false', got %q", result["disabled"])
	}
}

func TestFlattenJSON_NilValue(t *testing.T) {
	obj := map[string]any{"field": nil}
	result := make(map[string]string)
	flattenJSON(obj, "", result)
	if result["field"] != "" {
		t.Errorf("expected empty string for nil, got %q", result["field"])
	}
}

func TestFlattenJSON_NestedObject(t *testing.T) {
	obj := map[string]any{
		"parent": map[string]any{
			"child": "value",
		},
	}
	result := make(map[string]string)
	flattenJSON(obj, "", result)
	if result["parent_child"] != "value" {
		t.Errorf("expected 'value' at 'parent_child', got %q", result["parent_child"])
	}
}

func TestFlattenJSON_WithPrefix(t *testing.T) {
	obj := map[string]any{"key": "val"}
	result := make(map[string]string)
	flattenJSON(obj, "prefix", result)
	if result["prefix_key"] != "val" {
		t.Errorf("expected 'val' at 'prefix_key', got %q", result["prefix_key"])
	}
}

func TestFlattenJSON_IntValue(t *testing.T) {
	obj := map[string]any{"num": 10}
	result := make(map[string]string)
	flattenJSON(obj, "", result)
	if result["num"] != "10" {
		t.Errorf("expected '10', got %q", result["num"])
	}
}

func TestFlattenJSON_FloatDecimal(t *testing.T) {
	obj := map[string]any{"ratio": float64(3.14)}
	result := make(map[string]string)
	flattenJSON(obj, "", result)
	if result["ratio"] != "3.14" {
		t.Errorf("expected '3.14', got %q", result["ratio"])
	}
}

func TestFlattenJSON_ArrayValue(t *testing.T) {
	obj := map[string]any{
		"items": []any{"a", "b"},
	}
	result := make(map[string]string)
	flattenJSON(obj, "", result)
	if result["items"] != `["a","b"]` {
		t.Errorf("expected JSON array, got %q", result["items"])
	}
}

func TestSetNestedAndFlattenRoundtrip(t *testing.T) {
	obj := make(map[string]any)
	setNestedValue(obj, "parent_child", "value")
	setNestedValue(obj, "parent_other", "42")
	setNestedValue(obj, "top", "true")

	result := make(map[string]string)
	flattenJSON(obj, "", result)

	if result["parent_child"] != "value" {
		t.Errorf("expected 'value', got %q", result["parent_child"])
	}
	if result["parent_other"] != "42" {
		t.Errorf("expected '42', got %q", result["parent_other"])
	}
	if result["top"] != "true" {
		t.Errorf("expected 'true', got %q", result["top"])
	}
}

func TestDesiredDeployedValue_Configured(t *testing.T) {
	if desiredDeployedValue(types.BoolValue(true)) != true {
		t.Error("expected true for configured true")
	}
	if desiredDeployedValue(types.BoolValue(false)) != false {
		t.Error("expected false for configured false")
	}
}

func TestDesiredDeployedValue_NullDefaults(t *testing.T) {
	if desiredDeployedValue(types.BoolNull()) != true {
		t.Error("expected true as default for null value")
	}
}

func TestDesiredDeployedValue_UnknownDefaults(t *testing.T) {
	if desiredDeployedValue(types.BoolUnknown()) != true {
		t.Error("expected true as default for unknown value")
	}
}

// TestIsDeleteMaybeComplete pins which failed DELETE replies may take Delete's
// warning branch, since that branch returns no error diagnostic and so drops
// the blueprint from Terraform state.
func TestIsDeleteMaybeComplete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "jamf json 500",
			err:  &jamfplatform.APIResponseError{StatusCode: 500, Body: `{"httpStatus":500}`},
			want: true,
		},
		{
			name: "jamf json 409",
			err:  &jamfplatform.APIResponseError{StatusCode: 409, Body: `{"httpStatus":409}`},
			want: false,
		},
		{
			name: "non-api error",
			err:  errors.New("dial tcp: connection refused"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isDeleteMaybeComplete(tt.err); got != tt.want {
				t.Errorf("isDeleteMaybeComplete(%v) = %t, want %t", tt.err, got, tt.want)
			}
		})
	}
}

// TestIsDeleteMaybeComplete_EdgePageIsNotCompletion covers the case the table
// above cannot reach. An edge error page is marked inside the SDK's unexported
// transport, so a hand-built *APIResponseError can never carry the marker and a
// table case for it would assert nothing — hence the stub.
//
// A CloudFront 502 is a server error, so before the IsEdgeBlocked exclusion it
// took the warning branch and dropped a live, still-deployed blueprint from
// state for a request that never reached Jamf at all.
func TestIsDeleteMaybeComplete_EdgePageIsNotCompletion(t *testing.T) {
	t.Parallel()

	err := gatewaystub.ErrorFrom(t, gatewaystub.Reply{
		Status:      http.StatusBadGateway,
		ContentType: "text/html",
		Body:        gatewaystub.CloudFrontPage(gatewaystub.CloudFrontBadGateway),
	})

	if isDeleteMaybeComplete(err) {
		t.Errorf("isDeleteMaybeComplete = true for a CloudFront 502 page, so Delete drops a "+
			"blueprint that was never deleted from state: %v", err)
	}
}
