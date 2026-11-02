// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
)

// createdThenUnreadableClient returns a device-group client pointed at a stub
// server that accepts the create and then fails every read. That is what a
// large smart group looks like when the platform is slow to read it back.
func createdThenUnreadableClient(t *testing.T, groupID string) *devicegroups.Client {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": groupID, "href": "/v1/device-groups/" + groupID})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/device-groups") {
			_ = json.NewEncoder(w).Encode(map[string]any{"totalCount": 0, "results": []any{}})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "the read-back failed"})
	})
	return devicegroups.New(newProIDMockClient(t, handler))
}

// staticGroupRawPlan builds a create plan for a static computer group with no
// declared members, which keeps Create off the smart-criteria resolvers.
func staticGroupRawPlan(ctx context.Context, groupSchema resourceschema.Schema) tftypes.Value {
	object := groupSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["name"] = tftypes.NewValue(tftypes.String, "unit-test-group")
	values["device_type"] = tftypes.NewValue(tftypes.String, "computer")
	values["group_type"] = tftypes.NewValue(tftypes.String, "static")
	values["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	values["jamf_pro_id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	values["member_count"] = tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	return tftypes.NewValue(object, values)
}

// TestCreate_FailedReadBackKeepsTheNewID pins the state a create writes when the
// read-back fails. Without it the group exists on the tenant, Terraform records
// nothing, and the next apply is refused because the name is already in use.
// It is a warning so the group is not tainted and replaced for being slow.
func TestCreate_FailedReadBackKeepsTheNewID(t *testing.T) {
	ctx := context.Background()
	const groupID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	r := &DeviceGroupResource{client: createdThenUnreadableClient(t, groupID)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	raw := staticGroupRawPlan(ctx, schemaResp.Schema)
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a failed read-back must not be an error, or the group is tainted: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Fatal("a failed read-back must be reported as a warning")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created group must be recorded in state, or the apply orphans it on the tenant")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("state must be wholly known, got %s", resp.State.Raw)
	}

	var state DeviceGroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != groupID {
		t.Errorf("id = %q, want %q", got, groupID)
	}
	if !state.MemberCount.IsNull() {
		t.Errorf("member_count = %s, want null until the next refresh", state.MemberCount)
	}

	detail := resp.Diagnostics.Warnings()[0].Detail()
	for _, want := range []string{groupID, "the read-back failed"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}
