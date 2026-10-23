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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*ClearPasscodeAction)(nil)
var _ action.ActionWithConfigure = (*ClearPasscodeAction)(nil)
var _ action.ActionWithConfigValidators = (*ClearPasscodeAction)(nil)

// ClearPasscodeAction clears the passcode on an iPhone or iPad.
//
// The unlock token belongs to one device, and commandData is shared across a
// batch, so the action takes one device. Jamf Pro refuses the command without a
// non-empty token and does not check the token it is given (a made-up one was
// accepted, 2026-10-22), so a wrong token fails on the device, not here.
type ClearPasscodeAction struct {
	mdmAction
}

type ClearPasscodeActionModel struct {
	ManagementID types.String         `tfsdk:"management_id"`
	SerialNumber types.String         `tfsdk:"serial_number"`
	UnlockToken  types.String         `tfsdk:"unlock_token"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewClearPasscodeAction() action.Action {
	return &ClearPasscodeAction{}
}

func (a *ClearPasscodeAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_clear_passcode"
}

func (a *ClearPasscodeAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetAttributes("iPhone or iPad")
	attrs["unlock_token"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Unlock token for the device. When you leave this out, the action reads it from the device's inventory in Jamf Pro." + secretAttrNote,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Clears the passcode on an iPhone or iPad. If the device already has this command waiting, Jamf Pro queues no second copy." + singleTargetNote + clearPasscodePrivileges,
		Attributes:          attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *ClearPasscodeAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetConfigValidators()
}

func (a *ClearPasscodeAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *ClearPasscodeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data ClearPasscodeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementID, ok := a.resolveManagementID(ctx, resp, data.ManagementID, data.SerialNumber)
	if !ok {
		return
	}

	token := data.UnlockToken.ValueString()
	if !helpers.IsConfiguredValue(data.UnlockToken) {
		resolved, ok := a.resolveUnlockToken(ctx, resp, managementID)
		if !ok {
			return
		}
		token = resolved
	}

	a.sendCommand(ctx, resp, managementID, clearPasscodeCommand(token), "Clear Passcode")
}

func clearPasscodeCommand(token string) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType:   string(pro.MDMCommandTypeClearPasscode),
		CLEARPASSCODE: &pro.ClearPasscodeCommand{CommandType: pro.MDMCommandTypeClearPasscode, UnlockToken: token},
	}
}
