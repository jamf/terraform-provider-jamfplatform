// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var _ action.Action = (*DeleteUserAction)(nil)
var _ action.ActionWithConfigure = (*DeleteUserAction)(nil)
var _ action.ActionWithConfigValidators = (*DeleteUserAction)(nil)

// DeleteUserAction removes user accounts from Shared iPads.
//
// Jamf Pro checks none of the command's fields (wire-probed 2026-10-22): it
// accepted a name, delete-all, both, neither and an empty name, and did not
// check the device is a Shared iPad (a Mac accepted it too). The rule that
// exactly one of user_name and delete_all_users must be set therefore lives in
// this action's validators, and so does the rule that delete_all_users can only
// be true, since false would read as "delete nobody" while sending nothing.
type DeleteUserAction struct {
	mdmAction
}

type DeleteUserActionModel struct {
	ManagementIDs  types.List           `tfsdk:"management_ids"`
	SerialNumbers  types.List           `tfsdk:"serial_numbers"`
	UserName       types.String         `tfsdk:"user_name"`
	DeleteAllUsers types.Bool           `tfsdk:"delete_all_users"`
	ForceDeletion  types.Bool           `tfsdk:"force_deletion"`
	Timeouts       actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewDeleteUserAction() action.Action {
	return &DeleteUserAction{}
}

func (a *DeleteUserAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_delete_user"
}

func (a *DeleteUserAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetListAttributes("Shared iPad")
	attrs["user_name"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The account to remove. Set this or `delete_all_users`.",
		Validators:          []validator.String{hasVisibleText},
	}
	attrs["delete_all_users"] = actionschema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Set to `true` to remove every account. Set this or `user_name`.",
		Validators:          []validator.Bool{boolvalidator.Equals(true)},
	}
	attrs["force_deletion"] = actionschema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Remove the account even when it has data that has not synced yet. That data is lost.",
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Removes a user account from one or more Shared iPads. Every targeted iPad gets the same choice of `user_name` or `delete_all_users`." +
			"\n\nA Shared iPad that already has this command waiting keeps that command and queues no other, so a second account you name in the meantime is not removed. Remove one account at a time and wait for each command to finish before sending the next." +
			batchNote + deleteUserPrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *DeleteUserAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return append(deviceTargetListConfigValidators(),
		actionvalidator.ExactlyOneOf(
			path.MatchRoot("user_name"),
			path.MatchRoot("delete_all_users"),
		),
	)
}

func (a *DeleteUserAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *DeleteUserAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data DeleteUserActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, deleteUserCommand(data), "Delete User")
}

func deleteUserCommand(data DeleteUserActionModel) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeDeleteUser),
		DELETEUSER: &pro.DeleteUserCommand{
			CommandType:    pro.MDMCommandTypeDeleteUser,
			DeleteAllUsers: data.DeleteAllUsers.ValueBoolPointer(),
			ForceDeletion:  data.ForceDeletion.ValueBoolPointer(),
			UserName:       data.UserName.ValueStringPointer(),
		},
	}
}
