// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// repeatedEAs is the issue #568 shape, in the order the 2026-11-11 probe wrote it and
// Jamf Pro returned it: identical entries, one name with two different values, an
// entry with no value and one with an empty value, out of alphabetical order.
func repeatedEAs() []pro.InventoryPreloadExtensionAttribute {
	return []pro.InventoryPreloadExtensionAttribute{
		{Name: "Z", Value: new("z")},
		{Name: "A", Value: new("a")},
		{Name: "M", Value: new("m")},
		{Name: "A", Value: new("a")},
		{Name: "A", Value: new("a2")},
		{Name: "N"},
		{Name: "N"},
		{Name: "E", Value: new("")},
	}
}

// eaString renders a model's entries as name=value, `name` alone for a null value, so
// order and null-vs-empty are both visible in a failure.
func eaString(t *testing.T, models []extensionAttributeModel) string {
	t.Helper()
	parts := make([]string, 0, len(models))
	for _, m := range models {
		if m.Value.IsNull() {
			parts = append(parts, m.Name.ValueString())
			continue
		}
		parts = append(parts, m.Name.ValueString()+"="+m.Value.ValueString())
	}
	return strings.Join(parts, " ")
}

const wantRepeated = "Z=z A=a M=m A=a A=a2 N N E="

// resourceSchema returns the current resource schema.
func resourceSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	(&InventoryPreloadRecordResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp
}

// TestAssignResourceModel_RepeatedExtensionAttributesKeepServerOrder pins issue #568:
// repeats flatten with no diagnostics, in the server's order, and the model is
// accepted by tfsdk.State.Set (a set used to fail here with Duplicate Set Element).
func TestAssignResourceModel_RepeatedExtensionAttributesKeepServerOrder(t *testing.T) {
	ctx := context.Background()
	eas := repeatedEAs()
	rec := &pro.InventoryPreloadRecordV2{ID: new("7"), SerialNumber: "tf-568b-unit", DeviceType: "Computer", ExtensionAttributes: &eas}

	state := InventoryPreloadRecordResourceModel{Timeouts: helpers.NewResourceTimeoutsNullValue(inventoryPreloadRecordTimeoutAttributeTypes)}
	if diags := assignInventoryPreloadRecordResourceModel(ctx, &state, rec); diags.HasError() {
		t.Fatalf("assign: %v", diags)
	}
	var models []extensionAttributeModel
	if diags := state.ExtensionAttributes.ElementsAs(ctx, &models, false); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
	if got := eaString(t, models); got != wantRepeated {
		t.Errorf("extension_attributes = %q, want %q", got, wantRepeated)
	}

	sch := resourceSchema(t).Schema
	tfState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := tfState.Set(ctx, &state); diags.HasError() {
		t.Fatalf("State.Set: %v", diags)
	}
}

// TestAssignDataSourceModel_RepeatedExtensionAttributes covers the data source, which
// shares the flattener.
func TestAssignDataSourceModel_RepeatedExtensionAttributes(t *testing.T) {
	ctx := context.Background()
	eas := repeatedEAs()
	rec := &pro.InventoryPreloadRecordV2{ID: new("7"), SerialNumber: "tf-568b-unit", DeviceType: "Computer", ExtensionAttributes: &eas}
	var state InventoryPreloadRecordDataSourceModel
	if diags := assignInventoryPreloadRecordDataSourceModel(ctx, &state, rec); diags.HasError() {
		t.Fatalf("assign: %v", diags)
	}
	var models []extensionAttributeModel
	if diags := state.ExtensionAttributes.ElementsAs(ctx, &models, false); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
	if got := eaString(t, models); got != wantRepeated {
		t.Errorf("extension_attributes = %q, want %q", got, wantRepeated)
	}
}

// TestBuildInput_RepeatedExtensionAttributesSentInPlanOrder checks the write side:
// every planned entry goes out, in plan order.
func TestBuildInput_RepeatedExtensionAttributesSentInPlanOrder(t *testing.T) {
	ctx := context.Background()
	eas := repeatedEAs()
	planList, diags := flattenExtensionAttributes(ctx, &eas)
	if diags.HasError() {
		t.Fatalf("flatten: %v", diags)
	}
	plan := InventoryPreloadRecordResourceModel{SerialNumber: types.StringValue("s"), DeviceType: types.StringValue("Computer"), ExtensionAttributes: planList}
	got, diags := buildInventoryPreloadRecordInput(ctx, plan)
	if diags.HasError() {
		t.Fatalf("build: %v", diags)
	}
	sent, _ := json.Marshal(got.ExtensionAttributes)
	want, _ := json.Marshal(eas)
	if string(sent) != string(want) {
		t.Errorf("sent %s, want %s", sent, want)
	}
}

// TestUpgradeStateCoversEveryPriorVersion fails when the schema version is bumped
// without an upgrader for the version before it.
func TestUpgradeStateCoversEveryPriorVersion(t *testing.T) {
	version := resourceSchema(t).Schema.Version
	upgraders := (&InventoryPreloadRecordResource{}).UpgradeState(context.Background())
	for v := range version {
		if _, ok := upgraders[v]; !ok {
			t.Errorf("no state upgrader for schema version %d (current %d)", v, version)
		}
	}
	if len(upgraders) != int(version) {
		t.Errorf("got %d upgraders for schema version %d", len(upgraders), version)
	}
}

// v0StateJSON is state as the v0 provider wrote it: extension_attributes as a set, in
// the set's own order, every other attribute alongside.
const v0StateJSON = `{
	"id": "7",
	"serial_number": "tf-568b-unit",
	"device_type": "Computer",
	"username": "u", "full_name": null, "email_address": null, "phone_number": null,
	"position": null, "department": null, "building": null, "room": null,
	"po_number": null, "po_date": null, "warranty_expiration": null, "lease_expiration": null,
	"apple_care_id": null, "life_expectancy": null, "purchase_price": null,
	"purchasing_contact": null, "purchasing_account": null, "bar_code_1": null,
	"bar_code_2": null, "asset_tag": "", "vendor": null,
	"extension_attributes": [
		{"name": "A", "value": "a"},
		{"name": "E", "value": ""},
		{"name": "M", "value": null},
		{"name": "Z", "value": "z"}
	],
	"timeouts": null
}`

// TestUpgradeState_V0KeepsExtensionAttributesVerbatim decodes a v0 fixture through
// the v0 upgrader: every attribute survives and the stored order is kept as it was.
func TestUpgradeState_V0KeepsExtensionAttributesVerbatim(t *testing.T) {
	ctx := context.Background()
	r := &InventoryPreloadRecordResource{}
	sch := resourceSchema(t).Schema

	req := resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: []byte(v0StateJSON)}}
	resp := resource.UpgradeStateResponse{State: tfsdk.State{Schema: sch}}
	r.UpgradeState(ctx)[0].StateUpgrader(ctx, req, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade: %v", resp.Diagnostics)
	}
	if resp.DynamicValue == nil {
		t.Fatal("upgrader returned no state")
	}
	value, err := resp.DynamicValue.Unmarshal(sch.Type().TerraformType(ctx))
	if err != nil {
		t.Fatalf("decode upgraded state: %v", err)
	}
	var model InventoryPreloadRecordResourceModel
	if diags := (tfsdk.State{Schema: sch, Raw: value}).Get(ctx, &model); diags.HasError() {
		t.Fatalf("get: %v", diags)
	}
	var models []extensionAttributeModel
	if diags := model.ExtensionAttributes.ElementsAs(ctx, &models, false); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
	if got, want := eaString(t, models), "A=a E= M Z=z"; got != want {
		t.Errorf("extension_attributes = %q, want %q", got, want)
	}
	if model.ID.ValueString() != "7" || model.Username.ValueString() != "u" || model.AssetTag.IsNull() || !model.FullName.IsNull() {
		t.Errorf("scalars not carried verbatim: id=%v username=%v asset_tag=%v full_name=%v", model.ID, model.Username, model.AssetTag, model.FullName)
	}
}

// newStubProClient serves the Jamf Pro endpoints from handle.
// testhelpers.NewMockClient does the same, but internal/testhelpers imports the
// provider, which imports this package, so an in-package test cannot use it.
func newStubProClient(t *testing.T, handle http.HandlerFunc) *pro.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	return pro.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// TestInventoryPreloadRecordList_RepeatsDoNotFailTheQuery pins the issue #568 query
// failure: with config generation on, a record with repeated extension attributes
// hydrates, in server order, and the clean record after it is still returned with no
// trailing skip warning.
func TestInventoryPreloadRecordList_RepeatsDoNotFailTheQuery(t *testing.T) {
	ctx := context.Background()
	eas := repeatedEAs()
	records := []pro.InventoryPreloadRecordV2{
		{ID: new("1"), SerialNumber: "tf-568b-repeats", DeviceType: "Computer", ExtensionAttributes: &eas},
		{ID: new("2"), SerialNumber: "tf-568b-clean", DeviceType: "Computer", ExtensionAttributes: &[]pro.InventoryPreloadExtensionAttribute{}},
	}
	client := newStubProClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/inventory-preload/records") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"totalCount": len(records), "results": records})
	})

	lr := &InventoryPreloadRecordListResource{client: client}
	schemaResp := resourceSchema(t)
	var identityResp resource.IdentitySchemaResponse
	(&InventoryPreloadRecordResource{}).IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	var configResp list.ListResourceSchemaResponse
	lr.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &configResp)
	configType := configResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	var stream list.ListResultsStream
	lr.List(ctx, list.ListRequest{
		Config: tfsdk.Config{Schema: configResp.Schema, Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
			"filter":   tftypes.NewValue(configType.AttributeTypes["filter"], nil),
			"timeouts": tftypes.NewValue(configType.AttributeTypes["timeouts"], nil),
		})},
		IncludeResource:        true,
		ResourceSchema:         schemaResp.Schema,
		ResourceIdentitySchema: identityResp.IdentitySchema,
	}, &stream)
	if stream.Results == nil {
		t.Fatal("List left the stream unset")
	}
	var results []list.ListResult
	for res := range stream.Results {
		results = append(results, res)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (and no trailing skip warning): %v", len(results), results)
	}
	for _, res := range results {
		if len(res.Diagnostics) != 0 {
			t.Errorf("%s: unexpected diagnostics: %v", res.DisplayName, res.Diagnostics)
		}
	}
	if results[0].DisplayName != "tf-568b-repeats" || results[1].DisplayName != "tf-568b-clean" {
		t.Errorf("results = %q, %q", results[0].DisplayName, results[1].DisplayName)
	}
	var state InventoryPreloadRecordResourceModel
	if diags := results[0].Resource.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the repeats record: %v", diags)
	}
	var models []extensionAttributeModel
	if diags := state.ExtensionAttributes.ElementsAs(ctx, &models, false); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
	if got := eaString(t, models); got != wantRepeated {
		t.Errorf("extension_attributes = %q, want %q", got, wantRepeated)
	}
}
