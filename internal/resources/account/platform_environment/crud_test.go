// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// exchange is one recorded request.
type exchange struct {
	method string
	path   string
	body   string
}

// newStubClient returns a Jamf Account client pointed at a stub server driven by
// handle.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// sso_domain's crud_test.go: the CRUD methods hold a concrete *account.Client,
// and the stub is local rather than testhelpers.NewMockClient because that
// package reaches the provider package under the acceptance build tag, and this
// package is one the provider registers.
func newStubClient(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body string)) *account.Client {
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
		raw, _ := io.ReadAll(r.Body)
		handle(w, r, string(raw))
	}))
	t.Cleanup(server.Close)
	return account.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

const (
	sandboxEnvironmentBody = `{"id":"env-1","href":"/v1/environments/env-1","name":"tf-unit","label":"SANDBOX","metaRegion":"EU"}`
	globalEnvironmentBody  = `{"id":"env-2","href":"/v1/environments/env-2","name":"tf-unit-global","label":"TRIAL","metaRegion":null}`

	oneEnvironmentListBody  = `{"totalCount":1,"results":[` + sandboxEnvironmentBody + `]}`
	twoEnvironmentsListBody = `{"totalCount":2,"results":[` + sandboxEnvironmentBody + `,` + globalEnvironmentBody + `]}`
	emptyListBody           = `{"totalCount":0,"results":[]}`

	serverFailureBody = `{"httpStatus":500,"traceId":"t","errors":[{"code":"UPSTREAM_ERROR","field":null,"description":"The request could not be completed"}]}`
)

// tenantListBody builds a tenant collection from (id, name, environmentId)
// triples, with an empty environment id meaning unassigned.
func tenantListBody(rows ...[3]string) string {
	items := make([]string, 0, len(rows))
	for _, row := range rows {
		environment := "null"
		if row[2] != "" {
			environment = `"` + row[2] + `"`
		}
		items = append(items, `{"id":"`+row[0]+`","name":"`+row[1]+`","product":"pro","type":"management","metaRegion":"EU","environmentId":`+environment+`}`)
	}
	return `{"totalCount":` + strconv.Itoa(len(rows)) + `,"results":[` + strings.Join(items, ",") + `]}`
}

// objectRaw builds the raw object for a schema with the given string attributes
// set. Attributes absent from set take fill, which is nil (null) or
// tftypes.UnknownValue, and timeouts is always null.
func objectRaw(ctx context.Context, typ attr.Type, set map[string]string, fill any) tftypes.Value {
	object := typ.TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		switch value, ok := set[name]; {
		case name == "timeouts":
			values[name] = tftypes.NewValue(attributeType, nil)
		case ok:
			values[name] = tftypes.NewValue(attributeType, value)
		default:
			values[name] = tftypes.NewValue(attributeType, fill)
		}
	}
	return tftypes.NewValue(object, values)
}

// resourceUnderTest wires a stub client into the resource and returns it with its
// schema and identity, so each test states only the exchange it cares about.
func resourceUnderTest(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, body string)) (*PlatformEnvironmentResource, resourceschema.Schema, *tfsdk.ResourceIdentity) {
	t.Helper()
	ctx := context.Background()
	r := &PlatformEnvironmentResource{client: newStubClient(t, handle)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	return r, schemaResp.Schema, &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema}
}

// serve answers every request with the status and body given.
func serve(status int, body string) func(w http.ResponseWriter, r *http.Request, body string) {
	return func(w http.ResponseWriter, _ *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// recorder wraps a handler so a test can read back every request it saw.
func recorder(calls *[]exchange, handle func(w http.ResponseWriter, r *http.Request, body string)) func(w http.ResponseWriter, r *http.Request, body string) {
	return func(w http.ResponseWriter, r *http.Request, body string) {
		*calls = append(*calls, exchange{method: r.Method, path: r.URL.Path, body: body})
		handle(w, r, body)
	}
}

func describeCalls(calls []exchange) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.method + " " + c.path
	}
	return out
}

func TestCreate_AdoptsTheCreateResponse(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, identity := resourceUnderTest(t, recorder(&calls, serve(http.StatusCreated, sandboxEnvironmentBody)))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: objectRaw(ctx, s.Type(), map[string]string{"name": "tf-unit", "type": "SANDBOX", "region": "EU"}, tftypes.UnknownValue)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if len(calls) != 1 || calls[0].method != "POST" || calls[0].path != "/organization/v1/environments" {
		t.Fatalf("create issued %v, want exactly one create call", describeCalls(calls))
	}
	if !strings.Contains(calls[0].body, `"metaRegion":"EU"`) || !strings.Contains(calls[0].body, `"label":"SANDBOX"`) {
		t.Errorf("create body %s does not carry the type and region", calls[0].body)
	}

	var state PlatformEnvironmentResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if state.ID.ValueString() != "env-1" || state.Type.ValueString() != "SANDBOX" || state.Region.ValueString() != "EU" {
		t.Errorf("state = %+v, want the create response adopted", state)
	}
}

func TestCreate_OmitsAnUnsetRegion(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, identity := resourceUnderTest(t, recorder(&calls, serve(http.StatusCreated, globalEnvironmentBody)))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: objectRaw(ctx, s.Type(), map[string]string{"name": "tf-unit-global", "type": "TRIAL"}, nil)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if strings.Contains(calls[0].body, "metaRegion") {
		t.Errorf("create body %s sends a region, which would pin a global environment", calls[0].body)
	}
	var state PlatformEnvironmentResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if !state.Region.IsNull() {
		t.Errorf("region = %v, want null for a global environment", state.Region)
	}
}

func TestCreate_Failure(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, serve(http.StatusBadRequest, `{"httpStatus":400,"errors":[{"code":"FIELD_VALIDATION","field":"name","description":"Environment name is required"}]}`))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: objectRaw(ctx, s.Type(), map[string]string{"name": "x", "type": "SANDBOX"}, nil)},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused create must be reported")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a refused create must write no state")
	}
}

func TestRead_MatchesTheCollectionOnTheIdentifier(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, identity := resourceUnderTest(t, recorder(&calls, serve(http.StatusOK, twoEnvironmentsListBody)))

	state := objectRaw(ctx, s.Type(), map[string]string{"id": "env-2", "name": "stale", "type": "BETA"}, nil)
	resp := resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}, Identity: identity}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if len(calls) != 1 || calls[0].method != "GET" || calls[0].path != "/organization/v1/environments" {
		t.Errorf("read issued %v, want exactly one collection read", describeCalls(calls))
	}
	var got PlatformEnvironmentResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got.Name.ValueString() != "tf-unit-global" || got.Type.ValueString() != "TRIAL" || !got.Region.IsNull() {
		t.Errorf("read adopted %+v, want the entry with id env-2", got)
	}
}

func TestRead_MissingEnvironmentIsRemovedFromState(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, serve(http.StatusOK, emptyListBody))

	state := objectRaw(ctx, s.Type(), map[string]string{"id": "env-1", "name": "tf-unit", "type": "SANDBOX"}, nil)
	resp := resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}, Identity: identity}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("an environment deleted outside Terraform must be removed from state")
	}
}

func TestRead_ListFailureSurfaces(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, serve(http.StatusInternalServerError, serverFailureBody))

	state := objectRaw(ctx, s.Type(), map[string]string{"id": "env-1"}, nil)
	resp := resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state}, Identity: identity}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed list must not be read as the environment having vanished")
	}
}

func TestUpdate_SendsNameAndTypeThenAdoptsTheListing(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	renamed := `{"id":"env-1","href":"/v1/environments/env-1","name":"tf-unit-renamed","label":"BETA","metaRegion":"EU"}`
	r, s, identity := resourceUnderTest(t, recorder(&calls, func(w http.ResponseWriter, req *http.Request, body string) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"totalCount":1,"results":[` + renamed + `]}`))
	}))

	plan := objectRaw(ctx, s.Type(), map[string]string{"id": "env-1", "name": "tf-unit-renamed", "type": "BETA", "region": "EU"}, nil)
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: plan}, State: tfsdk.State{Schema: s, Raw: plan}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	got := describeCalls(calls)
	if len(got) != 2 || got[0] != "PUT /organization/v1/environments/env-1" || got[1] != "GET /organization/v1/environments" {
		t.Fatalf("update issued %v, want the write then one listing", got)
	}
	if strings.Contains(calls[0].body, "metaRegion") {
		t.Errorf("update body %s sends a region, which Jamf Account ignores", calls[0].body)
	}
	var state PlatformEnvironmentResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if state.Name.ValueString() != "tf-unit-renamed" || state.Type.ValueString() != "BETA" {
		t.Errorf("state = %+v, want the stored values adopted", state)
	}
}

func TestUpdate_EnvironmentGoneIsNamedNotRawFailure(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodPut {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(serverFailureBody))
			return
		}
		_, _ = w.Write([]byte(emptyListBody))
	})

	plan := objectRaw(ctx, s.Type(), map[string]string{"id": "env-1", "name": "n", "type": "BETA"}, nil)
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: plan}, State: tfsdk.State{Schema: s, Raw: plan}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("updating a vanished environment must fail")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "-refresh-only") {
		t.Errorf("detail %q does not name the remedy", detail)
	}
}

func TestUpdate_GenuineFailureSurfacesTheServerError(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodPut {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"httpStatus":400,"errors":[{"code":"FIELD_VALIDATION","field":"name","description":"Environment name is required"}]}`))
			return
		}
		_, _ = w.Write([]byte(oneEnvironmentListBody))
	})

	plan := objectRaw(ctx, s.Type(), map[string]string{"id": "env-1", "name": "n", "type": "BETA"}, nil)
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: plan}, State: tfsdk.State{Schema: s, Raw: plan}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused update must be reported")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "Environment name is required") {
		t.Errorf("detail %q does not carry the server's reason", detail)
	}
}

func deleteState(ctx context.Context, s resourceschema.Schema) tftypes.Value {
	return objectRaw(ctx, s.Type(), map[string]string{"id": "env-1", "name": "tf-unit", "type": "SANDBOX", "region": "EU"}, nil)
}

func TestDelete_CleanPath(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, _ := resourceUnderTest(t, recorder(&calls, func(w http.ResponseWriter, req *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(tenantListBody([3]string{"t-1", "other", "env-9"}, [3]string{"t-2", "free", ""})))
	}))

	state := deleteState(ctx, s)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("delete diagnostics: %v", resp.Diagnostics)
	}
	got := describeCalls(calls)
	if len(got) != 2 || got[0] != "GET /organization/v1/tenants" || got[1] != "DELETE /organization/v1/environments/env-1" {
		t.Errorf("delete issued %v, want the tenant read then the delete", got)
	}
}

func TestDelete_RefusesAnEnvironmentThatHoldsTenants(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, _ := resourceUnderTest(t, recorder(&calls, serve(http.StatusOK, tenantListBody([3]string{"t-1", "held-one", "env-1"}, [3]string{"t-2", "held-two", "env-1"}, [3]string{"t-3", "elsewhere", "env-9"}))))

	state := deleteState(ctx, s)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("destroying an environment that holds tenants must be refused")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"held-one", "held-two", "t-1", "t-2", "terraform state rm"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
	if strings.Contains(detail, "elsewhere") {
		t.Errorf("detail %q names a tenant of another environment", detail)
	}
	for _, call := range calls {
		if call.method == http.MethodDelete {
			t.Errorf("a refused destroy issued %s %s", call.method, call.path)
		}
	}
}

func TestDelete_AlreadyDeletedAnswering500IsSuccess(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, _ := resourceUnderTest(t, recorder(&calls, func(w http.ResponseWriter, req *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(serverFailureBody))
		case strings.HasSuffix(req.URL.Path, "/tenants"):
			_, _ = w.Write([]byte(tenantListBody()))
		default:
			_, _ = w.Write([]byte(emptyListBody))
		}
	}))

	state := deleteState(ctx, s)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an environment already gone must not fail the destroy: %v", resp.Diagnostics)
	}
	if got := describeCalls(calls); len(got) != 3 || got[2] != "GET /organization/v1/environments" {
		t.Errorf("delete issued %v, want the failure followed by a listing", got)
	}
}

func TestDelete_FailureWhileStillListedIsAnError(t *testing.T) {
	ctx := context.Background()

	r, s, _ := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(serverFailureBody))
		case strings.HasSuffix(req.URL.Path, "/tenants"):
			_, _ = w.Write([]byte(tenantListBody()))
		default:
			_, _ = w.Write([]byte(oneEnvironmentListBody))
		}
	})

	state := deleteState(ctx, s)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed delete of an environment that is still listed must be reported")
	}
}

func TestDelete_TenantListFailureSurfaces(t *testing.T) {
	ctx := context.Background()
	var calls []exchange

	r, s, _ := resourceUnderTest(t, recorder(&calls, serve(http.StatusInternalServerError, serverFailureBody)))

	state := deleteState(ctx, s)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("an unreadable tenant list must stop the destroy, not be read as an empty one")
	}
	for _, call := range calls {
		if call.method == http.MethodDelete {
			t.Error("the delete must not be sent when the preflight could not be made")
		}
	}
}
