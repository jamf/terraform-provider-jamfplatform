// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var _ action.Action = (*SetRecoveryLockAction)(nil)
var _ action.ActionWithConfigure = (*SetRecoveryLockAction)(nil)
var _ action.ActionWithConfigValidators = (*SetRecoveryLockAction)(nil)

// SetRecoveryLockAction sets the Recovery Lock password on Apple silicon Macs.
//
// The password is the same for every device in the batch. The command carries
// only newPassword: Apple's SetRecoveryLock also takes a CurrentPassword that a
// device holding a password requires, which the Jamf command has no field for,
// so Jamf Pro supplies it. That is unverified against a device. An empty
// newPassword clears the lock (Apple's specification), which is clear_recovery_lock.
type SetRecoveryLockAction struct {
	mdmAction
}

type SetRecoveryLockActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Password      types.String         `tfsdk:"password"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewSetRecoveryLockAction() action.Action {
	return &SetRecoveryLockAction{}
}

func (a *SetRecoveryLockAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_set_recovery_lock"
}

func (a *SetRecoveryLockAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetListAttributes("computer")
	attrs["password"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The Recovery Lock password. Every targeted computer gets the same one. To remove the password, use `jamfplatform_pro_clear_recovery_lock`." + secretAttrNote,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Sets the Recovery Lock password on one or more Macs with Apple silicon. If a computer already has this command waiting, for example because it is offline, Jamf Pro keeps that command and queues no other, so a password you change in the meantime does not reach the computer." +
			batchNote + setRecoveryLockPrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *SetRecoveryLockAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *SetRecoveryLockAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *SetRecoveryLockAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data SetRecoveryLockActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, setRecoveryLockCommand(data.Password.ValueString()), "Set Recovery Lock")
}

// setRecoveryLockCommand builds the command for a password. The pointer is
// always set: Jamf Pro refuses a missing password, and an empty one clears.
func setRecoveryLockCommand(password string) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeSetRecoveryLock),
		SETRECOVERYLOCK: &pro.SetRecoveryLockCommand{
			CommandType: pro.MDMCommandTypeSetRecoveryLock,
			NewPassword: &password,
		},
	}
}
