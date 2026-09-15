// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// writeError makes a refusal the way the transport would, so the diagnostic
// translation is exercised against a real error rather than a hand-built one.
func writeError(t *testing.T, status int, body string) error {
	t.Helper()
	client := newStubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	_, err := client.GetConnection(context.Background(), unitConnectionID)
	if err == nil {
		t.Fatal("the stub was expected to refuse the read")
	}
	return err
}

// TestAppendWriteDiagnostics_UpstreamFailureOnCreatePointsAtTheConfiguration
// pins the create half of the internal-failure diagnostic.
//
// Wire-probed: a create is refused this way for an unclaimed or unverified
// domain, a missing required value, a settings block disagreeing with the
// declared family, an illegal name, and an organization already holding as many
// connections as Jamf allows. So the create diagnostic must work through those
// rather than blame Jamf, which is what an operator can act on. It must also not
// tell them to use the console, because creating works.
func TestAppendWriteDiagnostics_UpstreamFailureOnCreatePointsAtTheConfiguration(t *testing.T) {
	var diags diag.Diagnostics
	if !appendWriteDiagnostics(&diags, actionCreate, writeError(t, http.StatusInternalServerError, upstreamErrorBody)) {
		t.Fatal("an internal failure must be recognised and translated")
	}

	detail := diags.Errors()[0].Detail()
	for _, want := range []string{
		"`domains`",
		"letters and digits only",
		"`connection_type`",
		"as many connections as Jamf Account allows",
		"unit-trace-0001",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not mention %q:\n%s", want, detail)
		}
	}
	for _, unwanted := range []string{
		"known fault in Jamf Account",
		"before the request is examined",
	} {
		if strings.Contains(detail, unwanted) {
			t.Errorf("a refused create must not be attributed to Jamf, but the detail says %q:\n%s", unwanted, detail)
		}
	}
}

// TestAppendWriteDiagnostics_UpstreamFailureOnChangeBlamesJamf pins the change
// half, which is the one that genuinely is Jamf's fault: every update is refused
// this way, including one carrying the exact values a create accepts, so the
// diagnostic points at the console rather than at the configuration.
func TestAppendWriteDiagnostics_UpstreamFailureOnChangeBlamesJamf(t *testing.T) {
	var diags diag.Diagnostics
	if !appendWriteDiagnostics(&diags, actionChange, writeError(t, http.StatusInternalServerError, upstreamErrorBody)) {
		t.Fatal("an internal failure must be recognised and translated")
	}

	detail := diags.Errors()[0].Detail()
	for _, want := range []string{
		"known fault in Jamf Account",
		"Jamf Account console",
		"unit-trace-0001",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not mention %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "every attempt to create") {
		t.Errorf("the change diagnostic must not claim creates are refused too:\n%s", detail)
	}
}

// TestAppendWriteDiagnostics_TranslatesTheTwoAttributedRefusals pins the two
// refusals Jamf does report something useful about. An unrecognised value is
// named in the message but the field it was on never is, so the message is passed
// through verbatim; a missing top-level field is one of the three Jamf names.
func TestAppendWriteDiagnostics_TranslatesTheTwoAttributedRefusals(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"an unrecognised value": {
			http.StatusBadRequest,
			`{"errors":[{"code":"BAD_REQUEST","field":null,"description":"Unsupported region: MARS"}],"httpStatus":400}`,
			"Unsupported region: MARS",
		},
		"a missing required field": {
			http.StatusBadRequest,
			`{"errors":[{"code":"FIELD_VALIDATION","field":"domains","description":"must not be empty"}],"httpStatus":400}`,
			"at least one verified domain name",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			if !appendWriteDiagnostics(&diags, "create", writeError(t, tc.status, tc.body)) {
				t.Fatal("the refusal must be recognised and translated")
			}
			if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, tc.want) {
				t.Errorf("detail does not mention %q:\n%s", tc.want, detail)
			}
		})
	}
}

// TestAppendWriteDiagnostics_LeavesAnUnrecognisedRefusalAlone pins the fall
// through. A code this package does not translate has to be reported by the
// caller with the underlying message, rather than swallowed or dressed up as
// something it is not.
func TestAppendWriteDiagnostics_LeavesAnUnrecognisedRefusalAlone(t *testing.T) {
	var diags diag.Diagnostics
	err := writeError(t, http.StatusForbidden,
		`{"errors":[{"code":"BAD_PERMISSIONS","field":null,"description":"Forbidden"}],"httpStatus":403}`)

	if appendWriteDiagnostics(&diags, "create", err) {
		t.Error("an untranslated code must be left to the caller")
	}
	if len(diags) != 0 {
		t.Errorf("an untranslated code produced %v", diags)
	}
}

// TestAppendWriteDiagnostics_IgnoresANonJamfFailure pins that a transport
// failure — no response body at all — is left to the caller too.
func TestAppendWriteDiagnostics_IgnoresANonJamfFailure(t *testing.T) {
	var diags diag.Diagnostics
	if appendWriteDiagnostics(&diags, "create", context.Canceled) {
		t.Error("a failure with no Jamf response must be left to the caller")
	}
}

// TestAppendConsentFlowDiagnostics pins the refusal and, just as importantly, that
// it does not fire on an ordinary connection.
func TestAppendConsentFlowDiagnostics(t *testing.T) {
	var diags diag.Diagnostics
	if appendConsentFlowDiagnostics(&diags, nil) {
		t.Error("nothing must be refused for an absent connection")
	}
	if appendConsentFlowDiagnostics(&diags, oidcConnectionRead()) {
		t.Error("an ordinary connection must not be refused")
	}

	consent := oidcConnectionRead()
	consent.ConsentFlow = true
	consent.Name = "tf-unit-consent"
	if !appendConsentFlowDiagnostics(&diags, consent) {
		t.Fatal("a connection using Microsoft admin consent must be refused")
	}
	detail := diags.Errors()[0].Detail()
	for _, want := range []string{"tf-unit-consent", "no client registration of its own", "terraform state rm"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not mention %q:\n%s", want, detail)
		}
	}
}

// TestAppendConsentFlowUpdateDiagnostics pins the update refusal, which is keyed
// on state rather than on how Jamf answers — a wire-keyed check could not tell
// this apart from the fault refusing every write.
func TestAppendConsentFlowUpdateDiagnostics(t *testing.T) {
	var diags diag.Diagnostics
	appendConsentFlowUpdateDiagnostics(&diags, "tf-unit-consent")

	if !diags.HasError() {
		t.Fatal("changing such a connection must be refused")
	}
	detail := diags.Errors()[0].Detail()
	for _, want := range []string{"tf-unit-consent", "no request was made", "terraform state rm"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not mention %q:\n%s", want, detail)
		}
	}
}

// TestAppendGhostConnectionDiagnostics pins the diagnostic for the second class
// of unmanageable connection, including the part a reader needs most: why
// Terraform kept it rather than planning to make it again.
func TestAppendGhostConnectionDiagnostics(t *testing.T) {
	var diags diag.Diagnostics
	appendGhostConnectionDiagnostics(&diags, "con_unittest0003", "tf-unit-ghost")

	if !diags.HasError() {
		t.Fatal("a connection Jamf lists but cannot read must be reported")
	}
	detail := diags.Errors()[0].Detail()
	for _, want := range []string{"con_unittest0003", "tf-unit-ghost", "duplicate", "Jamf Support"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not mention %q:\n%s", want, detail)
		}
	}
}

// TestFindSummary pins the collection lookup by identifier.
func TestFindSummary(t *testing.T) {
	summaries := []account.ConnectionSummary{*oidcSummaryRead(), {ID: "con_unittest0005"}}

	if got := findSummary(summaries, unitConnectionID); got == nil || got.ID != unitConnectionID {
		t.Errorf("findSummary = %+v, want the matching entry", got)
	}
	if got := findSummary(summaries, "con_unittestabsent"); got != nil {
		t.Errorf("findSummary = %+v, want nothing", got)
	}
	if got := findSummary(nil, unitConnectionID); got != nil {
		t.Errorf("findSummary over an empty collection = %+v, want nothing", got)
	}
}

// TestFindSummariesByName pins that a name lookup returns every match rather
// than the first. The stored name is not a unique key, and a caller reporting an
// ambiguity is more useful than one silently picking.
func TestFindSummariesByName(t *testing.T) {
	summaries := []account.ConnectionSummary{
		{ID: "con_unittest0001", Name: unitConnectionName},
		{ID: "con_unittest0004", Name: unitConnectionName},
		{ID: "con_unittest0005", Name: "tf-unit-other"},
	}

	if got := findSummariesByName(summaries, unitConnectionName); len(got) != 2 {
		t.Errorf("findSummariesByName returned %d entries, want both namesakes", len(got))
	}
	if got := findSummariesByName(summaries, "tf-unit-absent"); len(got) != 0 {
		t.Errorf("findSummariesByName returned %v, want nothing", got)
	}
}

// TestSummaryNames pins the rendering a diagnostic uses to let an operator pick
// between namesakes, sorted so the list is stable.
func TestSummaryNames(t *testing.T) {
	got := summaryNames([]account.ConnectionSummary{
		{ID: "con_unittest0004", Name: "zebra"},
		{ID: "con_unittest0001", Name: "apple"},
	})
	if got != `"apple" (con_unittest0001), "zebra" (con_unittest0004)` {
		t.Errorf("summaryNames = %q, want a sorted list naming each identifier", got)
	}
}

// TestSplitFilterGroups pins the read half of the comma-separated group list,
// including the case that carries meaning: an empty string is no groups, not one
// empty group.
func TestSplitFilterGroups(t *testing.T) {
	cases := map[string][]string{
		"":                {},
		"   ":             {},
		"jamf":            {"jamf"},
		"jamf,admins":     {"jamf", "admins"},
		" jamf , admins ": {"jamf", "admins"},
		"jamf,,admins":    {"jamf", "admins"},
		"jamf,admins,":    {"jamf", "admins"},
	}

	for input, want := range cases {
		got := splitFilterGroups(input)
		if len(got) != len(want) {
			t.Errorf("splitFilterGroups(%q) = %v, want %v", input, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("splitFilterGroups(%q) = %v, want %v", input, got, want)
				break
			}
		}
	}
}

// TestJoinFilterGroups pins the write half, and the sort that makes it
// deterministic: the attribute is a set, whose members reach the provider in an
// arbitrary order.
func TestJoinFilterGroups(t *testing.T) {
	if got := joinFilterGroups([]string{"zebra", "apple", "mango"}); got != "apple,mango,zebra" {
		t.Errorf("joinFilterGroups = %q, want the members sorted", got)
	}
	if got := joinFilterGroups(nil); got != "" {
		t.Errorf("joinFilterGroups(nil) = %q, want an empty list", got)
	}
}

// TestFilterGroupsRoundTrip is the statement the two tests above imply but do not
// make: whatever set goes out has to come back.
func TestFilterGroupsRoundTrip(t *testing.T) {
	for _, groups := range [][]string{{}, {"jamf"}, {"jamf", "admins", "everyone"}} {
		back := splitFilterGroups(joinFilterGroups(groups))
		if len(back) != len(groups) {
			t.Errorf("%v round-tripped to %v", groups, back)
		}
	}
}

// TestEquivalentJSON pins the comparison the claim-mapping reconcile rests on,
// including the malformed case: a document that will not parse is compared
// byte-wise so it is at least stable against itself, and the validator is what
// reports it against the right attribute.
func TestEquivalentJSON(t *testing.T) {
	cases := map[string]struct {
		left, right string
		want        bool
	}{
		"reordered keys":      {`{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		"reindented":          {`{"a":1}`, "{\n  \"a\": 1\n}", true},
		"numbers by value":    {`{"a":1}`, `{"a":1.0}`, true},
		"different values":    {`{"a":1}`, `{"a":2}`, false},
		"extra key":           {`{"a":1}`, `{"a":1,"b":2}`, false},
		"nested equal":        {`{"a":{"b":[1,2]}}`, `{"a":{"b":[1,2]}}`, true},
		"nested reordered":    {`{"a":{"b":[1,2]}}`, `{"a":{"b":[2,1]}}`, false},
		"malformed and same":  {`not json`, `not json`, true},
		"malformed differing": {`not json`, `also not json`, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := equivalentJSON(tc.left, tc.right); got != tc.want {
				t.Errorf("equivalentJSON(%q, %q) = %v, want %v", tc.left, tc.right, got, tc.want)
			}
		})
	}
}

// TestStringOrNull pins that an empty value from Jamf is reported as absent,
// which is how Jamf reports a field it holds no value for.
func TestStringOrNull(t *testing.T) {
	if got := stringOrNull(nil); !got.IsNull() {
		t.Errorf("stringOrNull(nil) = %s, want nothing", got)
	}
	if got := stringOrNull(new("")); !got.IsNull() {
		t.Errorf("stringOrNull(empty) = %s, want nothing", got)
	}
	if got := stringOrNull(new("value")); got.ValueString() != "value" {
		t.Errorf("stringOrNull = %s", got)
	}
}

// TestBoolAndInt64OrNull pins that an absent boolean or number stays absent
// rather than becoming false or zero, which would read as a setting Jamf had
// turned off.
func TestBoolAndInt64OrNull(t *testing.T) {
	if got := boolOrNull(nil); !got.IsNull() {
		t.Errorf("boolOrNull(nil) = %s, want nothing", got)
	}
	if got := boolOrNull(new(false)); got.IsNull() || got.ValueBool() {
		t.Errorf("boolOrNull(false) = %s, want a known false", got)
	}
	if got := int64OrNull(nil); !got.IsNull() {
		t.Errorf("int64OrNull(nil) = %s, want nothing", got)
	}
	if got := int64OrNull(new(0)); got.IsNull() || got.ValueInt64() != 0 {
		t.Errorf("int64OrNull(0) = %s, want a known zero", got)
	}
}

// TestSetToStrings pins that an absent set reads as an empty collection rather
// than nothing, so a payload field Jamf requires is never sent as an absence.
func TestSetToStrings(t *testing.T) {
	ctx := context.Background()

	for name, set := range map[string]types.Set{
		"absent":  types.SetNull(types.StringType),
		"unknown": types.SetUnknown(types.StringType),
	} {
		got, diags := setToStrings(ctx, set)
		if diags.HasError() {
			t.Fatalf("%s: %v", name, diags)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("%s set gave %v, want an empty collection", name, got)
		}
	}

	got, diags := setToStrings(ctx, mustStringSet("one", "two"))
	if diags.HasError() {
		t.Fatalf("reading a populated set: %v", diags)
	}
	if len(got) != 2 {
		t.Errorf("setToStrings = %v, want both members", got)
	}
}

// TestStringsToSet pins that the result is always known, which is what lets a
// read-only collection resolve at apply rather than staying unknown.
func TestStringsToSet(t *testing.T) {
	set, diags := stringsToSet(nil)
	if diags.HasError() {
		t.Fatalf("building an empty set: %v", diags)
	}
	if set.IsNull() || set.IsUnknown() {
		t.Errorf("stringsToSet(nil) = %s, want a known empty set", set)
	}
	if len(set.Elements()) != 0 {
		t.Errorf("stringsToSet(nil) = %s, want no members", set)
	}
}

// TestTraceIDOrUnknown pins that the diagnostic always has something to tell an
// operator to quote, even when the refusal carried no trace identifier.
func TestTraceIDOrUnknown(t *testing.T) {
	var diags diag.Diagnostics
	err := writeError(t, http.StatusInternalServerError,
		`{"errors":[{"code":"UPSTREAM_ERROR","field":null,"description":"The request could not be completed"}],"httpStatus":500}`)

	if !appendWriteDiagnostics(&diags, "create", err) {
		t.Fatal("the refusal must be recognised")
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "quote the trace identifier") {
		t.Errorf("detail does not say what to quote:\n%s", detail)
	}
}
