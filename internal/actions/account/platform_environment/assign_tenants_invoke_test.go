// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platformenvironmentaction

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// fakeOrganization is a stub of the organization API: a fixed set of platform
// environments, and tenants whose environment the write changes. Writes are
// counted so a test can assert that a refused assignment sent nothing.
type fakeOrganization struct {
	mu sync.Mutex

	environments []string
	tenants      map[string]*fakeTenant
	order        []string

	assignStatus    int
	assignBody      string
	assignAppliesTo []string
	landOnFailure   []string
	assignRequests  []string
	listTenantsFail int
	tenantListCalls int
}

type fakeTenant struct {
	name        string
	region      string
	environment string
}

func (f *fakeOrganization) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/auth/token":
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
	case r.Method == http.MethodGet && r.URL.Path == "/organization/v1/environments":
		_, _ = w.Write([]byte(`{"totalCount":` + itoa(len(f.environments)) + `,"results":[` + strings.Join(f.environments, ",") + `]}`))
	case r.Method == http.MethodGet && r.URL.Path == "/organization/v1/tenants":
		f.tenantListCalls++
		if f.listTenantsFail > 0 && f.tenantListCalls >= f.listTenantsFail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"httpStatus":500,"errors":[{"code":"UPSTREAM_ERROR","field":null,"description":"The request could not be completed"}]}`))
			return
		}
		rows := make([]string, 0, len(f.order))
		for _, id := range f.order {
			t := f.tenants[id]
			environment := "null"
			if t.environment != "" {
				environment = `"` + t.environment + `"`
			}
			rows = append(rows, `{"id":"`+id+`","name":"`+t.name+`","product":"pro","type":"management","metaRegion":"`+t.region+`","environmentId":`+environment+`}`)
		}
		_, _ = w.Write([]byte(`{"totalCount":` + itoa(len(rows)) + `,"results":[` + strings.Join(rows, ",") + `]}`))
	case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/tenants"):
		raw, _ := io.ReadAll(r.Body)
		f.assignRequests = append(f.assignRequests, r.URL.Path+" "+string(raw))
		status := f.assignStatus
		if status == 0 {
			status = http.StatusNoContent
		}
		if status == http.StatusNoContent {
			var body struct {
				TenantIDs []string `json:"tenantIds"`
			}
			_ = json.Unmarshal(raw, &body)
			environment := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/organization/v1/environments/"), "/tenants")
			for _, id := range body.TenantIDs {
				if applies := f.assignAppliesTo; applies != nil && !contains(applies, id) {
					continue
				}
				if t, ok := f.tenants[id]; ok {
					t.environment = environment
				}
			}
		}
		if status != http.StatusNoContent {
			environment := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/organization/v1/environments/"), "/tenants")
			for _, id := range f.landOnFailure {
				f.tenants[id].environment = environment
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(f.assignBody))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func contains(values []string, v string) bool {
	return slices.Contains(values, v)
}

func environmentJSON(id, name, region string) string {
	metaRegion := "null"
	if region != "" {
		metaRegion = `"` + region + `"`
	}
	return `{"id":"` + id + `","href":"/v1/environments/` + id + `","name":"` + name + `","label":"SANDBOX","metaRegion":` + metaRegion + `}`
}

// newOrganization builds the standard fixture: a region-pinned platform
// environment (env-eu), a global one (env-global), and four tenants: two
// unassigned in the EU, one unassigned in the US, one already in env-eu and one
// in another platform environment.
func newOrganization() *fakeOrganization {
	return &fakeOrganization{
		environments: []string{
			environmentJSON("env-eu", "Europe", "EU"),
			environmentJSON("env-global", "Anywhere", ""),
			environmentJSON("env-other", "Other", "EU"),
		},
		tenants: map[string]*fakeTenant{
			"t-eu-1":    {name: "eu-one", region: "EU"},
			"t-eu-2":    {name: "eu-two", region: "EU"},
			"t-us-1":    {name: "us-one", region: "US"},
			"t-in":      {name: "already-in", region: "EU", environment: "env-eu"},
			"t-elsewhr": {name: "elsewhere", region: "EU", environment: "env-other"},
		},
		order: []string{"t-eu-1", "t-eu-2", "t-us-1", "t-in", "t-elsewhr"},
	}
}

// invoke runs Invoke against the fake with the given configuration and returns
// the response and every progress message sent.
func invoke(t *testing.T, f *fakeOrganization, environmentID string, tenantIDs []string) (action.InvokeResponse, []string) {
	t.Helper()
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(server.Close)

	a := &AssignTenantsAction{client: account.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))}

	var schemaResp action.SchemaResponse
	a.Schema(ctx, action.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	elements := make([]attr.Value, len(tenantIDs))
	for i, id := range tenantIDs {
		elements[i] = types.StringValue(id)
	}
	set, diags := types.SetValue(types.StringType, elements)
	if diags.HasError() {
		t.Fatalf("building the set: %v", diags)
	}

	object := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	setValue, err := set.ToTerraformValue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]tftypes.Value{}
	for name, attributeType := range object.AttributeTypes {
		switch name {
		case "environment_id":
			values[name] = tftypes.NewValue(attributeType, environmentID)
		case "tenant_ids":
			values[name] = setValue
		default:
			values[name] = tftypes.NewValue(attributeType, nil)
		}
	}

	var progress []string
	resp := action.InvokeResponse{SendProgress: func(event action.InvokeProgressEvent) {
		progress = append(progress, event.Message)
	}}
	a.Invoke(ctx, action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(object, values)}}, &resp)
	return resp, progress
}

func errorText(resp action.InvokeResponse) string {
	var parts []string
	for _, d := range resp.Diagnostics.Errors() {
		parts = append(parts, d.Summary()+"\n"+d.Detail())
	}
	return strings.Join(parts, "\n")
}

func TestInvoke_AssignsUnassignedTenantsAndConfirmsThem(t *testing.T) {
	f := newOrganization()
	resp, progress := invoke(t, f, "env-eu", []string{"t-eu-1", "t-eu-2"})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.assignRequests) != 1 {
		t.Fatalf("sent %v, want exactly one assignment", f.assignRequests)
	}
	request := f.assignRequests[0]
	if !strings.HasPrefix(request, "/organization/v1/environments/env-eu/tenants ") ||
		!strings.Contains(request, `"t-eu-1"`) || !strings.Contains(request, `"t-eu-2"`) {
		t.Errorf("request %q does not carry both tenants to the platform environment", request)
	}
	if f.tenants["t-eu-1"].environment != "env-eu" || f.tenants["t-eu-2"].environment != "env-eu" {
		t.Error("the fake did not record the assignment")
	}
	if len(progress) == 0 || !strings.Contains(progress[0], "cannot be undone") {
		t.Errorf("progress %v does not warn that the assignment is permanent", progress)
	}
}

func TestInvoke_GlobalEnvironmentAcceptsAnyRegion(t *testing.T) {
	f := newOrganization()
	resp, _ := invoke(t, f, "env-global", []string{"t-eu-1", "t-us-1"})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.assignRequests) != 1 {
		t.Fatalf("sent %v, want one assignment", f.assignRequests)
	}
}

func TestInvoke_SkipsTenantsAlreadyInTheEnvironment(t *testing.T) {
	f := newOrganization()
	resp, progress := invoke(t, f, "env-eu", []string{"t-in", "t-eu-1"})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.assignRequests) != 1 || strings.Contains(f.assignRequests[0], "t-in") {
		t.Errorf("sent %v, the tenant already in the platform environment must not be resent", f.assignRequests)
	}
	if !strings.Contains(strings.Join(progress, "\n"), "skipping 1 already in it") {
		t.Errorf("progress %v does not say a tenant was skipped", progress)
	}
}

func TestInvoke_EveryTenantAlreadyInIsASuccessfulNoOp(t *testing.T) {
	f := newOrganization()
	resp, progress := invoke(t, f, "env-eu", []string{"t-in"})

	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if len(f.assignRequests) != 0 {
		t.Errorf("sent %v, want nothing", f.assignRequests)
	}
	if len(progress) != 1 || !strings.Contains(progress[0], "nothing to add") {
		t.Errorf("progress %v", progress)
	}
}

func TestInvoke_RefusesEveryProblemAtOnceAndSendsNothing(t *testing.T) {
	f := newOrganization()
	resp, _ := invoke(t, f, "env-eu", []string{"t-eu-1", "t-us-1", "t-elsewhr", "t-ghost"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a tenant that cannot join must be refused")
	}
	if len(f.assignRequests) != 0 {
		t.Errorf("sent %v, a refused assignment must send nothing, not even the valid tenant", f.assignRequests)
	}
	text := errorText(resp)
	for _, want := range []string{
		"t-ghost: not a tenant of this organization",
		"us-one",
		"hosted in US, but the platform environment is pinned to EU",
		"elsewhere",
		"already in another platform environment (env-other)",
		"3 of the 4 tenants",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostic does not mention %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "eu-one") {
		t.Errorf("diagnostic names a tenant that is fine:\n%s", text)
	}
}

func TestInvoke_UnknownEnvironmentIsNamed(t *testing.T) {
	f := newOrganization()
	resp, _ := invoke(t, f, "env-missing", []string{"t-eu-1"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a platform environment that does not exist must be refused")
	}
	if text := errorText(resp); !strings.Contains(text, "env-missing") || !strings.Contains(text, "jamfplatform_account_platform_environments") {
		t.Errorf("diagnostic %q does not name the identifier and the data source", text)
	}
	if len(f.assignRequests) != 0 {
		t.Errorf("sent %v", f.assignRequests)
	}
}

func TestInvoke_ServerRefusalIsReportedWithTheReason(t *testing.T) {
	f := newOrganization()
	f.assignStatus = http.StatusBadRequest
	f.assignBody = `{"httpStatus":400,"errors":[{"code":"FIELD_VALIDATION","field":"tenantIds","description":"Tenant IDs are required"}]}`
	resp, _ := invoke(t, f, "env-eu", []string{"t-eu-1"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused write must be reported")
	}
	if text := errorText(resp); !strings.Contains(text, "Tenant IDs are required") {
		t.Errorf("diagnostic %q does not carry the server's reason", text)
	}
}

func TestInvoke_AcceptedButNotReportedFails(t *testing.T) {
	f := newOrganization()
	f.assignAppliesTo = []string{"t-eu-1"}
	resp, _ := invoke(t, f, "env-eu", []string{"t-eu-1", "t-eu-2"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a 204 that did not move every tenant must not read as success")
	}
	text := errorText(resp)
	if !strings.Contains(text, "1 tenant(s) do not report") || !strings.Contains(text, "t-eu-2") || strings.Contains(text, "t-eu-1") {
		t.Errorf("diagnostic %q must name only the tenant that did not take", text)
	}
}

func TestInvoke_ReadBackFailureIsNotSuccess(t *testing.T) {
	f := newOrganization()
	f.listTenantsFail = 2
	resp, _ := invoke(t, f, "env-eu", []string{"t-eu-1"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("an assignment that could not be confirmed must not read as success")
	}
	if len(f.assignRequests) != 1 {
		t.Errorf("sent %v, want the one write", f.assignRequests)
	}
	if text := errorText(resp); !strings.Contains(text, "could not be confirmed") {
		t.Errorf("diagnostic %q does not say the outcome is unknown", text)
	}
}

func TestInvoke_PreCheckListFailureSendsNothing(t *testing.T) {
	f := newOrganization()
	f.listTenantsFail = 1
	resp, _ := invoke(t, f, "env-eu", []string{"t-eu-1"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("an unreadable tenant list must stop the action")
	}
	if len(f.assignRequests) != 0 {
		t.Errorf("sent %v, the write must not go ahead without the pre-check", f.assignRequests)
	}
}

func TestInvoke_UnconfiguredClientIsReported(t *testing.T) {
	ctx := context.Background()
	a := &AssignTenantsAction{}
	var schemaResp action.SchemaResponse
	a.Schema(ctx, action.SchemaRequest{}, &schemaResp)

	object := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	resp := action.InvokeResponse{SendProgress: func(action.InvokeProgressEvent) {}}
	a.Invoke(ctx, action.InvokeRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(object, values)}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("an unconfigured action must say so rather than panic")
	}
}

func TestInvoke_FailedWriteReportsTenantsThatLandedAnyway(t *testing.T) {
	f := newOrganization()
	f.assignStatus = http.StatusInternalServerError
	f.assignBody = `{"httpStatus":500,"errors":[{"code":"UPSTREAM_ERROR","field":null,"description":"The request could not be completed"}]}`
	f.landOnFailure = []string{"t-eu-2"}
	resp, _ := invoke(t, f, "env-eu", []string{"t-eu-1", "t-eu-2"})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed write must be reported")
	}
	if text := errorText(resp); !strings.Contains(text, "Some of them are in the platform environment now") {
		t.Errorf("diagnostic %q does not report the tenant that is in", text)
	}
}
