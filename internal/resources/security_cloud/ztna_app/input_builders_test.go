// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestRoutingToWireEmitsExplicitNulls is the single most load-bearing test in this
// package. Merge patch merges the routing object field by field, so a direct-routing
// body that *omits* the gateway and resolution mode leaves the previous values in
// place and the server refuses the write — the SDK's emitNullForOptional entry for
// this schema exists so a nil pointer marshals as `null` instead. This asserts the
// marshalled JSON, not the struct, because the struct is identical either way and
// only the tag decides which body goes out.
func TestRoutingToWireEmitsExplicitNulls(t *testing.T) {
	routing := routingToWire(&RoutingModel{
		TrafficRouting: types.StringValue(routingModeLabels[securitycloud.RoutingTypeDirect]),
		GatewayID:      types.StringNull(),
		RoutingMode:    types.StringNull(),
	})

	body, err := json.Marshal(routing)
	if err != nil {
		t.Fatalf("marshalling routing: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding routing: %v", err)
	}
	for _, key := range []string{"gatewayId", "dnsIpResolutionType"} {
		raw, present := decoded[key]
		if !present {
			t.Fatalf("%s must be present as an explicit null, or a routing transition to direct routing "+
				"cannot clear it; body was %s", key, body)
		}
		if string(raw) != "null" {
			t.Errorf("%s = %s, want null", key, raw)
		}
	}
}

// TestRoutingToWireTranslatesLabels pins that both enumerated members reach the wire
// as the values the API stores, not as the admin-UI labels the schema exposes.
func TestRoutingToWireTranslatesLabels(t *testing.T) {
	routing := routingToWire(&RoutingModel{
		TrafficRouting: types.StringValue(routingModeLabels[securitycloud.RoutingTypeCustom]),
		GatewayID:      types.StringValue("a7d2"),
		RoutingMode:    types.StringValue("Legacy"),
	})

	if routing.Type != securitycloud.RoutingTypeCustom {
		t.Errorf("Type = %q, want %q", routing.Type, securitycloud.RoutingTypeCustom)
	}
	if routing.GatewayID == nil || *routing.GatewayID != "a7d2" {
		t.Errorf("GatewayID = %v, want a7d2", routing.GatewayID)
	}
	if routing.DnsIpResolutionType == nil || *routing.DnsIpResolutionType != securitycloud.RoutingDnsIpResolutionTypeIPv4 {
		t.Errorf("DnsIpResolutionType = %v, want %q", routing.DnsIpResolutionType, securitycloud.RoutingDnsIpResolutionTypeIPv4)
	}
}

// TestRoutingToWireStartsFromTheOperatorLiteral pins the operator-visible half of
// the routing contract: the exact string an operator types in HCL has to reach the
// wire as the matching stored type. TestRoutingToWireTranslatesLabels cannot pin it,
// because it takes its input from routingModeLabels and then asserts the output
// against that same table's key — transposing the table's two values passes it
// unchanged. Starting from the literal is what makes a transposition fail, and a
// transposition is a misroute: traffic an operator asked to keep off the access
// gateway sent through it, or ZTNA traffic silently going direct.
func TestRoutingToWireStartsFromTheOperatorLiteral(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"Encrypt and route via ZTNA", securitycloud.RoutingTypeCustom},
		{"Direct device routing", securitycloud.RoutingTypeDirect},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			got := routingToWire(&RoutingModel{
				TrafficRouting: types.StringValue(tc.label),
				GatewayID:      types.StringNull(),
				RoutingMode:    types.StringNull(),
			})
			if got.Type != tc.want {
				t.Errorf("traffic_routing = %q reached the wire as %q, want %q", tc.label, got.Type, tc.want)
			}
		})
	}
}

// TestAssignmentsAllUsersFollowsThePlan pins that all_device_groups reaches the wire
// rather than being hard-coded. Every other builder test leaves it at the basePlan
// value of true and asserts something else, so replacing the read with a literal
// `false` survives the whole suite — an app scoped to every device silently narrowed
// to the named groups, or to nothing at all. Both builders are asserted because they
// call assignmentsFromPlan independently and a regression could land in either.
func TestAssignmentsAllUsersFollowsThePlan(t *testing.T) {
	cases := []struct {
		name      string
		allGroups bool
	}{
		{"every device group", true},
		{"named groups only", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := basePlan(t)
			plan.Name = types.StringValue("Internal CRM")
			plan.AllDeviceGroups = types.BoolValue(tc.allGroups)
			plan.DeviceGroupIDs = stringSetFor(t, []string{"group-a"})

			create, diags := buildAppCreateInput(context.Background(), plan)
			if diags.HasError() {
				t.Fatalf("building create input: %s", diags)
			}
			if got := create.Assignments.Inclusions.AllUsers; got != tc.allGroups {
				t.Errorf("create allUsers = %v, want %v", got, tc.allGroups)
			}

			patch, patchDiags := buildAppPatchInput(context.Background(), plan)
			if patchDiags.HasError() {
				t.Fatalf("building patch input: %s", patchDiags)
			}
			if patch.Assignments == nil {
				t.Fatal("the patch body must carry assignments, or the configuration is not authoritative")
			}
			if got := patch.Assignments.Inclusions.AllUsers; got != tc.allGroups {
				t.Errorf("patch allUsers = %v, want %v", got, tc.allGroups)
			}
		})
	}
}

// TestRoutingToWireIgnoresUnknowns pins that a value Terraform has not resolved yet
// is sent as null rather than as an empty string, which the enum would refuse.
func TestRoutingToWireIgnoresUnknowns(t *testing.T) {
	routing := routingToWire(&RoutingModel{
		TrafficRouting: types.StringValue(routingModeLabels[securitycloud.RoutingTypeCustom]),
		GatewayID:      types.StringUnknown(),
		RoutingMode:    types.StringUnknown(),
	})
	if routing.GatewayID != nil || routing.DnsIpResolutionType != nil {
		t.Errorf("unknown members must not reach the wire: gateway=%v resolution=%v",
			routing.GatewayID, routing.DnsIpResolutionType)
	}
}

// TestBuildAppCreateInputCustom pins the create body for a custom application.
func TestBuildAppCreateInputCustom(t *testing.T) {
	plan := basePlan(t)
	plan.Name = types.StringValue("Internal CRM")
	plan.Hostnames = stringSetFor(t, []string{"crm.example.com"})
	plan.DirectIPsAndSubnets = stringSetFor(t, []string{"10.1.2.0/24"})

	input, diags := buildAppCreateInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building create input: %s", diags)
	}

	if input.Name == nil || *input.Name != "Internal CRM" {
		t.Errorf("Name = %v, want Internal CRM", input.Name)
	}
	if input.PredefinedAppID != nil {
		t.Errorf("PredefinedAppID = %v, want nil for a custom application", *input.PredefinedAppID)
	}
	if input.CategoryName != "Uncategorized" {
		t.Errorf("CategoryName = %q", input.CategoryName)
	}
	if input.Hostnames == nil || len(*input.Hostnames) != 1 {
		t.Errorf("Hostnames = %v", input.Hostnames)
	}
	if input.BareIps == nil || len(*input.BareIps) != 1 {
		t.Errorf("BareIps = %v", input.BareIps)
	}
}

// TestBuildAppCreateInputPredefined pins that a predefined application sends its
// definition ID and no name — the server would accept a name and silently discard
// it, so sending one would put a value in the request that never comes back.
func TestBuildAppCreateInputPredefined(t *testing.T) {
	plan := basePlan(t)
	plan.PredefinedAppID = types.StringValue("2aaa401c-232e-4db1-8384-6a94d9fc264e")

	input, diags := buildAppCreateInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building create input: %s", diags)
	}
	if input.Name != nil {
		t.Errorf("Name = %v, want nil for a predefined application", *input.Name)
	}
	if input.PredefinedAppID == nil {
		t.Fatal("PredefinedAppID must be sent")
	}
}

// TestBuildAppCreateInputSendsEmptyCollections pins that an absent collection goes
// out as `[]` rather than being omitted. On create the two are indistinguishable, but
// the same builder shape is what makes clearing work on update, and asserting it
// here is what stops a well-meaning "omit when empty" change from breaking that
// silently.
func TestBuildAppCreateInputSendsEmptyCollections(t *testing.T) {
	plan := basePlan(t)
	plan.Name = types.StringValue("Bare")

	input, diags := buildAppCreateInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building create input: %s", diags)
	}

	if input.Hostnames == nil || len(*input.Hostnames) != 0 {
		t.Errorf("Hostnames = %v, want an empty slice", input.Hostnames)
	}
	if input.BareIps == nil || len(*input.BareIps) != 0 {
		t.Errorf("BareIps = %v, want an empty slice", input.BareIps)
	}
	if input.Assignments.Inclusions.Groups == nil || len(*input.Assignments.Inclusions.Groups) != 0 {
		t.Errorf("Groups = %v, want an empty slice", input.Assignments.Inclusions.Groups)
	}
	if input.GroupOverrides == nil || input.GroupOverrides.RoutingOverrides == nil {
		t.Fatal("GroupOverrides must be sent so removing every override clears them")
	}
	if len(*input.GroupOverrides.RoutingOverrides) != 0 {
		t.Errorf("RoutingOverrides = %v, want empty", *input.GroupOverrides.RoutingOverrides)
	}
}

// TestBuildAppPatchInputClearsCollections pins the update semantics that follow from
// every collection being Optional rather than Optional+Computed: an attribute removed
// from the configuration has to clear on the server, which merge patch only does when
// the key is present and empty.
func TestBuildAppPatchInputClearsCollections(t *testing.T) {
	plan := basePlan(t)
	plan.Name = types.StringValue("Internal CRM")
	plan.ID = types.StringValue("00000000-0000-0000-0000-000000000001")

	input, diags := buildAppPatchInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building patch input: %s", diags)
	}

	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshalling patch: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding patch: %v", err)
	}
	for _, key := range []string{"hostnames", "bareIps", "groupOverrides", "assignments", "routing", "categoryName"} {
		if _, present := decoded[key]; !present {
			t.Errorf("%s must be present in the patch body, or the configuration is not authoritative; got %s", key, body)
		}
	}
	for _, key := range []string{"hostnames", "bareIps"} {
		if string(decoded[key]) != "[]" {
			t.Errorf("%s = %s, want [] so an absent attribute clears", key, decoded[key])
		}
	}
}

// TestBuildAppPatchInputSkipsNameForPredefined pins that the patch body carries no
// name for a predefined application. The server accepts one and ignores it, so
// sending it would be a write that silently does nothing.
func TestBuildAppPatchInputSkipsNameForPredefined(t *testing.T) {
	plan := basePlan(t)
	plan.PredefinedAppID = types.StringValue("2aaa401c")
	plan.Name = types.StringValue("should not be sent")

	input, diags := buildAppPatchInput(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building patch input: %s", diags)
	}
	if input.Name != nil {
		t.Errorf("Name = %v, want nil for a predefined application", *input.Name)
	}
}

// TestSecurityToWire pins the three card mappings, including the one no reader would
// guess: the admin UI's "Access requires Jamf Trust to be enabled" is the wire's
// dohIntegration.blocking.
func TestSecurityToWire(t *testing.T) {
	security := &SecurityModel{
		ManagedDevice: &SecurityControlModel{
			Enabled:                 types.BoolValue(true),
			DevicePushNotifications: types.BoolValue(false),
		},
		DeviceRisk: &DeviceRiskModel{
			Enabled:                 types.BoolValue(true),
			DenyAtRiskLevel:         types.StringValue("Medium"),
			DevicePushNotifications: types.BoolValue(true),
		},
		JamfTrust: &SecurityControlModel{
			Enabled:                 types.BoolValue(true),
			DevicePushNotifications: types.BoolValue(true),
		},
	}

	got := securityToWire(security)
	if got == nil {
		t.Fatal("securityToWire returned nil for a populated block")
	}
	if got.DeviceManagementBasedAccess == nil || !got.DeviceManagementBasedAccess.Enabled {
		t.Error("managed_device.enabled did not reach deviceManagementBasedAccess.enabled")
	}
	if got.DeviceManagementBasedAccess.NotificationsEnabled {
		t.Error("managed_device.device_push_notifications did not reach notificationsEnabled")
	}
	if got.RiskControls == nil || got.RiskControls.LevelThreshold != securitycloud.RiskControlsLevelThresholdMedium {
		t.Errorf("deny_at_risk_level did not translate: %v", got.RiskControls)
	}
	if got.DohIntegration == nil || !got.DohIntegration.Blocking {
		t.Error("jamf_trust.enabled did not reach dohIntegration.blocking")
	}
}

// TestSecurityToWireOmitsUndeclaredCards pins the Optional-only contract: a card the
// configuration does not mention is left out of the body entirely, so merge patch
// leaves Jamf's own setting alone.
func TestSecurityToWireOmitsUndeclaredCards(t *testing.T) {
	got := securityToWire(&SecurityModel{
		JamfTrust: &SecurityControlModel{
			Enabled:                 types.BoolValue(true),
			DevicePushNotifications: types.BoolValue(true),
		},
	})
	if got.DeviceManagementBasedAccess != nil || got.RiskControls != nil {
		t.Error("an undeclared card must not reach the wire")
	}
	if got.DohIntegration == nil {
		t.Error("a declared card must reach the wire")
	}

	body, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshalling security: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding security: %v", err)
	}
	for _, key := range []string{"deviceManagementBasedAccess", "riskControls"} {
		if _, present := decoded[key]; present {
			t.Errorf("%s must be absent from the body, not null, or merge patch would clear it: %s", key, body)
		}
	}
}

// TestSecurityToWireNilBlock pins that omitting the whole block sends no security at
// all, leaving every card as Jamf holds it.
func TestSecurityToWireNilBlock(t *testing.T) {
	if got := securityToWire(nil); got != nil {
		t.Errorf("securityToWire(nil) = %v, want nil", got)
	}
}

// TestGroupOverridesFromPlan pins that each override's groups and routing both reach
// the wire, and that an absent list yields an empty slice rather than nil.
func TestGroupOverridesFromPlan(t *testing.T) {
	plan := basePlan(t)
	plan.RoutingOverrides = overrideListFor(t, [][]string{{"group-a"}, {"group-b"}})

	overrides, diags := groupOverridesFromPlan(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building overrides: %s", diags)
	}
	if len(overrides) != 2 {
		t.Fatalf("got %d overrides, want 2", len(overrides))
	}
	if overrides[0].Routing.Type != securitycloud.RoutingTypeDirect {
		t.Errorf("override routing did not translate: %q", overrides[0].Routing.Type)
	}

	plan.RoutingOverrides = types.ListNull(routingOverrideObjectType)
	overrides, diags = groupOverridesFromPlan(context.Background(), plan)
	if diags.HasError() {
		t.Fatalf("building overrides: %s", diags)
	}
	if overrides == nil || len(overrides) != 0 {
		t.Errorf("an absent override list must yield an empty slice, got %v", overrides)
	}
}

// basePlan returns a minimal valid plan model: a custom application with direct
// routing, all device groups, and no collections.
func basePlan(t *testing.T) ZtnaAppResourceModel {
	t.Helper()
	return ZtnaAppResourceModel{
		Name:                types.StringNull(),
		PredefinedAppID:     types.StringNull(),
		Category:            types.StringValue("Uncategorized"),
		Hostnames:           types.SetNull(types.StringType),
		DirectIPsAndSubnets: types.SetNull(types.StringType),
		AllDeviceGroups:     types.BoolValue(true),
		DeviceGroupIDs:      types.SetNull(types.StringType),
		Routing: &RoutingModel{
			TrafficRouting: types.StringValue(routingModeLabels[securitycloud.RoutingTypeDirect]),
			GatewayID:      types.StringNull(),
			RoutingMode:    types.StringNull(),
		},
		RoutingOverrides: types.ListNull(routingOverrideObjectType),
	}
}
