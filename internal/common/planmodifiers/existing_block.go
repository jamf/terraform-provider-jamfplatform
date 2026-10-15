// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package planmodifiers

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UseStateForUnknownInExistingBlockString returns a plan modifier for an
// Optional+Computed string inside an Optional single-nested block. It behaves
// as UseStateForUnknown except when the block is new in this plan, where it
// leaves the value Unknown. See carryPrior for the rule.
func UseStateForUnknownInExistingBlockString() planmodifier.String {
	return existingBlockString{}
}

// UseStateForUnknownInExistingBlockBool is the bool sibling of
// UseStateForUnknownInExistingBlockString.
func UseStateForUnknownInExistingBlockBool() planmodifier.Bool {
	return existingBlockBool{}
}

// UseStateForUnknownInExistingBlockInt64 is the int64 sibling of
// UseStateForUnknownInExistingBlockString.
func UseStateForUnknownInExistingBlockInt64() planmodifier.Int64 {
	return existingBlockInt64{}
}

// carryPrior decides whether a leaf with an Unknown plan value takes its prior
// value, given that the prior value is known to be null or not.
//
// A non-null prior is always carried. A null prior is the case two stock
// modifiers answer differently, and each is wrong in one direction:
//
//   - UseStateForUnknown carries it. When the enclosing block goes from omitted
//     to declared, every leaf in it holds null in state, so the plan says null,
//     apply returns the server's default, and Terraform fails with "Provider
//     produced inconsistent result after apply".
//   - UseNonNullStateForUnknown leaves it Unknown. A leaf Jamf Pro never returns
//     then reads "(known after apply)" in every plan that differs from state at
//     all, because the framework marks each Computed attribute with a null prior
//     Unknown before any modifier runs. A modifier elsewhere that collapses its
//     own change back to a no-op (the CRLF/LF preferences equality on
//     jamfplatform_pro_mobile_device_app) leaves these behind, and `plan
//     -refresh=false` is never empty.
//
// So a null prior is carried exactly when the block holding the leaf already
// exists in state, which is the case UseStateForUnknown gets right, and left
// Unknown when the block is new, the case it gets wrong.
//
// A top-level leaf has no block and behaves as UseStateForUnknown. A leaf
// inside a Set or List element is never carried from a null prior: a new
// element has none, and the element's own path cannot be read back from state.
// If the block cannot be read, the leaf stays Unknown, which apply always
// settles correctly.
func carryPrior(ctx context.Context, leaf path.Path, state tfsdk.State, priorIsNull bool) bool {
	if !priorIsNull {
		return true
	}
	parent := leaf.ParentPath()
	if len(parent.Steps()) == 0 {
		return true
	}
	for _, step := range parent.Steps() {
		switch step.(type) {
		case path.PathStepElementKeyInt, path.PathStepElementKeyString, path.PathStepElementKeyValue:
			return false
		}
	}
	var block types.Object
	if diags := state.GetAttribute(ctx, parent, &block); diags.HasError() {
		return false
	}
	return !block.IsNull() && !block.IsUnknown()
}

const existingBlockDescription = "Carry the prior value forward, unless the block holding this attribute is new, in which case leave it unknown for the service to populate."

type existingBlockString struct{}

func (existingBlockString) Description(context.Context) string { return existingBlockDescription }

func (m existingBlockString) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (existingBlockString) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	if carryPrior(ctx, req.Path, req.State, req.StateValue.IsNull()) {
		resp.PlanValue = req.StateValue
	}
}

type existingBlockBool struct{}

func (existingBlockBool) Description(context.Context) string { return existingBlockDescription }

func (m existingBlockBool) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (existingBlockBool) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	if carryPrior(ctx, req.Path, req.State, req.StateValue.IsNull()) {
		resp.PlanValue = req.StateValue
	}
}

type existingBlockInt64 struct{}

func (existingBlockInt64) Description(context.Context) string { return existingBlockDescription }

func (m existingBlockInt64) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (existingBlockInt64) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.State.Raw.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	if carryPrior(ctx, req.Path, req.State, req.StateValue.IsNull()) {
		resp.PlanValue = req.StateValue
	}
}
