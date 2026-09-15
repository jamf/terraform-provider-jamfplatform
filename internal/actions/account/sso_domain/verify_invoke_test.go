// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// newStubClient returns a Jamf Account client pointed at a stub server driven by
// handle, which answers the token exchange itself.
//
// The seam is the HTTP boundary rather than an injected interface: the action
// holds a concrete *account.Client, and an interface introduced only for a test
// would be a bigger change than the behaviour it pins. The stub is local rather
// than testhelpers.NewMockClient because testhelpers reaches the provider package
// under the acceptance build tag, and the provider registers this action —
// importing it from an in-package test makes that a cycle.
func newStubClient(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) *account.Client {
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
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	return account.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// domainBody is the Domain body a verification answers with, carrying the status
// the outcome has to be read off.
func domainBody(status string) string {
	return `{
		"id": "26917",
		"createdByName": null,
		"accountId": "001ABCDEFGHIJKLMNO",
		"domain": "tf-unit.example",
		"verificationKey": "verification-key-claim",
		"domainStatus": "` + status + `",
		"createdDate": "2026-09-02T12:33:32.658Z",
		"lastModifiedDate": "2026-09-02T12:33:32.658Z",
		"lastVerificationDate": null,
		"verificationExpirationDate": "2026-09-16T12:33:32.658Z",
		"sharedDomain": false,
		"verifiedTldId": null
	}`
}

// domainListBody is the collection a name lookup scans.
const domainListBody = `{"totalCount":1,"results":[` + `{
	"id": "26917",
	"accountId": "001ABCDEFGHIJKLMNO",
	"domain": "tf-unit.example",
	"verificationKey": "verification-key-claim",
	"domainStatus": "PENDING",
	"sharedDomain": false
}` + `]}`

// invokeConfig builds an action configuration setting exactly one identifier,
// which is what the ConfigValidator guarantees Invoke receives.
func invokeConfig(ctx context.Context, s actionschema.Schema, attribute, value string) tfsdk.Config {
	object := s.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		if name == attribute {
			values[name] = tftypes.NewValue(attributeType, value)
			continue
		}
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(object, values)}
}

// invokeVerify runs Invoke against a stub driven by handle, with the named
// identifier configured, and returns the response together with every progress
// message it sent.
//
// SendProgress is supplied by the framework in a real run, so a test that leaves
// it nil panics on the first report rather than failing an assertion.
func invokeVerify(t *testing.T, attribute, value string, handle func(w http.ResponseWriter, r *http.Request)) (action.InvokeResponse, []string) {
	t.Helper()
	ctx := context.Background()

	a := &VerifySSODomainAction{}
	a.client = newStubClient(t, handle)

	var schemaResp action.SchemaResponse
	a.Schema(ctx, action.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	var progress []string
	resp := action.InvokeResponse{
		SendProgress: func(event action.InvokeProgressEvent) {
			progress = append(progress, event.Message)
		},
	}
	a.Invoke(ctx, action.InvokeRequest{Config: invokeConfig(ctx, schemaResp.Schema, attribute, value)}, &resp)
	return resp, progress
}

// containsFragment reports whether any message carries fragment.
func containsFragment(messages []string, fragment string) bool {
	for _, message := range messages {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

// TestInvoke_VerifiedIsReportedAsProgress is the success path, and the control
// for every failure assertion below: without it a check that "not verified is an
// error" would pass just as well against an action that always errored.
func TestInvoke_VerifiedIsReportedAsProgress(t *testing.T) {
	var calls []string

	resp, progress := invokeVerify(t, "domain_id", "26917", func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(domainBody(string(account.DomainStatusVerified))))
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("a verified domain must not fail the run: %v", resp.Diagnostics)
	}
	if len(calls) != 1 || calls[0] != "POST /sso/v1/domains/26917/actions/verify" {
		t.Errorf("invoke issued %v, want the verification alone — a configured identifier skips the lookup", calls)
	}
	if !containsFragment(progress, "is verified") {
		t.Errorf("progress %v does not report the outcome", progress)
	}
	if !containsFragment(progress, "lapses on 2026-09-16T12:33:32Z") {
		t.Errorf("progress %v does not report when the verification lapses", progress)
	}
}

// TestInvoke_AlreadyVerifiedIsNotAFailure pins the short-circuit ahead of the
// generic diagnostic. Jamf refuses to re-check a verified domain with 409, and
// reporting that as an error would make the action fail on its second run for
// having succeeded on its first — which no re-runnable pipeline can live with.
func TestInvoke_AlreadyVerifiedIsNotAFailure(t *testing.T) {
	resp, progress := invokeVerify(t, "domain_id", "26917", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"errors":[{"code":"CONFLICT","field":null,"description":"Domain is already verified"}],"httpStatus":409}`))
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("an already-verified domain is the state that was asked for, not a failure: %v", resp.Diagnostics)
	}
	if !containsFragment(progress, "already verified") {
		t.Errorf("progress %v does not say why nothing was checked", progress)
	}
}

// TestInvoke_UnprovenOwnershipIsAnError pins the decision the whole action turns
// on: Jamf reports "checked, and ownership is still not proven" exactly the way
// it reports success — 200 with the full body and the status untouched — so the
// status has to be classified and an unproven domain has to fail the run.
func TestInvoke_UnprovenOwnershipIsAnError(t *testing.T) {
	resp, _ := invokeVerify(t, "domain_id", "26917", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(domainBody(string(account.DomainStatusPending))))
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a 200 carrying an unchanged status must not be reported as success")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Domain ownership was not verified" {
		t.Errorf("summary = %q", got)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "verification_txt_record") {
		t.Errorf("detail %q does not name the record to publish", detail)
	}
}

// TestInvoke_UnrecognisedStatusIsNeitherOutcome pins the third branch. A status
// this provider does not know is deliberately not folded into either of the
// others: read as verified it would report success on a domain nothing has
// proven, and read as unverified it would fail an apply on a status that may well
// mean success.
func TestInvoke_UnrecognisedStatusIsNeitherOutcome(t *testing.T) {
	resp, _ := invokeVerify(t, "domain_id", "26917", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(domainBody("SOMETHING_JAMF_ADDED_LATER")))
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a status the provider cannot classify must be reported, not treated as proven")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Jamf Account reported a verification status this provider does not recognise" {
		t.Errorf("summary = %q", got)
	}
}

// TestInvoke_RateLimitIsNamedRatherThanRelayed pins the translation of the
// refusal every first verification after a claim hits. The raw body says only
// "Can only verify once every five minutes" with field null, which reads like a
// provider bug rather than something to wait out.
func TestInvoke_RateLimitIsNamedRatherThanRelayed(t *testing.T) {
	resp, _ := invokeVerify(t, "domain_id", "26917", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"BAD_REQUEST","field":null,"description":"Can only verify once every five minutes"}],"httpStatus":400}`))
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused verification must be reported")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Jamf Account allows only one domain verification every five minutes" {
		t.Errorf("summary = %q, want the named rate-limit diagnostic rather than the generic refusal", got)
	}
}

// TestInvoke_ResolvesADomainNamedByName pins the lookup a name needs. Nothing
// resolves a domain name directly — there is no per-domain read route and no
// route takes a name — so naming one costs a scan of the collection before the
// verification can be issued.
func TestInvoke_ResolvesADomainNamedByName(t *testing.T) {
	var calls []string

	resp, _ := invokeVerify(t, "domain", "TF-Unit.example", func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodGet {
			_, _ = w.Write([]byte(domainListBody))
			return
		}
		_, _ = w.Write([]byte(domainBody(string(account.DomainStatusVerified))))
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("invoke diagnostics: %v", resp.Diagnostics)
	}
	want := []string{"GET /sso/v1/domains", "POST /sso/v1/domains/26917/actions/verify"}
	if len(calls) != len(want) || calls[0] != want[0] || calls[1] != want[1] {
		t.Errorf("invoke issued %v, want %v — the match is case-insensitive because Jamf stores a claim lower-cased", calls, want)
	}
}

// TestInvoke_UnclaimedDomainNamesTheAttribute pins the other end of that lookup:
// a name the organization has not claimed must land on `domain`, the value the
// practitioner can change, and issue no verification.
func TestInvoke_UnclaimedDomainNamesTheAttribute(t *testing.T) {
	var calls []string

	resp, _ := invokeVerify(t, "domain", "tf-unit-absent.example", func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(domainListBody))
	})

	if !resp.Diagnostics.HasError() {
		t.Fatal("a domain the organization has not claimed must be reported")
	}
	if len(calls) != 1 {
		t.Errorf("invoke issued %v, want the collection scan alone", calls)
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Domain is not claimed by this organization" {
		t.Errorf("summary = %q", got)
	}
}
