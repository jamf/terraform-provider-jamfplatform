// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package computertarget resolves a Jamf Pro computer inventory id from the
// serial-number / management-id / udid selector shared by device-targeting
// actions (e.g. redeploy_management_framework, jamf_protect_deployment_retry).
// The device data sources surface a management id (UUID), not the Jamf Pro id,
// so a management id is mapped through the computer inventory; a serial number
// and a hardware UDID each resolve directly.
//
// # Endpoint status
//
// This package reads /v4/computers-inventory, which is current and carries no
// deprecation marker. Its RSQL filter contract is unchanged from V3 (wire-probed
// against Jamf Pro 11.31.1 on 2026-09-01: filtering on general.managementId
// returns the single matching record). V1, V2 and V3 were withdrawn from the SDK
// rather than from the server — a live 11.31.1 tenant still serves /v3 — but V4
// is the only version with a generated client.
//
// Deliberately not routed through /preview/computers, which also carries
// managementId: it accepts no filter, so resolving one computer would mean
// paging the entire computer list — an unbounded per-invocation cost that grows
// with fleet size.
package computertarget

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ResolveComputerID ensures exactly one computer identifier is provided and
// returns the Jamf Pro computer id (an integer string). It reports its progress
// and any failure through resp; the boolean is false when the caller should
// stop (a diagnostic has been added).
func ResolveComputerID(ctx context.Context, client *pro.Client, resp *action.InvokeResponse, managementIDAttr, serialNumberAttr, udidAttr types.String) (string, bool) {
	hasID := helpers.IsConfiguredValue(managementIDAttr)
	hasSerial := helpers.IsConfiguredValue(serialNumberAttr)
	hasUDID := helpers.IsConfiguredValue(udidAttr)

	n := 0
	for _, on := range []bool{hasID, hasSerial, hasUDID} {
		if on {
			n++
		}
	}

	switch {
	case n > 1:
		resp.Diagnostics.AddError(
			"Multiple Device Identifiers Provided",
			"Specify only one of management_id, serial_number, or udid when invoking this action.",
		)
		return "", false
	case hasID:
		managementID := managementIDAttr.ValueString()
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Resolving management id %s", managementID)})

		matches, err := client.ListComputersInventoryV4(ctx, []string{pro.ComputerSectionV4General}, nil, fmt.Sprintf("general.managementId==%q", managementID))
		if err != nil {
			resp.Diagnostics.AddError(
				"Computer Lookup Failed",
				fmt.Sprintf("Unable to look up computer for management id %s: %s", managementID, helpers.APIErrorDetail(err)),
			)
			return "", false
		}
		if len(matches) == 0 || matches[0].ID == "" {
			resp.Diagnostics.AddError(
				"Computer Not Found",
				fmt.Sprintf("No computer matched management id %s.", managementID),
			)
			return "", false
		}
		return matches[0].ID, true
	case hasSerial:
		serial := serialNumberAttr.ValueString()
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Resolving serial number %s", serial)})

		id, err := client.ResolveComputerInventoryV4IDBySerialNumber(ctx, serial)
		if err != nil {
			resp.Diagnostics.AddError(
				"Computer Lookup Failed",
				fmt.Sprintf("Unable to resolve serial number %s to a computer id: %s", serial, helpers.APIErrorDetail(err)),
			)
			return "", false
		}
		return id, true
	case hasUDID:
		udid := udidAttr.ValueString()
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Resolving udid %s", udid)})

		id, err := client.ResolveComputerInventoryV4IDByUDID(ctx, udid)
		if err != nil {
			resp.Diagnostics.AddError(
				"Computer Lookup Failed",
				fmt.Sprintf("Unable to resolve udid %s to a computer id: %s", udid, helpers.APIErrorDetail(err)),
			)
			return "", false
		}
		return id, true
	default:
		resp.Diagnostics.AddError(
			"Missing Device Identifier",
			"Specify one of management_id, serial_number, or udid to select the computer.",
		)
		return "", false
	}
}
