// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package deviceactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	daSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*EraseAction)(nil)
var _ action.ActionWithConfigure = (*EraseAction)(nil)

// EraseAction invokes a Jamf Platform erase command for a device.
type EraseAction struct {
	deviceAction
}

type EraseActionModel struct {
	DeviceID               types.String `tfsdk:"device_id"`
	SerialNumber           types.String `tfsdk:"serial_number"`
	PreserveDataPlan       types.Bool   `tfsdk:"preserve_data_plan"`
	DisallowProximitySetup types.Bool   `tfsdk:"disallow_proximity_setup"`
	ClearActivationLock    types.Bool   `tfsdk:"clear_activation_lock"`
	ReturnToService        types.Bool   `tfsdk:"return_to_service"`
	Pin                    types.String `tfsdk:"pin"`
}

func NewEraseAction() action.Action {
	return &EraseAction{}
}

func (a *EraseAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_erase"
}

func (a *EraseAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := deviceTargetAttributes()
	attrs["preserve_data_plan"] = actionschema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Preserve the data plan on an iPhone or iPad with eSIM functionality, if one exists. Applies to mobile devices only.",
		Validators: []validator.Bool{
			boolvalidator.ConflictsWith(path.MatchRoot("pin")),
		},
	}
	attrs["disallow_proximity_setup"] = actionschema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Disable Proximity Setup on the next reboot and skip the pane in Setup Assistant. Applies to mobile devices only.",
		Validators: []validator.Bool{
			boolvalidator.ConflictsWith(path.MatchRoot("pin")),
		},
	}
	attrs["clear_activation_lock"] = actionschema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "Clear the activation lock on the device.",
	}
	attrs["return_to_service"] = actionschema.BoolAttribute{
		Optional:            true,
		MarkdownDescription: "The device will be returned to service after the erase is complete. Applies to mobile devices only.",
		Validators: []validator.Bool{
			boolvalidator.ConflictsWith(path.MatchRoot("pin")),
		},
	}
	// Deliberately neither WriteOnly nor Sensitive — see secretAttrNote.
	attrs["pin"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "The six-character PIN for Find My. Applies to computers only." + secretAttrNote,
		Validators: []validator.String{
			stringvalidator.LengthBetween(6, 6),
			stringvalidator.ConflictsWith(
				path.MatchRoot("preserve_data_plan"),
				path.MatchRoot("disallow_proximity_setup"),
				path.MatchRoot("return_to_service"),
			),
		},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Requests that a device erase its content and settings. Requires **Device Management Actions API** access." + eraseDevicePrivileges,
		Attributes:          attrs,
	}
}

func (a *EraseAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetConfigValidators()
}

func (a *EraseAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *EraseAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data EraseActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deviceID, ok := a.resolveDeviceIdentifier(ctx, resp, data.DeviceID, data.SerialNumber)
	if !ok {
		return
	}
	request := buildEraseRequestPayload(data)

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting erase for device %s", deviceID)})

	commands, err := a.actions.EraseDevice(ctx, deviceID, request)
	if err != nil {
		resp.Diagnostics.AddError(
			"Erase Device Failed",
			fmt.Sprintf("Unable to erase device %s: %s", deviceID, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Erase request accepted for device %s (%d command(s))", deviceID, len(commands))})
}

func buildEraseRequestPayload(data EraseActionModel) *daSDK.EraseDeviceRequest {
	req := &daSDK.EraseDeviceRequest{}
	var hasOptions bool

	if v := data.PreserveDataPlan.ValueBoolPointer(); v != nil {
		req.PreserveDataPlan = v
		hasOptions = true
	}
	if v := data.DisallowProximitySetup.ValueBoolPointer(); v != nil {
		req.DisallowProximitySetup = v
		hasOptions = true
	}
	if v := data.ClearActivationLock.ValueBoolPointer(); v != nil {
		req.ClearActivationLock = v
		hasOptions = true
	}
	if v := data.ReturnToService.ValueBoolPointer(); v != nil {
		req.ReturnToService = v
		hasOptions = true
	}
	if v := data.Pin.ValueStringPointer(); v != nil {
		req.Pin = v
		hasOptions = true
	}

	if !hasOptions {
		return nil
	}

	return req
}
