// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
)

// The verbatim wire blob from the spike (§1): base64 of
// {"uuid":"E5EFCA3E-892C-4CCE-B2C1-6CA1A32E9153","serverId":"31"}.
const (
	wireBlob = "eyJ1dWlkIjoiRTVFRkNBM0UtODkyQy00Q0NFLUIyQzEtNkNBMUEzMkU5MTUzIiwic2VydmVySWQiOiIzMSJ9"
	wireUUID = "E5EFCA3E-892C-4CCE-B2C1-6CA1A32E9153"
)

// fakeSearcher returns canned results (or an error) for SearchLdapGroupsV1.
type fakeSearcher struct {
	results []pro.LdapGroup
	err     error
}

func (f *fakeSearcher) SearchLdapGroupsV1(_ context.Context, _ string) (*pro.LdapGroupSearchResults, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pro.LdapGroupSearchResults{Results: f.results, TotalCount: len(f.results)}, nil
}

func adminGroup() []pro.LdapGroup {
	return []pro.LdapGroup{{Name: "Admins", ID: "37158", LdapServerID: 31, UUID: wireUUID}}
}

func TestEncodeDSGroupValue_ByteStableWithUI(t *testing.T) {
	got := encodeDSGroupValue(dsGroupRef{UUID: wireUUID, ServerID: "31"})
	if got != wireBlob {
		t.Fatalf("encodeDSGroupValue = %q, want the UI-canonical blob %q", got, wireBlob)
	}
}

func TestParseEncodedValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantOK  bool
		wantErr bool
	}{
		{"valid blob", wireBlob, true, false},
		{"group name with underscore (not base64)", "DataJARLDAPS_JamfPro_Admins", false, false},
		{"short non-b64 name", "Admins", false, false},
		// valid base64 of JSON that is not a DS-group ref → treat as a name
		{"base64 of unrelated json object", b64JSON(`{"foo":"bar"}`), false, false},
		// looked encoded but malformed → error
		{"empty uuid (Okta trap)", b64JSON(`{"uuid":"","serverId":"31"}`), false, true},
		// the uuid is an opaque directory id — an Okta-style id (not a hex UUID) is
		// valid; only emptiness is a defect.
		{"opaque non-hex uuid (Okta)", b64JSON(`{"uuid":"00g14cxwfog4FUbSv698","serverId":"31"}`), true, false},
		{"empty serverId", b64JSON(`{"uuid":"` + wireUUID + `","serverId":""}`), false, true},
		{"serverId only, missing uuid", b64JSON(`{"serverId":"31"}`), false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok, err := ParseEncodedValue(tt.value)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveValue_PassThroughEncoded(t *testing.T) {
	// A searcher that would error if called — proving no resolution happens.
	f := &fakeSearcher{err: errors.New("should not be called")}
	got, err := ResolveValue(context.Background(), f, wireBlob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wireBlob {
		t.Errorf("got %q, want verbatim %q", got, wireBlob)
	}
}

func TestResolveValue_ResolvesName(t *testing.T) {
	f := &fakeSearcher{results: adminGroup()}
	got, err := ResolveValue(context.Background(), f, "Admins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wireBlob {
		t.Errorf("got %q, want %q", got, wireBlob)
	}
}

func TestResolveValue_Errors(t *testing.T) {
	tests := []struct {
		name    string
		f       *fakeSearcher
		value   string
		wantErr error
	}{
		{"not found", &fakeSearcher{results: nil}, "Missing", ldapgroups.ErrGroupNotFound},
		{"ambiguous", &fakeSearcher{results: []pro.LdapGroup{
			{Name: "Admins", LdapServerID: 31, UUID: "u1"},
			{Name: "Admins", LdapServerID: 42, UUID: "u2"},
		}}, "Admins", ldapgroups.ErrAmbiguousGroup},
		{"no uuid", &fakeSearcher{results: []pro.LdapGroup{
			{Name: "Admins", LdapServerID: 31, UUID: ""},
		}}, "Admins", ErrNoUUID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveValue(context.Background(), tt.f, tt.value)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want wrapping %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveValue_MalformedBlobFailsClosed(t *testing.T) {
	f := &fakeSearcher{err: errors.New("should not be called")}
	_, err := ResolveValue(context.Background(), f, b64JSON(`{"uuid":"","serverId":"31"}`))
	if err == nil {
		t.Fatal("expected fail-closed error for malformed encoded value")
	}
}

func TestReadValue(t *testing.T) {
	otherBlob := encodeDSGroupValue(dsGroupRef{UUID: "FA257BC0-5F97-44D8-898E-2F4A19F4A413", ServerID: "31"})

	tests := []struct {
		name  string
		f     *fakeSearcher
		wire  string
		prior string
		want  string
	}{
		{"import (no prior) keeps wire", &fakeSearcher{}, wireBlob, "", wireBlob},
		{"prior base64 equals wire keeps prior", &fakeSearcher{}, wireBlob, wireBlob, wireBlob},
		{"prior base64 differs from wire → drift", &fakeSearcher{}, wireBlob, otherBlob, wireBlob},
		{"prior name still resolves to wire keeps name", &fakeSearcher{results: adminGroup()}, wireBlob, "Admins", "Admins"},
		{"prior name resolves elsewhere → drift", &fakeSearcher{results: adminGroup()}, otherBlob, "Admins", otherBlob},
		{"prior name no longer found (soft) → wire", &fakeSearcher{results: nil}, wireBlob, "Admins", wireBlob},
		{"prior name resolution errors (soft) → wire", &fakeSearcher{err: errors.New("boom")}, wireBlob, "Admins", wireBlob},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadValue(context.Background(), tt.f, tt.wire, tt.prior)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetMembership(t *testing.T) {
	if !IsDSGroupCriterion(dsAssignedUser) {
		t.Error("dsAssignedUser should be in set A")
	}
	if IsDSGroupCriterion("Application Title") {
		t.Error("a normal criterion should not be in set A")
	}
	// The MDM criterion's canonical token is title-case "Mdm", not "MDM".
	if !IsDSGroupCriterion(dsULLIMdm) {
		t.Error("the Mdm criterion should be in set A")
	}
	if IsDSGroupCriterion("User last logged in - MDM directory service group") {
		t.Error("the all-caps MDM form is not a recognised criterion")
	}
	// set B per class.
	if !isAllowedDSGroupCriterion(ObjectTypeComputer, dsULLIComputer) {
		t.Error("computer should accept the ULLI-Computer criterion")
	}
	if isAllowedDSGroupCriterion(ObjectTypeMobile, dsULLIComputer) {
		t.Error("mobile must reject the ULLI-Computer criterion")
	}
	// Mdm is accepted on computer + mobile, rejected on user.
	if !isAllowedDSGroupCriterion(ObjectTypeComputer, dsULLIMdm) {
		t.Error("computer should accept the Mdm criterion")
	}
	if !isAllowedDSGroupCriterion(ObjectTypeMobile, dsULLIMdm) {
		t.Error("mobile should accept the Mdm criterion")
	}
	if isAllowedDSGroupCriterion(ObjectTypeUser, dsULLIMdm) {
		t.Error("user surfaces must reject the Mdm criterion")
	}
	if isAllowedDSGroupCriterion(ObjectTypeUser, dsAssignedUser) {
		t.Error("user surfaces accept only Username, not Assigned User")
	}
	if !isAllowedDSGroupCriterion(ObjectTypeUser, dsUsername) {
		t.Error("user surfaces accept Username")
	}
	if got := supportedDSGroupNames(ObjectTypeUser); len(got) != 1 || got[0] != dsUsername {
		t.Errorf("supportedDSGroupNames(user) = %v, want [Username…]", got)
	}
}

func TestResolveDSGroupCriteria(t *testing.T) {
	f := &fakeSearcher{results: adminGroup()}
	models := []CriterionModel{
		{Name: types.StringValue("Application Title"), Value: types.StringValue("Firefox")},
		{Name: types.StringValue(dsUsername), Value: types.StringValue("Admins")},
	}
	out, authored, diags := ResolveDSGroupCriteria(context.Background(), f, ObjectTypeComputer, models)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if out[0].Value.ValueString() != "Firefox" {
		t.Errorf("non-DS criterion mutated: %q", out[0].Value.ValueString())
	}
	if out[1].Value.ValueString() != wireBlob {
		t.Errorf("DS criterion value = %q, want %q", out[1].Value.ValueString(), wireBlob)
	}
	if authored[wireBlob] != "Admins" {
		t.Errorf("authored map = %v, want %q→Admins", authored, wireBlob)
	}
	// input must not be mutated in place.
	if models[1].Value.ValueString() != "Admins" {
		t.Errorf("input slice was mutated: %q", models[1].Value.ValueString())
	}
}

func TestResolveDSGroupCriteria_FailClosedCrossClass(t *testing.T) {
	f := &fakeSearcher{results: adminGroup()}
	// Assigned User is set A but NOT accepted on the user class (set B).
	models := []CriterionModel{
		{Name: types.StringValue(dsAssignedUser), Value: types.StringValue("Admins")},
	}
	_, _, diags := ResolveDSGroupCriteria(context.Background(), f, ObjectTypeUser, models)
	if !diags.HasError() {
		t.Fatal("expected fail-closed diag for cross-class DS criterion")
	}
}

func TestRestoreAuthoredDSGroupCriteria(t *testing.T) {
	authored := map[string]string{wireBlob: "Admins"}
	flattened := []CriterionModel{
		{Name: types.StringValue(dsUsername), Value: types.StringValue(wireBlob)},
		{Name: types.StringValue("Application Title"), Value: types.StringValue("Firefox")},
	}
	out := RestoreAuthoredDSGroupCriteria(flattened, authored)
	if out[0].Value.ValueString() != "Admins" {
		t.Errorf("restored value = %q, want Admins", out[0].Value.ValueString())
	}
	if out[1].Value.ValueString() != "Firefox" {
		t.Errorf("non-DS criterion changed: %q", out[1].Value.ValueString())
	}
}

func TestReadbackDSGroupCriteria(t *testing.T) {
	f := &fakeSearcher{results: adminGroup()}
	wireModels := []CriterionModel{
		{Name: types.StringValue(dsUsername), Value: types.StringValue(wireBlob)},
	}
	prior := []CriterionModel{
		{Name: types.StringValue(dsUsername), Value: types.StringValue("Admins")},
	}
	out := ReadbackDSGroupCriteria(context.Background(), f, ObjectTypeComputer, wireModels, prior)
	if out[0].Value.ValueString() != "Admins" {
		t.Errorf("readback value = %q, want Admins (name preserved)", out[0].Value.ValueString())
	}

	// No prior (import) → wire base64 kept.
	out = ReadbackDSGroupCriteria(context.Background(), f, ObjectTypeComputer, wireModels, nil)
	if out[0].Value.ValueString() != wireBlob {
		t.Errorf("readback with no prior = %q, want wire blob", out[0].Value.ValueString())
	}
}

func TestDSGroupValuesEquivalent(t *testing.T) {
	otherBlob := encodeDSGroupValue(dsGroupRef{UUID: "FA257BC0-5F97-44D8-898E-2F4A19F4A413", ServerID: "31"})
	tests := []struct {
		name string
		f    *fakeSearcher
		a, b string
		want bool
	}{
		{"identical strings short-circuit (no api)", &fakeSearcher{err: errors.New("must not call")}, wireBlob, wireBlob, true},
		{"name vs equivalent base64", &fakeSearcher{results: adminGroup()}, "Admins", wireBlob, true},
		// Same group, lowercase uuid in the pasted blob → semantic match.
		{"name vs lowercase-uuid blob", &fakeSearcher{results: adminGroup()}, "Admins", b64JSON(`{"uuid":"e5efca3e-892c-4cce-b2c1-6ca1a32e9153","serverId":"31"}`), true},
		{"name vs different base64", &fakeSearcher{results: adminGroup()}, "Admins", otherBlob, false},
		{"unresolvable name (soft) → not equivalent", &fakeSearcher{results: nil}, "Gone", wireBlob, false},
		{"transient error (soft) → not equivalent", &fakeSearcher{err: errors.New("boom")}, "Admins", wireBlob, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DSGroupValuesEquivalent(context.Background(), tt.f, tt.a, tt.b); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSuppressEquivalentDSGroupValues(t *testing.T) {
	f := &fakeSearcher{results: adminGroup()}
	// planned uses the NAME; prior state holds the equivalent base64 → suppress.
	planned := []CriterionModel{
		{Name: types.StringValue(dsUsername), Value: types.StringValue("Admins")},
		{Name: types.StringValue("Application Title"), Value: types.StringValue("Firefox")},
	}
	prior := []CriterionModel{
		{Name: types.StringValue(dsUsername), Value: types.StringValue(wireBlob)},
		{Name: types.StringValue("Application Title"), Value: types.StringValue("Firefox")},
	}
	out := SuppressEquivalentDSGroupValues(context.Background(), f, planned, prior)
	if out[0].Value.ValueString() != wireBlob {
		t.Errorf("equivalent DS value not suppressed: got %q, want prior %q", out[0].Value.ValueString(), wireBlob)
	}
	if out[1].Value.ValueString() != "Firefox" {
		t.Errorf("non-DS criterion changed: %q", out[1].Value.ValueString())
	}

	// An Unknown sibling field (priority churns to Unknown when the criteria list
	// changes) must be reconciled to prior on a confirmed swap, else the element
	// stays != state and defeats the no-op.
	plannedUnknownPrio := []CriterionModel{
		{Name: types.StringValue(dsUsername), Priority: types.Int64Unknown(), Value: types.StringValue("Admins")},
	}
	priorKnownPrio := []CriterionModel{
		{Name: types.StringValue(dsUsername), Priority: types.Int64Value(0), Value: types.StringValue(wireBlob)},
	}
	out = SuppressEquivalentDSGroupValues(context.Background(), f, plannedUnknownPrio, priorKnownPrio)
	if out[0].Priority.IsUnknown() || out[0].Priority.ValueInt64() != 0 {
		t.Errorf("unknown priority not reconciled to prior: got %v", out[0].Priority)
	}
	if !CriteriaModelsEqual(out, priorKnownPrio) {
		t.Error("after suppression a pure swap should equal prior state")
	}

	// Misaligned (different name at index) → no suppression.
	priorMis := []CriterionModel{
		{Name: types.StringValue(dsAssignedUser), Value: types.StringValue(wireBlob)},
	}
	out = SuppressEquivalentDSGroupValues(context.Background(), f, planned, priorMis)
	if out[0].Value.ValueString() != "Admins" {
		t.Errorf("misaligned prior should not suppress: got %q", out[0].Value.ValueString())
	}

	// Different group → no suppression (real change shows a diff).
	priorDiff := []CriterionModel{
		{Name: types.StringValue(dsUsername), Value: types.StringValue(encodeDSGroupValue(dsGroupRef{UUID: "FA257BC0-5F97-44D8-898E-2F4A19F4A413", ServerID: "31"}))},
	}
	out = SuppressEquivalentDSGroupValues(context.Background(), f, planned[:1], priorDiff)
	if out[0].Value.ValueString() != "Admins" {
		t.Errorf("different group should not suppress: got %q", out[0].Value.ValueString())
	}
}

func TestCriteriaModelsEqual(t *testing.T) {
	base := []CriterionModel{
		{Name: types.StringValue(dsUsername), SearchType: types.StringValue("member of"), Value: types.StringValue(wireBlob)},
	}
	same := []CriterionModel{
		{Name: types.StringValue(dsUsername), SearchType: types.StringValue("member of"), Value: types.StringValue(wireBlob)},
	}
	diffValue := []CriterionModel{
		{Name: types.StringValue(dsUsername), SearchType: types.StringValue("member of"), Value: types.StringValue("Admins")},
	}
	if !CriteriaModelsEqual(base, same) {
		t.Error("identical slices should be equal")
	}
	if CriteriaModelsEqual(base, diffValue) {
		t.Error("differing value should not be equal")
	}
	if CriteriaModelsEqual(base, base[:0]) {
		t.Error("differing length should not be equal")
	}
}

// b64JSON base64-encodes a JSON literal for table tests.
func b64JSON(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
