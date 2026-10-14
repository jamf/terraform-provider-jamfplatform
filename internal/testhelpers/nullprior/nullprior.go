// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package nullprior holds the schema-wide guard for the "block goes from
// omitted to declared" failure. It is a package of its own rather than part of
// testhelpers because testhelpers imports the provider under the acceptance
// tag, which would make every in-package unit test that uses it an import cycle.
package nullprior

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// AssertOptionalBlockLeavesPlanUnknown checks every Optional+Computed or
// Computed-only string, bool and int64 inside an Optional single-nested block
// of sch.
//
// When such a block goes from omitted to declared, state holds null for each
// leaf in it. A modifier that copies the prior value (UseStateForUnknown) plans
// that null, apply returns the server's default, and Terraform rejects the
// result with "Provider produced inconsistent result after apply". The leaf has
// to plan Unknown when its prior is null and the block is new, which is what
// UseNonNullStateForUnknown and planmodifiers.UseStateForUnknownInExistingBlock*
// do, so each plan modifier is run with a null prior, an Unknown plan value and
// a state in which the enclosing block is null, and must leave the value Unknown.
//
// A Computed-only leaf (a server-derived count, say) fails the same way, so it
// is checked too. Only the stock carry modifiers are run on one, recognised by
// their "Once set" description: a custom or mirror modifier there reads sibling
// attributes the walk does not have, and is covered by a test of its own.
//
// Leaves outside an Optional block (a top-level attribute, or one inside a
// Required block) cannot see the transition and are skipped. So are the
// elements of a Set or List, which key their modifiers by element id.
func AssertOptionalBlockLeavesPlanUnknown(t *testing.T, sch schema.Schema) {
	t.Helper()
	walk(t, path.Empty(), sch.Attributes, false)
}

func walk(t *testing.T, parent path.Path, attrs map[string]schema.Attribute, underOptionalBlock bool) {
	t.Helper()
	for name, attr := range attrs {
		p := parent.AtName(name)
		switch a := attr.(type) {
		case schema.SingleNestedAttribute:
			walk(t, p, a.Attributes, underOptionalBlock || (a.Optional && !a.Required))
		case schema.StringAttribute:
			if underOptionalBlock && a.Computed {
				checkString(t, p, a, !a.Optional)
			}
		case schema.BoolAttribute:
			if underOptionalBlock && a.Computed {
				checkBool(t, p, a, !a.Optional)
			}
		case schema.Int64Attribute:
			if underOptionalBlock && a.Computed {
				checkInt64(t, p, a, !a.Optional)
			}
		}
	}
}

// nullParentFixture returns a state, plan and config in which the leaf's
// enclosing block is null and every block above it exists, which is the state
// of a block that is new in this plan. The schema is the leaf's own attribute
// inside a chain of Optional blocks named by its path, so a modifier that reads
// the enclosing block from state can do so.
func nullParentFixture(p path.Path, leaf schema.Attribute) (tfsdk.State, tfsdk.Plan, tfsdk.Config) {
	ctx := context.Background()
	var names []string
	for _, step := range p.Steps() {
		names = append(names, string(step.(path.PathStepAttributeName)))
	}
	n := len(names)

	attrs := map[string]schema.Attribute{names[n-1]: leaf}
	for i := n - 2; i >= 0; i-- {
		attrs = map[string]schema.Attribute{names[i]: schema.SingleNestedAttribute{Optional: true, Attributes: attrs}}
	}
	sch := schema.Schema{Attributes: attrs}

	rootType := sch.Type().TerraformType(ctx).(tftypes.Object)
	blockTypes := make([]tftypes.Type, n-1)
	var parent tftypes.Type = rootType
	for k := 0; k < n-1; k++ {
		parent = parent.(tftypes.Object).AttributeTypes[names[k]]
		blockTypes[k] = parent
	}

	cur := tftypes.NewValue(blockTypes[n-2], nil)
	for k := n - 3; k >= 0; k-- {
		cur = tftypes.NewValue(blockTypes[k], map[string]tftypes.Value{names[k+1]: cur})
	}
	raw := tftypes.NewValue(rootType, map[string]tftypes.Value{names[0]: cur})
	return tfsdk.State{Schema: sch, Raw: raw}, tfsdk.Plan{Schema: sch, Raw: raw}, tfsdk.Config{Schema: sch, Raw: raw}
}

func report(t *testing.T, p path.Path, planned interface{ IsUnknown() bool }, modifier string) {
	t.Helper()
	if !planned.IsUnknown() {
		t.Errorf("%s: %s plans a known value from a null prior. When its block goes from omitted to declared, apply returns the server's default and fails with %q. Use planmodifiers.UseStateForUnknownInExistingBlock* for a leaf of a single-nested block, or UseNonNullStateForUnknown for a leaf of a Set or List element (STYLE_GUIDE rule 1a).",
			p, modifier, "Provider produced inconsistent result after apply")
	}
}

func checkString(t *testing.T, p path.Path, a schema.StringAttribute, stockOnly bool) {
	t.Helper()
	ctx := context.Background()
	st, pl, cfg := nullParentFixture(p, a)
	for _, m := range a.PlanModifiers {
		if stockOnly && !strings.HasPrefix(m.Description(ctx), "Once set") {
			continue
		}
		req := planmodifier.StringRequest{Path: p, State: st, Plan: pl, Config: cfg, ConfigValue: types.StringNull(), StateValue: types.StringNull(), PlanValue: types.StringUnknown()}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		m.PlanModifyString(ctx, req, resp)
		report(t, p, resp.PlanValue, m.Description(ctx))
	}
}

func checkBool(t *testing.T, p path.Path, a schema.BoolAttribute, stockOnly bool) {
	t.Helper()
	ctx := context.Background()
	st, pl, cfg := nullParentFixture(p, a)
	for _, m := range a.PlanModifiers {
		if stockOnly && !strings.HasPrefix(m.Description(ctx), "Once set") {
			continue
		}
		req := planmodifier.BoolRequest{Path: p, State: st, Plan: pl, Config: cfg, ConfigValue: types.BoolNull(), StateValue: types.BoolNull(), PlanValue: types.BoolUnknown()}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		m.PlanModifyBool(ctx, req, resp)
		report(t, p, resp.PlanValue, m.Description(ctx))
	}
}

func checkInt64(t *testing.T, p path.Path, a schema.Int64Attribute, stockOnly bool) {
	t.Helper()
	ctx := context.Background()
	st, pl, cfg := nullParentFixture(p, a)
	for _, m := range a.PlanModifiers {
		if stockOnly && !strings.HasPrefix(m.Description(ctx), "Once set") {
			continue
		}
		req := planmodifier.Int64Request{Path: p, State: st, Plan: pl, Config: cfg, ConfigValue: types.Int64Null(), StateValue: types.Int64Null(), PlanValue: types.Int64Unknown()}
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
		m.PlanModifyInt64(ctx, req, resp)
		report(t, p, resp.PlanValue, m.Description(ctx))
	}
}
