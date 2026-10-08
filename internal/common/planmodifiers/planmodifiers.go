// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package planmodifiers provides shared Terraform Plugin Framework plan
// modifiers for use across all resource packages.
package planmodifiers

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ResetIfSourceChangedString returns a plan modifier for Optional+Computed
// string attributes whose server-derived value depends on the bytes of a
// *_file_source upload input. When any watched source attribute differs
// between state and plan, the attribute's plan value is left Unknown so
// the server can populate a fresh value during apply. When all sources are
// unchanged, the prior state value carries forward (UseStateForUnknown
// semantics) so trivial metadata updates do not churn the diff.
func ResetIfSourceChangedString(sourcePaths ...path.Expression) planmodifier.String {
	return resetStringIfSourceChanged{sources: sourcePaths}
}

// readSourceStrings extracts the watched source attribute from both plan
// and state for the string plan-modifier variant. Returns ok=false if
// either read produced a diagnostic; the modifier should bail in that case.
func readSourceStrings(ctx context.Context, req planmodifier.StringRequest, sourcePath path.Expression, resp *planmodifier.StringResponse) (planSrc, stateSrc types.String, ok bool) {
	planPaths, diags := req.Plan.PathMatches(ctx, sourcePath)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || len(planPaths) == 0 {
		return planSrc, stateSrc, false
	}
	statePaths, diags := req.State.PathMatches(ctx, sourcePath)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || len(statePaths) == 0 {
		return planSrc, stateSrc, false
	}
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, planPaths[0], &planSrc)...)
	if resp.Diagnostics.HasError() {
		return planSrc, stateSrc, false
	}
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, statePaths[0], &stateSrc)...)
	if resp.Diagnostics.HasError() {
		return planSrc, stateSrc, false
	}
	return planSrc, stateSrc, true
}

type resetStringIfSourceChanged struct{ sources []path.Expression }

func (m resetStringIfSourceChanged) Description(_ context.Context) string {
	return "Carry prior state forward when the watched source attribute is unchanged; otherwise leave it Unknown for the service to populate."
}

func (m resetStringIfSourceChanged) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m resetStringIfSourceChanged) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() {
		return
	}

	allEqual := true
	for _, src := range m.sources {
		planSrc, stateSrc, ok := readSourceStrings(ctx, req, src, resp)
		if !ok {
			return
		}
		if !planSrc.Equal(stateSrc) {
			allEqual = false
			break
		}
	}

	if override, ok := DecideResetForUnchangedString(allEqual, req.StateValue); ok {
		resp.PlanValue = override
	}
}

// DecideResetForUnchangedString centralises the override decision for the
// String plan modifier so it can be unit-tested without constructing a
// full tfsdk.Plan / State. The surrounding plumbing in PlanModifyString
// (Raw null guards, config-value precedence, path reads) is covered by
// the acceptance suite.
//
// Returns (value, true) when the caller should overwrite resp.PlanValue,
// or (zero, false) to leave it untouched (default Unknown).
//
//   - sourceEqual=false → (_, false): leave Unknown; apply writes the fresh server value.
//   - sourceEqual=true + state known + state non-null → (state, true): carry forward.
//   - state null or unknown → (_, false).
func DecideResetForUnchangedString(sourceEqual bool, stateValue types.String) (types.String, bool) {
	if !sourceEqual {
		return types.StringNull(), false
	}
	if stateValue.IsNull() || stateValue.IsUnknown() {
		return types.StringNull(), false
	}
	return stateValue, true
}

// MirrorOfString returns a plan modifier for a Computed-only string attribute
// that Jamf Pro derives from one or more SIBLING attributes the same resource
// manages — a mirror, not an independent server-assigned value.
//
// Neither of the obvious answers is right for a mirror. Plain
// UseStateForUnknown promises the value cannot change unless the practitioner
// changes this attribute, but it changes when the *source* changes, so the plan
// carries a stale value into an apply that returns a fresh one and Terraform
// reports "Provider produced inconsistent result after apply". Omitting the
// modifier entirely leaves the attribute Unknown on every plan, which makes
// `terraform plan -refresh=false` permanently dirty for a value nobody touched.
//
// So: carry the prior value forward while every watched source is unchanged
// between state and plan, and go Unknown as soon as one of them moves. The
// sources may be of any type — pass StringSource or BoolSource per path.
//
// Used by jamfplatform_pro_mobile_device_app for general.description (mirrors
// self_service.self_service_description) and general.deployment_type (mirrors
// general.deploy_automatically).
func MirrorOfString(sources ...SourceComparer) planmodifier.String {
	return mirrorOfString{sources: sources}
}

// SourceComparer reports whether one watched source attribute is unchanged by
// this plan. ok is false when the value could not be read at all, which the
// caller treats as "cannot tell" and resolves conservatively.
type SourceComparer func(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) (unchanged, ok bool)

// StringSource watches a string attribute.
func StringSource(p path.Expression) SourceComparer { return compareSource[types.String](p) }

// BoolSource watches a bool attribute.
func BoolSource(p path.Expression) SourceComparer { return compareSource[types.Bool](p) }

// NestedStringSource watches one string attribute inside a single-nested block.
//
// Prefer it to a path into an Optional block. When the block is null,
// PathMatches resolves the expression only as far as the block, so a leaf
// watcher receives the object's path and fails with "Cannot use attr.Value
// basetypes.StringValue, only basetypes.ObjectValue is supported". This reads
// the block first and the leaf from it.
//
// Do not watch the whole block either. compareSource calls a block unchanged
// only when it is null in config. A declared block with an unset
// Optional+Computed leaf is null there and populated in state, so the two never
// compare equal, and the mirror goes Unknown in every plan that changes
// anything else in the resource, even when the leaf it mirrors did not move.
//
// The result per case:
//   - A null block, or a null leaf, is unchanged. The practitioner is not
//     moving the source, and an unset Optional+Computed leaf carries its prior
//     value forward itself.
//   - An Unknown block or leaf is changed. The value is not known until apply
//     (interpolated from a resource still being created), so a prior mirror
//     value may be stale by then and Terraform would reject the result.
//   - A leaf set where state has no block or no value is changed.
//   - A set leaf is unchanged exactly when it equals the state value.
//   - A block that cannot be read, or a leaf the block does not have, cannot be
//     judged, so the mirror stays Unknown.
func NestedStringSource(block path.Expression, attribute string) SourceComparer {
	return func(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) (bool, bool) {
		configBlock, ok := readSourceObject(ctx, req.Config.PathMatches, req.Config.GetAttribute, block, resp)
		if !ok {
			return false, false
		}
		if configBlock.IsUnknown() {
			return false, true
		}
		if configBlock.IsNull() {
			return true, true
		}

		configLeaf, ok := configBlock.Attributes()[attribute].(types.String)
		if !ok {
			return false, false
		}
		if configLeaf.IsUnknown() {
			return false, true
		}
		if configLeaf.IsNull() {
			return true, true
		}

		stateBlock, ok := readSourceObject(ctx, req.State.PathMatches, req.State.GetAttribute, block, resp)
		if !ok {
			return false, false
		}
		if stateBlock.IsNull() || stateBlock.IsUnknown() {
			return false, true
		}
		stateLeaf, ok := stateBlock.Attributes()[attribute].(types.String)
		if !ok {
			return false, false
		}
		return configLeaf.Equal(stateLeaf), true
	}
}

// readSourceObject resolves a single-nested block from either the config or the
// state, taking that side's PathMatches and GetAttribute so the two sides share
// one read. ok is false when the block could not be read at all.
func readSourceObject(
	ctx context.Context,
	pathMatches func(context.Context, path.Expression) (path.Paths, diag.Diagnostics),
	getAttribute func(context.Context, path.Path, any) diag.Diagnostics,
	block path.Expression,
	resp *planmodifier.StringResponse,
) (obj types.Object, ok bool) {
	paths, diags := pathMatches(ctx, block)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() || len(paths) == 0 {
		return obj, false
	}
	if diags := getAttribute(ctx, paths[0], &obj); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return obj, false
	}
	return obj, true
}

// compareSource builds a SourceComparer for any framework value type. The type
// parameter is what lets GetAttribute reflect into a concrete target: an
// attr.Value interface target is not something the framework can populate.
//
// The source is read from the CONFIG, not the plan. Reading a sibling from
// req.Plan looks natural and is unreliable: the plan is the proposed new state,
// where a Computed sibling with no configured value is still Unknown until its
// own plan modifier runs, and modifier order within a nested object is not
// something a caller controls. So a plan read intermittently sees Unknown,
// concludes "changed", and leaves the mirror Unknown on a plan where nothing
// moved — which is exactly the dirty-plan symptom this modifier exists to fix.
//
// The config is stable, and it answers the question that actually matters. A
// source the practitioner has not configured cannot be changing: its own
// Optional+Computed handling carries the prior value forward. A source they
// have configured is unchanged precisely when the configured value equals what
// state holds. A null configured value therefore reports unchanged: the
// practitioner is not moving it. An Unknown one reports changed: it resolves
// during apply (interpolated from a resource still being created), the server
// mirrors whatever it resolves to, and a carried-forward prior value would
// reach apply as a stale plan that Terraform rejects.
func compareSource[T attr.Value](p path.Expression) SourceComparer {
	return func(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) (bool, bool) {
		configPaths, diags := req.Config.PathMatches(ctx, p)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() || len(configPaths) == 0 {
			return false, false
		}
		statePaths, diags := req.State.PathMatches(ctx, p)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() || len(statePaths) == 0 {
			return false, false
		}
		var configSrc, stateSrc T
		if diags := req.Config.GetAttribute(ctx, configPaths[0], &configSrc); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return false, false
		}
		if diags := req.State.GetAttribute(ctx, statePaths[0], &stateSrc); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return false, false
		}
		if configSrc.IsUnknown() {
			return false, true
		}
		if configSrc.IsNull() {
			return true, true
		}
		return configSrc.Equal(stateSrc), true
	}
}

type mirrorOfString struct{ sources []SourceComparer }

func (m mirrorOfString) Description(_ context.Context) string {
	return "Carry the prior value forward while every attribute this one mirrors is unchanged; go Unknown as soon as one of them moves."
}

func (m mirrorOfString) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }

// PlanModifyString carries the prior value forward when every watched source is
// unchanged, and otherwise leaves the plan value Unknown for the apply to fill.
//
// Three early returns leave it Unknown. Create has no prior state and destroy
// has no plan, so neither has anything to carry. A source that could not be read
// is treated as "cannot tell": an apply then writes whatever the server says,
// which is always right, where assuming "unchanged" would risk the
// inconsistent-result error this modifier exists to avoid. And a source that
// moved means the mirror moves with it.
//
// The carry itself differs from DecideResetForUnchangedString in taking a NULL
// prior value as well. A mirror whose source has no value has none either, and
// refusing to carry null leaves `plan -refresh=false` permanently dirty on
// exactly the configurations that never touch the source, which is most of
// them. Unknown prior state is the one thing not carried, there being no value
// there to carry.
func (m mirrorOfString) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	for _, src := range m.sources {
		unchanged, ok := src(ctx, req, resp)
		if !ok || !unchanged {
			return
		}
	}

	if !req.StateValue.IsUnknown() {
		resp.PlanValue = req.StateValue
	}
}
