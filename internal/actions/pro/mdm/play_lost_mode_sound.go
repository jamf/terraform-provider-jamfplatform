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

var _ action.Action = (*PlayLostModeSoundAction)(nil)
var _ action.ActionWithConfigure = (*PlayLostModeSoundAction)(nil)
var _ action.ActionWithConfigValidators = (*PlayLostModeSoundAction)(nil)

// PlayLostModeSoundAction plays the Lost Mode sound on supervised mobile devices.
type PlayLostModeSoundAction struct {
	mdmAction
}

type PlayLostModeSoundActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewPlayLostModeSoundAction() action.Action {
	return &PlayLostModeSoundAction{}
}

func (a *PlayLostModeSoundAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_play_lost_mode_sound"
}

func (a *PlayLostModeSoundAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Plays a sound on one or more supervised iPhones and iPads in Lost Mode so someone can find them. If a device already has this command waiting, Jamf Pro queues no second copy." + batchNote + playLostModeSoundPrivileges,
		Attributes:          targetListAttributes("mobile device"),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *PlayLostModeSoundAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *PlayLostModeSoundAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *PlayLostModeSoundAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data PlayLostModeSoundActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, playLostModeSoundCommand(), "Play Lost Mode Sound")
}

func playLostModeSoundCommand() *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType:       string(pro.MDMCommandTypePlayLostModeSound),
		PLAYLOSTMODESOUND: &pro.PlayLostModeSoundCommand{CommandType: pro.MDMCommandTypePlayLostModeSound},
	}
}
