// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package category

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildCategoryInput converts the Terraform plan model into an SDK Category payload.
func buildCategoryInput(plan CategoryResourceModel) *pro.Category {
	return &pro.Category{
		Name:     plan.Name.ValueString(),
		Priority: int(plan.Priority.ValueInt64()),
	}
}
