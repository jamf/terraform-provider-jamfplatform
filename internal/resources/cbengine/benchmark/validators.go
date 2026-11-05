// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// unchangedBenchmarkNote is appended to a finding the plan reports as a warning, so the operator can
// tell a value they mistyped from one the baseline moved out from under.
const unchangedBenchmarkNote = "This benchmark has not changed since the last apply, so the plan warns instead of failing. " +
	"A newer baseline revision can withdraw a rule or narrow a value after a benchmark exists, and a benchmark " +
	"cannot be edited in place."

// baselineFinding is one thing the baseline says is wrong with the configuration.
type baselineFinding struct {
	path    path.Path
	summary string
	detail  string
}

// ruleInput is one configured rule, reduced to the values the baseline check reads. A value that is
// unknown or null at plan time is carried as such, because the check skips it rather than guessing.
type ruleInput struct {
	index int
	id    types.String
	odv   types.String
}

// osVersionInput is one configured entry of selected_os_versions, with the set element it came from
// so a finding can be attached to it.
type osVersionInput struct {
	element   attr.Value
	osType    types.String
	osVersion types.Int64
}

// benchmarkInput is the configuration values the baseline check reads.
type benchmarkInput struct {
	baselineID types.String
	rules      []ruleInput
	osVersions []osVersionInput
}

// readBenchmarkInput reads the baseline-dependent attributes from the configuration. It reads each
// attribute as its typed framework value rather than decoding the whole model, because a decode
// collapses an unknown nested value to a zero value and the check would then treat it as absent
// (STYLE_GUIDE §Cross-field validation). An attribute that cannot be read is left empty, which skips
// its checks.
func readBenchmarkInput(ctx context.Context, req resource.ModifyPlanRequest) benchmarkInput {
	var in benchmarkInput

	if diags := req.Config.GetAttribute(ctx, path.Root("source_baseline_id"), &in.baselineID); diags.HasError() {
		return benchmarkInput{}
	}

	var rules types.List
	if diags := req.Config.GetAttribute(ctx, path.Root("rules"), &rules); !diags.HasError() && helpers.IsConfiguredValue(rules) {
		for i, element := range rules.Elements() {
			object, ok := element.(types.Object)
			if !ok || !helpers.IsConfiguredValue(object) {
				continue
			}
			id, _ := object.Attributes()["id"].(types.String)
			odv, _ := object.Attributes()["odv_value"].(types.String)
			in.rules = append(in.rules, ruleInput{index: i, id: id, odv: odv})
		}
	}

	var versions types.Set
	if diags := req.Config.GetAttribute(ctx, path.Root("selected_os_versions"), &versions); !diags.HasError() && helpers.IsConfiguredValue(versions) {
		for _, element := range versions.Elements() {
			object, ok := element.(types.Object)
			if !ok || !helpers.IsConfiguredValue(object) {
				continue
			}
			osType, _ := object.Attributes()["os_type"].(types.String)
			osVersion, _ := object.Attributes()["os_version"].(types.Int64)
			in.osVersions = append(in.osVersions, osVersionInput{element: element, osType: osType, osVersion: osVersion})
		}
	}
	return in
}

// checkable reports whether the configuration holds anything a baseline read could check.
func (in benchmarkInput) checkable() bool {
	return len(in.rules) > 0 || len(in.osVersions) > 0
}

// checkAgainstBaseline compares the configuration with one baseline and returns what is wrong with
// it. Every input that is unknown or null is skipped.
//
// The API stays the authority. Each check below moves a refusal wire-probed on 2026-11-05 to plan
// time and makes it specific, and a check is written only for a rule the API was seen to enforce:
//
//   - A rule id absent from the baseline, and an OS version absent from the baseline's available
//     versions, are refused 400. An OS version a rule does not support is accepted, so that is not
//     checked.
//   - An INTEGER value that is not an integer, or is below Min, is refused 400. Max is enforced only
//     when it is not zero: Max is a plain integer with no way to say "unset", the API stores 999999
//     for a rule whose Max is 0, and most INTEGER definitions today carry Max 0.
//   - An ENUM value outside EnumValues is refused 400, compared case-sensitively.
//   - A STRING value that does not match Validation.Regex is refused 400. The one baseline rule that
//     carries a regex today is a STRING, so the check is scoped to that type.
//   - A value for a rule with no ODV is accepted and discarded, so Terraform then fails the apply
//     because state no longer matches the plan. That makes it an error here.
//
// Two things are deliberately not checked. A REGEX value is itself a pattern in the platform's own
// regular expression dialect, which RE2 cannot parse faithfully (lookahead is valid there and not
// here), so checking it would reject patterns the platform accepts. And no baseline defines a
// BOOLEAN ODV today, so its accepted spelling could not be probed.
func checkAgainstBaseline(baselineID string, baseline *compliancebenchmarks.SourcedRules, in benchmarkInput) []baselineFinding {
	var findings []baselineFinding

	known := make(map[string]compliancebenchmarks.RuleInfo, len(baseline.Rules))
	for _, rule := range baseline.Rules {
		known[rule.ID] = rule
	}

	for _, rule := range in.rules {
		if !helpers.IsConfiguredValue(rule.id) {
			continue
		}
		definition, ok := known[rule.id.ValueString()]
		if !ok {
			findings = append(findings, baselineFinding{
				path:    path.Root("rules").AtListIndex(rule.index).AtName("id"),
				summary: "Rule not in the baseline",
				detail: fmt.Sprintf("Baseline %q has no rule %q. Read the rule ids of a baseline from the jamfplatform_cbengine_rules data source.",
					baselineID, rule.id.ValueString()),
			})
			continue
		}
		if finding, ok := checkODV(baselineID, rule, definition); ok {
			findings = append(findings, finding)
		}
	}

	findings = append(findings, checkOSVersions(baselineID, baseline.AvailableOsVersions, in.osVersions)...)
	return findings
}

// checkOSVersions reports each selected OS version the baseline does not offer.
func checkOSVersions(baselineID string, available []compliancebenchmarks.OsVersion, selected []osVersionInput) []baselineFinding {
	var findings []baselineFinding
	for _, version := range selected {
		if !helpers.IsConfiguredValue(version.osType) || !helpers.IsConfiguredValue(version.osVersion) {
			continue
		}
		offered := slices.ContainsFunc(available, func(v compliancebenchmarks.OsVersion) bool {
			return v.OsType == version.osType.ValueString() && int64(v.OsVersion) == version.osVersion.ValueInt64()
		})
		if offered {
			continue
		}
		findings = append(findings, baselineFinding{
			path:    path.Root("selected_os_versions").AtSetValue(version.element),
			summary: "OS version not available for the baseline",
			detail: fmt.Sprintf("Baseline %q does not offer %s %d. Available: %s.",
				baselineID, version.osType.ValueString(), version.osVersion.ValueInt64(), renderOSVersions(available)),
		})
	}
	return findings
}

// renderOSVersions renders the available versions for a diagnostic, in a stable order.
func renderOSVersions(available []compliancebenchmarks.OsVersion) string {
	sorted := slices.Clone(available)
	slices.SortFunc(sorted, func(a, b compliancebenchmarks.OsVersion) int {
		if c := strings.Compare(a.OsType, b.OsType); c != 0 {
			return c
		}
		return a.OsVersion - b.OsVersion
	})
	rendered := make([]string, 0, len(sorted))
	for _, v := range sorted {
		rendered = append(rendered, fmt.Sprintf("%s %d", v.OsType, v.OsVersion))
	}
	if len(rendered) == 0 {
		return "none"
	}
	return strings.Join(rendered, ", ")
}

// checkODV validates one rule's configured odv_value against the rule's ODV definition. The
// definition's own Value is the baseline default and constrains nothing, so it is never read.
func checkODV(baselineID string, rule ruleInput, definition compliancebenchmarks.RuleInfo) (baselineFinding, bool) {
	if !helpers.IsConfiguredValue(rule.odv) || rule.odv.ValueString() == "" {
		return baselineFinding{}, false
	}
	value := rule.odv.ValueString()
	at := path.Root("rules").AtListIndex(rule.index).AtName("odv_value")
	invalid := func(detail string) (baselineFinding, bool) {
		return baselineFinding{path: at, summary: "Invalid organization-defined value", detail: detail}, true
	}

	if definition.ODV == nil {
		return baselineFinding{
			path:    at,
			summary: "Rule takes no organization-defined value",
			detail: fmt.Sprintf("Rule %q in baseline %q has no organization-defined value. The platform discards a value you set, "+
				"and the apply then fails because the state no longer matches the plan. Remove odv_value.", definition.ID, baselineID),
		}, true
	}

	constraints := definition.ODV.Validation
	switch definition.ODV.Type {
	case compliancebenchmarks.OrganizationDefinedValueTypeInteger:
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return invalid(fmt.Sprintf("Rule %q takes an integer, and %q is not one.", definition.ID, value))
		}
		if constraints == nil {
			return baselineFinding{}, false
		}
		if number < int64(constraints.Min) {
			return invalid(fmt.Sprintf("Rule %q takes an integer of at least %d, and %d is below that.", definition.ID, constraints.Min, number))
		}
		if constraints.Max != 0 && number > int64(constraints.Max) {
			return invalid(fmt.Sprintf("Rule %q takes an integer of at most %d, and %d is above that.", definition.ID, constraints.Max, number))
		}
	case compliancebenchmarks.OrganizationDefinedValueTypeEnum:
		if constraints == nil || len(constraints.EnumValues) == 0 || slices.Contains(constraints.EnumValues, value) {
			return baselineFinding{}, false
		}
		return invalid(fmt.Sprintf("Rule %q does not accept %q. Allowed values, matched exactly: %s.",
			definition.ID, value, strings.Join(constraints.EnumValues, ", ")))
	case compliancebenchmarks.OrganizationDefinedValueTypeString:
		if constraints == nil || constraints.Regex == "" {
			return baselineFinding{}, false
		}
		pattern, err := regexp.Compile(constraints.Regex)
		if err != nil || pattern.MatchString(value) {
			return baselineFinding{}, false
		}
		return invalid(fmt.Sprintf("Rule %q takes a value matching %s, and %q does not.", definition.ID, constraints.Regex, value))
	}
	return baselineFinding{}, false
}

// writesBenchmark reports whether this plan creates the benchmark or replaces it. A benchmark has no
// in-place update, so a plan that differs from state at all is a replacement.
func writesBenchmark(req resource.ModifyPlanRequest) bool {
	return req.State.Raw.IsNull() || !req.Plan.Raw.Equal(req.State.Raw)
}

// validateAgainstBaseline checks the configured rules, organization-defined values and OS versions
// against the source baseline, so a refusal the API would give at apply is reported by plan.
//
// It runs when source_baseline_id is known and reads the baseline once per provider instance. The
// findings are errors when this plan writes the benchmark, and warnings when it does not: ModifyPlan
// runs for every benchmark in the configuration, the baseline is live data that a new revision can
// narrow after a benchmark was created, and an error there would fail every plan in the workspace
// until the benchmark was replaced. A baseline that cannot be read produces one warning and leaves
// the decision to the apply.
func (r *BenchmarkResource) validateAgainstBaseline(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.baselines == nil || req.Plan.Raw.IsNull() {
		return
	}
	in := readBenchmarkInput(ctx, req)
	if !helpers.IsConfiguredValue(in.baselineID) || in.baselineID.ValueString() == "" || !in.checkable() {
		return
	}
	baselineID := in.baselineID.ValueString()
	at := path.Root("source_baseline_id")
	changed := writesBenchmark(req)

	report := func(location path.Path, summary, detail string) {
		if changed {
			resp.Diagnostics.AddAttributeError(location, summary, detail)
			return
		}
		resp.Diagnostics.AddAttributeWarning(location, summary, detail+" "+unchangedBenchmarkNote)
	}

	baseline, err := r.baselines.Rules(ctx, baselineID)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			report(at, "Baseline not found",
				fmt.Sprintf("The platform has no baseline %q. Read the baseline ids from the jamfplatform_cbengine_baselines data source.", baselineID))
			return
		}
		tflog.Debug(ctx, "baseline read failed during plan", map[string]any{"baseline_id": baselineID, "error": err.Error()})
		resp.Diagnostics.AddAttributeWarning(at, "Baseline validation unavailable",
			fmt.Sprintf("The provider could not read baseline %q, so it did not check the rules, organization-defined values or OS versions in this plan: %s\n\n"+
				"The check is advisory and the plan continues. The platform validates the benchmark when it creates it.",
				baselineID, helpers.APIErrorDetail(err)))
		return
	}

	for _, finding := range checkAgainstBaseline(baselineID, baseline, in) {
		report(finding.path, finding.summary, finding.detail)
	}
}
