// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// groupIDPattern matches the device group identifiers a blueprint uses. Device
// groups are addressed by their Platform identifier throughout blueprints,
// including inside activation condition expressions, where they appear as
// quoted literals.
var groupIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// activationConditionsReason explains why an activation condition caveats the
// figure rather than adjusting it.
//
// The expression language can require devices or groups, rule them out, and
// combine several tests with boolean operators. Deciding what an expression as
// a whole does to the audience is not tractable here, and guessing would
// produce a confident figure that is wrong in an unpredictable direction — so
// every non-empty expression caveats the figure. When an expression names
// groups, those are additionally surfaced by name, which is the part a
// reviewer cannot easily get for themselves — an activation condition shows
// identifiers, not names.
const activationConditionsReason = "activation conditions can require or rule out devices or groups, so their effect on the audience is not counted here"

// blueprintImpactScope converts a blueprint's device group targeting into the
// shape the impact package counts.
//
// device_groups is the blueprint's audience and is counted. Any non-empty
// activation condition on a component block gates
// which devices the blueprint applies to even when it names no group (an
// OS-version test narrows just as a group test does), so each one caveats the
// figure as ambiguous. Groups the expressions name are additionally reported
// by name but deliberately left out of the figure.
func blueprintImpactScope(ctx context.Context, m *BlueprintResourceModel) impact.Scope {
	b := impact.NewScopeBuilder(ctx, impact.DeviceTypeAny)
	if m == nil {
		return b.Scope()
	}
	b.PlatformGroups("device_groups", m.DeviceGroups)

	var mentioned []string
	conditions := 0
	collect := func(expr string) {
		if strings.TrimSpace(expr) == "" {
			return
		}
		conditions++
		mentioned = append(mentioned, groupIDPattern.FindAllString(expr, -1)...)
	}
	for _, blk := range m.ComponentBlocks {
		if !blk.ActivationConditions.IsNull() && !blk.ActivationConditions.IsUnknown() {
			collect(blk.ActivationConditions.ValueString())
		}
	}
	if conditions == 0 {
		return b.Scope()
	}
	s := b.Scope()
	// Values carries the group count when the expressions name groups, and the
	// expression count otherwise — the caveat must always show a real figure.
	values := conditions
	if len(mentioned) > 0 {
		s.MentionedPlatformIDs = dedupe(mentioned)
		values = len(s.MentionedPlatformIDs)
	}
	s.Unresolvable = append(s.Unresolvable, impact.Unresolvable{
		Path:   "activation_conditions",
		Reason: activationConditionsReason,
		Effect: impact.Ambiguous,
		Values: values,
	})
	return s
}

// dedupe returns ids with duplicates removed, preserving first-seen order.
func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// reportDeviceGroupImpact emits the plan-time impact alert for a blueprint's
// device group targeting.
//
// A blueprint is a deployable object in Jamf Pro's terms. It is also the case
// where the alert earns the most: a blueprint's audience is expressed as bare
// identifiers, so how many devices it reaches is not evident from reading the
// configuration.
func (r *BlueprintResource) reportDeviceGroupImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	impact.ReportPlan(ctx, req, resp, impact.PlanReport{
		Cache: r.impact,
		Path:  path.Root("device_groups"),
		Label: "blueprint",
	}, blueprintImpactScope)
}
