// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestAssignAppResourceModelCollapsesEmptyCollections pins the reconciliation that
// stops an application with no host names from diffing against its own refresh
// forever. The endpoint returns `[]` rather than omitting an empty collection, and
// the schema refuses an explicit empty collection, so `[]` can only have come from an
// absent attribute and has to read back as null.
func TestAssignAppResourceModelCollapsesEmptyCollections(t *testing.T) {
	state := ZtnaAppResourceModel{}
	app := &securitycloud.App{
		ID:           "00000000-0000-0000-0000-000000000001",
		CategoryName: "Uncategorized",
		Hostnames:    []string{},
		BareIps:      []string{},
		Assignments: &securitycloud.Assignments{
			Inclusions: securitycloud.AssignmentsInclusions{AllUsers: true, Groups: &[]string{}},
		},
		Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
	}

	diags := assignAppResourceModel(context.Background(), &state, app)
	if diags.HasError() {
		t.Fatalf("assigning state: %s", diags)
	}

	for name, set := range map[string]types.Set{
		"hostnames":              state.Hostnames,
		"direct_ips_and_subnets": state.DirectIPsAndSubnets,
		"device_group_ids":       state.DeviceGroupIDs,
	} {
		if !set.IsNull() {
			t.Errorf("%s must collapse an empty response to null, got %s", name, set)
		}
	}
	if !state.RoutingOverrides.IsNull() {
		t.Errorf("routing_overrides must collapse an empty response to null, got %s", state.RoutingOverrides)
	}
}

// TestAssignAppResourceModelDerivesAppType pins the derived attribute for both forms.
func TestAssignAppResourceModelDerivesAppType(t *testing.T) {
	predefined := "2aaa401c-232e-4db1-8384-6a94d9fc264e"
	name := "Internal CRM"

	cases := []struct {
		label   string
		app     *securitycloud.App
		wantTyp string
		wantNil bool
	}{
		{"predefined", &securitycloud.App{PredefinedAppID: &predefined}, appTypePredefined, true},
		{"custom", &securitycloud.App{Name: &name}, appTypeCustom, false},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			tc.app.Routing = &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect}
			state := ZtnaAppResourceModel{}
			if diags := assignAppResourceModel(context.Background(), &state, tc.app); diags.HasError() {
				t.Fatalf("assigning state: %s", diags)
			}
			if state.AppType.ValueString() != tc.wantTyp {
				t.Errorf("app_type = %q, want %q", state.AppType.ValueString(), tc.wantTyp)
			}
			if state.Name.IsNull() != tc.wantNil {
				t.Errorf("name null = %v, want %v", state.Name.IsNull(), tc.wantNil)
			}
		})
	}
}

// TestAssignAppResourceModelGatesSecurityOnTheTarget is the reconciliation that keeps
// state honest about what the configuration asked for. The server always returns all
// three cards with its own defaults, so copying the response across would put values
// in state the operator never wrote and the next plan would show them as removals.
func TestAssignAppResourceModelGatesSecurityOnTheTarget(t *testing.T) {
	response := &securitycloud.AppSecurity{
		DeviceManagementBasedAccess: &securitycloud.DeviceManagementBasedAccess{Enabled: true, NotificationsEnabled: true},
		RiskControls: &securitycloud.RiskControls{
			Enabled:              true,
			LevelThreshold:       securitycloud.RiskControlsLevelThresholdHigh,
			NotificationsEnabled: true,
		},
		DohIntegration: &securitycloud.DohIntegration{Blocking: true, NotificationsEnabled: true},
	}

	t.Run("no security block declared", func(t *testing.T) {
		state := ZtnaAppResourceModel{}
		app := &securitycloud.App{Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect}, Security: response}
		if diags := assignAppResourceModel(context.Background(), &state, app); diags.HasError() {
			t.Fatalf("assigning state: %s", diags)
		}
		if state.Security != nil {
			t.Errorf("security must stay absent when the configuration never declared it, got %+v", state.Security)
		}
	})

	t.Run("one card declared", func(t *testing.T) {
		state := ZtnaAppResourceModel{
			Security: &SecurityModel{JamfTrust: &SecurityControlModel{}},
		}
		app := &securitycloud.App{Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect}, Security: response}
		if diags := assignAppResourceModel(context.Background(), &state, app); diags.HasError() {
			t.Fatalf("assigning state: %s", diags)
		}
		if state.Security == nil || state.Security.JamfTrust == nil {
			t.Fatal("the declared card must be populated")
		}
		if !state.Security.JamfTrust.Enabled.ValueBool() {
			t.Error("jamf_trust.enabled must come from dohIntegration.blocking")
		}
		if state.Security.ManagedDevice != nil || state.Security.DeviceRisk != nil {
			t.Error("an undeclared card must stay absent even though the server reports one")
		}
	})
}

// TestRoutingFromWireMapsLabels pins the read side of both enum translations,
// including that an absent resolution mode stays absent rather than becoming a
// label.
func TestRoutingFromWireMapsLabels(t *testing.T) {
	gateway := "a7d2"
	ipv4 := securitycloud.RoutingDnsIpResolutionTypeIPv4

	viaZTNA := routingFromWire(&securitycloud.Routing{
		Type:                securitycloud.RoutingTypeCustom,
		GatewayID:           &gateway,
		DnsIpResolutionType: &ipv4,
	})
	if viaZTNA.TrafficRouting.ValueString() != routingModeLabels[securitycloud.RoutingTypeCustom] {
		t.Errorf("mode = %q", viaZTNA.TrafficRouting.ValueString())
	}
	if viaZTNA.RoutingMode.ValueString() != "Legacy" {
		t.Errorf("routing_mode = %q, want Legacy", viaZTNA.RoutingMode.ValueString())
	}

	direct := routingFromWire(&securitycloud.Routing{Type: securitycloud.RoutingTypeDirect})
	if !direct.GatewayID.IsNull() || !direct.RoutingMode.IsNull() {
		t.Errorf("direct routing must read back with both members null, got gateway=%s resolution=%s",
			direct.GatewayID, direct.RoutingMode)
	}
	if routingFromWire(nil) != nil {
		t.Error("routingFromWire(nil) must stay nil")
	}
}

// TestAssignAppDataSourceModelKeepsEmptyCollections pins the difference between the
// two read paths. A data source reports what the server holds with no configuration
// to reconcile against, so an empty collection stays empty rather than collapsing,
// and all three security cards are always populated.
func TestAssignAppDataSourceModelKeepsEmptyCollections(t *testing.T) {
	var ds ZtnaAppDataSourceModel
	app := &securitycloud.App{
		ID:           "00000000-0000-0000-0000-000000000001",
		CategoryName: "Uncategorized",
		Hostnames:    []string{},
		BareIps:      []string{},
		Assignments: &securitycloud.Assignments{
			Inclusions: securitycloud.AssignmentsInclusions{AllUsers: true, Groups: &[]string{}},
		},
		Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
		Security: &securitycloud.AppSecurity{
			DeviceManagementBasedAccess: &securitycloud.DeviceManagementBasedAccess{},
			RiskControls:                &securitycloud.RiskControls{LevelThreshold: securitycloud.RiskControlsLevelThresholdLow},
			DohIntegration:              &securitycloud.DohIntegration{},
		},
	}

	if diags := assignAppDataSourceModel(context.Background(), &ds, app); diags.HasError() {
		t.Fatalf("assigning data source state: %s", diags)
	}

	for name, list := range map[string]types.List{
		"hostnames":              ds.Hostnames,
		"direct_ips_and_subnets": ds.DirectIPsAndSubnets,
		"device_group_ids":       ds.DeviceGroupIDs,
		"routing_overrides":      ds.RoutingOverrides,
	} {
		if list.IsNull() {
			t.Errorf("%s must stay an empty list on the data source, got null", name)
		}
		if len(list.Elements()) != 0 {
			t.Errorf("%s = %s, want empty", name, list)
		}
	}
	if ds.Security.IsNull() {
		t.Fatal("the data source must always report the security block")
	}
}

// TestAssignAppResourceModelTolerantOfNoAssignments pins that a response missing the
// assignments object does not panic or produce a half-filled model. The spec marks it
// required, but every optional pointer on a generated type is one a server change
// could start omitting.
func TestAssignAppResourceModelTolerantOfNoAssignments(t *testing.T) {
	state := ZtnaAppResourceModel{}
	app := &securitycloud.App{
		ID:      "00000000-0000-0000-0000-000000000001",
		Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
	}
	if diags := assignAppResourceModel(context.Background(), &state, app); diags.HasError() {
		t.Fatalf("assigning state: %s", diags)
	}
	if state.AllDeviceGroups.ValueBool() {
		t.Error("all_device_groups must default to false when the response carries no assignments")
	}
	if !state.DeviceGroupIDs.IsNull() {
		t.Error("device_group_ids must be null when the response carries no assignments")
	}
}

// TestAssignAppResourceModelReadsAllUsers pins the read side of all_device_groups.
// The assignment could be a hard-coded `types.BoolValue(false)` and nothing would
// fail: TestAssignAppResourceModelCollapsesEmptyCollections feeds `AllUsers: true`
// but asserts only the collapsed sets, and
// TestAssignAppResourceModelTolerantOfNoAssignments asserts false, which a
// hard-coded false satisfies. Getting it wrong makes an app scoped to every device
// read back as if it were not, so the next plan proposes narrowing the scope of a
// live application. Both directions are asserted, and the data source path is
// covered alongside because it reads the same field through separate code.
func TestAssignAppResourceModelReadsAllUsers(t *testing.T) {
	cases := []struct {
		name     string
		allUsers bool
	}{
		{"every device group", true},
		{"named groups only", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &securitycloud.App{
				ID:      "00000000-0000-0000-0000-000000000001",
				Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
				Assignments: &securitycloud.Assignments{
					Inclusions: securitycloud.AssignmentsInclusions{
						AllUsers: tc.allUsers,
						Groups:   &[]string{"group-a"},
					},
				},
			}

			state := ZtnaAppResourceModel{}
			if diags := assignAppResourceModel(context.Background(), &state, app); diags.HasError() {
				t.Fatalf("assigning state: %s", diags)
			}
			if got := state.AllDeviceGroups.ValueBool(); got != tc.allUsers {
				t.Errorf("all_device_groups = %v, want %v", got, tc.allUsers)
			}

			var ds ZtnaAppDataSourceModel
			if diags := assignAppDataSourceModel(context.Background(), &ds, app); diags.HasError() {
				t.Fatalf("assigning data source state: %s", diags)
			}
			if got := ds.AllDeviceGroups.ValueBool(); got != tc.allUsers {
				t.Errorf("data source all_device_groups = %v, want %v", got, tc.allUsers)
			}
		})
	}
}

// TestRoutingOverrideListValuePopulated is the only unit test that reaches the
// non-empty branch of either routing_overrides read path. Every other test in the
// package feeds an empty or nil override list, so only the `len(entries) == 0` early
// return runs and replacing an entry's GroupIds with `[]string{}` survives the whole
// suite — a per-group override reading back as naming no groups, which the next plan
// proposes writing back over the server's real scope.
//
// Three things are asserted that no other test can reach. The group IDs have to
// round-trip. The nested routing object has to carry the translated admin-UI label
// rather than the value the wire sent. And the nested device_group_ids collection has
// to keep the two paths' deliberate asymmetry — a set on the resource, because the
// server re-orders and de-duplicates it, and a list on the data source, which reports
// what the server holds (see the type tables in schema_types.go).
//
// Outer order is asserted positionally because the server round-trips override order
// verbatim: three overrides sent deliberately non-sorted came back in the order sent
// (wire-confirmed 2026-08-30), which is why routing_overrides is a list and not a set.
func TestRoutingOverrideListValuePopulated(t *testing.T) {
	gateway := "a7d2"
	ipv4 := securitycloud.RoutingDnsIpResolutionTypeIPv4
	overrides := &securitycloud.GroupOverrides{
		RoutingOverrides: &[]securitycloud.RoutingOverride{
			{
				GroupIds: []string{"group-b", "group-a"},
				Routing: securitycloud.Routing{
					Type:                securitycloud.RoutingTypeCustom,
					GatewayID:           &gateway,
					DnsIpResolutionType: &ipv4,
				},
			},
			{
				GroupIds: []string{"group-d", "group-c"},
				Routing:  securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
			},
		},
	}

	wantGroups := [][]string{{"group-a", "group-b"}, {"group-c", "group-d"}}
	wantRouting := []string{"Encrypt and route via ZTNA", "Direct device routing"}

	t.Run("resource", func(t *testing.T) {
		list, diags := routingOverrideListValue(context.Background(), overrides)
		if diags.HasError() {
			t.Fatalf("building routing_overrides: %s", diags)
		}
		if len(list.Elements()) != len(wantGroups) {
			t.Fatalf("routing_overrides = %s, want %d entries", list, len(wantGroups))
		}
		for i := range wantGroups {
			override := overrideElement(t, list, i)
			if got := overrideGroupIDs(t, override); !slices.Equal(got, wantGroups[i]) {
				t.Errorf("routing_overrides[%d].device_group_ids = %v, want %v", i, got, wantGroups[i])
			}
			if got := overrideTrafficRouting(t, override); got != wantRouting[i] {
				t.Errorf("routing_overrides[%d].routing.traffic_routing = %q, want %q", i, got, wantRouting[i])
			}
			if raw := override.Attributes()["device_group_ids"]; !isSet(raw) {
				t.Errorf("routing_overrides[%d].device_group_ids is %T, want a set on the resource", i, raw)
			}
		}
	})

	t.Run("data source", func(t *testing.T) {
		list, diags := dsRoutingOverrideListValue(context.Background(), overrides)
		if diags.HasError() {
			t.Fatalf("building routing_overrides: %s", diags)
		}
		if len(list.Elements()) != len(wantGroups) {
			t.Fatalf("routing_overrides = %s, want %d entries", list, len(wantGroups))
		}
		for i := range wantGroups {
			override := overrideElement(t, list, i)
			if got := overrideGroupIDs(t, override); !slices.Equal(got, wantGroups[i]) {
				t.Errorf("routing_overrides[%d].device_group_ids = %v, want %v", i, got, wantGroups[i])
			}
			if got := overrideTrafficRouting(t, override); got != wantRouting[i] {
				t.Errorf("routing_overrides[%d].routing.traffic_routing = %q, want %q", i, got, wantRouting[i])
			}
			if raw := override.Attributes()["device_group_ids"]; !isList(raw) {
				t.Errorf("routing_overrides[%d].device_group_ids is %T, want a list on the data source", i, raw)
			}
		}
	})
}

// overrideElement returns one entry of a routing_overrides list as an object value.
func overrideElement(t *testing.T, list types.List, index int) types.Object {
	t.Helper()
	elements := list.Elements()
	if index >= len(elements) {
		t.Fatalf("routing_overrides has %d entries, want at least %d", len(elements), index+1)
	}
	obj, ok := elements[index].(types.Object)
	if !ok {
		t.Fatalf("routing_overrides[%d] is %T, want an object", index, elements[index])
	}
	return obj
}

// overrideGroupIDs reads one override's device_group_ids as a sorted slice, accepting
// either the resource's set or the data source's list so one assertion serves both.
func overrideGroupIDs(t *testing.T, override types.Object) []string {
	t.Helper()
	raw := override.Attributes()["device_group_ids"]
	collection, ok := raw.(interface{ Elements() []attr.Value })
	if !ok {
		t.Fatalf("device_group_ids is %T, want a collection", raw)
	}
	out := make([]string, 0, len(collection.Elements()))
	for _, element := range collection.Elements() {
		value, ok := element.(types.String)
		if !ok {
			t.Fatalf("device_group_ids element is %T, want a string", element)
		}
		out = append(out, value.ValueString())
	}
	slices.Sort(out)
	return out
}

// overrideTrafficRouting reads one override's nested routing label.
func overrideTrafficRouting(t *testing.T, override types.Object) string {
	t.Helper()
	routing, ok := override.Attributes()["routing"].(types.Object)
	if !ok {
		t.Fatalf("routing is %T, want an object", override.Attributes()["routing"])
	}
	label, ok := routing.Attributes()["traffic_routing"].(types.String)
	if !ok {
		t.Fatalf("traffic_routing is %T, want a string", routing.Attributes()["traffic_routing"])
	}
	return label.ValueString()
}

// isSet and isList report the concrete collection type of a nested attribute, which
// is the resource-versus-data-source asymmetry the schema declares.
func isSet(value attr.Value) bool {
	_, ok := value.(types.Set)
	return ok
}

func isList(value attr.Value) bool {
	_, ok := value.(types.List)
	return ok
}

// TestBuildAppsResultModel pins that the plural result carries the same values the
// singular data source would, since it is built from the same assignment.
func TestBuildAppsResultModel(t *testing.T) {
	name := "Internal CRM"
	app := securitycloud.App{
		ID:           "00000000-0000-0000-0000-000000000002",
		Name:         &name,
		CategoryName: "Technology",
		Hostnames:    []string{"crm.example.com"},
		Assignments: &securitycloud.Assignments{
			Inclusions: securitycloud.AssignmentsInclusions{AllUsers: false, Groups: &[]string{"group-a"}},
		},
		Routing: &securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
	}

	result, diags := buildAppsResultModel(context.Background(), app)
	if diags.HasError() {
		t.Fatalf("building result: %s", diags)
	}
	if result.Name.ValueString() != name {
		t.Errorf("name = %q", result.Name.ValueString())
	}
	if result.AppType.ValueString() != appTypeCustom {
		t.Errorf("app_type = %q, want %q", result.AppType.ValueString(), appTypeCustom)
	}
	if len(result.DeviceGroupIDs.Elements()) != 1 {
		t.Errorf("device_group_ids = %s", result.DeviceGroupIDs)
	}
}

// TestDisplayNameFor pins the list-result label, which has to fall back twice: a
// predefined application has no name, and nothing guarantees a definition ID either.
func TestDisplayNameFor(t *testing.T) {
	name := "Internal CRM"
	predefined := "2aaa401c"
	empty := ""

	cases := []struct {
		label string
		app   securitycloud.App
		want  string
	}{
		{"named", securitycloud.App{ID: "id-1", Name: &name}, name},
		{"predefined", securitycloud.App{ID: "id-2", PredefinedAppID: &predefined}, predefined},
		{"empty name falls through", securitycloud.App{ID: "id-3", Name: &empty, PredefinedAppID: &predefined}, predefined},
		{"neither", securitycloud.App{ID: "id-4"}, "id-4"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			if got := displayNameFor(tc.app); got != tc.want {
				t.Errorf("displayNameFor = %q, want %q", got, tc.want)
			}
		})
	}
}
