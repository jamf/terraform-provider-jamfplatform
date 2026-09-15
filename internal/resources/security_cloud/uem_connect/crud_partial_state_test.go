// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// createdThenUnreadableClient returns a Jamf Security Cloud client pointed at a stub
// server that accepts the create and the settings writes and then fails the read
// back, which is the sequence that leaves an integration on the tenant the handler
// cannot see.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// dns_search_domain's crud_test.go: the handlers hold a concrete
// *securitycloud.Client, and an interface introduced only for a test would be a
// bigger change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package under
// the acceptance build tag, and this package is one of the resources the provider
// registers — importing it from an in-package test makes that a cycle.
func createdThenUnreadableClient(t *testing.T, connectorID string) *securitycloud.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": connectorID})
		case r.Method != http.MethodGet:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the read-back failed"})
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// platformTenantPlan builds a create plan for the platform_tenant form with both
// mapping blocks declared but most of their fields left out.
//
// That combination is the one that fills state with unknowns: the server address is
// resolved from the tenant, and every mapping field is Optional and Computed with no
// default, so UseNonNullStateForUnknown has no prior state to hold on to during a
// create.
func platformTenantPlan(ctx context.Context, connectorSchema resourceschema.Schema) tftypes.Value {
	object := connectorSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	tenantType := object.AttributeTypes["platform_tenant"].(tftypes.Object)
	values["platform_tenant"] = tftypes.NewValue(tenantType, map[string]tftypes.Value{
		"tenant_id": tftypes.NewValue(tftypes.String, "tenant-1"),
	})
	values["uem_vendor"] = tftypes.NewValue(tftypes.String, vendorJamfPro)
	values["enabled"] = tftypes.NewValue(tftypes.Bool, true)
	values["scheduled_sync_enabled"] = tftypes.NewValue(tftypes.Bool, true)
	values["sync_refresh_interval_minutes"] = tftypes.NewValue(tftypes.Number, defaultSyncRefreshIntervalMinutes)
	values["uem_auto_delete_behavior"] = tftypes.NewValue(tftypes.String, defaultUEMAutoDeleteBehaviour)
	values["unmanaged_sync_threshold"] = tftypes.NewValue(tftypes.Number, 0)
	values["device_risk_uem_signaling_enabled"] = tftypes.NewValue(tftypes.Bool, false)
	values["disable_sync_on_auth_error"] = tftypes.NewValue(tftypes.Bool, true)
	values["concurrent_device_sync_enabled"] = tftypes.NewValue(tftypes.Bool, true)

	mappingType := object.AttributeTypes["user_data_field_mapping"].(tftypes.Object)
	values["user_data_field_mapping"] = tftypes.NewValue(mappingType, map[string]tftypes.Value{
		"device_name":  tftypes.NewValue(tftypes.String, deviceNameMappingValues[0]),
		"user_name":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"user_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"phone_number": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(mappingType.AttributeTypes["email"], nil),
	})

	groupType := object.AttributeTypes["group_membership_mapping"].(tftypes.Object)
	values["group_membership_mapping"] = tftypes.NewValue(groupType, map[string]tftypes.Value{
		"enabled":                         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"default_security_cloud_group_id": tftypes.NewValue(tftypes.String, nil),
		"mappings":                        tftypes.NewValue(groupType.AttributeTypes["mappings"], nil),
	})

	values["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	values["uem_server_url"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	return tftypes.NewValue(object, values)
}

// TestCreate_FailedReadBackKeepsTheNewID pins the state committed before the trailing
// read-back. The integration exists on the tenant from the first write onwards, and a
// tenant holds one, so a create that recorded nothing would send the operator's retry
// into the one-per-tenant refusal rather than to convergence.
//
// The fully-known assertion is what makes the diagnostic reachable: Terraform answers
// an unknown value in the state a failed apply returns with an "invalid result object
// after apply" error of its own, which would bury it.
func TestCreate_FailedReadBackKeepsTheNewID(t *testing.T) {
	ctx := context.Background()
	const connectorID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	r := &UEMConnectResource{client: createdThenUnreadableClient(t, connectorID)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	raw := platformTenantPlan(ctx, schemaResp.Schema)
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed read-back must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created integration must be recorded in state, or the retry meets the one-per-tenant refusal")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state UEMConnectResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != connectorID {
		t.Errorf("id = %q, want %q", got, connectorID)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{connectorID, "do not re-create it"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}
