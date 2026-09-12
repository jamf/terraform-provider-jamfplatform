// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// updateTestEbookID is the ebook the Update tests write. Jamf Pro numbers
// ebooks, so this is a small integer rather than a UUID.
const updateTestEbookID = "12"

// updateTestEbookXML is what the stub serves from both GETs Update makes — the
// read-merge-write pre-read and the read-back that refreshes state. The stored
// scope carries no classes, which is the shape the two-request delivery exists
// for: a class list reaches Jamf Pro only while the stored category is empty.
const updateTestEbookXML = `<ebook>
  <general>
    <id>` + updateTestEbookID + `</id>
    <name>Field Guide</name>
    <url>https://example.org/guide.pdf</url>
    <deployment_type>Self Service</deployment_type>
    <free>true</free>
  </general>
  <scope>
    <all_computers>false</all_computers>
    <all_mobile_devices>false</all_mobile_devices>
    <all_jss_users>false</all_jss_users>
    <departments>
      <department><id>9</id><name>Library</name></department>
    </departments>
    <classes/>
  </scope>
</ebook>`

// putRecorder captures the body of every PUT the stub serves, in the order the
// server saw them, which is the only place the two-request ordering is
// observable: UpdateEbookByID answers 201 with an empty body, so a swapped
// pair is indistinguishable from the correct one anywhere else.
//
// The mutex is not ceremony — the handler runs on the stub server's goroutine
// and the assertions run on the test's, and the HTTP round trip between them is
// not a happens-before edge the race detector is obliged to see.
type putRecorder struct {
	mu     sync.Mutex
	bodies []string
}

// record appends one PUT body and reports how many have now been seen.
func (p *putRecorder) record(body string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bodies = append(p.bodies, body)
	return len(p.bodies)
}

// snapshot returns the bodies recorded so far.
func (p *putRecorder) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.bodies...)
}

// updateStubResource returns a resource wired to a stub server answering the
// two GETs Update makes and giving the n-th PUT the n-th status in putStatuses
// (the last entry repeating), so a test can fail either request in isolation.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// account_group's own CRUD tests: Update holds a concrete *proclassic.Client,
// and an interface introduced only for a test would be a bigger change than
// the behaviour it pins. SDK retries are disabled so a failing status records
// exactly one request.
func updateStubResource(t *testing.T, recorder *putRecorder, putStatuses ...int) *EbookResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading the PUT body: %v", err)
			}
			seen := recorder.record(string(body))
			status := putStatuses[min(seen, len(putStatuses))-1]
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(status)
			if status != http.StatusCreated {
				_, _ = w.Write([]byte(`<ebook><error>the write failed</error></ebook>`))
			}
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(updateTestEbookXML))
		default:
			w.WriteHeader(http.StatusNotImplemented)
		}
	}))
	t.Cleanup(server.Close)

	client := jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0))
	return &EbookResource{client: proclassic.New(client)}
}

// runUpdateWithClasses drives Update against such a stub with a plan whose
// scope declares classIDs as its only managed target category, and returns the
// response together with the PUT bodies the stub saw.
func runUpdateWithClasses(t *testing.T, classIDs types.Set, putStatuses ...int) (resource.UpdateResponse, []string) {
	t.Helper()
	ctx := context.Background()
	recorder := &putRecorder{}
	r := updateStubResource(t, recorder, putStatuses...)

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	targets := EbookScopeModel{}.TargetsOrZero()
	targets.ClassIDs = classIDs
	model := EbookResourceModel{
		ID: types.StringValue(updateTestEbookID),
		General: &EbookGeneralModel{
			Name: types.StringValue("Field Guide"),
			URL:  types.StringValue("https://example.org/guide.pdf"),
		},
		Scope:    &EbookScopeModel{Targets: &targets},
		Timeouts: helpers.NewResourceTimeoutsNullValue(ebookTimeoutAttributeTypes),
	}
	seed := tfsdk.State{Schema: schemaResp.Schema}
	if diags := seed.Set(ctx, &model); diags.HasError() {
		t.Fatalf("seeding the update plan: %v", diags)
	}

	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: seed.Raw.Copy()},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: seed.Raw.Copy()},
		State: tfsdk.State{Schema: schemaResp.Schema, Raw: seed.Raw.Copy()},
	}, &resp)
	return resp, recorder.snapshot()
}

// TestUpdate_ScopeWithClassesEmptiesTheCategoryFirst pins the ordering of the
// two-request delivery, which is the whole of issue #428's fix and is invisible
// in a single body: Jamf Pro stores <classes> only while the stored category is
// empty, so the request carrying the change must empty the category and the
// request carrying the members must come second. A swapped pair loses the
// classes on every apply and still answers 201 twice.
func TestUpdate_ScopeWithClassesEmptiesTheCategoryFirst(t *testing.T) {
	resp, bodies := runUpdateWithClasses(t, idSet("41", "42"), http.StatusCreated)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an update the server applied must not raise an error: %v", resp.Diagnostics.Errors())
	}
	if len(bodies) != 2 {
		t.Fatalf("a scope carrying class members must go out as two requests, got %d: %v", len(bodies), bodies)
	}
	if !strings.Contains(bodies[0], "<classes></classes>") || strings.Contains(bodies[0], "<class>") {
		t.Errorf("the first request must empty the class category and carry no members: %s", bodies[0])
	}
	if !strings.Contains(bodies[0], "<department><id>9</id>") {
		t.Errorf("the first request must carry the rest of the merged scope: %s", bodies[0])
	}
	for _, want := range []string{"<class><id>41</id></class>", "<class><id>42</id></class>"} {
		if !strings.Contains(bodies[1], want) {
			t.Errorf("the second request must carry %s: %s", want, bodies[1])
		}
	}
}

// TestUpdate_ScopeWithoutClassesIssuesOneRequest is the control on that
// ordering: the extra write is gated on the category, not the resource, so a
// scope with no class members must cost exactly one PUT. A gate that fired for
// every scope would double every ebook apply's write load against a rate-capped
// classic endpoint.
func TestUpdate_ScopeWithoutClassesIssuesOneRequest(t *testing.T) {
	resp, bodies := runUpdateWithClasses(t, idSet(), http.StatusCreated)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an update the server applied must not raise an error: %v", resp.Diagnostics.Errors())
	}
	if len(bodies) != 1 {
		t.Fatalf("a scope with no class members must go out as one request, got %d: %v", len(bodies), bodies)
	}
	if !strings.Contains(bodies[0], "<classes></classes>") {
		t.Errorf("a declared-empty class category must still land as the explicit clear gesture: %s", bodies[0])
	}
}

// TestUpdate_FailedRestoreReportsTheEmptiedClasses pins the diagnostic the
// second request's failure needs. The first request has already emptied the
// stored class list, so the generic "Error updating Jamf Pro ebook" summary
// would tell an operator nothing was applied at the one moment an unmanaged
// class list has just been destroyed.
func TestUpdate_FailedRestoreReportsTheEmptiedClasses(t *testing.T) {
	resp, bodies := runUpdateWithClasses(t, idSet("41"), http.StatusCreated, http.StatusInternalServerError)

	if len(bodies) != 2 {
		t.Fatalf("expected the clearing request to succeed and the restore to be attempted, got %d: %v", len(bodies), bodies)
	}
	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error diagnostic, got %v", errs)
	}
	if got := errs[0].Summary(); got != "Ebook updated, but its scope classes are now empty in Jamf Pro" {
		t.Fatalf("summary = %q, want the class-loss summary rather than the generic update failure", got)
	}
	if detail := errs[0].Detail(); !strings.Contains(detail, "ebook "+updateTestEbookID) {
		t.Errorf("detail must name the ebook whose classes were emptied: %s", detail)
	}
}

// TestUpdate_FailedSingleRequestReportsTheGenericError is the control on both
// class-loss diagnostics: an ebook whose scope carries no class members loses
// nothing when its one request fails, so it must report the plain update
// failure. Without this, a class-loss message that fired unconditionally — the
// `cleared != nil` guard dropped from either failure branch — would tell every
// operator whose update failed that a category they never managed is now empty.
func TestUpdate_FailedSingleRequestReportsTheGenericError(t *testing.T) {
	resp, bodies := runUpdateWithClasses(t, idSet(), http.StatusInternalServerError)

	if len(bodies) != 1 {
		t.Fatalf("a scope with no class members must go out as one request, got %d: %v", len(bodies), bodies)
	}
	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error diagnostic, got %v", errs)
	}
	if got := errs[0].Summary(); got != "Error updating Jamf Pro ebook" {
		t.Fatalf("summary = %q, want the generic update failure: no class list was at stake", got)
	}
}

// TestUpdate_FailedClearingReportsTheRiskToTheClasses pins the other half of
// that pair. An error return is not proof the server applied nothing — a
// deadline expiring in flight leaves the write committed — so the request whose
// only purpose is emptying the class list must say so, not report the generic
// failure that reads as "nothing was applied".
func TestUpdate_FailedClearingReportsTheRiskToTheClasses(t *testing.T) {
	resp, bodies := runUpdateWithClasses(t, idSet("41"), http.StatusInternalServerError)

	if len(bodies) != 1 {
		t.Fatalf("a failed clearing request must not be followed by the restore, got %d: %v", len(bodies), bodies)
	}
	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error diagnostic, got %v", errs)
	}
	if got := errs[0].Summary(); got != "Error updating Jamf Pro ebook, and its scope classes may now be empty" {
		t.Fatalf("summary = %q, want the class-may-be-empty summary rather than the generic update failure", got)
	}
	if detail := errs[0].Detail(); !strings.Contains(detail, "ebook "+updateTestEbookID) {
		t.Errorf("detail must name the ebook whose classes are at risk: %s", detail)
	}
}
