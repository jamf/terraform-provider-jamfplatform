// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

func testBaseline() *compliancebenchmarks.SourcedRules {
	return &compliancebenchmarks.SourcedRules{
		AvailableOsVersions: []compliancebenchmarks.OsVersion{
			{OsType: compliancebenchmarks.OsVersionOsTypeMacOs, OsVersion: 27},
			{OsType: compliancebenchmarks.OsVersionOsTypeMacOs, OsVersion: 15},
			{OsType: compliancebenchmarks.OsVersionOsTypeMacOs, OsVersion: 26},
		},
		Rules: []compliancebenchmarks.RuleInfo{
			{ID: "plain_rule"},
			{ID: "min_length", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type:       compliancebenchmarks.OrganizationDefinedValueTypeInteger,
				Value:      "14",
				Validation: &compliancebenchmarks.ValidationConstraints{Min: 1, Max: 16},
			}},
			{ID: "uncapped", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type:       compliancebenchmarks.OrganizationDefinedValueTypeInteger,
				Validation: &compliancebenchmarks.ValidationConstraints{Min: 0, Max: 0},
			}},
			{ID: "no_validation", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type: compliancebenchmarks.OrganizationDefinedValueTypeInteger,
			}},
			{ID: "storage", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type:       compliancebenchmarks.OrganizationDefinedValueTypeEnum,
				Validation: &compliancebenchmarks.ValidationConstraints{EnumValues: []string{"Allowed", "ReadOnly", "Disallowed"}},
			}},
			{ID: "retention", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type:       compliancebenchmarks.OrganizationDefinedValueTypeString,
				Validation: &compliancebenchmarks.ValidationConstraints{Regex: `^\d+[shdy]$`},
			}},
			{ID: "free_string", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type: compliancebenchmarks.OrganizationDefinedValueTypeString,
			}},
			{ID: "pattern", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type: compliancebenchmarks.OrganizationDefinedValueTypeRegex,
			}},
			{ID: "flag", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type: compliancebenchmarks.OrganizationDefinedValueTypeBoolean,
			}},
			{ID: "bad_regex_definition", ODV: &compliancebenchmarks.OrganizationDefinedValue{
				Type:       compliancebenchmarks.OrganizationDefinedValueTypeString,
				Validation: &compliancebenchmarks.ValidationConstraints{Regex: `(?=lookahead)x`},
			}},
		},
	}
}

func rule(index int, id, odv types.String) ruleInput {
	return ruleInput{index: index, id: id, odv: odv}
}

func osVersion(osType string, version int64) osVersionInput {
	object := types.ObjectValueMust(
		map[string]attr.Type{"os_type": types.StringType, "os_version": types.Int64Type},
		map[string]attr.Value{"os_type": types.StringValue(osType), "os_version": types.Int64Value(version)},
	)
	return osVersionInput{element: object, osType: types.StringValue(osType), osVersion: types.Int64Value(version)}
}

func TestCheckAgainstBaseline_RuleIDs(t *testing.T) {
	cases := map[string]struct {
		id        types.String
		wantCount int
	}{
		"known rule":   {types.StringValue("plain_rule"), 0},
		"unknown rule": {types.StringValue("no_such_rule"), 1},
		"unknown id":   {types.StringUnknown(), 0},
		"null id":      {types.StringNull(), 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			findings := checkAgainstBaseline("cis_lvl1", testBaseline(), benchmarkInput{
				rules: []ruleInput{rule(2, tc.id, types.StringNull())},
			})
			if len(findings) != tc.wantCount {
				t.Fatalf("findings = %+v, want %d", findings, tc.wantCount)
			}
			if tc.wantCount == 0 {
				return
			}
			if want := path.Root("rules").AtListIndex(2).AtName("id"); !findings[0].path.Equal(want) {
				t.Errorf("path = %s, want %s", findings[0].path, want)
			}
			for _, part := range []string{"cis_lvl1", "no_such_rule", "jamfplatform_cbengine_rules"} {
				if !strings.Contains(findings[0].detail, part) {
					t.Errorf("detail %q does not name %q", findings[0].detail, part)
				}
			}
		})
	}
}

func TestCheckAgainstBaseline_OSVersions(t *testing.T) {
	baseline := testBaseline()

	if got := checkOSVersions("cis_lvl1", baseline.AvailableOsVersions, []osVersionInput{osVersion("MAC_OS", 15), osVersion("MAC_OS", 26)}); len(got) != 0 {
		t.Fatalf("offered versions produced findings: %+v", got)
	}

	findings := checkOSVersions("cis_lvl1", baseline.AvailableOsVersions, []osVersionInput{osVersion("MAC_OS", 12), osVersion("IOS", 26)})
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want 2", findings)
	}
	if !strings.Contains(findings[0].detail, "Available: MAC_OS 15, MAC_OS 26, MAC_OS 27.") {
		t.Errorf("detail %q does not list the available pairs in order", findings[0].detail)
	}
	if !strings.Contains(findings[0].detail, "MAC_OS 12") || !strings.Contains(findings[1].detail, "IOS 26") {
		t.Errorf("details do not name the offending pairs: %q, %q", findings[0].detail, findings[1].detail)
	}

	unknown := osVersionInput{element: types.ObjectUnknown(nil), osType: types.StringUnknown(), osVersion: types.Int64Value(1)}
	if got := checkOSVersions("cis_lvl1", baseline.AvailableOsVersions, []osVersionInput{unknown}); len(got) != 0 {
		t.Errorf("an unknown os_type produced findings: %+v", got)
	}
	unknown = osVersionInput{element: types.ObjectUnknown(nil), osType: types.StringValue("MAC_OS"), osVersion: types.Int64Unknown()}
	if got := checkOSVersions("cis_lvl1", baseline.AvailableOsVersions, []osVersionInput{unknown}); len(got) != 0 {
		t.Errorf("an unknown os_version produced findings: %+v", got)
	}
}

func TestCheckAgainstBaseline_ODVValues(t *testing.T) {
	cases := []struct {
		name    string
		ruleID  string
		odv     types.String
		want    string
		wantAny bool
	}{
		{"no ODV defined", "plain_rule", types.StringValue("5"), "has no organization-defined value", true},
		{"no ODV defined and no value", "plain_rule", types.StringNull(), "", false},
		{"integer in range", "min_length", types.StringValue("16"), "", false},
		{"integer at minimum", "min_length", types.StringValue("1"), "", false},
		{"integer above max", "min_length", types.StringValue("99"), "at most 16", true},
		{"integer below min", "min_length", types.StringValue("0"), "at least 1", true},
		{"integer not a number", "min_length", types.StringValue("abc"), "is not one", true},
		{"integer with whitespace", "min_length", types.StringValue(" 5"), "is not one", true},
		{"max zero is unset", "uncapped", types.StringValue("999999"), "", false},
		{"min zero still applies", "uncapped", types.StringValue("-5"), "at least 0", true},
		{"integer without constraints", "no_validation", types.StringValue("-5"), "", false},
		{"enum member", "storage", types.StringValue("ReadOnly"), "", false},
		{"enum non-member", "storage", types.StringValue("Bogus"), "Allowed, ReadOnly, Disallowed", true},
		{"enum is case sensitive", "storage", types.StringValue("readonly"), "matched exactly", true},
		{"string matching regex", "retention", types.StringValue("60d"), "", false},
		{"string not matching regex", "retention", types.StringValue("forever"), `^\d+[shdy]$`, true},
		{"string without regex", "free_string", types.StringValue("anything"), "", false},
		{"regex definition RE2 cannot compile is skipped", "bad_regex_definition", types.StringValue("anything"), "", false},
		{"REGEX type value is never checked", "pattern", types.StringValue("([unclosed"), "", false},
		{"BOOLEAN type value is never checked", "flag", types.StringValue("maybe"), "", false},
		{"empty value is unset", "min_length", types.StringValue(""), "", false},
		{"unknown value", "min_length", types.StringUnknown(), "", false},
		{"null value", "min_length", types.StringNull(), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := checkAgainstBaseline("cis_lvl1", testBaseline(), benchmarkInput{
				rules: []ruleInput{rule(0, types.StringValue(tc.ruleID), tc.odv)},
			})
			if !tc.wantAny {
				if len(findings) != 0 {
					t.Fatalf("findings = %+v, want none", findings)
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("findings = %+v, want 1", findings)
			}
			if want := path.Root("rules").AtListIndex(0).AtName("odv_value"); !findings[0].path.Equal(want) {
				t.Errorf("path = %s, want %s", findings[0].path, want)
			}
			if !strings.Contains(findings[0].detail, tc.want) {
				t.Errorf("detail %q does not contain %q", findings[0].detail, tc.want)
			}
			if !strings.Contains(findings[0].detail, tc.ruleID) {
				t.Errorf("detail %q does not name the rule", findings[0].detail)
			}
		})
	}
}

func TestCheckAgainstBaseline_ODVIsNotComparedWithTheDefault(t *testing.T) {
	baseline := &compliancebenchmarks.SourcedRules{Rules: []compliancebenchmarks.RuleInfo{{
		ID: "min_length",
		ODV: &compliancebenchmarks.OrganizationDefinedValue{
			Type:       compliancebenchmarks.OrganizationDefinedValueTypeInteger,
			Value:      "14",
			Validation: &compliancebenchmarks.ValidationConstraints{Min: 1, Max: 16},
		},
	}}}
	findings := checkAgainstBaseline("cis_lvl1", baseline, benchmarkInput{
		rules: []ruleInput{rule(0, types.StringValue("min_length"), types.StringValue("6"))},
	})
	if len(findings) != 0 {
		t.Fatalf("a value other than the baseline default was refused: %+v", findings)
	}
}

func benchmarkTestObjectType(t *testing.T) tftypes.Object {
	t.Helper()
	objectType, ok := benchmarkResourceSchema(context.Background()).Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("the benchmark schema is not an object")
	}
	return objectType
}

func nullObjectValues(objectType tftypes.Object) map[string]tftypes.Value {
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return values
}

type testRule struct {
	id  tftypes.Value
	odv tftypes.Value
}

func ruleValue(t *testing.T, objectType tftypes.Object, r testRule) tftypes.Value {
	t.Helper()
	ruleType := objectType.AttributeTypes["rules"].(tftypes.List).ElementType.(tftypes.Object)
	values := nullObjectValues(ruleType)
	values["id"] = r.id
	values["odv_value"] = r.odv
	values["enabled"] = tftypes.NewValue(tftypes.Bool, true)
	return tftypes.NewValue(ruleType, values)
}

func benchmarkValue(t *testing.T, baselineID tftypes.Value, rules ...testRule) tftypes.Value {
	t.Helper()
	objectType := benchmarkTestObjectType(t)
	values := nullObjectValues(objectType)
	values["title"] = tftypes.NewValue(tftypes.String, "tf-acc-validators")
	values["source_baseline_id"] = baselineID
	ruleValues := make([]tftypes.Value, 0, len(rules))
	for _, r := range rules {
		ruleValues = append(ruleValues, ruleValue(t, objectType, r))
	}
	values["rules"] = tftypes.NewValue(objectType.AttributeTypes["rules"], ruleValues)
	return tftypes.NewValue(objectType, values)
}

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }

func unknownString() tftypes.Value { return tftypes.NewValue(tftypes.String, tftypes.UnknownValue) }

func nullString() tftypes.Value { return tftypes.NewValue(tftypes.String, nil) }

type planHarness struct {
	t        *testing.T
	resource *BenchmarkResource
	hits     *atomic.Int32
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func newPlanHarness(t *testing.T, handler http.HandlerFunc) *planHarness {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			respondJSON(w, http.StatusOK, map[string]any{"access_token": "t", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client := jamfplatform.NewClient(server.URL, "id", "secret", jamfplatform.WithRetryPolicy(0, 0, 0), jamfplatform.WithMinRequestInterval(0))
	return &planHarness{
		t:        t,
		resource: &BenchmarkResource{baselines: providerdata.NewBaselineRulesCache(compliancebenchmarks.New(client))},
		hits:     &hits,
	}
}

func (h *planHarness) run(config, plan, state tftypes.Value) diag.Diagnostics {
	h.t.Helper()
	schema := benchmarkResourceSchema(context.Background())
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: schema, Raw: config},
		Plan:   tfsdk.Plan{Schema: schema, Raw: plan},
		State:  tfsdk.State{Schema: schema, Raw: state},
	}
	var resp resource.ModifyPlanResponse
	h.resource.validateAgainstBaseline(context.Background(), req, &resp)
	return resp.Diagnostics
}

func baselineHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, testBaseline())
	}
}

func nullState(t *testing.T) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(benchmarkTestObjectType(t), nil)
}

func summaries(diags diag.Diagnostics) []string {
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Severity().String()+": "+d.Summary())
	}
	return out
}

func TestValidateAgainstBaseline_CreateReportsErrors(t *testing.T) {
	h := newPlanHarness(t, baselineHandler())
	config := benchmarkValue(t, str("cis_lvl1"),
		testRule{str("plain_rule"), nullString()},
		testRule{str("no_such_rule"), nullString()},
		testRule{str("min_length"), str("99")},
	)

	diags := h.run(config, config, nullState(t))

	if got := summaries(diags); len(got) != 2 || got[0] != "Error: Rule not in the baseline" || got[1] != "Error: Invalid organization-defined value" {
		t.Fatalf("diagnostics = %v", got)
	}
	if want := path.Root("rules").AtListIndex(1).AtName("id"); !diags[0].(diag.DiagnosticWithPath).Path().Equal(want) {
		t.Errorf("first finding path = %s, want %s", diags[0].(diag.DiagnosticWithPath).Path(), want)
	}
}

func TestValidateAgainstBaseline_SkipsUnknownInputs(t *testing.T) {
	cases := map[string]tftypes.Value{
		"unknown baseline":    benchmarkValue(t, unknownString(), testRule{str("no_such_rule"), nullString()}),
		"null baseline":       benchmarkValue(t, nullString(), testRule{str("no_such_rule"), nullString()}),
		"empty baseline":      benchmarkValue(t, str(""), testRule{str("no_such_rule"), nullString()}),
		"unknown rule id":     benchmarkValue(t, str("cis_lvl1"), testRule{unknownString(), nullString()}),
		"unknown odv_value":   benchmarkValue(t, str("cis_lvl1"), testRule{str("min_length"), unknownString()}),
		"no odv_value":        benchmarkValue(t, str("cis_lvl1"), testRule{str("min_length"), nullString()}),
		"every input is good": benchmarkValue(t, str("cis_lvl1"), testRule{str("min_length"), str("8")}),
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			h := newPlanHarness(t, baselineHandler())
			diags := h.run(config, config, nullState(t))
			if len(diags) != 0 {
				t.Fatalf("diagnostics = %v, want none", summaries(diags))
			}
		})
	}
}

func TestValidateAgainstBaseline_UnknownBaselineMakesNoRequest(t *testing.T) {
	h := newPlanHarness(t, baselineHandler())
	config := benchmarkValue(t, unknownString(), testRule{str("plain_rule"), nullString()})
	h.run(config, config, nullState(t))
	if got := h.hits.Load(); got != 0 {
		t.Errorf("requests with an unknown source_baseline_id = %d, want 0", got)
	}
}

func TestValidateAgainstBaseline_DestroyMakesNoRequest(t *testing.T) {
	h := newPlanHarness(t, baselineHandler())
	state := benchmarkValue(t, str("cis_lvl1"), testRule{str("no_such_rule"), nullString()})
	diags := h.run(tftypes.NewValue(benchmarkTestObjectType(t), nil), tftypes.NewValue(benchmarkTestObjectType(t), nil), state)
	if len(diags) != 0 || h.hits.Load() != 0 {
		t.Errorf("destroy produced diagnostics %v and %d requests", summaries(diags), h.hits.Load())
	}
}

func TestValidateAgainstBaseline_UnchangedBenchmarkWarns(t *testing.T) {
	h := newPlanHarness(t, baselineHandler())
	config := benchmarkValue(t, str("cis_lvl1"), testRule{str("no_such_rule"), nullString()})

	diags := h.run(config, config, config)

	if got := summaries(diags); len(got) != 1 || got[0] != "Warning: Rule not in the baseline" {
		t.Fatalf("diagnostics = %v, want one warning", got)
	}
	if !strings.Contains(diags[0].Detail(), "has not changed since the last apply") {
		t.Errorf("warning %q does not say why it is not an error", diags[0].Detail())
	}
}

func TestValidateAgainstBaseline_ChangedBenchmarkErrors(t *testing.T) {
	h := newPlanHarness(t, baselineHandler())
	state := benchmarkValue(t, str("cis_lvl1"), testRule{str("plain_rule"), nullString()})
	config := benchmarkValue(t, str("cis_lvl1"), testRule{str("no_such_rule"), nullString()})

	diags := h.run(config, config, state)

	if got := summaries(diags); len(got) != 1 || got[0] != "Error: Rule not in the baseline" {
		t.Fatalf("diagnostics = %v, want one error", got)
	}
}

func TestValidateAgainstBaseline_BaselineReadFailureWarns(t *testing.T) {
	h := newPlanHarness(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusServiceUnavailable, map[string]any{"message": "unavailable"})
	})
	config := benchmarkValue(t, str("cis_lvl1"), testRule{str("no_such_rule"), nullString()})

	diags := h.run(config, config, nullState(t))

	if got := summaries(diags); len(got) != 1 || got[0] != "Warning: Baseline validation unavailable" {
		t.Fatalf("diagnostics = %v, want one warning", got)
	}
	if diags.HasError() {
		t.Error("a baseline that cannot be read must not fail the plan")
	}
}

func TestValidateAgainstBaseline_UnknownBaselineIDErrors(t *testing.T) {
	h := newPlanHarness(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusNotFound, map[string]any{"message": "Unknown baseline ID 'nope'", "error": "BaselineNotFoundException"})
	})
	config := benchmarkValue(t, str("nope"), testRule{str("plain_rule"), nullString()})

	diags := h.run(config, config, nullState(t))

	if got := summaries(diags); len(got) != 1 || got[0] != "Error: Baseline not found" {
		t.Fatalf("diagnostics = %v, want one error", got)
	}
	if !strings.Contains(diags[0].Detail(), "jamfplatform_cbengine_baselines") {
		t.Errorf("detail %q does not point at the baselines data source", diags[0].Detail())
	}
}

func TestValidateAgainstBaseline_SeveralBenchmarksReadTheBaselineOnce(t *testing.T) {
	h := newPlanHarness(t, baselineHandler())
	config := benchmarkValue(t, str("cis_lvl1"), testRule{str("plain_rule"), nullString()})
	for range 3 {
		h.run(config, config, nullState(t))
	}
	if got := h.hits.Load(); got != 1 {
		t.Errorf("requests for three benchmarks on one baseline = %d, want 1", got)
	}
}

func TestValidateAgainstBaseline_NilCacheIsDisabled(t *testing.T) {
	r := &BenchmarkResource{}
	config := benchmarkValue(t, str("cis_lvl1"), testRule{str("no_such_rule"), nullString()})
	schema := benchmarkResourceSchema(context.Background())
	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: schema, Raw: config},
		Plan:   tfsdk.Plan{Schema: schema, Raw: config},
		State:  tfsdk.State{Schema: schema, Raw: nullState(t)},
	}
	var resp resource.ModifyPlanResponse
	r.validateAgainstBaseline(context.Background(), req, &resp)
	if len(resp.Diagnostics) != 0 {
		t.Errorf("diagnostics = %v, want none", summaries(resp.Diagnostics))
	}
}
