// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildScriptInput converts the Terraform plan model into an SDK Script payload.
// Null or Unknown plan values become omitted (nil) fields — Jamf Pro rejects an
// explicit empty string on `categoryId` and a wired-in `categoryId: ""` on an
// Optional+Computed attribute would surface as HTTP 500. helpers.OptionalStringPointer
// nils both Null *and* Unknown; see STYLE_GUIDE.md §Server-derived computed fields.
//
// The endpoint replaces the whole record, so a field missing from the body
// would be reset — but no user-settable attribute can reach this builder as
// Null on an update. Every one is Optional+Computed with UseStateForUnknown, so
// dropping it from configuration yields an Unknown that the plan modifier fills
// from prior state; the resulting known value is re-emitted and the stored value
// survives. TestAccResource_ProScript_SplitOwnership asserts that contract. Null
// reaches the builder only on Create, where there is no prior value to preserve.
// An explicit "" is a known value, so it arrives as a pointer to the empty
// string and clears the field — except on `categoryId`, which rejects it.
func buildScriptInput(plan ScriptResourceModel) *pro.Script {
	return &pro.Script{
		Name:           plan.Name.ValueString(),
		CategoryID:     helpers.OptionalStringPointer(plan.CategoryID),
		Info:           helpers.OptionalStringPointer(plan.Info),
		Notes:          helpers.OptionalStringPointer(plan.Notes),
		OsRequirements: helpers.OptionalStringPointer(plan.OsRequirements),
		Priority:       helpers.OptionalStringPointer(plan.Priority),
		Parameter4:     helpers.OptionalStringPointer(plan.Parameter4),
		Parameter5:     helpers.OptionalStringPointer(plan.Parameter5),
		Parameter6:     helpers.OptionalStringPointer(plan.Parameter6),
		Parameter7:     helpers.OptionalStringPointer(plan.Parameter7),
		Parameter8:     helpers.OptionalStringPointer(plan.Parameter8),
		Parameter9:     helpers.OptionalStringPointer(plan.Parameter9),
		Parameter10:    helpers.OptionalStringPointer(plan.Parameter10),
		Parameter11:    helpers.OptionalStringPointer(plan.Parameter11),
		ScriptContents: helpers.OptionalStringPointer(plan.ScriptContents),
	}
}
