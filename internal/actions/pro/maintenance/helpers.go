// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package maintenanceactions implements the fire-once Jamf Pro device maintenance
// actions: jamfplatform_pro_redeploy_management_framework (redeploy the Jamf
// management framework to a computer) and jamfplatform_pro_flush_policy_logs
// (flush policy logs older than a given interval).
package maintenanceactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/computertarget"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty: these maintenance operations ride continuously
// deployed Jamf Pro endpoints with no meaningful version floor.
const minJamfProVersion = ""

// maintenanceAction shares Configure logic across the maintenance actions. It
// holds the Jamf Pro client (redeploy management framework) and the ProClassic
// client (log flush).
type maintenanceAction struct {
	client  *pro.Client
	classic *proclassic.Client
}

// configure binds the provider-supplied clients to the action.
func (a *maintenanceAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	if _, ok := req.ProviderData.(*providerdata.Data); !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *providerdata.Data, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "maintenance")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	classic, cdiags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "maintenance")
	resp.Diagnostics.Append(cdiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	a.client = client
	a.classic = classic
}

// ensureClient guarantees the Jamf Pro client was configured before Invoke.
func (a *maintenanceAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}
	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Platform client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}

// ensureClassicClient guarantees the ProClassic client was configured before Invoke.
func (a *maintenanceAction) ensureClassicClient(resp *action.InvokeResponse) bool {
	if a.classic != nil {
		return true
	}
	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Platform client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}

// computerTargetAttributes returns the management_id / serial_number / udid
// selector for actions that target a single computer by its Jamf Pro inventory
// record.
//
// Exactly-one-of is enforced by computerTargetConfigValidators rather than by
// per-attribute ConflictsWith, so that supplying NO identifier also fails at
// plan time instead of part-way through the apply.
func computerTargetAttributes() map[string]actionschema.Attribute {
	return map[string]actionschema.Attribute{
		"management_id": actionschema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Jamf Pro Management ID of the computer. This is the `id` reported by the `jamfplatform_devices`/`jamfplatform_device` data sources. Set exactly one of this, `serial_number`, or `udid`.",
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
		"serial_number": actionschema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Serial number of the computer (case-sensitive). Set exactly one of this, `management_id`, or `udid`.",
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
		"udid": actionschema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Hardware UDID of the computer. Set exactly one of this, `management_id`, or `serial_number`.",
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
	}
}

// computerTargetConfigValidators is the plan-time counterpart to
// computerTargetAttributes: exactly one of management_id / serial_number / udid
// selects the computer.
func computerTargetConfigValidators() []action.ConfigValidator {
	return []action.ConfigValidator{
		actionvalidator.ExactlyOneOf(
			path.MatchRoot("management_id"),
			path.MatchRoot("serial_number"),
			path.MatchRoot("udid"),
		),
	}
}

// resolveComputerID delegates to the shared computertarget resolver: exactly
// one of management_id / serial_number / udid selects a Jamf Pro computer id.
func (a *maintenanceAction) resolveComputerID(ctx context.Context, resp *action.InvokeResponse, managementIDAttr, serialNumberAttr, udidAttr types.String) (string, bool) {
	return computertarget.ResolveComputerID(ctx, a.client, resp, managementIDAttr, serialNumberAttr, udidAttr)
}
