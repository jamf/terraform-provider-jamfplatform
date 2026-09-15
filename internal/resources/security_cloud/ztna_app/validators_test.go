// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestNormalisedHostname pins the two normalisations Jamf Security Cloud applies and
// the provider therefore refuses, because `hostnames` is Optional rather than
// Optional+Computed and so cannot be rewritten at plan time.
func TestNormalisedHostname(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr string
	}{
		{"already normalised", "sub.example.com", ""},
		{"wildcard", "*.example.com", ""},
		{"bare wildcard", "*", ""},
		{"digits and hyphens", "web-01.example.com", ""},

		{"uppercase", "Sub.Example.COM", "lower-case"},
		{"single uppercase letter", "sub.example.coM", "lower-case"},
		{"trailing dot", "sub.example.com.", "trailing"},
		{"uppercase and trailing dot", "SUB.EXAMPLE.COM.", "trailing"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			normalisedHostname().ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("hostnames"),
				ConfigValue: types.StringValue(tc.value),
			}, resp)

			if tc.wantErr == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("expected %q to be accepted, got %s", tc.value, resp.Diagnostics)
				}
				return
			}
			if !resp.Diagnostics.HasError() {
				t.Fatalf("expected %q to be refused", tc.value)
			}
			if !strings.Contains(diagnosticsText(resp.Diagnostics), tc.wantErr) {
				t.Fatalf("expected the refusal of %q to mention %q, got %s", tc.value, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestNormalisedHostnameSuggestsTheFix pins that the lower-case refusal names the
// spelling to write, so the fix does not have to be worked out.
func TestNormalisedHostnameSuggestsTheFix(t *testing.T) {
	resp := &validator.StringResponse{}
	normalisedHostname().ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("hostnames"),
		ConfigValue: types.StringValue("Sub.Example.COM"),
	}, resp)
	if !strings.Contains(diagnosticsText(resp.Diagnostics), "sub.example.com") {
		t.Fatalf("expected the refusal to name the normalised form, got %s", resp.Diagnostics)
	}
}

// TestCoversHostname pins the overlap rule the server enforces: a wildcard covers
// subdomains but not the parent, so a parent listed alongside its own wildcard is
// legal and a subdomain listed alongside it is not.
//
// Coverage is strictly about wildcards, which is why an entry does not cover an
// identical one. Two identical entries would be a duplicate rather than an overlap,
// and the Set type rules that out before any validator sees it.
func TestCoversHostname(t *testing.T) {
	cases := []struct {
		pattern   string
		candidate string
		want      bool
	}{
		{"*", "anything.example.com", true},
		{"*", "*.example.com", true},
		{"*.example.com", "sub.example.com", true},
		{"*.example.com", "deep.sub.example.com", true},
		{"*.example.com", "example.com", false},
		{"*.example.com", "notexample.com", false},
		{"*.example.com", "sub.other.com", false},
		{"*.example.com", "host.example.com.attacker.net", false},
		{"*.example.com", "x.example.commercial.net", false},
		{"example.com", "sub.example.com", false},
		{"sub.example.com", "sub.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.pattern+" covers "+tc.candidate, func(t *testing.T) {
			if got := coversHostname(tc.pattern, tc.candidate); got != tc.want {
				t.Errorf("coversHostname(%q, %q) = %v, want %v", tc.pattern, tc.candidate, got, tc.want)
			}
		})
	}
}

// TestValidateRouting pins all four cross-field rules on a routing block. The server
// answers every one of them with the same opaque sentence, so this is where the
// distinction actually gets made.
func TestValidateRouting(t *testing.T) {
	viaZTNA := routingModeLabels[securitycloud.RoutingTypeCustom]
	direct := routingModeLabels[securitycloud.RoutingTypeDirect]

	cases := []struct {
		name    string
		routing *RoutingModel
		wantErr string
	}{
		{"nil block defers", nil, ""},
		{
			"via ztna, complete",
			&RoutingModel{TrafficRouting: types.StringValue(viaZTNA), GatewayID: types.StringValue("a7d2"), RoutingMode: types.StringValue("Standard")},
			"",
		},
		{
			"direct, empty",
			&RoutingModel{TrafficRouting: types.StringValue(direct), GatewayID: types.StringNull(), RoutingMode: types.StringNull()},
			"",
		},
		{
			"via ztna without a gateway",
			&RoutingModel{TrafficRouting: types.StringValue(viaZTNA), GatewayID: types.StringNull(), RoutingMode: types.StringValue("Standard")},
			"needs an access gateway",
		},
		{
			"via ztna without a routing mode",
			&RoutingModel{TrafficRouting: types.StringValue(viaZTNA), GatewayID: types.StringValue("a7d2"), RoutingMode: types.StringNull()},
			"needs a routing mode",
		},
		{
			"direct with a gateway",
			&RoutingModel{TrafficRouting: types.StringValue(direct), GatewayID: types.StringValue("a7d2"), RoutingMode: types.StringNull()},
			"does not use an access gateway",
		},
		{
			"direct with a routing mode",
			&RoutingModel{TrafficRouting: types.StringValue(direct), GatewayID: types.StringNull(), RoutingMode: types.StringValue("Standard")},
			"has no routing mode",
		},
		{
			"unknown mode defers",
			&RoutingModel{TrafficRouting: types.StringUnknown(), GatewayID: types.StringNull(), RoutingMode: types.StringNull()},
			"",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateRouting(tc.routing, path.Root("routing"), &diags)

			if tc.wantErr == "" {
				if diags.HasError() {
					t.Fatalf("expected no error, got %s", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("expected an error")
			}
			if !strings.Contains(diagnosticsText(diags), tc.wantErr) {
				t.Fatalf("expected the error to mention %q, got %s", tc.wantErr, diags)
			}
		})
	}
}

// TestValidateRoutingReportsBothMissingFields pins that a `CUSTOM` block missing both
// members reports both, rather than stopping at the first. The server names neither,
// so a caller fixing one at a time would round-trip twice.
func TestValidateRoutingReportsBothMissingFields(t *testing.T) {
	var diags diag.Diagnostics
	validateRouting(&RoutingModel{
		TrafficRouting: types.StringValue(routingModeLabels[securitycloud.RoutingTypeCustom]),
		GatewayID:      types.StringNull(),
		RoutingMode:    types.StringNull(),
	}, path.Root("routing"), &diags)

	if got := diags.ErrorsCount(); got != 2 {
		t.Fatalf("expected 2 errors, got %d: %s", got, diags)
	}
}

// diagnosticsText flattens diagnostics into one string for substring assertions.
func diagnosticsText(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags.Errors() {
		b.WriteString(d.Summary())
		b.WriteString(" ")
		b.WriteString(d.Detail())
		b.WriteString("\n")
	}
	return b.String()
}

// TestValidateAppForm pins the mutual exclusivity of the two forms. The
// combination case is the one that matters most, because the server accepts it and
// then discards the name — so this validator is the only thing standing between an
// operator and a silently ignored configuration.
func TestValidateAppForm(t *testing.T) {
	cases := []struct {
		name       string
		appName    types.String
		predefined types.String
		wantErr    string
	}{
		{"custom", types.StringValue("Internal CRM"), types.StringNull(), ""},
		{"predefined", types.StringNull(), types.StringValue("2aaa401c"), ""},
		{"both", types.StringValue("My Slack"), types.StringValue("2aaa401c"), "cannot be renamed"},
		{"neither", types.StringNull(), types.StringNull(), "needs a name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateAppForm(ZtnaAppResourceModel{Name: tc.appName, PredefinedAppID: tc.predefined}, &diags)

			if tc.wantErr == "" {
				if diags.HasError() {
					t.Fatalf("expected no error, got %s", diags)
				}
				return
			}
			if !strings.Contains(diagnosticsText(diags), tc.wantErr) {
				t.Fatalf("expected the error to mention %q, got %s", tc.wantErr, diags)
			}
		})
	}
}

// TestValidateHostnameOverlap pins the plan-time overlap check against the pairing
// the server refuses, and against the pairing it allows — a parent domain listed
// alongside its own wildcard, which is the whole reason coversHostname excludes the
// parent.
func TestValidateHostnameOverlap(t *testing.T) {
	cases := []struct {
		name      string
		hostnames []string
		wantErr   bool
	}{
		{"disjoint", []string{"a.example.com", "b.example.com"}, false},
		{"parent alongside its wildcard", []string{"example.com", "*.example.com"}, false},
		{"two unrelated wildcards", []string{"*.example.com", "*.other.com"}, false},
		{"absent", nil, false},

		{"subdomain under a wildcard", []string{"*.example.com", "sub.example.com"}, true},
		{"deep subdomain under a wildcard", []string{"*.example.com", "deep.sub.example.com"}, true},
		{"full tunnel alongside anything", []string{"*", "example.com"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := ZtnaAppResourceModel{Hostnames: stringSetFor(t, tc.hostnames)}
			var diags diag.Diagnostics
			validateHostnameOverlap(context.Background(), config, &diags)

			if got := diags.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (%s)", got, tc.wantErr, diags)
			}
		})
	}
}

// TestValidateHostnameOverlapNamesBothEntries pins that the diagnostic names the
// covered entry and the wildcard covering it. The server's own refusal names
// neither, so a set of twenty host names would otherwise be a manual search.
func TestValidateHostnameOverlapNamesBothEntries(t *testing.T) {
	config := ZtnaAppResourceModel{Hostnames: stringSetFor(t, []string{"*.example.com", "sub.example.com"})}
	var diags diag.Diagnostics
	validateHostnameOverlap(context.Background(), config, &diags)

	text := diagnosticsText(diags)
	for _, want := range []string{"sub.example.com", "*.example.com"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected the error to name %q, got %s", want, text)
		}
	}
}

// TestValidateDeviceGroupAssignment pins the three assignment rules, including the
// one the server does not enforce: a group list sent alongside "all device groups"
// is accepted and ignored, so it has to be refused here or it would disappear from
// state on the first refresh.
func TestValidateDeviceGroupAssignment(t *testing.T) {
	const groupA = "aaaaaaaa-0000-0000-0000-000000000000"
	const groupB = "bbbbbbbb-0000-0000-0000-000000000000"
	const groupC = "cccccccc-0000-0000-0000-000000000000"

	cases := []struct {
		name      string
		allGroups bool
		assigned  []string
		overrides [][]string
		wantErr   string
	}{
		{"all groups, no list", true, nil, nil, ""},
		{"selected groups", false, []string{groupA, groupB}, nil, ""},
		{"override on an assigned group", false, []string{groupA, groupB}, [][]string{{groupA}}, ""},
		{"overrides on distinct assigned groups", false, []string{groupA, groupB}, [][]string{{groupA}, {groupB}}, ""},
		{"all groups with an override on any group", true, nil, [][]string{{groupC}}, ""},
		{"selected groups, empty list", false, nil, nil, ""},

		{"all groups with a list", true, []string{groupA}, nil, "conflict with all device groups"},
		{"override on an unassigned group", false, []string{groupA}, [][]string{{groupC}}, "unassigned device group"},
		{"group in two overrides", false, []string{groupA}, [][]string{{groupA}, {groupA}}, "more than one routing override"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := ZtnaAppResourceModel{
				AllDeviceGroups:  types.BoolValue(tc.allGroups),
				DeviceGroupIDs:   stringSetFor(t, tc.assigned),
				RoutingOverrides: overrideListFor(t, tc.overrides),
			}
			var diags diag.Diagnostics
			validateDeviceGroupAssignment(context.Background(), config, &diags)

			if tc.wantErr == "" {
				if diags.HasError() {
					t.Fatalf("expected no error, got %s", diags)
				}
				return
			}
			if !strings.Contains(diagnosticsText(diags), tc.wantErr) {
				t.Fatalf("expected the error to mention %q, got %s", tc.wantErr, diags)
			}
		})
	}
}

// TestValidateAllRoutingWalksOverrides pins that an override's routing is held to the
// same rules as the application's own, and that the diagnostic points at the
// offending index. The server does index its own message here, but only for the
// routing shape — not for the group rules.
func TestValidateAllRoutingWalksOverrides(t *testing.T) {
	direct := routingModeLabels[securitycloud.RoutingTypeDirect]
	viaZTNA := routingModeLabels[securitycloud.RoutingTypeCustom]

	config := ZtnaAppResourceModel{
		Routing: &RoutingModel{
			TrafficRouting: types.StringValue(direct),
			GatewayID:      types.StringNull(),
			RoutingMode:    types.StringNull(),
		},
		RoutingOverrides: overrideListWithRouting(t, []*RoutingModel{
			{TrafficRouting: types.StringValue(direct), GatewayID: types.StringNull(), RoutingMode: types.StringNull()},
			{TrafficRouting: types.StringValue(viaZTNA), GatewayID: types.StringNull(), RoutingMode: types.StringValue("Standard")},
		}),
	}

	var diags diag.Diagnostics
	validateAllRouting(context.Background(), config, &diags)

	if got := diags.ErrorsCount(); got != 1 {
		t.Fatalf("expected exactly the second override to fail, got %d errors: %s", got, diags)
	}
	withPath, ok := diags.Errors()[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatal("expected the error to be attached to an attribute path")
	}
	if got := withPath.Path().String(); got != "routing_overrides[1].routing.gateway_id" {
		t.Errorf("error is attached to %s, want routing_overrides[1].routing.gateway_id", got)
	}
}

// stringSetFor builds a set of strings, or a null set for an absent collection.
func stringSetFor(t *testing.T, values []string) types.Set {
	t.Helper()
	if values == nil {
		return types.SetNull(types.StringType)
	}
	set, diags := types.SetValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building set: %s", diags)
	}
	return set
}

// overrideListFor builds a routing_overrides list whose entries differ only in the
// groups they name, with a valid direct-routing block throughout.
func overrideListFor(t *testing.T, groups [][]string) types.List {
	t.Helper()
	if groups == nil {
		return types.ListNull(routingOverrideObjectType)
	}
	routings := make([]*RoutingModel, 0, len(groups))
	for range groups {
		routings = append(routings, &RoutingModel{
			TrafficRouting: types.StringValue(routingModeLabels[securitycloud.RoutingTypeDirect]),
			GatewayID:      types.StringNull(),
			RoutingMode:    types.StringNull(),
		})
	}
	return buildOverrideList(t, groups, routings)
}

// overrideListWithRouting builds a routing_overrides list whose entries differ in
// their routing block, each naming a distinct group.
func overrideListWithRouting(t *testing.T, routings []*RoutingModel) types.List {
	t.Helper()
	groups := make([][]string, 0, len(routings))
	for i := range routings {
		groups = append(groups, []string{"group-" + string(rune('a'+i))})
	}
	return buildOverrideList(t, groups, routings)
}

// buildOverrideList assembles the framework list value from parallel slices.
func buildOverrideList(t *testing.T, groups [][]string, routings []*RoutingModel) types.List {
	t.Helper()
	models := make([]RoutingOverrideModel, 0, len(groups))
	for i := range groups {
		set, diags := types.SetValueFrom(context.Background(), types.StringType, groups[i])
		if diags.HasError() {
			t.Fatalf("building override group set: %s", diags)
		}
		models = append(models, RoutingOverrideModel{DeviceGroupIDs: set, Routing: routings[i]})
	}
	list, diags := types.ListValueFrom(context.Background(), routingOverrideObjectType, models)
	if diags.HasError() {
		t.Fatalf("building override list: %s", diags)
	}
	return list
}

// TestKnownStrings pins the unknown-value handling every config validator here rests
// on. ElementsAs, which this replaced, refuses a collection holding an unknown
// element — and an unknown element is the commonest case there is, because a group ID
// usually references a device group the same plan is about to create. That made every
// check silently dead in exactly the configuration they exist for.
func TestKnownStrings(t *testing.T) {
	known, diags := types.SetValueFrom(context.Background(), types.StringType, []string{"a", "b"})
	if diags.HasError() {
		t.Fatalf("building set: %s", diags)
	}
	partial, diags := types.SetValue(types.StringType, []attr.Value{
		types.StringValue("a"),
		types.StringUnknown(),
	})
	if diags.HasError() {
		t.Fatalf("building set: %s", diags)
	}

	cases := []struct {
		name         string
		set          types.Set
		wantValues   int
		wantComplete bool
	}{
		{"null", types.SetNull(types.StringType), 0, true},
		{"unknown", types.SetUnknown(types.StringType), 0, false},
		{"fully known", known, 2, true},
		{"one unknown element", partial, 1, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values, complete := knownStrings(tc.set)
			if len(values) != tc.wantValues {
				t.Errorf("got %d known values, want %d: %v", len(values), tc.wantValues, values)
			}
			if complete != tc.wantComplete {
				t.Errorf("complete = %v, want %v", complete, tc.wantComplete)
			}
		})
	}
}

// TestValidateDeviceGroupAssignmentDefersOnUnknownAssignment pins the one rule that
// needs the whole collection. A group whose assignment is not yet resolved must not be
// reported as unassigned — that would fail the plan for a perfectly valid
// configuration referencing a device group the same apply creates.
func TestValidateDeviceGroupAssignmentDefersOnUnknownAssignment(t *testing.T) {
	assigned, diags := types.SetValue(types.StringType, []attr.Value{types.StringUnknown()})
	if diags.HasError() {
		t.Fatalf("building set: %s", diags)
	}

	config := ZtnaAppResourceModel{
		AllDeviceGroups:  types.BoolValue(false),
		DeviceGroupIDs:   assigned,
		RoutingOverrides: overrideListFor(t, [][]string{{"aaaaaaaa-0000-0000-0000-000000000000"}}),
	}

	var got diag.Diagnostics
	validateDeviceGroupAssignment(context.Background(), config, &got)
	if got.HasError() {
		t.Fatalf("expected the subset rule to defer while the assignment is unresolved, got %s", got)
	}
}

// TestValidateDeviceGroupAssignmentStillCatchesDuplicatesWhenIncomplete pins the
// other half of that decision: two known colliding values are a real error whatever
// else is unresolved, so the duplicate rule must not defer with the subset rule.
func TestValidateDeviceGroupAssignmentStillCatchesDuplicatesWhenIncomplete(t *testing.T) {
	assigned, diags := types.SetValue(types.StringType, []attr.Value{types.StringUnknown()})
	if diags.HasError() {
		t.Fatalf("building set: %s", diags)
	}

	const group = "aaaaaaaa-0000-0000-0000-000000000000"
	config := ZtnaAppResourceModel{
		AllDeviceGroups:  types.BoolValue(false),
		DeviceGroupIDs:   assigned,
		RoutingOverrides: overrideListFor(t, [][]string{{group}, {group}}),
	}

	var got diag.Diagnostics
	validateDeviceGroupAssignment(context.Background(), config, &got)
	if !strings.Contains(diagnosticsText(got), "more than one routing override") {
		t.Fatalf("expected the duplicate rule to still fire, got %s", got)
	}
}

// TestValidateDeviceGroupAssignmentAllowsSelectedGroups is the regression test for a
// bug this package shipped with briefly: a boolvalidator.ConflictsWith on
// all_device_groups fired whenever both attributes were configured, and
// all_device_groups is Required — so it is always configured, and device_group_ids
// could never be set at all. The rule is conditional on the bool's value, which only a
// config validator can see.
func TestValidateDeviceGroupAssignmentAllowsSelectedGroups(t *testing.T) {
	config := ZtnaAppResourceModel{
		AllDeviceGroups:  types.BoolValue(false),
		DeviceGroupIDs:   stringSetFor(t, []string{"aaaaaaaa-0000-0000-0000-000000000000"}),
		RoutingOverrides: types.ListNull(routingOverrideObjectType),
	}

	var got diag.Diagnostics
	validateDeviceGroupAssignment(context.Background(), config, &got)
	if got.HasError() {
		t.Fatalf("selected device groups must be accepted alongside all_device_groups = false, got %s", got)
	}
}

// TestSchemaDoesNotRefuseSelectedGroups is the schema-level half of the same
// regression. The config validator above cannot see a schema validator, so a
// ConflictsWith reintroduced on either attribute would pass every test here unless
// something asserts its absence.
func TestSchemaDoesNotRefuseSelectedGroups(t *testing.T) {
	s := resourceSchema(t)

	boolAttr, ok := s.Attributes["all_device_groups"].(rschema.BoolAttribute)
	if !ok {
		t.Fatalf("all_device_groups must be a BoolAttribute, got %T", s.Attributes["all_device_groups"])
	}
	if len(boolAttr.Validators) != 0 {
		t.Errorf("all_device_groups must carry no schema validators: a ConflictsWith here fires whenever "+
			"both attributes are configured, and this attribute is Required, so device_group_ids could "+
			"never be set. Got %d", len(boolAttr.Validators))
	}

	setAttr, ok := s.Attributes["device_group_ids"].(rschema.SetAttribute)
	if !ok {
		t.Fatalf("device_group_ids must be a SetAttribute, got %T", s.Attributes["device_group_ids"])
	}
	for _, v := range setAttr.Validators {
		if strings.Contains(v.Description(context.Background()), "all_device_groups") {
			t.Errorf("device_group_ids must not conflict-check against all_device_groups in the schema: %s",
				v.Description(context.Background()))
		}
	}
}

// TestValidateAppFormTreatsUnknownAsPresent pins that the form rules key on whether
// an attribute was written, not on whether Terraform has resolved it. An attribute
// left out of a configuration is null; an attribute written from a variable or
// another resource is unknown. So `!IsNull()` is the correct presence test here, and
// the combination rule — a forbidden-when check, which STYLE_GUIDE notes is the safe
// direction — must still fire when both are unresolved.
func TestValidateAppFormTreatsUnknownAsPresent(t *testing.T) {
	cases := []struct {
		name       string
		appName    types.String
		predefined types.String
		wantErr    string
	}{
		{"unknown name alone is the custom form", types.StringUnknown(), types.StringNull(), ""},
		{"unknown predefined id alone is the predefined form", types.StringNull(), types.StringUnknown(), ""},
		{"both unresolved is still both", types.StringUnknown(), types.StringUnknown(), "cannot be renamed"},
		{"unknown name alongside a known predefined id", types.StringUnknown(), types.StringValue("2aaa401c"), "cannot be renamed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			validateAppForm(ZtnaAppResourceModel{Name: tc.appName, PredefinedAppID: tc.predefined}, &diags)

			if tc.wantErr == "" {
				if diags.HasError() {
					t.Fatalf("expected no error, got %s", diags)
				}
				return
			}
			if !strings.Contains(diagnosticsText(diags), tc.wantErr) {
				t.Fatalf("expected the error to mention %q, got %s", tc.wantErr, diags)
			}
		})
	}
}

// TestValidateDeviceGroupAssignmentDefersOnUnknownAllDeviceGroups pins the
// discriminator's own unknown guard. all_device_groups is Required with no default,
// so it is unknown whenever it comes from a variable or another resource, and
// ValueBool() reads an unknown as false — which is wrong in both directions.
//
// Read as false, a to-be-true flag lets `device_group_ids` past the conflict rule and
// the apply then fails with "Provider produced inconsistent result after apply"; and
// it runs the subset rule, which does not apply when the flag is true, refusing a
// configuration the server accepts. Per STYLE_GUIDE §Config-time validators MUST
// defer on unknown values, both cases must defer.
func TestValidateDeviceGroupAssignmentDefersOnUnknownAllDeviceGroups(t *testing.T) {
	const assignedGroup = "aaaaaaaa-0000-0000-0000-000000000000"
	const otherGroup = "cccccccc-0000-0000-0000-000000000000"

	cases := []struct {
		name      string
		assigned  []string
		overrides [][]string
	}{
		{"a group list that would conflict if the flag resolved true", []string{assignedGroup}, nil},
		{"an override on a group absent from the list", []string{assignedGroup}, [][]string{{otherGroup}}},
		{"an override with no group list at all", nil, [][]string{{otherGroup}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := ZtnaAppResourceModel{
				AllDeviceGroups:  types.BoolUnknown(),
				DeviceGroupIDs:   stringSetFor(t, tc.assigned),
				RoutingOverrides: overrideListFor(t, tc.overrides),
			}

			var got diag.Diagnostics
			validateDeviceGroupAssignment(context.Background(), config, &got)
			if got.HasError() {
				t.Fatalf("expected both rules to defer while all_device_groups is unresolved, got %s", got)
			}
		})
	}
}

// TestValidateDeviceGroupAssignmentStillFiresOnKnownAllDeviceGroups is the other half
// of that guard: deferring on unknown must not become deferring always, which the
// test above would pass just as happily if the whole function returned early.
func TestValidateDeviceGroupAssignmentStillFiresOnKnownAllDeviceGroups(t *testing.T) {
	config := ZtnaAppResourceModel{
		AllDeviceGroups:  types.BoolValue(true),
		DeviceGroupIDs:   stringSetFor(t, []string{"aaaaaaaa-0000-0000-0000-000000000000"}),
		RoutingOverrides: types.ListNull(routingOverrideObjectType),
	}

	var got diag.Diagnostics
	validateDeviceGroupAssignment(context.Background(), config, &got)
	if !strings.Contains(diagnosticsText(got), "conflict with all device groups") {
		t.Fatalf("expected the conflict rule to fire on a resolved flag, got %s", got)
	}
}

// TestRoutingOverrideModelsDefersOnUnknownElement pins the reader that replaced
// ElementsAs. ElementsAs refuses a list holding an unknown element outright, and its
// diagnostics were being discarded — so one override whose routing block came from a
// variable silently disabled every per-override rule in two config validators.
//
// The positional alignment is the other half: an unresolved element must still occupy
// its index, or every diagnostic after it would name the wrong one.
func TestRoutingOverrideModelsDefersOnUnknownElement(t *testing.T) {
	overrides := routingOverrideModels(ZtnaAppResourceModel{
		RoutingOverrides: overrideListOf(t,
			overrideElementValue(t, []string{"group-a"}, directRoutingObject(t)),
			types.ObjectUnknown(routingOverrideObjectType.AttrTypes),
			overrideElementValue(t, []string{"group-c"}, types.ObjectUnknown(routingObjectType.AttrTypes)),
		),
	})

	if len(overrides) != 3 {
		t.Fatalf("expected an entry per configured element, got %d", len(overrides))
	}
	if overrides[0] == nil || overrides[0].Routing == nil {
		t.Fatalf("expected the resolved element to survive, got %+v", overrides[0])
	}
	if groups, complete := knownStrings(overrides[0].DeviceGroupIDs); len(groups) != 1 || groups[0] != "group-a" || !complete {
		t.Errorf("expected the resolved element to keep its groups, got %v (complete=%v)", groups, complete)
	}
	if overrides[1] != nil {
		t.Errorf("expected an unresolved element to read as nil, got %+v", overrides[1])
	}
	if overrides[2] == nil {
		t.Fatal("expected an element with an unresolved routing block to still be read")
	}
	if overrides[2].Routing != nil {
		t.Errorf("expected an unresolved routing block to read as nil, got %+v", overrides[2].Routing)
	}
}

// TestRoutingOverrideModelsAbsentAndUnresolvedLists pins the two whole-collection
// cases, which have to read as "nothing to check" rather than as an error.
func TestRoutingOverrideModelsAbsentAndUnresolvedLists(t *testing.T) {
	for name, list := range map[string]types.List{
		"absent":     types.ListNull(routingOverrideObjectType),
		"unresolved": types.ListUnknown(routingOverrideObjectType),
	} {
		t.Run(name, func(t *testing.T) {
			if got := routingOverrideModels(ZtnaAppResourceModel{RoutingOverrides: list}); got != nil {
				t.Errorf("expected no overrides, got %+v", got)
			}
		})
	}
}

// TestValidateAllRoutingDefersOnUnresolvedOverrideRouting pins that an override whose
// routing block is unresolved defers, and — the half that matters more — that the
// checks on its resolved siblings still run, at the index the operator wrote. The
// discarded ElementsAs diagnostics used to skip all of them, leaving the server's
// unattributed `400 [INVALID_FIELD] routing: Routing definition is not valid.` as the
// only feedback.
func TestValidateAllRoutingDefersOnUnresolvedOverrideRouting(t *testing.T) {
	viaZTNA := routingModeLabels[securitycloud.RoutingTypeCustom]

	config := ZtnaAppResourceModel{
		RoutingOverrides: overrideListOf(t,
			overrideElementValue(t, []string{"group-a"}, types.ObjectUnknown(routingObjectType.AttrTypes)),
			overrideElementValue(t, []string{"group-b"}, routingObject(t, types.StringValue(viaZTNA), types.StringNull(), types.StringValue("Standard"))),
		),
	}

	var diags diag.Diagnostics
	validateAllRouting(context.Background(), config, &diags)

	if got := diags.ErrorsCount(); got != 1 {
		t.Fatalf("expected exactly the resolved override to fail, got %d errors: %s", got, diags)
	}
	withPath, ok := diags.Errors()[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatal("expected the error to be attached to an attribute path")
	}
	if got := withPath.Path().String(); got != "routing_overrides[1].routing.gateway_id" {
		t.Errorf("error is attached to %s, want routing_overrides[1].routing.gateway_id", got)
	}
}

// TestValidateAllRoutingDefersOnWhollyUnresolvedOverride pins the same for an element
// Terraform has not resolved at all.
func TestValidateAllRoutingDefersOnWhollyUnresolvedOverride(t *testing.T) {
	config := ZtnaAppResourceModel{
		RoutingOverrides: overrideListOf(t, types.ObjectUnknown(routingOverrideObjectType.AttrTypes)),
	}

	var diags diag.Diagnostics
	validateAllRouting(context.Background(), config, &diags)
	if diags.HasError() {
		t.Fatalf("expected an unresolved override to defer, got %s", diags)
	}
}

// TestValidateDeviceGroupAssignmentSurvivesUnresolvedOverride pins that the group
// rules keep working around an unresolved override, and that the index the duplicate
// diagnostic quotes is the one in the configuration rather than a count of the
// elements that happened to resolve.
func TestValidateDeviceGroupAssignmentSurvivesUnresolvedOverride(t *testing.T) {
	const group = "aaaaaaaa-0000-0000-0000-000000000000"

	config := ZtnaAppResourceModel{
		AllDeviceGroups: types.BoolValue(false),
		DeviceGroupIDs:  stringSetFor(t, []string{group}),
		RoutingOverrides: overrideListOf(t,
			types.ObjectUnknown(routingOverrideObjectType.AttrTypes),
			overrideElementValue(t, []string{group}, directRoutingObject(t)),
			overrideElementValue(t, []string{group}, directRoutingObject(t)),
		),
	}

	var diags diag.Diagnostics
	validateDeviceGroupAssignment(context.Background(), config, &diags)

	text := diagnosticsText(diags)
	if !strings.Contains(text, "more than one routing override") {
		t.Fatalf("expected the duplicate rule to still fire past an unresolved element, got %s", diags)
	}
	if !strings.Contains(text, "at index 1") {
		t.Errorf("expected the diagnostic to quote the configured index, got %s", text)
	}
}

// TestValidateResourceReachesEveryRule pins the wire from the framework to each inner
// rule. Every ValidateResource in this file was at zero coverage: deleting the
// validateX call from any of the four left the whole package suite green, and the
// registration test only counts the returned slice, so dropping one validator and
// duplicating another passed that too.
func TestValidateResourceReachesEveryRule(t *testing.T) {
	viaZTNA := routingModeLabels[securitycloud.RoutingTypeCustom]

	cases := []struct {
		name      string
		validator resource.ConfigValidator
		overrides map[string]tftypes.Value
		wantErr   string
	}{
		{
			name:      "app form",
			validator: appFormValidator{},
			wantErr:   "needs a name",
		},
		{
			name:      "routing combination",
			validator: routingCombinationValidator{},
			overrides: map[string]tftypes.Value{"routing": routingTFValue(t, viaZTNA, nil, nil)},
			wantErr:   "needs an access gateway",
		},
		{
			name:      "device group assignment",
			validator: deviceGroupAssignmentValidator{},
			overrides: map[string]tftypes.Value{
				"all_device_groups": tftypes.NewValue(tftypes.Bool, true),
				"device_group_ids":  stringSetTFValue("aaaaaaaa-0000-0000-0000-000000000000"),
			},
			wantErr: "conflict with all device groups",
		},
		{
			name:      "hostname overlap",
			validator: hostnameOverlapValidator{},
			overrides: map[string]tftypes.Value{"hostnames": stringSetTFValue("*", "example.com")},
			wantErr:   "Host names overlap",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp resource.ValidateConfigResponse
			tc.validator.ValidateResource(context.Background(), resource.ValidateConfigRequest{
				Config: resourceConfigWith(t, tc.overrides),
			}, &resp)

			if !strings.Contains(diagnosticsText(resp.Diagnostics), tc.wantErr) {
				t.Fatalf("expected %T to report %q, got %s", tc.validator, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

// TestValidateResourceDefersOnUnresolvedRouting pins the config read every one of the
// four validators shares. `routing` is Required and decodes into a Go struct pointer,
// so `routing = var.routing` in a module made req.Config.Get refuse the whole decode
// with "the target type cannot handle unknown values" — reported to the operator as a
// provider bug, and taking out all four validators at once.
func TestValidateResourceDefersOnUnresolvedRouting(t *testing.T) {
	config := resourceConfigWith(t, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, "Internal CRM"),
		"all_device_groups": tftypes.NewValue(tftypes.Bool, false),
		"routing":           tftypes.NewValue(routingTFType(t), tftypes.UnknownValue),
	})

	for _, v := range configValidatorsUnderTest() {
		var resp resource.ValidateConfigResponse
		v.ValidateResource(context.Background(), resource.ValidateConfigRequest{Config: config}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%T must defer while `routing` is unresolved, got %s", v, resp.Diagnostics)
		}
	}
}

// TestValidateResourceDefersOnUnresolvedAllDeviceGroups is the framework-level form of
// the unknown-discriminator guard, fed tftypes.UnknownValue as STYLE_GUIDE prescribes:
// acceptance tests use literal HCL, which is always known, and so cannot reach this
// path at all.
func TestValidateResourceDefersOnUnresolvedAllDeviceGroups(t *testing.T) {
	config := resourceConfigWith(t, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, "Internal CRM"),
		"all_device_groups": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"device_group_ids":  stringSetTFValue("aaaaaaaa-0000-0000-0000-000000000000"),
	})

	for _, v := range configValidatorsUnderTest() {
		var resp resource.ValidateConfigResponse
		v.ValidateResource(context.Background(), resource.ValidateConfigRequest{Config: config}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%T must defer while `all_device_groups` is unresolved, got %s", v, resp.Diagnostics)
		}
	}
}

// configValidatorsUnderTest lists the four validators this file defines. It is
// deliberately a literal rather than a call to the resource's ConfigValidators, so a
// validator dropped from the registration cannot quietly drop it from these tests too.
func configValidatorsUnderTest() []resource.ConfigValidator {
	return []resource.ConfigValidator{
		appFormValidator{},
		routingCombinationValidator{},
		deviceGroupAssignmentValidator{},
		hostnameOverlapValidator{},
	}
}

// resourceConfigWith builds a framework configuration from the resource schema with
// every attribute null, then replaces the named ones. Building it from the real schema
// is what makes these tests exercise the same decode the framework performs.
func resourceConfigWith(t *testing.T, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	s := resourceSchema(t)
	objectType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("expected the resource schema to be an object type, got %T", s.Type().TerraformType(context.Background()))
	}

	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	for name, value := range overrides {
		if _, ok := objectType.AttributeTypes[name]; !ok {
			t.Fatalf("the resource schema has no attribute %q — a rename left this test behind", name)
		}
		values[name] = value
	}
	return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objectType, values)}
}

// routingTFType returns the wire type of a routing block.
func routingTFType(t *testing.T) tftypes.Type {
	t.Helper()
	return routingObjectType.TerraformType(context.Background())
}

// routingTFValue builds a routing block at the wire level, so a member can be handed
// tftypes.UnknownValue.
func routingTFValue(t *testing.T, trafficRouting, gatewayID, routingMode any) tftypes.Value {
	t.Helper()
	return tftypes.NewValue(routingTFType(t), map[string]tftypes.Value{
		"traffic_routing": tftypes.NewValue(tftypes.String, trafficRouting),
		"gateway_id":      tftypes.NewValue(tftypes.String, gatewayID),
		"routing_mode":    tftypes.NewValue(tftypes.String, routingMode),
	})
}

// stringSetTFValue builds a set of strings at the wire level.
func stringSetTFValue(values ...string) tftypes.Value {
	elements := make([]tftypes.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, tftypes.NewValue(tftypes.String, value))
	}
	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elements)
}

// routingObject builds a routing block as a framework object value.
func routingObject(t *testing.T, trafficRouting, gatewayID, routingMode types.String) types.Object {
	t.Helper()
	object, diags := types.ObjectValue(routingObjectType.AttrTypes, map[string]attr.Value{
		"traffic_routing": trafficRouting,
		"gateway_id":      gatewayID,
		"routing_mode":    routingMode,
	})
	if diags.HasError() {
		t.Fatalf("building routing object: %s", diags)
	}
	return object
}

// directRoutingObject builds the routing block every valid fixture here uses.
func directRoutingObject(t *testing.T) types.Object {
	t.Helper()
	return routingObject(t,
		types.StringValue(routingModeLabels[securitycloud.RoutingTypeDirect]),
		types.StringNull(),
		types.StringNull(),
	)
}

// overrideElementValue builds one routing_overrides element, accepting any routing value so
// an unresolved block can be placed inside a resolved element.
func overrideElementValue(t *testing.T, groups []string, routing attr.Value) attr.Value {
	t.Helper()
	set, diags := types.SetValueFrom(context.Background(), types.StringType, groups)
	if diags.HasError() {
		t.Fatalf("building override group set: %s", diags)
	}
	object, diags := types.ObjectValue(routingOverrideObjectType.AttrTypes, map[string]attr.Value{
		"device_group_ids": set,
		"routing":          routing,
	})
	if diags.HasError() {
		t.Fatalf("building override element: %s", diags)
	}
	return object
}

// overrideListOf assembles a routing_overrides list from element values, which
// buildOverrideList cannot do because it goes through a Go model and so cannot hold an
// unresolved value.
func overrideListOf(t *testing.T, elements ...attr.Value) types.List {
	t.Helper()
	list, diags := types.ListValue(routingOverrideObjectType, elements)
	if diags.HasError() {
		t.Fatalf("building override list: %s", diags)
	}
	return list
}
