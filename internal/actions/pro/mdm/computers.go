// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/action"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// requireComputers fails unless every id is a computer in Jamf Pro.
//
// Nothing else catches a mobile device: Jamf Pro queues a computer-only command
// for an iPad and answers 201 (wire-probed 2026-11-10), and a device group of
// either type hands back plain management ids, so passing the members of the
// wrong group would otherwise queue commands that can never run. The computer
// inventory is the one place that tells the types apart, because the Platform
// device list carries no type. Filtering it by management id drops everything
// that is not a computer, so what is missing from the answer is what is not one.
func (a *mdmAction) requireComputers(ctx context.Context, resp *action.InvokeResponse, ids []string) bool {
	chunks := chunkSerialsByEncodedSize(ids, serialFilterBudgetBytes)

	computers := make(map[string]bool, len(ids))
	for i, chunk := range chunks {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf(
			"Checking %d device(s) are computers (request %d of %d)", len(chunk), i+1, len(chunks))})

		quoted := make([]string, 0, len(chunk))
		for _, id := range chunk {
			quoted = append(quoted, quoteRSQLValue(id))
		}

		found, err := a.client.ListComputersInventoryV4(ctx, []string{"GENERAL"}, nil, "general.managementId=in=("+strings.Join(quoted, ",")+")")
		if err != nil {
			resp.Diagnostics.AddError(
				"Computer Lookup Failed",
				fmt.Sprintf("Unable to check that %d device(s) are computers: %s", len(chunk), helpers.APIErrorDetail(err)),
			)
			return false
		}
		for _, c := range found {
			if c.General != nil {
				computers[c.General.ManagementID] = true
			}
		}
	}

	var notComputers []string
	for _, id := range ids {
		if !computers[id] {
			notComputers = append(notComputers, id)
		}
	}
	if len(notComputers) > 0 {
		resp.Diagnostics.AddError(
			"Not A Computer",
			fmt.Sprintf("%d of %d target(s) are not computers in Jamf Pro, and this command applies only to Macs: %s. The members of a mobile device group are mobile devices. Use a computer group.",
				len(notComputers), len(ids), strings.Join(notComputers, ", ")),
		)
		return false
	}
	return true
}
