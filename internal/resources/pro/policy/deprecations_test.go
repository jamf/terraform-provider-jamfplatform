// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func policySchemaForTest(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	NewPolicyResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestPolicyResource_DeprecatedAttributesNameTheirReplacement(t *testing.T) {
	t.Parallel()
	s := policySchemaForTest(t)
	general, ok := s.Attributes["general"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatal("expected general to be SingleNestedAttribute")
	}

	cases := []struct {
		name        string
		attr        rschema.Attribute
		replacement string
	}{
		{"scripts", s.Attributes["scripts"], "script_assignments"},
		{"printers", s.Attributes["printers"], "printer_assignments"},
		{"dock_items", s.Attributes["dock_items"], "dock_item_assignments"},
		{"general.date_time_limitations", general.Attributes["date_time_limitations"], "server_side_limitations"},
		{"general.network_limitations", general.Attributes["network_limitations"], "general.network_requirements"},
		{"general.override_default_settings", general.Attributes["override_default_settings"], "general.target_drive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.attr == nil {
				t.Fatalf("%s missing from schema", tc.name)
			}
			msg := tc.attr.GetDeprecationMessage()
			if msg == "" {
				t.Fatalf("%s carries no DeprecationMessage", tc.name)
			}
			if !strings.Contains(msg, "`"+tc.replacement+"`") {
				t.Errorf("%s: notice must name %q, got %q", tc.name, tc.replacement, msg)
			}
			if !strings.Contains(msg, "2027-02-15") {
				t.Errorf("%s: notice must carry the earliest removal date, got %q", tc.name, msg)
			}
			if !strings.Contains(tc.attr.GetMarkdownDescription(), msg) {
				t.Errorf("%s: the rendered description must repeat the notice, because tfplugindocs omits DeprecationMessage", tc.name)
			}
		})
	}
}

func TestPolicyResource_ReplacementsAreNotDeprecatedAndExcludeTheirPredecessor(t *testing.T) {
	t.Parallel()
	s := policySchemaForTest(t)
	for _, name := range []string{"script_assignments", "printer_assignments", "dock_item_assignments", "server_side_limitations", "client_side_limitations"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Fatalf("expected replacement attribute %q", name)
		}
		if attr.GetDeprecationMessage() != "" {
			t.Errorf("%s is the replacement and must not be deprecated", name)
		}
		if !attr.IsOptional() || attr.IsComputed() {
			t.Errorf("%s must be Optional and not Computed so omitting it leaves the server value alone", name)
		}
	}
	for _, name := range []string{"scripts", "printers", "dock_items"} {
		nested, ok := s.Attributes[name].(rschema.SingleNestedAttribute)
		if !ok || len(nested.Validators) == 0 {
			t.Errorf("%s must conflict with its replacement via an object validator", name)
		}
	}
	general := s.Attributes["general"].(rschema.SingleNestedAttribute)
	if dtl := general.Attributes["date_time_limitations"].(rschema.SingleNestedAttribute); len(dtl.Validators) == 0 {
		t.Error("general.date_time_limitations must conflict with both successor blocks")
	}
}

func TestPolicyResource_FlatAssignmentsShareItemSchemaWithWrapper(t *testing.T) {
	t.Parallel()
	s := policySchemaForTest(t)
	pairs := []struct{ wrapper, inner, flat string }{
		{"scripts", "scripts", "script_assignments"},
		{"printers", "printers", "printer_assignments"},
		{"dock_items", "dock_items", "dock_item_assignments"},
	}
	for _, p := range pairs {
		wrapper := s.Attributes[p.wrapper].(rschema.SingleNestedAttribute)
		oldSet := wrapper.Attributes[p.inner].(rschema.SetNestedAttribute)
		newSet := s.Attributes[p.flat].(rschema.SetNestedAttribute)
		if !reflect.DeepEqual(attrNames(oldSet.NestedObject.Attributes), attrNames(newSet.NestedObject.Attributes)) {
			t.Errorf("%s and %s must expose the same item attributes", p.wrapper, p.flat)
		}
	}
	general := s.Attributes["general"].(rschema.SingleNestedAttribute)
	oldDTL := general.Attributes["date_time_limitations"].(rschema.SingleNestedAttribute)
	server := s.Attributes["server_side_limitations"].(rschema.SingleNestedAttribute)
	client := s.Attributes["client_side_limitations"].(rschema.SingleNestedAttribute)
	want := append(attrNames(server.Attributes), attrNames(client.Attributes)...)
	sortStrings(want)
	if !reflect.DeepEqual(attrNames(oldDTL.Attributes), want) {
		t.Errorf("server_side_limitations and client_side_limitations together must expose exactly the attributes of general.date_time_limitations: %v vs %v", attrNames(oldDTL.Attributes), want)
	}
	if len(server.Attributes) != 2 || len(client.Attributes) != 3 {
		t.Errorf("expected 2 server-side and 3 client-side attributes, got %d and %d", len(server.Attributes), len(client.Attributes))
	}
}

func attrNames(m map[string]rschema.Attribute) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func TestBuildPolicyInput_FlatAssignmentsMatchDeprecatedWrapper(t *testing.T) {
	t.Parallel()
	scripts := []PolicyScriptItemModel{{ID: types.StringValue("3"), Priority: types.StringValue("After"), Parameter4: types.StringValue("p4")}}
	printers := []PolicyPrinterItemModel{{ID: types.StringValue("4"), Action: types.StringValue("Map"), MakeDefault: types.BoolValue(true)}}
	docks := []PolicyDockItemModel{{ID: types.StringValue("5"), Action: types.StringValue("Add To Beginning")}}
	general := &PolicyGeneralModel{Name: types.StringValue("n")}

	old, diags := buildPolicyInput(context.Background(), PolicyResourceModel{
		General:   general,
		Scripts:   &PolicyScriptsModel{Scripts: scripts},
		Printers:  &PolicyPrintersModel{Printers: printers},
		DockItems: &PolicyDockItemsModel{DockItems: docks},
	}, nil)
	if diags.HasError() {
		t.Fatalf("deprecated shape: %v", diags)
	}
	flat, diags := buildPolicyInput(context.Background(), PolicyResourceModel{
		General:             general,
		ScriptAssignments:   scripts,
		PrinterAssignments:  printers,
		DockItemAssignments: docks,
	}, nil)
	if diags.HasError() {
		t.Fatalf("flat shape: %v", diags)
	}
	if !reflect.DeepEqual(old, flat) {
		t.Errorf("flat attributes must produce the same wire payload as the deprecated wrappers\nold:  %+v\nflat: %+v", old, flat)
	}
	if flat.Scripts == nil || flat.Printers == nil || flat.DockItems == nil {
		t.Fatalf("expected scripts, printers and dock items on the wire; got %+v", flat)
	}
}

func TestBuildPolicyInput_LimitationBlocksLandInGeneral(t *testing.T) {
	t.Parallel()
	general := &PolicyGeneralModel{Name: types.StringValue("n")}
	old, _ := buildPolicyInput(context.Background(), PolicyResourceModel{
		General: &PolicyGeneralModel{Name: types.StringValue("n"), DateTimeLimitations: &PolicyGeneralDateTimeLimitationsModel{
			ActivationDate: types.StringValue("2027-01-01 01:00:00"),
			ExpirationDate: types.StringNull(),
			NoExecuteOn:    stringSet(t, "Sun"),
			NoExecuteStart: types.StringNull(),
			NoExecuteEnd:   types.StringNull(),
		}},
	}, nil)
	flat, diags := buildPolicyInput(context.Background(), PolicyResourceModel{
		General: general,
		ServerSideLimitations: &PolicyServerSideLimitationsModel{
			ActivationDate: types.StringValue("2027-01-01 01:00:00"),
			ExpirationDate: types.StringNull(),
		},
		ClientSideLimitations: &PolicyClientSideLimitationsModel{
			NoExecuteOn:    stringSet(t, "Sun"),
			NoExecuteStart: types.StringNull(),
			NoExecuteEnd:   types.StringNull(),
		},
	}, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if flat.General == nil || flat.General.DateTimeLimitations == nil {
		t.Fatalf("expected <general><date_time_limitations> on the wire; got %+v", flat.General)
	}
	if !reflect.DeepEqual(old, flat) {
		t.Errorf("the two blocks must produce the same wire payload as general.date_time_limitations")
	}

	serverOnly, _ := buildPolicyInput(context.Background(), PolicyResourceModel{
		General:               general,
		ServerSideLimitations: &PolicyServerSideLimitationsModel{ActivationDate: types.StringValue("2027-01-01 01:00:00"), ExpirationDate: types.StringNull()},
	}, nil)
	if got := serverOnly.General.DateTimeLimitations; got == nil || got.NoExecuteOn != nil || got.NoExecuteStart != nil || got.NoExecuteEnd != nil {
		t.Errorf("an absent client_side_limitations must send nothing for its attributes, so the server keeps them: %+v", got)
	}
}

func TestAssignPolicyResourceModel_RefreshesOnlyTheShapeTheCallerManages(t *testing.T) {
	t.Parallel()
	wire := &proclassic.Policy{
		General: &proclassic.PolicyGeneral{
			Name: new("n"),
			DateTimeLimitations: &proclassic.PolicyGeneralDateTimeLimitations{
				ActivationDate: new("2027-01-01 01:00:00"),
			},
		},
		Scripts:   &proclassic.PolicyScripts{Script: &[]proclassic.PolicyScriptsScriptItem{{ID: new(3), Priority: new("After")}}},
		Printers:  &proclassic.PolicyPrinters{Printer: &[]proclassic.PolicyPrintersPrinterItem{{ID: new(4), Action: new("install")}}},
		DockItems: &proclassic.PolicyDockItems{DockItem: &[]proclassic.PolicyDockItemsDockItemItem{{ID: new(5), Action: new("Add To Beginning")}}},
	}

	t.Run("flat attributes managed", func(t *testing.T) {
		t.Parallel()
		state := &PolicyResourceModel{
			General:               &PolicyGeneralModel{},
			ScriptAssignments:     []PolicyScriptItemModel{{ID: types.StringValue("3")}},
			PrinterAssignments:    []PolicyPrinterItemModel{{ID: types.StringValue("4")}},
			DockItemAssignments:   []PolicyDockItemModel{{ID: types.StringValue("5")}},
			ServerSideLimitations: &PolicyServerSideLimitationsModel{},
			ClientSideLimitations: &PolicyClientSideLimitationsModel{},
		}
		if diags := assignPolicyResourceModel(context.Background(), state, wire, false); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if len(state.ScriptAssignments) != 1 || state.ScriptAssignments[0].Priority.ValueString() != "After" {
			t.Errorf("script_assignments not refreshed: %+v", state.ScriptAssignments)
		}
		if len(state.PrinterAssignments) != 1 || state.PrinterAssignments[0].Action.ValueString() != "Map" {
			t.Errorf("printer_assignments not refreshed: %+v", state.PrinterAssignments)
		}
		if len(state.DockItemAssignments) != 1 || state.DockItemAssignments[0].Name.IsUnknown() {
			t.Errorf("dock_item_assignments not refreshed: %+v", state.DockItemAssignments)
		}
		if state.ServerSideLimitations.ActivationDate.ValueString() != "2027-01-01 01:00:00" {
			t.Errorf("server_side_limitations not refreshed: %+v", state.ServerSideLimitations)
		}
		if !state.ClientSideLimitations.NoExecuteOn.IsNull() {
			t.Errorf("client_side_limitations.no_execute_on must stay null when the wire has none: %+v", state.ClientSideLimitations)
		}
		if state.Scripts != nil || state.Printers != nil || state.DockItems != nil || state.General.DateTimeLimitations != nil {
			t.Errorf("deprecated blocks must stay null when the caller manages the flat ones")
		}
	})

	t.Run("deprecated blocks managed", func(t *testing.T) {
		t.Parallel()
		state := &PolicyResourceModel{
			General: &PolicyGeneralModel{DateTimeLimitations: &PolicyGeneralDateTimeLimitationsModel{}},
			Scripts: &PolicyScriptsModel{}, Printers: &PolicyPrintersModel{}, DockItems: &PolicyDockItemsModel{},
		}
		if diags := assignPolicyResourceModel(context.Background(), state, wire, false); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if len(state.Scripts.Scripts) != 1 || len(state.Printers.Printers) != 1 || len(state.DockItems.DockItems) != 1 {
			t.Errorf("deprecated blocks must keep refreshing while they are configured: %+v", state)
		}
		if state.General.DateTimeLimitations.ActivationDate.ValueString() != "2027-01-01 01:00:00" {
			t.Errorf("general.date_time_limitations must keep refreshing: %+v", state.General.DateTimeLimitations)
		}
		if state.ScriptAssignments != nil || state.PrinterAssignments != nil || state.DockItemAssignments != nil || state.ServerSideLimitations != nil || state.ClientSideLimitations != nil {
			t.Errorf("flat attributes must stay null when the caller manages the deprecated blocks")
		}
	})

	t.Run("nothing managed", func(t *testing.T) {
		t.Parallel()
		state := &PolicyResourceModel{General: &PolicyGeneralModel{}}
		if diags := assignPolicyResourceModel(context.Background(), state, wire, false); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if state.ScriptAssignments != nil || state.PrinterAssignments != nil || state.DockItemAssignments != nil || state.ServerSideLimitations != nil || state.ClientSideLimitations != nil ||
			state.Scripts != nil || state.Printers != nil || state.DockItems != nil || state.General.DateTimeLimitations != nil {
			t.Errorf("an unmanaged section must stay null: %+v", state)
		}
	})
}
