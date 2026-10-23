// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var _ action.Action = (*DisableLostModeAction)(nil)
var _ action.ActionWithConfigure = (*DisableLostModeAction)(nil)
var _ action.ActionWithConfigValidators = (*DisableLostModeAction)(nil)

// DisableLostModeAction turns off Lost Mode on supervised mobile devices.
type DisableLostModeAction struct {
	mdmAction
}

type DisableLostModeActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewDisableLostModeAction() action.Action {
	return &DisableLostModeAction{}
}

func (a *DisableLostModeAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_disable_lost_mode"
}

func (a *DisableLostModeAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Turns off Lost Mode on one or more supervised iPhones and iPads. If a device already has this command waiting, Jamf Pro queues no second copy." + batchNote + disableLostModePrivileges,
		Attributes:          targetListAttributes("mobile device"),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *DisableLostModeAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *DisableLostModeAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *DisableLostModeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data DisableLostModeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, disableLostModeCommand(), "Disable Lost Mode")
}

func disableLostModeCommand() *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType:     string(pro.MDMCommandTypeDisableLostMode),
		DISABLELOSTMODE: &pro.DisableLostModeCommand{CommandType: pro.MDMCommandTypeDisableLostMode},
	}
}
