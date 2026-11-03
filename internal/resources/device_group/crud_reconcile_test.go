// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/gatewaystub"
)

const (
	reconcileTestName   = "dg-reconcile"
	reconcileTestFirst  = "11111111-1111-1111-1111-111111111111"
	reconcileTestSecond = "22222222-2222-2222-2222-222222222222"
)

type postHandler func(f *fakeDeviceGroups, body devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter)

// fakeDeviceGroups is a stateful stand-in for the Platform device group API. Its
// POST is a hook so a test can reproduce the failure under test: make the group
// and then fail the request, which is what the real service does to a large
// smart group.
type fakeDeviceGroups struct {
	mu        sync.Mutex
	groups    map[string]devicegroups.DeviceGroupReadRepresentationV1
	post      postHandler
	postCalls int
	listCalls int
}

func newFakeDeviceGroups(post postHandler) *fakeDeviceGroups {
	return &fakeDeviceGroups{groups: map[string]devicegroups.DeviceGroupReadRepresentationV1{}, post: post}
}

func (f *fakeDeviceGroups) store(id string, body devicegroups.DeviceGroupCreateRepresentationV1) {
	f.mu.Lock()
	defer f.mu.Unlock()
	desc := ""
	if body.Description != nil {
		desc = *body.Description
	}
	f.groups[id] = devicegroups.DeviceGroupReadRepresentationV1{
		ID: id, Name: body.Name, Description: desc, DeviceType: body.DeviceType, GroupType: body.GroupType,
		Criteria: body.Criteria,
	}
}

func (f *fakeDeviceGroups) counts() (posts, lists int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.postCalls, f.listCalls
}

func (f *fakeDeviceGroups) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.Contains(r.URL.Path, "/pro/"):
		respondJSON(w, http.StatusNotFound, map[string]any{"httpStatus": 404})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/device-groups"):
		var body devicegroups.DeviceGroupCreateRepresentationV1
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.postCalls++
		f.mu.Unlock()
		f.post(f, body, w)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/device-groups"):
		f.mu.Lock()
		f.listCalls++
		results := make([]devicegroups.DeviceGroupListReadRepresentationV1, 0, len(f.groups))
		for _, g := range f.groups {
			results = append(results, devicegroups.DeviceGroupListReadRepresentationV1{
				ID: g.ID, Name: g.Name, Description: g.Description, DeviceType: g.DeviceType, GroupType: g.GroupType,
			})
		}
		f.mu.Unlock()
		sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
		respondJSON(w, http.StatusOK, map[string]any{"results": results, "hasNext": false, "totalCount": len(results)})
	case r.Method == http.MethodGet:
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		f.mu.Lock()
		g, ok := f.groups[id]
		f.mu.Unlock()
		if !ok {
			respondJSON(w, http.StatusNotFound, map[string]any{"httpStatus": 404})
			return
		}
		respondJSON(w, http.StatusOK, g)
	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func failWith(status int) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		respondJSON(w, status, map[string]any{
			"httpStatus": status,
			"errors":     []map[string]any{{"code": "FAILED", "description": "A resource by the specified name already exists."}},
		})
	}
}

func edgePage(status int, heading string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(gatewaystub.CloudFrontPage(heading)))
	}
}

func createdThenFail(status int, ids ...string) postHandler {
	return func(f *fakeDeviceGroups, body devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter) {
		for _, id := range ids {
			f.store(id, body)
		}
		failWith(status)(w)
	}
}

func failOnly(status int) postHandler {
	return func(_ *fakeDeviceGroups, _ devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter) {
		failWith(status)(w)
	}
}

func succeeds(id string) postHandler {
	return func(f *fakeDeviceGroups, body devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter) {
		f.store(id, body)
		respondJSON(w, http.StatusCreated, map[string]any{"id": id, "href": "/device-groups/" + id})
	}
}

func smartCriteria() *[]devicegroups.DeviceGroupCriteriaRepresentationV1 {
	return &[]devicegroups.DeviceGroupCriteriaRepresentationV1{{
		Order: 0, AttributeName: "Serial Number", Operator: "LIKE", AttributeValue: "DGREPROC", JoinType: "AND",
	}}
}

func fastReconcile(t *testing.T) {
	t.Helper()
	attempts, interval := reconcileAttempts, reconcileInterval
	reconcileAttempts, reconcileInterval = 2, time.Millisecond
	t.Cleanup(func() { reconcileAttempts, reconcileInterval = attempts, interval })
}

// runCreate drives Create for a smart computer group named reconcileTestName.
func runCreate(t *testing.T, f *fakeDeviceGroups) (resource.CreateResponse, *providerdata.Data) {
	t.Helper()
	fastReconcile(t)
	ctx := context.Background()
	c := newMockClient(t, f)
	pd := providerdata.New(c)
	r := &DeviceGroupResource{client: devicegroups.New(c), proClient: pro.New(c), pd: pd}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	model := DeviceGroupResourceModel{
		ID:          types.StringUnknown(),
		JamfProID:   types.StringUnknown(),
		Name:        types.StringValue(reconcileTestName),
		Description: types.StringNull(),
		DeviceType:  types.StringValue("computer"),
		GroupType:   types.StringValue("smart"),
		Criteria: []DeviceGroupCriteriaModel{{
			Order:                 types.Int64Value(0),
			AttributeName:         types.StringValue("Serial Number"),
			Operator:              types.StringValue("like"),
			AttributeValue:        types.StringValue("DGREPROC"),
			JoinType:              types.StringValue("and"),
			HasOpeningParenthesis: types.BoolNull(),
			HasClosingParenthesis: types.BoolNull(),
		}},
		Members:     types.SetNull(types.StringType),
		MemberCount: types.Int64Unknown(),
		Timeouts:    helpers.NewResourceTimeoutsNullValue(deviceGroupTimeoutAttributeTypes),
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("building the plan: %v", diags)
	}
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw.Copy()}

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: plan.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: config}, &resp)
	return resp, pd
}

func stateID(t *testing.T, resp resource.CreateResponse) string {
	t.Helper()
	var id types.String
	if diags := resp.State.GetAttribute(context.Background(), pathRootID(), &id); diags.HasError() {
		t.Fatalf("reading id from state: %v", diags)
	}
	return id.ValueString()
}

func assertGateReleased(t *testing.T, pd *providerdata.Data) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := pd.AcquireDeviceGroupWrite(ctx)
	if err != nil {
		t.Fatalf("Create left the write gate held: %v", err)
	}
	release()
}

func TestCreate_CleanSuccessDoesNotReconcile(t *testing.T) {
	f := newFakeDeviceGroups(succeeds(reconcileTestFirst))
	resp, pd := runCreate(t, f)

	if resp.Diagnostics.HasError() || len(resp.Diagnostics.Warnings()) != 0 {
		t.Fatalf("a clean create must raise nothing: %v", resp.Diagnostics)
	}
	if got := stateID(t, resp); got != reconcileTestFirst {
		t.Fatalf("id = %q, want %q", got, reconcileTestFirst)
	}
	if _, lists := f.counts(); lists != 1 {
		t.Fatalf("a clean create costs one lookup (the pre-create snapshot), got %d", lists)
	}
	assertGateReleased(t, pd)
}

func TestCreate_AdoptsGroupAfterNameExists422(t *testing.T) {
	f := newFakeDeviceGroups(createdThenFail(http.StatusUnprocessableEntity, reconcileTestFirst))
	resp, pd := runCreate(t, f)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a create the server applied must be adopted, not failed: %v", resp.Diagnostics.Errors())
	}
	if got := stateID(t, resp); got != reconcileTestFirst {
		t.Fatalf("id = %q, want the group the failed create made", got)
	}
	warns := resp.Diagnostics.Warnings()
	if len(warns) != 1 || !strings.Contains(warns[0].Summary(), "adopted") {
		t.Fatalf("adoption must be reported as exactly one warning, got %v", warns)
	}
	if posts, _ := f.counts(); posts != 1 {
		t.Fatalf("the provider must never re-send the POST, sent %d", posts)
	}
	assertGateReleased(t, pd)
}

func TestCreate_AdoptsLowestIDAndReportsDuplicatesAfter504(t *testing.T) {
	f := newFakeDeviceGroups(createdThenFail(http.StatusGatewayTimeout, reconcileTestSecond, reconcileTestFirst))
	resp, _ := runCreate(t, f)

	if resp.Diagnostics.HasError() {
		t.Fatalf("duplicates from one request must not fail the create: %v", resp.Diagnostics.Errors())
	}
	if got := stateID(t, resp); got != reconcileTestFirst {
		t.Fatalf("id = %q, want the lowest ID %q so the choice is deterministic", got, reconcileTestFirst)
	}
	warns := resp.Diagnostics.Warnings()
	if len(warns) != 1 || !strings.Contains(warns[0].Detail(), reconcileTestSecond) {
		t.Fatalf("the warning must name the duplicate to delete, got %v", warns)
	}
	if strings.Contains(warns[0].Detail(), reconcileTestFirst+",") {
		t.Fatalf("the adopted group must not be listed as an extra: %s", warns[0].Detail())
	}
	if len(f.groups) != 2 {
		t.Fatalf("duplicates must not be deleted automatically, %d groups remain", len(f.groups))
	}
}

func TestCreate_PreExistingSameNameIsAGenuine422(t *testing.T) {
	f := newFakeDeviceGroups(failOnly(http.StatusUnprocessableEntity))
	f.store(reconcileTestFirst, devicegroups.DeviceGroupCreateRepresentationV1{
		Name: reconcileTestName, DeviceType: "COMPUTER", GroupType: "SMART", Criteria: smartCriteria(),
	})
	resp, pd := runCreate(t, f)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a name taken before the request is a real collision and must fail")
	}
	if len(resp.Diagnostics.Warnings()) != 0 {
		t.Fatalf("nothing may be adopted: %v", resp.Diagnostics.Warnings())
	}
	assertGateReleased(t, pd)
}

func TestCreate_DoesNotAdoptAGroupThatDiffersFromTheRequest(t *testing.T) {
	other := func(f *fakeDeviceGroups, body devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter) {
		body.Criteria = &[]devicegroups.DeviceGroupCriteriaRepresentationV1{{
			Order: 0, AttributeName: "Serial Number", Operator: "LIKE", AttributeValue: "SOMETHING-ELSE", JoinType: "AND",
		}}
		f.store(reconcileTestFirst, body)
		failWith(http.StatusGatewayTimeout)(w)
	}
	resp, _ := runCreate(t, newFakeDeviceGroups(other))

	if !resp.Diagnostics.HasError() {
		t.Fatal("a same-name group with different criteria is someone else's and must not be adopted")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, reconcileTestFirst) || !strings.Contains(detail, "not adopted") {
		t.Fatalf("the error must name the group it declined: %s", detail)
	}
}

func TestCreate_AnyMismatchedCandidateBlocksAdoption(t *testing.T) {
	post := func(f *fakeDeviceGroups, body devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter) {
		f.store(reconcileTestFirst, body)
		body.GroupType = "STATIC"
		body.Criteria = nil
		f.store(reconcileTestSecond, body)
		failWith(http.StatusGatewayTimeout)(w)
	}
	resp, _ := runCreate(t, newFakeDeviceGroups(post))

	if !resp.Diagnostics.HasError() || len(resp.Diagnostics.Warnings()) != 0 {
		t.Fatalf("one matching and one foreign group must adopt nothing: %v", resp.Diagnostics)
	}
}

func TestCreate_NoGroupAppearedReportsTheOriginalError(t *testing.T) {
	f := newFakeDeviceGroups(failOnly(http.StatusBadGateway))
	resp, pd := runCreate(t, f)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a 502 that made nothing must still fail")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); strings.Contains(detail, "not adopted") {
		t.Fatalf("with no candidate there is nothing to decline: %s", detail)
	}
	if _, lists := f.counts(); lists != 1+reconcileAttempts {
		t.Fatalf("expected the snapshot plus %d reconcile attempts, got %d lookups", reconcileAttempts, lists)
	}
	assertGateReleased(t, pd)
}

func TestCreate_DefiniteRejectionIsNotReconciled(t *testing.T) {
	f := newFakeDeviceGroups(failOnly(http.StatusBadRequest))
	resp, _ := runCreate(t, f)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a 400 must fail")
	}
	if _, lists := f.counts(); lists != 1 {
		t.Fatalf("a refused request has nothing to reconcile; expected only the snapshot, got %d lookups", lists)
	}
}

func TestIsAmbiguousCreateFailure(t *testing.T) {
	f := newFakeDeviceGroups(failOnly(http.StatusOK))
	c := newMockClient(t, f)
	client := devicegroups.New(c)
	cases := map[int]bool{
		http.StatusBadRequest:          false,
		http.StatusForbidden:           false,
		http.StatusConflict:            false,
		http.StatusUnprocessableEntity: true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusServiceUnavailable:  true,
		http.StatusGatewayTimeout:      true,
	}
	for status, want := range cases {
		f.post = failOnly(status)
		_, err := client.CreateDeviceGroup(context.Background(), &devicegroups.DeviceGroupCreateRepresentationV1{Name: "x"})
		if err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		if got := isAmbiguousCreateFailure(err); got != want {
			t.Errorf("status %d: ambiguous = %v, want %v", status, got, want)
		}
	}
	if isAmbiguousCreateFailure(context.Canceled) {
		t.Error("an operator interrupt is not ambiguous")
	}
	if !isAmbiguousCreateFailure(context.DeadlineExceeded) {
		t.Error("the create's own deadline leaves the outcome unknown")
	}
}

func TestCriteriaEquivalent(t *testing.T) {
	yes := true
	base := func() *[]devicegroups.DeviceGroupCriteriaRepresentationV1 { return smartCriteria() }
	withParens := base()
	(*withParens)[0].HasOpeningParenthesis = &yes
	lowerJoin := base()
	(*lowerJoin)[0].JoinType = "and"
	otherValue := base()
	(*otherValue)[0].AttributeValue = "other"

	for name, tc := range map[string]struct {
		stored *[]devicegroups.DeviceGroupCriteriaRepresentationV1
		want   bool
	}{
		"identical":         {base(), true},
		"join type case":    {lowerJoin, true},
		"different value":   {otherValue, false},
		"extra parenthesis": {withParens, false},
		"missing":           {nil, false},
		"extra criterion":   {&[]devicegroups.DeviceGroupCriteriaRepresentationV1{(*base())[0], (*base())[0]}, false},
	} {
		if got := criteriaEquivalent(base(), tc.stored); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
	if !criteriaEquivalent(nil, nil) {
		t.Error("no criteria on either side is equivalent")
	}
}

func pathRootID() path.Path { return path.Root("id") }

func TestCreate_AdoptionWarningDoesNotRepeatTheGatewayAdvice(t *testing.T) {
	post := func(f *fakeDeviceGroups, body devicegroups.DeviceGroupCreateRepresentationV1, w http.ResponseWriter) {
		f.store(reconcileTestFirst, body)
		edgePage(http.StatusGatewayTimeout, gatewaystub.CloudFrontGatewayTimeout)(w)
	}
	resp, _ := runCreate(t, newFakeDeviceGroups(post))

	if resp.Diagnostics.HasError() {
		t.Fatalf("a 504 edge page after a successful create must be adopted: %v", resp.Diagnostics.Errors())
	}
	warns := resp.Diagnostics.Warnings()
	if len(warns) != 1 {
		t.Fatalf("expected one warning, got %v", warns)
	}
	if strings.Contains(warns[0].Detail(), "nothing changed") {
		t.Fatalf("the warning must not carry advice that the request changed nothing: %s", warns[0].Detail())
	}
}

// newMockClient serves handler behind a token endpoint with SDK retries off.
// It is local because internal/testhelpers imports the provider, which imports
// this package: an in-package test cannot depend on it.
func newMockClient(t *testing.T, handler http.Handler) *jamfplatform.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			respondJSON(w, http.StatusOK, map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0))
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}
