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

var _ action.Action = (*RestartAction)(nil)
var _ action.ActionWithConfigure = (*RestartAction)(nil)
var _ action.ActionWithConfigValidators = (*RestartAction)(nil)

// RestartAction invokes a Jamf Platform restart command for a device.
type RestartAction struct {
	deviceAction
}

// RestartActionModel represents the action config schema.
type RestartActionModel struct {
	DeviceID     types.String         `tfsdk:"device_id"`
	SerialNumber types.String         `tfsdk:"serial_number"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

// NewRestartAction constructs the action.
func NewRestartAction() action.Action {
	return &RestartAction{}
}

func (a *RestartAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_restart"
}

func (a *RestartAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Requests that a device restart. Requires **Device Management Actions API** access." + restartDevicePrivileges,
		Attributes:          deviceTargetAttributes(),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *RestartAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetConfigValidators()
}

func (a *RestartAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *RestartAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data RestartActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deviceID, ok := a.resolveDeviceIdentifier(ctx, resp, data.DeviceID, data.SerialNumber)
	if !ok {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting restart for device %s", deviceID)})

	if _, err := a.actions.RestartDevice(ctx, deviceID); err != nil {
		resp.Diagnostics.AddError(
			"Restart Device Failed",
			fmt.Sprintf("Unable to restart device %s: %s", deviceID, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Restart request accepted for device %s", deviceID)})
}
