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

var _ action.Action = (*ClearRestrictionsPasswordAction)(nil)
var _ action.ActionWithConfigure = (*ClearRestrictionsPasswordAction)(nil)
var _ action.ActionWithConfigValidators = (*ClearRestrictionsPasswordAction)(nil)

// ClearRestrictionsPasswordAction clears the Screen Time passcode on mobile devices.
type ClearRestrictionsPasswordAction struct {
	mdmAction
}

type ClearRestrictionsPasswordActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewClearRestrictionsPasswordAction() action.Action {
	return &ClearRestrictionsPasswordAction{}
}

func (a *ClearRestrictionsPasswordAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_clear_restrictions_password"
}

func (a *ClearRestrictionsPasswordAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Clears the Screen Time (restrictions) passcode on one or more iPhones and iPads. If a mobile device already has this command waiting, Jamf Pro queues no second copy." + batchNote + clearRestrictionsPasswordPrivileges,
		Attributes:          targetListAttributes("mobile device"),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *ClearRestrictionsPasswordAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *ClearRestrictionsPasswordAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *ClearRestrictionsPasswordAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data ClearRestrictionsPasswordActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, clearRestrictionsPasswordCommand(), "Clear Restrictions Password")
}

func clearRestrictionsPasswordCommand() *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType:               string(pro.MDMCommandTypeClearRestrictionsPassword),
		CLEARRESTRICTIONSPASSWORD: &pro.ClearRestrictionsPasswordCommand{CommandType: pro.MDMCommandTypeClearRestrictionsPassword},
	}
}
