// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package deviceactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*UnmanageAction)(nil)
var _ action.ActionWithConfigure = (*UnmanageAction)(nil)
var _ action.ActionWithConfigValidators = (*UnmanageAction)(nil)

// UnmanageAction invokes a Jamf Platform unmanage command for a device.
type UnmanageAction struct {
	deviceAction
}

type UnmanageActionModel struct {
	DeviceID     types.String         `tfsdk:"device_id"`
	SerialNumber types.String         `tfsdk:"serial_number"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewUnmanageAction() action.Action {
	return &UnmanageAction{}
}

func (a *UnmanageAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_unmanage"
}

func (a *UnmanageAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Removes remote management from a device. Requires **Device Management Actions API** access." + unmanageDevicePrivileges,
		Attributes:          deviceTargetAttributes(),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *UnmanageAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetConfigValidators()
}

func (a *UnmanageAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *UnmanageAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data UnmanageActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deviceID, ok := a.resolveDeviceIdentifier(ctx, resp, data.DeviceID, data.SerialNumber)
	if !ok {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting unmanage for device %s", deviceID)})

	if _, err := a.actions.UnmanageDevice(ctx, deviceID); err != nil {
		resp.Diagnostics.AddError(
			"Unmanage Device Failed",
			fmt.Sprintf("Unable to unmanage device %s: %s", deviceID, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Unmanage request accepted for device %s", deviceID)})
}
