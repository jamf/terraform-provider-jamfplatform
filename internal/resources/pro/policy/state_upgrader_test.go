// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestRenameScriptParameterKeys verifies the v0→v1 key rename rewrites every
// parameterN key under scripts.scripts[*] to parameter_N, leaves all other
// state untouched, and is a no-op when the scripts block is absent or null.
func TestRenameScriptParameterKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string // canonical-compared as JSON; "" means equal to in
	}{
		{
			name: "renames populated parameters and preserves siblings",
			in: `{
				"id": "123",
				"general": {"name": "p", "frequency": "Ongoing"},
				"scripts": {"scripts": [
					{"id": "9", "priority": "Before", "parameter4": "a", "parameter11": "b"}
				]}
			}`,
			want: `{
				"id": "123",
				"general": {"name": "p", "frequency": "Ongoing"},
				"scripts": {"scripts": [
					{"id": "9", "priority": "Before", "parameter_4": "a", "parameter_11": "b"}
				]}
			}`,
		},
		{
			name: "preserves null-valued parameters as null under the new key",
			in: `{"scripts": {"scripts": [
				{"id": "9", "parameter4": null, "parameter5": "set"}
			]}}`,
			want: `{"scripts": {"scripts": [
				{"id": "9", "parameter_4": null, "parameter_5": "set"}
			]}}`,
		},
		{
			name: "handles multiple script items independently",
			in: `{"scripts": {"scripts": [
				{"id": "1", "parameter4": "x"},
				{"id": "2", "parameter10": "y"}
			]}}`,
			want: `{"scripts": {"scripts": [
				{"id": "1", "parameter_4": "x"},
				{"id": "2", "parameter_10": "y"}
			]}}`,
		},
		{
			name: "no-op when scripts is null",
			in:   `{"id": "1", "scripts": null}`,
			want: `{"id": "1", "scripts": null}`,
		},
		{
			name: "no-op when scripts.scripts is null",
			in:   `{"scripts": {"scripts": null}}`,
			want: `{"scripts": {"scripts": null}}`,
		},
		{
			name: "no-op when scripts block absent",
			in:   `{"id": "1", "general": {"name": "p"}}`,
			want: `{"id": "1", "general": {"name": "p"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := renameScriptParameterKeys([]byte(tc.in))
			if err != nil {
				t.Fatalf("renameScriptParameterKeys returned error: %s", err)
			}

			var gotVal, wantVal any
			if err := json.Unmarshal(got, &gotVal); err != nil {
				t.Fatalf("output is not valid JSON: %s", err)
			}
			if err := json.Unmarshal([]byte(tc.want), &wantVal); err != nil {
				t.Fatalf("want fixture is not valid JSON: %s", err)
			}
			if !reflect.DeepEqual(gotVal, wantVal) {
				t.Errorf("rewritten state mismatch:\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestDropRemovedV2Attributes pins the v1 → v2 migration. The
// attribute is gone from the schema, so a surplus key in prior state fails the
// decode outright — and the migration must leave every other value byte-exact,
// because it re-marshals the whole document to get there.
func TestDropRemovedV2Attributes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   string
		// want is compared as parsed JSON, so key order does not matter.
		want string
	}{
		{
			name: "drops the key and keeps its siblings",
			in:   `{"id":"7029","printers":{"leave_existing_default":true,"printers":[{"id":"12","action":"install","make_default":true}]}}`,
			want: `{"id":"7029","printers":{"printers":[{"id":"12","action":"install","make_default":true}]}}`,
		},
		{
			name: "drops the key when it is the only one, leaving an empty block",
			in:   `{"printers":{"leave_existing_default":false}}`,
			want: `{"printers":{}}`,
		},
		{
			name: "state that never carried the key is untouched",
			in:   `{"printers":{"printers":[{"id":"12"}]}}`,
			want: `{"printers":{"printers":[{"id":"12"}]}}`,
		},
		{
			name: "drops managed_password_length and keeps its siblings",
			in:   `{"management_account":{"action":"rotate","managed_password_wo_version":2,"managed_password_length":16}}`,
			want: `{"management_account":{"action":"rotate","managed_password_wo_version":2}}`,
		},
		{
			name: "drops both removed attributes in one pass",
			in:   `{"printers":{"leave_existing_default":true},"management_account":{"managed_password_length":16,"action":"rotate"}}`,
			want: `{"printers":{},"management_account":{"action":"rotate"}}`,
		},
		{
			name: "a null management_account block is untouched",
			in:   `{"management_account":null}`,
			want: `{"management_account":null}`,
		},
		{
			name: "a null printers block is untouched",
			in:   `{"id":"7029","printers":null}`,
			want: `{"id":"7029","printers":null}`,
		},
		{
			name: "an absent printers block is untouched",
			in:   `{"id":"7029","scripts":{"scripts":[]}}`,
			want: `{"id":"7029","scripts":{"scripts":[]}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dropRemovedV2Attributes([]byte(tc.in))
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			var gotAny, wantAny any
			if err := json.Unmarshal(got, &gotAny); err != nil {
				t.Fatalf("result is not valid JSON: %s", err)
			}
			if err := json.Unmarshal([]byte(tc.want), &wantAny); err != nil {
				t.Fatalf("fixture is not valid JSON: %s", err)
			}
			if !reflect.DeepEqual(gotAny, wantAny) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// TestDropRemovedV2Attributes_PreservesLargeIntegersVerbatim guards
// the hazard of a whole-document re-marshal: encoding/json turns a number into
// a float64 on the way through a map[string]any, which silently mangles a large
// Jamf Pro ID. The migration only ever unmarshals into json.RawMessage for that
// reason, and this test fails if someone loosens it.
func TestDropRemovedV2Attributes_PreservesLargeIntegersVerbatim(t *testing.T) {
	t.Parallel()

	const in = `{"printers":{"leave_existing_default":true,"printers":[{"id":9007199254740993}]}}`
	got, err := dropRemovedV2Attributes([]byte(in))
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !strings.Contains(string(got), "9007199254740993") {
		t.Errorf("large integer was reformatted, got %s", got)
	}
	if strings.Contains(string(got), "leave_existing_default") {
		t.Errorf("leave_existing_default survived, got %s", got)
	}
}

// TestUpgradeStateCoversEveryPriorVersion pins that an upgrader exists for
// every schema version below the current one. Forgetting one makes every
// existing workspace fail on `terraform plan` with "state snapshot was created
// by a newer schema version", which no unit test would otherwise catch.
func TestUpgradeStateCoversEveryPriorVersion(t *testing.T) {
	t.Parallel()

	r := &PolicyResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	current := schemaResp.Schema.Version

	upgraders := r.UpgradeState(context.Background())
	for v := range current {
		if _, ok := upgraders[v]; !ok {
			t.Errorf("no state upgrader registered for schema version %d (current is %d)", v, current)
		}
	}
	if len(upgraders) != int(current) {
		t.Errorf("got %d upgraders for a v%d schema, want %d", len(upgraders), current, current)
	}
}

// runPolicyUpgrader runs the registered upgrader for a stored version over raw
// prior-state JSON and decodes the result into the current model, which is
// what Terraform does with it next.
func runPolicyUpgrader(t *testing.T, version int64, raw string) PolicyResourceModel {
	t.Helper()
	ctx := context.Background()
	r := &PolicyResource{}
	up, ok := r.UpgradeState(ctx)[version]
	if !ok {
		t.Fatalf("no upgrader for v%d", version)
	}
	resp := &resource.UpgradeStateResponse{}
	up.StateUpgrader(ctx, resource.UpgradeStateRequest{RawState: &tfprotov6.RawState{JSON: []byte(raw)}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("v%d upgrade: %v", version, resp.Diagnostics)
	}
	s := policySchemaForTest(t)
	value, err := resp.DynamicValue.Unmarshal(s.Type().TerraformType(ctx))
	if err != nil {
		t.Fatalf("decoding upgraded state: %s", err)
	}
	var m PolicyResourceModel
	if diags := (tfsdk.State{Schema: s, Raw: value}).Get(ctx, &m); diags.HasError() {
		t.Fatalf("reading upgraded state: %v", diags)
	}
	return m
}

// v2PolicyState is prior state as schema v2 wrote it, with the eight
// collections issue #568 turned into lists still stored as sets. A set and a
// list share one JSON form, so the order here is whatever v2 stored.
const v2PolicyState = `{
	"id": "6971",
	"general": {"name": "p"},
	"script_assignments": [
		{"id": "12", "name": "B", "priority": "Before", "parameter_4": "x", "parameter_5": null, "parameter_6": null, "parameter_7": null, "parameter_8": null, "parameter_9": null, "parameter_10": null, "parameter_11": null},
		{"id": "10", "name": "A", "priority": "After", "parameter_4": "y", "parameter_5": "", "parameter_6": null, "parameter_7": null, "parameter_8": null, "parameter_9": null, "parameter_10": null, "parameter_11": null}
	],
	"printer_assignments": [{"id": "31", "name": "P2", "action": "Unmap", "make_default": false}, {"id": "30", "name": "P1", "action": "Map", "make_default": true}],
	"dock_item_assignments": [{"id": "41", "name": "D2", "action": "Remove"}, {"id": "40", "name": "D1", "action": "Add To End"}],
	"directory_bindings": [{"id": "51", "name": "B2"}, {"id": "50", "name": "B1"}],
	"packages": {"distribution_point": "default", "packages": [
		{"id": "21", "name": "Zzz", "action": "Cache", "fut": false, "feu": false, "update_autorun": false},
		{"id": "20", "name": "Aaa", "action": "Install", "fut": true, "feu": false, "update_autorun": false}
	]}
}`

// TestUpgradeState_V2KeepsEveryCollectionVerbatim pins the v2 → v3 step: the
// eight collections now decode as lists, every element and value survives, and
// the stored order is kept rather than re-sorted.
func TestUpgradeState_V2KeepsEveryCollectionVerbatim(t *testing.T) {
	t.Parallel()
	m := runPolicyUpgrader(t, 2, v2PolicyState)

	if len(m.ScriptAssignments) != 2 || m.ScriptAssignments[0].ID.ValueString() != "12" || m.ScriptAssignments[1].Parameter5.IsNull() || m.ScriptAssignments[1].Parameter5.ValueString() != "" {
		t.Errorf("script_assignments = %v", m.ScriptAssignments)
	}
	if len(m.PrinterAssignments) != 2 || m.PrinterAssignments[0].ID.ValueString() != "31" || !m.PrinterAssignments[1].MakeDefault.ValueBool() {
		t.Errorf("printer_assignments = %v", m.PrinterAssignments)
	}
	if len(m.DockItemAssignments) != 2 || m.DockItemAssignments[0].ID.ValueString() != "41" {
		t.Errorf("dock_item_assignments = %v", m.DockItemAssignments)
	}
	if len(m.DirectoryBindings) != 2 || m.DirectoryBindings[0].ID.ValueString() != "51" {
		t.Errorf("directory_bindings = %v", m.DirectoryBindings)
	}
	if m.Packages == nil || len(m.Packages.Packages) != 2 || m.Packages.Packages[0].ID.ValueString() != "21" || m.Packages.DistributionPoint.ValueString() != "default" {
		t.Errorf("packages = %v", m.Packages)
	}
}

// TestUpgradeState_V2DeprecatedWrappers is the same for state that still
// manages the deprecated wrapper blocks.
func TestUpgradeState_V2DeprecatedWrappers(t *testing.T) {
	t.Parallel()
	m := runPolicyUpgrader(t, 2, `{
		"id": "1",
		"scripts": {"scripts": [{"id": "10", "priority": "After", "parameter_4": "a"}, {"id": "9", "priority": "After", "parameter_4": "b"}]},
		"printers": {"printers": [{"id": "30", "action": "Map", "make_default": false}]},
		"dock_items": {"dock_items": [{"id": "40", "action": "Remove"}]}
	}`)
	if m.Scripts == nil || len(m.Scripts.Scripts) != 2 || m.Scripts.Scripts[0].ID.ValueString() != "10" {
		t.Errorf("scripts.scripts = %v", m.Scripts)
	}
	if m.Printers == nil || len(m.Printers.Printers) != 1 || m.DockItems == nil || len(m.DockItems.DockItems) != 1 {
		t.Errorf("printers = %v, dock_items = %v", m.Printers, m.DockItems)
	}
}

// TestUpgradeState_V0AndV1DecodeAgainstTheListSchema re-runs the older
// upgraders' fixtures against the current schema, since each of them must
// produce state the v3 list attributes accept.
func TestUpgradeState_V0AndV1DecodeAgainstTheListSchema(t *testing.T) {
	t.Parallel()
	m := runPolicyUpgrader(t, 0, `{
		"id": "123",
		"general": {"name": "p"},
		"scripts": {"scripts": [{"id": "9", "priority": "Before", "parameter4": "a", "parameter11": "b"}]},
		"printers": {"leave_existing_default": true, "printers": [{"id": "12", "action": "Map", "make_default": true}]}
	}`)
	if m.Scripts == nil || len(m.Scripts.Scripts) != 1 || m.Scripts.Scripts[0].Parameter4.ValueString() != "a" || m.Scripts.Scripts[0].Parameter11.ValueString() != "b" {
		t.Errorf("v0 scripts = %v", m.Scripts)
	}
	if m.Printers == nil || len(m.Printers.Printers) != 1 {
		t.Errorf("v0 printers = %v", m.Printers)
	}

	m = runPolicyUpgrader(t, 1, `{
		"id": "7029",
		"printers": {"leave_existing_default": true, "printers": [{"id": "12", "action": "Map", "make_default": true}]},
		"management_account": {"action": "rotate", "managed_password_length": 16},
		"script_assignments": [{"id": "9", "priority": "After"}, {"id": "8", "priority": "After"}]
	}`)
	if m.Printers == nil || len(m.Printers.Printers) != 1 || len(m.ScriptAssignments) != 2 || m.ScriptAssignments[0].ID.ValueString() != "9" {
		t.Errorf("v1 upgrade = printers %v, script_assignments %v", m.Printers, m.ScriptAssignments)
	}
}
