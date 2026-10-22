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

var _ action.Action = (*DisableRemoteDesktopAction)(nil)
var _ action.ActionWithConfigure = (*DisableRemoteDesktopAction)(nil)
var _ action.ActionWithConfigValidators = (*DisableRemoteDesktopAction)(nil)

// DisableRemoteDesktopAction turns off Remote Desktop on computers.
type DisableRemoteDesktopAction struct {
	mdmAction
}

type DisableRemoteDesktopActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewDisableRemoteDesktopAction() action.Action {
	return &DisableRemoteDesktopAction{}
}

func (a *DisableRemoteDesktopAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_disable_remote_desktop"
}

func (a *DisableRemoteDesktopAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Turns off Remote Desktop (remote management) on one or more computers. Jamf Pro refuses the command for a mobile device. If a computer already has this command waiting, Jamf Pro queues no second copy." + batchNote + disableRemoteDesktopPrivileges,
		Attributes:          targetListAttributes("computer"),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *DisableRemoteDesktopAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *DisableRemoteDesktopAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *DisableRemoteDesktopAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data DisableRemoteDesktopActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, disableRemoteDesktopCommand(), "Disable Remote Desktop")
}

func disableRemoteDesktopCommand() *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType:          string(pro.MDMCommandTypeDisableRemoteDesktop),
		DISABLEREMOTEDESKTOP: &pro.DisableRemoteDesktopCommand{CommandType: pro.MDMCommandTypeDisableRemoteDesktop},
	}
}
