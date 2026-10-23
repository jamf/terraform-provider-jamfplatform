// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var _ action.Action = (*UnlockUserAccountAction)(nil)
var _ action.ActionWithConfigure = (*UnlockUserAccountAction)(nil)
var _ action.ActionWithConfigValidators = (*UnlockUserAccountAction)(nil)

// UnlockUserAccountAction unlocks a local user account on computers. Jamf Pro
// refuses an empty or missing user name (wire-probed 2026-10-22) and does not
// check the device platform.
type UnlockUserAccountAction struct {
	mdmAction
}

type UnlockUserAccountActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	UserName      types.String         `tfsdk:"user_name"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewUnlockUserAccountAction() action.Action {
	return &UnlockUserAccountAction{}
}

func (a *UnlockUserAccountAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_unlock_user_account"
}

func (a *UnlockUserAccountAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetListAttributes("computer")
	attrs["user_name"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The local account to unlock.",
		Validators:          []validator.String{hasVisibleText},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Unlocks a local user account on one or more computers. Every targeted computer unlocks the account with the same `user_name`." +
			"\n\nIf a computer already has this command waiting, for example because it is offline, Jamf Pro keeps that command and queues no other, so a different account you name in the meantime is not unlocked." +
			batchNote + unlockUserAccountPrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *UnlockUserAccountAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *UnlockUserAccountAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *UnlockUserAccountAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data UnlockUserAccountActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, unlockUserAccountCommand(data), "Unlock User Account")
}

func unlockUserAccountCommand(data UnlockUserAccountActionModel) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeUnlockUserAccount),
		UNLOCKUSERACCOUNT: &pro.UnlockUserAccountCommand{
			CommandType: pro.MDMCommandTypeUnlockUserAccount,
			UserName:    data.UserName.ValueStringPointer(),
		},
	}
}
