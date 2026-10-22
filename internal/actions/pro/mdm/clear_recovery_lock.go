// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var _ action.Action = (*ClearRecoveryLockAction)(nil)
var _ action.ActionWithConfigure = (*ClearRecoveryLockAction)(nil)
var _ action.ActionWithConfigValidators = (*ClearRecoveryLockAction)(nil)

// ClearRecoveryLockAction removes the Recovery Lock password from Apple silicon
// Macs. It sends SET_RECOVERY_LOCK with an empty newPassword, which is how
// Apple's SetRecoveryLock specification says to clear it. Jamf Pro accepts the
// empty string (wire-probed 2026-10-22); the effect on a device is unverified.
type ClearRecoveryLockAction struct {
	mdmAction
}

type ClearRecoveryLockActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewClearRecoveryLockAction() action.Action {
	return &ClearRecoveryLockAction{}
}

func (a *ClearRecoveryLockAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_clear_recovery_lock"
}

func (a *ClearRecoveryLockAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Removes the Recovery Lock password from one or more Macs with Apple silicon. If a computer already has a Recovery Lock command waiting, Jamf Pro keeps that command and queues no other." +
			batchNote + clearRecoveryLockPrivileges,
		Attributes: targetListAttributes("computer"),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *ClearRecoveryLockAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *ClearRecoveryLockAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *ClearRecoveryLockAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data ClearRecoveryLockActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, setRecoveryLockCommand(""), "Clear Recovery Lock")
}
