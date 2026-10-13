// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package planmodifiers

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UseKeyedStateForUnknownString carries the prior value of an Optional+Computed
// string attribute of a set- or list-nested element onto the planned element, but only
// from the prior element whose key attribute equals the planned element's.
//
// It replaces UseNonNullStateForUnknown inside id-keyed sets. The framework
// pairs a planned set element with prior state by INDEX, not by identity, so
// the stock modifier copies whatever sits at the same position: re-keying an
// element, or reordering the set, hands the new element another object's
// values. For a server-derived value that is a perpetual or inconsistent diff;
// for a user-visible default it silently gives a new element the old element's
// settings. Keyed lookup carries only on a genuine match and otherwise leaves
// the value Unknown for the server to answer.
//
// Two matches on one key, or none, carry nothing: with duplicates there is no
// telling which prior element a planned one continues. A list pairs by index
// in the framework too, which is just as wrong once an element is inserted or
// reordered. See issue #464.
func UseKeyedStateForUnknownString(key string) planmodifier.String {
	return keyedString{key: key}
}

// UseKeyedStateForUnknownBool is UseKeyedStateForUnknownString for bool
// attributes.
func UseKeyedStateForUnknownBool(key string) planmodifier.Bool {
	return keyedBool{key: key}
}

type keyedString struct{ key string }

func (keyedString) Description(_ context.Context) string {
	return "Carry the prior value from the set element with the same key; otherwise leave it Unknown."
}

func (m keyedString) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

func (m keyedString) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	v, ok := keyedPriorValue(ctx, req.Plan, req.State, req.Path, m.key)
	if !ok {
		return
	}
	if s, isString := v.(types.String); isString && !s.IsNull() && !s.IsUnknown() {
		resp.PlanValue = s
	}
}

type keyedBool struct{ key string }

func (keyedBool) Description(_ context.Context) string {
	return "Carry the prior value from the set element with the same key; otherwise leave it Unknown."
}

func (m keyedBool) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

func (m keyedBool) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	v, ok := keyedPriorValue(ctx, req.Plan, req.State, req.Path, m.key)
	if !ok {
		return
	}
	if b, isBool := v.(types.Bool); isBool && !b.IsNull() && !b.IsUnknown() {
		resp.PlanValue = b
	}
}

// keyedPriorValue returns the prior value of the attribute at p from the one
// element of the enclosing set whose key attribute equals the planned
// element's. ok is false when there is nothing safe to carry: no prior state,
// an unreadable or unknown key, or zero or several prior elements with it.
func keyedPriorValue(ctx context.Context, plan tfsdk.Plan, state tfsdk.State, p path.Path, key string) (attr.Value, bool) {
	steps := p.Steps()
	if state.Raw.IsNull() || plan.Raw.IsNull() || len(steps) < 3 {
		return nil, false
	}
	name, isName := steps[len(steps)-1].(path.PathStepAttributeName)
	if !isName {
		return nil, false
	}
	elemPath := p.ParentPath()

	var planKey types.String
	if plan.GetAttribute(ctx, elemPath.AtName(key), &planKey).HasError() || planKey.IsNull() || planKey.IsUnknown() {
		return nil, false
	}

	var priorElems []attr.Value
	switch elemPath.Steps()[len(elemPath.Steps())-1].(type) {
	case path.PathStepElementKeyValue:
		var prior types.Set
		if state.GetAttribute(ctx, elemPath.ParentPath(), &prior).HasError() || prior.IsNull() || prior.IsUnknown() {
			return nil, false
		}
		priorElems = prior.Elements()
	case path.PathStepElementKeyInt:
		var prior types.List
		if state.GetAttribute(ctx, elemPath.ParentPath(), &prior).HasError() || prior.IsNull() || prior.IsUnknown() {
			return nil, false
		}
		priorElems = prior.Elements()
	default:
		return nil, false
	}

	var found attr.Value
	for _, e := range priorElems {
		obj, isObj := e.(types.Object)
		if !isObj || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		attrs := obj.Attributes()
		k, isString := attrs[key].(types.String)
		if !isString || !k.Equal(planKey) {
			continue
		}
		if found != nil {
			return nil, false
		}
		found = attrs[string(name)]
	}
	return found, found != nil
}
