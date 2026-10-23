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

var _ action.Action = (*LockDeviceAction)(nil)
var _ action.ActionWithConfigure = (*LockDeviceAction)(nil)
var _ action.ActionWithConfigValidators = (*LockDeviceAction)(nil)

// LockDeviceAction locks computers and mobile devices.
//
// Wire-probed 2026-10-22: Jamf Pro accepts the command with no PIN, even for a
// Mac, and refuses a PIN that is not six characters ("PIN must be 6 characters
// long"). It does not check what the characters are, so a non-numeric PIN is
// accepted and the device is the first to object. Every device in the batch gets
// the same PIN, message and phone number.
type LockDeviceAction struct {
	mdmAction
}

type LockDeviceActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Message       types.String         `tfsdk:"message"`
	PhoneNumber   types.String         `tfsdk:"phone_number"`
	Pin           types.String         `tfsdk:"pin"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewLockDeviceAction() action.Action {
	return &LockDeviceAction{}
}

func (a *LockDeviceAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_lock_device"
}

func (a *LockDeviceAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetListAttributes("device")
	attrs["message"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Message shown on the Lock Screen.",
		Validators:          []validator.String{hasVisibleText},
	}
	attrs["phone_number"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Phone number shown on the Lock Screen.",
		Validators:          []validator.String{hasVisibleText},
	}
	attrs["pin"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Six-character PIN that unlocks a computer afterwards. Mobile devices ignore it. Use six digits, which is what macOS expects." + secretAttrNote,
		Validators:          []validator.String{stringvalidator.LengthBetween(6, 6)},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Locks one or more computers or mobile devices. Every targeted device gets the same `pin`, `message` and `phone_number`." +
			"\n\nIf a device already has this command waiting, for example because it is offline, Jamf Pro keeps that command and queues no other, so a changed PIN or message does not reach the device." +
			batchNote + lockDevicePrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *LockDeviceAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *LockDeviceAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *LockDeviceAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data LockDeviceActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, lockDeviceCommand(data), "Lock Device")
}

func lockDeviceCommand(data LockDeviceActionModel) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeDeviceLock),
		DEVICELOCK: &pro.DeviceLockCommand{
			CommandType: pro.MDMCommandTypeDeviceLock,
			Message:     data.Message.ValueStringPointer(),
			PhoneNumber: data.PhoneNumber.ValueStringPointer(),
			Pin:         data.Pin.ValueStringPointer(),
		},
	}
}
