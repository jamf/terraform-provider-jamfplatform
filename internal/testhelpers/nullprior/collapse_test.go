// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package nullprior

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// knownPlanFixture returns a state holding prior and a plan and config holding
// configured, for a schema of one Optional+Computed attribute named leaf.
func knownPlanFixture(t *testing.T, leaf schema.Attribute, prior, configured tftypes.Value) (tfsdk.State, tfsdk.Plan, tfsdk.Config) {
	t.Helper()
	sch := schema.Schema{Attributes: map[string]schema.Attribute{"leaf": leaf}}
	rootType := sch.Type().TerraformType(context.Background())
	st := tftypes.NewValue(rootType, map[string]tftypes.Value{"leaf": prior})
	pl := tftypes.NewValue(rootType, map[string]tftypes.Value{"leaf": configured})
	return tfsdk.State{Schema: sch, Raw: st}, tfsdk.Plan{Schema: sch, Raw: pl}, tfsdk.Config{Schema: sch, Raw: pl}
}

// TestUnknownOnlyModifiersLeaveAKnownPlanAlone is the proof behind
// unknownOnlyModifiers: given a configured value that differs from prior state,
// including by newline style only, none of them plans anything but the
// configured value.
func TestUnknownOnlyModifiersLeaveAKnownPlanAlone(t *testing.T) {
	ctx := context.Background()
	stringPairs := [][2]string{{"new", "old"}, {"a\r\nb", "a\nb"}, {"", "x"}, {"x", ""}}
	for _, m := range unknownOnlyModifiers {
		name := reflect.TypeOf(m).String()
		switch mod := m.(type) {
		case planmodifier.String:
			for _, pair := range stringPairs {
				leaf := schema.StringAttribute{Optional: true, Computed: true}
				st, pl, cfg := knownPlanFixture(t, leaf, tftypes.NewValue(tftypes.String, pair[1]), tftypes.NewValue(tftypes.String, pair[0]))
				req := planmodifier.StringRequest{State: st, Plan: pl, Config: cfg, ConfigValue: types.StringValue(pair[0]), StateValue: types.StringValue(pair[1]), PlanValue: types.StringValue(pair[0])}
				resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
				mod.PlanModifyString(ctx, req, resp)
				if !resp.PlanValue.Equal(req.ConfigValue) {
					t.Errorf("%s: configured %q against prior %q planned %s", name, pair[0], pair[1], resp.PlanValue)
				}
			}
		case planmodifier.Bool:
			leaf := schema.BoolAttribute{Optional: true, Computed: true}
			st, pl, cfg := knownPlanFixture(t, leaf, tftypes.NewValue(tftypes.Bool, false), tftypes.NewValue(tftypes.Bool, true))
			req := planmodifier.BoolRequest{State: st, Plan: pl, Config: cfg, ConfigValue: types.BoolValue(true), StateValue: types.BoolValue(false), PlanValue: types.BoolValue(true)}
			resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
			mod.PlanModifyBool(ctx, req, resp)
			if !resp.PlanValue.Equal(req.ConfigValue) {
				t.Errorf("%s: configured true against prior false planned %s", name, resp.PlanValue)
			}
		case planmodifier.Int64:
			leaf := schema.Int64Attribute{Optional: true, Computed: true}
			st, pl, cfg := knownPlanFixture(t, leaf, tftypes.NewValue(tftypes.Number, 1), tftypes.NewValue(tftypes.Number, 2))
			req := planmodifier.Int64Request{State: st, Plan: pl, Config: cfg, ConfigValue: types.Int64Value(2), StateValue: types.Int64Value(1), PlanValue: types.Int64Value(2)}
			resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
			mod.PlanModifyInt64(ctx, req, resp)
			if !resp.PlanValue.Equal(req.ConfigValue) {
				t.Errorf("%s: configured 2 against prior 1 planned %s", name, resp.PlanValue)
			}
		default:
			t.Errorf("%s: no proof case for this modifier kind; add one before allowing it", name)
		}
	}
}

type suppressDiff struct{}

func (suppressDiff) Description(context.Context) string         { return "suppress" }
func (suppressDiff) MarkdownDescription(context.Context) string { return "suppress" }
func (suppressDiff) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	resp.PlanValue = req.StateValue
}

func TestCollapseProblemsFlagsASuppressingModifierOnAConfigurableAttribute(t *testing.T) {
	sch := schema.Schema{Attributes: map[string]schema.Attribute{
		"block": schema.SingleNestedAttribute{Optional: true, Attributes: map[string]schema.Attribute{
			"configurable": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{suppressDiff{}}},
			"derived":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{suppressDiff{}}},
			"carried":      schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()}},
		}},
		"items": schema.SetNestedAttribute{Optional: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{suppressDiff{}}},
			"value": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		}}},
	}}
	problems := collapseProblems(sch, ".")
	if len(problems) != 2 || !strings.HasPrefix(problems[0], "block.configurable:") || !strings.HasPrefix(problems[1], "items[*].id:") {
		t.Fatalf("want problems for block.configurable and items[*].id only, got %q", problems)
	}
}

func TestPlanWritesFlagsEveryKindOfPlanWrite(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.26\n")
	write("res/res.go", "package res\n\nimport _ \"example.com/m/dep\"\n\nfunc a(req, resp any) {\n\treq.Plan.Get(nil, nil)\n\tresp.Plan.Set(nil, nil)\n}\n")
	write("res/res_test.go", "package res\n\nfunc b(resp any) { resp.Plan.Set(nil, nil) }\n")
	write("dep/dep.go", "package dep\n\nfunc c(resp any) {\n\tresp.Plan.SetAttribute(nil, nil, nil)\n\tresp.Plan = nil\n\tresp.Plan.Raw = nil\n}\n")
	problems := planWrites(filepath.Join(root, "res"))
	if len(problems) != 4 {
		t.Fatalf("want 4 plan writes (one in res.go, three in the imported dep), got %d: %q", len(problems), problems)
	}
}
