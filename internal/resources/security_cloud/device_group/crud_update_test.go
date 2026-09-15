// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// requestLog records the method and path of every non-auth request the rename stub
// serves, so a test can assert which routes Update actually called rather than only
// what it did with the answers.
//
// The mutex is not ceremony: the handler runs on the stub server's own goroutine and
// the assertions run on the test's, and the HTTP round trip between them is not a
// happens-before edge the race detector is obliged to see.
type requestLog struct {
	mu      sync.Mutex
	entries []string
}

// record appends one "METHOD path" entry.
func (l *requestLog) record(method, path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, method+" "+path)
}

// snapshot returns the entries recorded so far.
func (l *requestLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

// renameStubClient returns a Jamf Security Cloud client pointed at a stub server that
// answers the rename with putStatus and, when that write succeeds, serves servedName
// from the follow-up read-back.
//
// The two halves are parameterised separately because the failure this pins is the one
// where they disagree: PUT /securitycloud/v2/groups/{id} answers 204 with no body, so
// a handler that accepts a write and drops it is indistinguishable from one that
// applied it until the group is read back through a different operation. Serving a
// different name is how that is manufactured without a tenant.
//
// The seam is the HTTP boundary rather than an injected interface, for the reasons
// createdThenUnreadableClient's doc comment gives — the handlers hold a concrete
// *securitycloud.Client, and testhelpers cannot be imported from an in-package test
// without a cycle through the provider package. Retries are disabled so a 5xx case
// records exactly one request.
func renameStubClient(t *testing.T, groupID, servedName string, putStatus int, log *requestLog) *securitycloud.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		log.record(r.Method, r.URL.Path)
		switch {
		case r.Method == http.MethodPut && putStatus == http.StatusNoContent:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(putStatus)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the rename failed"})
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": groupID, "name": servedName})
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// groupRawUpdatePlan builds an update plan holding the known ID and the configured
// name, which between them are everything Update reads.
func groupRawUpdatePlan(ctx context.Context, groupSchema resourceschema.Schema, groupID, name string) tftypes.Value {
	object := groupSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for attributeName, attributeType := range object.AttributeTypes {
		values[attributeName] = tftypes.NewValue(attributeType, nil)
	}
	values["id"] = tftypes.NewValue(tftypes.String, groupID)
	values["name"] = tftypes.NewValue(tftypes.String, name)
	return tftypes.NewValue(object, values)
}

// runUpdate drives Update against a stub answering the rename with putStatus and
// serving servedName from the read-back, and returns the response together with the
// requests the stub saw.
func runUpdate(t *testing.T, groupID, plannedName, servedName string, putStatus int) (resource.UpdateResponse, []string) {
	t.Helper()
	ctx := context.Background()
	log := &requestLog{}
	r := &DeviceGroupResource{client: renameStubClient(t, groupID, servedName, putStatus, log)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: groupRawUpdatePlan(ctx, schemaResp.Schema, groupID, plannedName)}
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: tfsdk.State{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	return resp, log.snapshot()
}

// TestUpdate_RenameWritesToTheV2RouteAndCommitsTheStoredName pins the happy path of
// the v2 write and its 204-plus-read-back sequence: the rename must land on
// PUT /securitycloud/v2/groups/{id} and the stored name must reach state.
//
// The method and path are asserted, not just the outcome, because both v1 write paths
// were withdrawn with the spec in SDK v0.22.0 and a silent revert to one would
// otherwise only surface on a credentialed acceptance lane. The read-back is asserted
// for the same reason: with the write's error discarded, or the read-back removed,
// this stub still answers and nothing else in the package notices.
func TestUpdate_RenameWritesToTheV2RouteAndCommitsTheStoredName(t *testing.T) {
	ctx := context.Background()
	const (
		groupID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
		renamed = "unit-test-group-renamed"
	)

	resp, requests := runUpdate(t, groupID, renamed, renamed, http.StatusNoContent)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a rename the server applied must not raise an error: %v", resp.Diagnostics.Errors())
	}
	for _, want := range []string{
		"PUT /securitycloud/v2/groups/" + groupID,
		"GET /securitycloud/v1/groups/" + groupID,
	} {
		if !slices.Contains(requests, want) {
			t.Errorf("requests %v do not include %q", requests, want)
		}
	}

	if resp.State.Raw.IsNull() {
		t.Fatal("a successful rename must be recorded in state")
	}
	var state DeviceGroupResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.Name.ValueString(); got != renamed {
		t.Errorf("name = %q, want %q", got, renamed)
	}
	if got := state.ID.ValueString(); got != groupID {
		t.Errorf("id = %q, want %q", got, groupID)
	}
}

// TestUpdate_AcceptedButUnappliedRenameIsRefused pins the guard the 204 makes
// necessary. The route answered 403 on 2026-08-29 and 404 until 2026-09-04, so a
// success status carries no evidence that the handler applied anything; a write the
// service accepts and drops must be reported, never committed from the served value.
func TestUpdate_AcceptedButUnappliedRenameIsRefused(t *testing.T) {
	const (
		groupID  = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
		original = "unit-test-group"
		renamed  = "unit-test-group-renamed"
	)

	resp, _ := runUpdate(t, groupID, renamed, original, http.StatusNoContent)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a 204 the server did not apply must be reported, not treated as a converged rename")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a discarded rename must not be committed to state")
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"PUT /securitycloud/v2/groups/" + groupID, original, renamed, "Retry the apply"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}

// TestUpdate_RefusedRenameIsNotCommitted pins the write's error being reported rather
// than discarded, and that nothing is read back or written once it fails.
func TestUpdate_RefusedRenameIsNotCommitted(t *testing.T) {
	const (
		groupID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
		renamed = "unit-test-group-renamed"
	)

	resp, requests := runUpdate(t, groupID, renamed, renamed, http.StatusInternalServerError)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused rename must be reported as an error")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a refused rename must not be committed to state")
	}
	for _, request := range requests {
		if strings.HasPrefix(request, "GET ") {
			t.Errorf("requests %v read the group back after the write was refused", requests)
			break
		}
	}
}
