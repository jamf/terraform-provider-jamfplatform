// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
)

// criteriaPrioritiesConfigValidator rejects an authored criterion priority that
// is not the criterion's own position in the list.
//
// The endpoint requires the priorities to run from zero upwards with no gaps, so
// the position is the only legal numbering and the input builder emits it. An
// authored value that disagrees is refused rather than silently replaced,
// because overwriting an Optional+Computed value that the plan carried aborts
// the apply with a post-apply inconsistency, and because someone who wrote a
// priority meant something by it.
//
// The rule spans the whole list rather than one element, so it reads `criteria`
// as a typed value through GetAttribute and not through a decode of the whole
// config, which fails outright when any nested value is unknown. Every unknown
// shape defers: a list that is unknown, and a list holding an unknown element,
// are both "not resolvable yet" rather than wrong, and erroring on either would
// make the resource unusable from a module.
type criteriaPrioritiesConfigValidator struct{}

// Description returns a plain-text description of the validator.
func (criteriaPrioritiesConfigValidator) Description(context.Context) string {
	return "each criterion's priority, when supplied, must equal its position in the criteria list"
}

// MarkdownDescription returns the Markdown description of the validator.
func (v criteriaPrioritiesConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource implements the plan-time check.
func (criteriaPrioritiesConfigValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var configured types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("criteria"), &configured)...)
	if resp.Diagnostics.HasError() || configured.IsNull() || configured.IsUnknown() {
		return
	}

	elements := configured.Elements()
	for _, element := range elements {
		if element.IsNull() || element.IsUnknown() {
			return
		}
	}

	models := make([]criteria.CriterionModel, 0, len(elements))
	resp.Diagnostics.Append(configured.ElementsAs(ctx, &models, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(criteria.ValidateCriteriaPriorities(path.Root("criteria"), models)...)
}
