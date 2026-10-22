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

var _ action.Action = (*CancelEnhancedLogCollectionAction)(nil)
var _ action.ActionWithConfigure = (*CancelEnhancedLogCollectionAction)(nil)
var _ action.ActionWithConfigValidators = (*CancelEnhancedLogCollectionAction)(nil)

// CancelEnhancedLogCollectionAction stops enhanced log collection on devices.
type CancelEnhancedLogCollectionAction struct {
	mdmAction
}

type CancelEnhancedLogCollectionActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewCancelEnhancedLogCollectionAction() action.Action {
	return &CancelEnhancedLogCollectionAction{}
}

func (a *CancelEnhancedLogCollectionAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cancel_enhanced_log_collection"
}

func (a *CancelEnhancedLogCollectionAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Stops an AppleCare enhanced log collection session that is in progress on one or more devices. Each device reports the outcome in its management history. If a device already has this command waiting, Jamf Pro queues no second copy." + enhancedLogCollectionNote + batchNote + cancelEnhancedLogCollectionPrivileges,
		Attributes:          targetListAttributes("device"),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *CancelEnhancedLogCollectionAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *CancelEnhancedLogCollectionAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configureWithFloor(ctx, req, resp, minJamfProVersionEnhancedLogCollection, "jamfplatform_pro_cancel_enhanced_log_collection")
}

func (a *CancelEnhancedLogCollectionAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data CancelEnhancedLogCollectionActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, cancelEnhancedLogCollectionCommand(), "Cancel Enhanced Log Collection")
}

func cancelEnhancedLogCollectionCommand() *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType:                 string(pro.MDMCommandTypeCancelEnhancedLogCollection),
		CANCELENHANCEDLOGCOLLECTION: &pro.CancelEnhancedLogCollectionCommand{CommandType: pro.MDMCommandTypeCancelEnhancedLogCollection},
	}
}
