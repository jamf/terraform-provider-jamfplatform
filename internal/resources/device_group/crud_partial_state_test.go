// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
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

const lostTestGroupName = "EPZ - App Installers - Update-Only - Visual Studio Code (tf)"

// lostResponseServer stubs a create that answers 504 after making the groups in
// made, the way a slow create looks through the CDN. existing is a same-named
// group that was there before the create. proIDs answers the Jamf Pro ID lookup.
func lostResponseServer(t *testing.T, existing string, made []string, proIDs map[string]string) *jamfplatform.Client {
	t.Helper()
	var posted atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			posted.Store(true)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write([]byte("<html>The request could not be satisfied</html>"))
		case strings.Contains(r.URL.Path, "/groups/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_ = json.NewEncoder(w).Encode(map[string]any{"groupPlatformId": id, "groupJamfProId": proIDs[id]})
		case strings.HasSuffix(r.URL.Path, "/device-groups"):
			if got := r.URL.Query().Get("filter"); !strings.Contains(got, lostTestGroupName) {
				t.Errorf("list filter %q does not name the group", got)
			}
			var ids []string
			if existing != "" {
				ids = append(ids, existing)
			}
			if posted.Load() {
				ids = append(ids, made...)
			}
			var results []map[string]any
			for _, id := range ids {
				results = append(results, map[string]any{"id": id, "name": lostTestGroupName, "deviceType": "COMPUTER", "groupType": "STATIC"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"totalCount": len(results), "results": results})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], "name": lostTestGroupName, "deviceType": "COMPUTER", "groupType": "STATIC", "memberCount": 2307,
			})
		}
	})
	return newProIDMockClient(t, handler)
}

// createWithLostResponse drives Create against server and returns the response.
func createWithLostResponse(t *testing.T, client *jamfplatform.Client) resource.CreateResponse {
	t.Helper()
	ctx := context.Background()
	old := deviceGroupCreateRetryDelay
	deviceGroupCreateRetryDelay = time.Millisecond
	t.Cleanup(func() { deviceGroupCreateRetryDelay = old })

	r := &DeviceGroupResource{client: devicegroups.New(client), proClient: pro.New(client), pd: providerdata.New(client)}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	raw := staticGroupRawPlan(ctx, schemaResp.Schema)
	object := raw.Type().(tftypes.Object)
	values := map[string]tftypes.Value{}
	_ = raw.As(&values)
	values["name"] = tftypes.NewValue(tftypes.String, lostTestGroupName)
	raw = tftypes.NewValue(object, values)

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &resp)
	return resp
}

func recordedID(t *testing.T, resp resource.CreateResponse) string {
	t.Helper()
	if resp.State.Raw.IsNull() {
		t.Fatal("the recovered group must be recorded in state")
	}
	var state DeviceGroupResourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	return state.ID.ValueString()
}

// TestCreate_LostResponseAdoptsTheNewGroup is the regression test for a CDN 504
// on a create that the platform completed: the next apply was refused for a
// duplicate name. A same-named group from before the create must not be taken.
func TestCreate_LostResponseAdoptsTheNewGroup(t *testing.T) {
	const existing, made = "11111111-1111-1111-1111-111111111111", "ae705e2c-26ef-4d52-88dc-725c69b98bbd"
	resp := createWithLostResponse(t, lostResponseServer(t, existing, []string{made}, map[string]string{made: "1554"}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := recordedID(t, resp); got != made {
		t.Errorf("id = %q, want the group the create made (%q)", got, made)
	}
}

// TestCreate_LostResponseKeepsTheHighestJamfProID covers one request making two
// groups, which happened on a production tenant. The highest Jamf Pro ID is kept
// and the other is named for deletion, not deleted.
func TestCreate_LostResponseKeepsTheHighestJamfProID(t *testing.T) {
	const first, second = "ae705e2c-26ef-4d52-88dc-725c69b98bbd", "466f0054-72b2-4af4-a19a-6d06fa27a829"
	resp := createWithLostResponse(t, lostResponseServer(t, "", []string{first, second}, map[string]string{first: "1554", second: "1555"}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := recordedID(t, resp); got != second {
		t.Errorf("id = %q, want the group with the highest Jamf Pro ID (%q)", got, second)
	}
	var named bool
	for _, w := range resp.Diagnostics.Warnings() {
		named = named || strings.Contains(w.Detail(), "Delete the others in Jamf: "+first)
	}
	if !named {
		t.Errorf("no warning names %q for deletion: %v", first, resp.Diagnostics)
	}
}

func TestCreateOutcomeUnknown(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"gateway timeout", &jamfplatform.APIResponseError{StatusCode: http.StatusGatewayTimeout}, true},
		{"server error", &jamfplatform.APIResponseError{StatusCode: http.StatusInternalServerError}, true},
		{"refused as a duplicate", &jamfplatform.APIResponseError{StatusCode: http.StatusConflict}, false},
		{"bad request", &jamfplatform.APIResponseError{StatusCode: http.StatusBadRequest}, false},
		{"lost connection", errors.New("connection reset by peer"), true},
		{"cancelled by the operator", context.Canceled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := createOutcomeUnknown(tc.err); got != tc.want {
				t.Errorf("createOutcomeUnknown = %v, want %v", got, tc.want)
			}
		})
	}
}
