// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
)

// criteriaPriorityValidator refuses an authored criterion priority that is not
// the criterion's own position in the list.
//
// Jamf Pro requires the priorities to run from 0 upwards with no gaps and
// answers 400 otherwise, so position is the only legal numbering and the builder
// emits it. Refusing a disagreeing value beats silently replacing it: an
// Optional+Computed attribute written with a value other than the planned one
// aborts the apply with "provider produced inconsistent result after apply", and
// an operator who wrote a priority meant something by it.
//
// The decision itself lives in criteria.ValidateCriteriaPriorities, shared with
// the rest of this family. This type is the plan-time seam.
type criteriaPriorityValidator struct{}

// Description returns a plain-text description of the validator.
func (criteriaPriorityValidator) Description(context.Context) string {
	return "each criterion's priority must equal its position in the criteria list"
}

// MarkdownDescription returns the Markdown description of the validator.
func (v criteriaPriorityValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource applies the priority check.
//
// The criteria list is read as its typed collection value rather than decoded
// into Go models, because decoding collapses an unknown element to nothing and a
// variable-driven or for_each-driven configuration is not a defect. Anything the
// walk cannot see as a known object defers to apply, where the server is the
// authority anyway.
func (criteriaPriorityValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var configured types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("criteria"), &configured)...)
	if resp.Diagnostics.HasError() || configured.IsNull() || configured.IsUnknown() {
		return
	}

	elements := configured.Elements()
	models := make([]criteria.CriterionModel, 0, len(elements))
	for _, element := range elements {
		object, ok := element.(basetypes.ObjectValue)
		if !ok || object.IsNull() || object.IsUnknown() {
			return
		}
		var model criteria.CriterionModel
		if diags := object.As(ctx, &model, basetypes.ObjectAsOptions{}); diags.HasError() {
			return
		}
		models = append(models, model)
	}

	resp.Diagnostics.Append(criteria.ValidateCriteriaPriorities(path.Root("criteria"), models)...)
}
