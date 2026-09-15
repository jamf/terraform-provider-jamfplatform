// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildDepartmentInput converts the Terraform plan model into an SDK Department payload.
// Department has only a single Required Name field, so no Optional/Unknown handling is needed.
func buildDepartmentInput(plan DepartmentResourceModel) *pro.Department {
	return &pro.Department{
		Name: plan.Name.ValueString(),
	}
}
