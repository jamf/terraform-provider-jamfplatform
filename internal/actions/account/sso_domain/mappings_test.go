// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// newInvokeResponse returns an InvokeResponse with a SendProgress the framework
// would otherwise supply, collecting the messages so a test can read them.
func newInvokeResponse() (*action.InvokeResponse, *[]string) {
	var messages []string
	resp := &action.InvokeResponse{
		SendProgress: func(event action.InvokeProgressEvent) {
			messages = append(messages, event.Message)
		},
	}
	return resp, &messages
}

// TestClassify_EveryStatus pins the classification of every verification status the
// service can report.
//
// PENDING is the case this whole action is shaped around: the verification answered
// successfully and proved nothing, so it must classify as not verified.
func TestClassify_EveryStatus(t *testing.T) {
	tests := []struct {
		status account.DomainStatus
		want   outcome
	}{
		{account.DomainStatusVerified, outcomeVerified},
		{account.DomainStatusManuallyVerified, outcomeVerified},
		{account.DomainStatusMsVerified, outcomeVerified},
		{account.DomainStatusPending, outcomeNotVerified},
		{account.DomainStatusUnverified, outcomeNotVerified},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			if got := classify(new(tc.status)); got != tc.want {
				t.Errorf("classify(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestClassify_AbsentAndUnknown pins that neither an absent status nor one this
// provider does not know is read as success.
//
// The wire models domainStatus as optional and the SDK types it as a bare string
// alias, so both are reachable without any code change — and reading either as
// verified would report proven ownership that nothing proved.
func TestClassify_AbsentAndUnknown(t *testing.T) {
	if got := classify(nil); got != outcomeUnrecognised {
		t.Errorf("classify(nil) = %v, want outcomeUnrecognised", got)
	}
	if got := classify(new("SOMETHING_NEW")); got != outcomeUnrecognised {
		t.Errorf("classify of an unknown status = %v, want outcomeUnrecognised", got)
	}
	if got := classify(new("")); got != outcomeUnrecognised {
		t.Errorf("classify of an empty status = %v, want outcomeUnrecognised", got)
	}
}

// TestClassify_CoversEverySDKStatus fails when the SDK gains a verification status
// this package does not classify.
//
// Without it a new status would land silently in the unrecognised branch, which
// fails every apply against a domain carrying it — including, if the new status
// means success, a domain that is working fine.
func TestClassify_CoversEverySDKStatus(t *testing.T) {
	for _, status := range account.DomainStatusValues() {
		inVerified := slices.Contains(verifiedStatuses, status)
		inUnverified := slices.Contains(unverifiedStatuses, status)

		switch {
		case inVerified && inUnverified:
			t.Errorf("status %q is classified as both verified and unverified", status)
		case !inVerified && !inUnverified:
			t.Errorf("status %q is not classified; add it to verifiedStatuses or unverifiedStatuses", status)
		}
	}
}

// TestClassify_NoInventedStatuses fails when this package classifies a status the
// SDK does not carry, which would mean a restated or misspelled value.
func TestClassify_NoInventedStatuses(t *testing.T) {
	known := account.DomainStatusValues()
	for _, status := range slices.Concat(verifiedStatuses, unverifiedStatuses) {
		if !slices.Contains(known, status) {
			t.Errorf("classified status %q is not in account.DomainStatusValues()", status)
		}
	}
}

// TestReportOutcome_PendingIsAnError is the crux of this action. Jamf answers a
// verification that proved nothing exactly as it answers one that succeeded, so the
// only thing standing between a practitioner and a green apply on an unverified
// domain is this classification.
func TestReportOutcome_PendingIsAnError(t *testing.T) {
	resp, messages := newInvokeResponse()

	reportOutcome(resp, &account.Domain{
		Domain:       "claimed.example",
		DomainStatus: new(account.DomainStatusPending),
	}, "claimed.example")

	if !resp.Diagnostics.HasError() {
		t.Fatalf("a PENDING domain must fail the invocation, got diagnostics: %v", resp.Diagnostics)
	}
	if len(*messages) != 0 {
		t.Errorf("no success progress should be sent for a PENDING domain, got %v", *messages)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, fragment := range []string{"still not proven", "PENDING", "verification_txt_record", "five minutes"} {
		if !strings.Contains(detail, fragment) {
			t.Errorf("diagnostic does not mention %q:\n%s", fragment, detail)
		}
	}
}

// TestReportOutcome_UnverifiedIsAnError covers the second unproven status, which a
// domain reaches by lapsing rather than by never having verified.
func TestReportOutcome_UnverifiedIsAnError(t *testing.T) {
	resp, _ := newInvokeResponse()

	reportOutcome(resp, &account.Domain{
		Domain:       "lapsed.example",
		DomainStatus: new(account.DomainStatusUnverified),
	}, "lapsed.example")

	if !resp.Diagnostics.HasError() {
		t.Fatal("an UNVERIFIED domain must fail the invocation")
	}
}

// TestReportOutcome_VerifiedReportsProgress pins that a proven domain succeeds
// quietly, naming the status and when the verification lapses.
func TestReportOutcome_VerifiedReportsProgress(t *testing.T) {
	lapse := time.Date(2026, 9, 16, 12, 38, 54, 0, time.UTC)
	resp, messages := newInvokeResponse()

	reportOutcome(resp, &account.Domain{
		Domain:                     "verified.example",
		DomainStatus:               new(account.DomainStatusVerified),
		VerificationExpirationDate: &lapse,
	}, "verified.example")

	if resp.Diagnostics.HasError() {
		t.Fatalf("a VERIFIED domain must not fail: %v", resp.Diagnostics)
	}
	if len(*messages) != 1 {
		t.Fatalf("expected one progress message, got %v", *messages)
	}
	for _, fragment := range []string{"verified.example", "VERIFIED", "2026-09-16T12:38:54Z"} {
		if !strings.Contains((*messages)[0], fragment) {
			t.Errorf("progress message does not mention %q:\n%s", fragment, (*messages)[0])
		}
	}
}

// TestReportOutcome_VerifiedWithoutLapseDate pins that an omitted lapse date leaves
// the message clean rather than rendering a zero time.
func TestReportOutcome_VerifiedWithoutLapseDate(t *testing.T) {
	resp, messages := newInvokeResponse()

	reportOutcome(resp, &account.Domain{
		Domain:       "verified.example",
		DomainStatus: new(account.DomainStatusManuallyVerified),
	}, "verified.example")

	if resp.Diagnostics.HasError() {
		t.Fatalf("a MANUALLY_VERIFIED domain must not fail: %v", resp.Diagnostics)
	}
	if len(*messages) != 1 {
		t.Fatalf("expected one progress message, got %v", *messages)
	}
	if strings.Contains((*messages)[0], "lapses") {
		t.Errorf("no lapse date was reported, so the message must not claim one:\n%s", (*messages)[0])
	}
}

// TestReportOutcome_UnrecognisedStatusIsAnError pins that an unknown status fails
// loudly rather than being read either way.
func TestReportOutcome_UnrecognisedStatusIsAnError(t *testing.T) {
	resp, _ := newInvokeResponse()

	reportOutcome(resp, &account.Domain{
		Domain:       "odd.example",
		DomainStatus: new("SOMETHING_NEW"),
	}, "odd.example")

	if !resp.Diagnostics.HasError() {
		t.Fatal("an unrecognised status must fail the invocation")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "SOMETHING_NEW") {
		t.Errorf("the diagnostic must name the status it could not read:\n%s", resp.Diagnostics.Errors()[0].Detail())
	}
}

// TestReportOutcome_FallsBackToTheConfiguredName pins that a body reporting no
// domain name still produces a message naming something the practitioner wrote.
func TestReportOutcome_FallsBackToTheConfiguredName(t *testing.T) {
	resp, _ := newInvokeResponse()

	reportOutcome(resp, &account.Domain{
		DomainStatus: new(account.DomainStatusPending),
	}, "26917")

	if !resp.Diagnostics.HasError() {
		t.Fatal("a PENDING domain must fail the invocation")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "26917") {
		t.Errorf("the diagnostic must name the configured identifier:\n%s", resp.Diagnostics.Errors()[0].Detail())
	}
}

// apiError builds the error shape the SDK surfaces for a refused request.
func apiError(status int, code, description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "https://us.api.jamfcloud.com/sso/v1/domains/26917/actions/verify",
		Errors:     []jamfplatform.ErrorDetail{{Code: code, Description: description}},
	}
}

// TestAppendInvokeDiagnostics_RateLimited pins the diagnostic for the refusal every
// practitioner meets first: the five-minute limit is measured from the domain's
// last change, and claiming it is a change, so the first verification after a claim
// is always refused.
func TestAppendInvokeDiagnostics_RateLimited(t *testing.T) {
	var diags diag.Diagnostics

	matched := appendInvokeDiagnostics(&diags,
		apiError(400, codeBadRequest, "Can only verify once every five minutes"),
		"claimed.example", path.Root("domain"))

	if !matched {
		t.Fatal("the five-minute refusal must be recognised")
	}
	if !diags.HasError() {
		t.Fatal("the five-minute refusal must produce an error")
	}

	summary := diags.Errors()[0].Summary()
	if !strings.Contains(summary, "five minutes") {
		t.Errorf("summary does not name the limit:\n%s", summary)
	}
	detail := diags.Errors()[0].Detail()
	for _, fragment := range []string{"claimed.example", "Claiming a domain counts as a change", "does not wait"} {
		if !strings.Contains(detail, fragment) {
			t.Errorf("detail does not mention %q:\n%s", fragment, detail)
		}
	}
}

// TestAppendInvokeDiagnostics_OtherBadRequest pins that the rate-limit diagnostic is
// not claimed for every refusal sharing its code.
//
// BAD_REQUEST is also what an invalid domain and a non-numeric identifier come back
// as, with Field null on all three, so the description is the only thing separating
// them — telling a practitioner to wait five minutes for a malformed identifier
// would send them off to wait for nothing.
func TestAppendInvokeDiagnostics_OtherBadRequest(t *testing.T) {
	var diags diag.Diagnostics

	matched := appendInvokeDiagnostics(&diags,
		apiError(400, codeBadRequest, "Invalid domain provided"),
		"not a domain", path.Root("domain"))

	if matched {
		t.Error("an unrelated BAD_REQUEST must not be translated as the five-minute limit")
	}
	if diags.HasError() {
		t.Errorf("an unrecognised refusal must be left to the caller's generic diagnostic: %v", diags)
	}
}

// TestAppendInvokeDiagnostics_NotFound pins that an unknown identifier is blamed on
// the attribute that carried it, and that the message says why one goes stale.
func TestAppendInvokeDiagnostics_NotFound(t *testing.T) {
	var diags diag.Diagnostics

	matched := appendInvokeDiagnostics(&diags,
		apiError(404, codeNotFound, "Unable to find domain by id: 99999999"),
		"99999999", path.Root("domain_id"))

	if !matched {
		t.Fatal("a NOT_FOUND refusal must be recognised")
	}
	if !diags.HasError() {
		t.Fatal("a NOT_FOUND refusal must produce an error")
	}
	if !strings.Contains(diags.Errors()[0].Detail(), "new identifier") {
		t.Errorf("detail does not explain why an identifier goes stale:\n%s", diags.Errors()[0].Detail())
	}
}

// TestAppendInvokeDiagnostics_NonAPIError pins that a transport failure is left to
// the caller rather than mistranslated.
func TestAppendInvokeDiagnostics_NonAPIError(t *testing.T) {
	var diags diag.Diagnostics

	if appendInvokeDiagnostics(&diags, errors.New("dial tcp: connection refused"), "claimed.example", path.Root("domain")) {
		t.Error("a non-API error must not be reported as recognised")
	}
	if diags.HasError() {
		t.Errorf("a non-API error must add no diagnostic: %v", diags)
	}
}

// TestStatusText_AbsentStatus pins that an absent status renders as words rather
// than as an empty gap in a sentence.
func TestStatusText_AbsentStatus(t *testing.T) {
	if got := statusText(nil); got == "" {
		t.Error("statusText(nil) must render something")
	}
	if got := statusText(new("")); got == "" {
		t.Error("statusText of an empty status must render something")
	}
	if got := statusText(new(account.DomainStatusVerified)); got != account.DomainStatusVerified {
		t.Errorf("statusText = %q, want %q", got, account.DomainStatusVerified)
	}
}

// TestResolveDomainID_UsesTheConfiguredIdentifier pins that the identifier form
// reaches the verification without a lookup, which is what lets a caller holding
// only the update permission use it.
//
// The action is zero-valued, so it holds no client: a test that passes proves no
// read was attempted, because attempting one would panic.
func TestResolveDomainID_UsesTheConfiguredIdentifier(t *testing.T) {
	var a ssoDomainAction
	var diags diag.Diagnostics

	id, ok := a.resolveDomainID(context.Background(), VerifySSODomainActionModel{
		Domain:   types.StringNull(),
		DomainID: types.StringValue("  26917  "),
	}, &diags)

	if !ok {
		t.Fatalf("resolving a configured identifier failed: %v", diags)
	}
	if id != "26917" {
		t.Errorf("id = %q, want %q — surrounding whitespace must be trimmed", id, "26917")
	}
}

// TestResolveDomainID_BlankIdentifiers pins that whitespace is refused by name
// rather than falling through to the other form.
//
// A blank domain_id used to fall through to the name lookup, which then reported a
// domain named "" as not claimed — a diagnostic blaming the wrong attribute for a
// mistake in the other one.
func TestResolveDomainID_BlankIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		model VerifySSODomainActionModel
		want  string
	}{
		{
			name:  "blank identifier",
			model: VerifySSODomainActionModel{Domain: types.StringNull(), DomainID: types.StringValue("   ")},
			want:  "domain_id",
		},
		{
			name:  "blank name",
			model: VerifySSODomainActionModel{Domain: types.StringValue("   "), DomainID: types.StringNull()},
			want:  "domain",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var a ssoDomainAction
			var diags diag.Diagnostics

			if _, ok := a.resolveDomainID(context.Background(), tc.model, &diags); ok {
				t.Fatal("a blank identifier must not resolve")
			}
			if !diags.HasError() {
				t.Fatal("a blank identifier must produce an error")
			}
			if got := diags.Errors()[0].(diag.DiagnosticWithPath).Path().String(); got != tc.want {
				t.Errorf("diagnostic blamed %q, want %q", got, tc.want)
			}
		})
	}
}

// TestConfiguredTarget pins which identifier the messages sent before the
// verification answers use: the name when one was given, since it is the only form
// the practitioner recognises, and the identifier otherwise.
func TestConfiguredTarget(t *testing.T) {
	byName := configuredTarget(VerifySSODomainActionModel{
		Domain:   types.StringValue("  claimed.example  "),
		DomainID: types.StringNull(),
	}, "26917")
	if byName != "claimed.example" {
		t.Errorf("target = %q, want %q", byName, "claimed.example")
	}

	byID := configuredTarget(VerifySSODomainActionModel{
		Domain:   types.StringNull(),
		DomainID: types.StringValue("26917"),
	}, "26917")
	if byID != "26917" {
		t.Errorf("target = %q, want %q", byID, "26917")
	}
}

// TestIsAlreadyVerified_Matches covers the 409 that means the domain is in the state
// the operator asked for, which the action reports as success rather than failure.
func TestIsAlreadyVerified_Matches(t *testing.T) {
	err := apiError(409, "CONFLICT", "Domain is already verified")
	if !isAlreadyVerified(err) {
		t.Fatal("a CONFLICT naming an already-verified domain must be recognised")
	}
}

// TestIsAlreadyVerified_IgnoresOtherConflicts is the point of matching the
// description as well as the code: a duplicate domain claim is also CONFLICT with a
// null field, and treating that as success would swallow a real failure.
func TestIsAlreadyVerified_IgnoresOtherConflicts(t *testing.T) {
	err := apiError(409, "CONFLICT", "Domain is already added to your organization")
	if isAlreadyVerified(err) {
		t.Fatal("a CONFLICT that is not about verification must not be read as success")
	}
}

// TestIsAlreadyVerified_NonAPIError guards the nil-APIError path, which a transport
// failure takes.
func TestIsAlreadyVerified_NonAPIError(t *testing.T) {
	if isAlreadyVerified(errors.New("connection reset")) {
		t.Fatal("a non-API error must not be read as success")
	}
}
