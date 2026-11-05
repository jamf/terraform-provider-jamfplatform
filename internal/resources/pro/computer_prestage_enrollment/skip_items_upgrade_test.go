// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// panesAddedInThisRelease are the panes with no deprecated twin that older state never held.
var panesAddedInThisRelease = []string{"liquid_glass"}

// stateFromBeforeTheRename builds the state an older provider wrote: every deprecated attribute
// and every pane that kept its name holds a value, and every renamed or new pane is null.
func stateFromBeforeTheRename(t *testing.T) (tfsdk.State, schema.Schema) {
	t.Helper()
	ctx := context.Background()
	s := schema.Schema{Attributes: map[string]schema.Attribute{"skip_setup_items": skipSetupItemsSchema()}}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}

	model := &SkipSetupItemsModel{}
	for _, it := range skipItems {
		replaced := it.deprecated != nil || slices.Contains(panesAddedInThisRelease, it.name)
		if replaced {
			*it.field(model) = types.BoolNull()
		} else {
			*it.field(model) = types.BoolValue(false)
		}
		if it.deprecated != nil {
			*it.deprecated.field(model) = types.BoolValue(true)
		}
	}
	if diags := state.SetAttribute(ctx, path.Root("skip_setup_items"), model); diags.HasError() {
		t.Fatalf("building prior state: %v", diags)
	}
	return state, s
}

func planModify(t *testing.T, a schema.BoolAttribute, name string, state tfsdk.State, prior types.Bool) types.Bool {
	t.Helper()
	req := planmodifier.BoolRequest{
		Path:        path.Root("skip_setup_items").AtName(name),
		PlanValue:   types.BoolUnknown(),
		ConfigValue: types.BoolNull(),
		StateValue:  prior,
		State:       state,
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
	for _, m := range a.PlanModifiers {
		m.PlanModifyBool(context.Background(), req, resp)
	}
	return resp.PlanValue
}

func TestSkipSetupItems_PanesNullInOlderStatePlanAsUnknown(t *testing.T) {
	t.Parallel()
	state, s := stateFromBeforeTheRename(t)
	block := s.Attributes["skip_setup_items"].(schema.SingleNestedAttribute)

	for _, it := range skipItems {
		replaced := it.deprecated != nil || slices.Contains(panesAddedInThisRelease, it.name)
		a := block.Attributes[it.name].(schema.BoolAttribute)
		prior := types.BoolValue(false)
		if replaced {
			prior = types.BoolNull()
		}
		got := planModify(t, a, it.name, state, prior)
		if replaced && !got.IsUnknown() {
			t.Errorf("%s: older state holds no value, so the plan must be unknown for apply to settle; got %s", it.name, got)
		}
		if !replaced && !got.Equal(prior) {
			t.Errorf("%s: a pane older state held must carry its value; got %s", it.name, got)
		}
	}
}

func TestBuildSkipSetupItemsMapOver_UnsetPaneKeepsTheServerValue(t *testing.T) {
	t.Parallel()
	server := map[string]bool{"LiquidGlass": true, "TOS": true}
	m := &SkipSetupItemsModel{
		LiquidGlass:        types.BoolUnknown(),
		TermsAndConditions: types.BoolUnknown(),
		TOS:                types.BoolNull(),
	}
	got := *buildSkipSetupItemsMapOver(m, server)
	if !got["LiquidGlass"] {
		t.Errorf("an unknown pane must send the server value, not false")
	}
	if !got["TOS"] {
		t.Errorf("a renamed pane with neither name set must send the server value, not false")
	}
}

func TestBuildSkipSetupItemsMapOver_ConfiguredValueBeatsTheServer(t *testing.T) {
	t.Parallel()
	server := map[string]bool{"LiquidGlass": true, "TOS": true}
	m := &SkipSetupItemsModel{
		LiquidGlass:        types.BoolValue(false),
		TermsAndConditions: types.BoolValue(false),
	}
	got := *buildSkipSetupItemsMapOver(m, server)
	if got["LiquidGlass"] || got["TOS"] {
		t.Errorf("a configured false must reach the wire over a server true; got %v", got)
	}
}

func TestBuildSkipSetupItemsMapOver_CreateStillDefaultsToFalse(t *testing.T) {
	t.Parallel()
	m := &SkipSetupItemsModel{LiquidGlass: types.BoolUnknown()}
	if (*buildSkipSetupItemsMap(m))["LiquidGlass"] {
		t.Errorf("with no server value a create must send false")
	}
}
