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

var _ action.Action = (*TriggerEnhancedLogCollectionAction)(nil)
var _ action.ActionWithConfigure = (*TriggerEnhancedLogCollectionAction)(nil)
var _ action.ActionWithConfigValidators = (*TriggerEnhancedLogCollectionAction)(nil)

// enhancedLogCollectionNote is shared by the trigger and cancel actions. Jamf Pro
// queues the command for any targeted device, but a device on an OS too old for
// the feature does nothing with it and reports no error, so a user needs to know
// where to look.
const enhancedLogCollectionNote = "\n\n" +
	"Requires Jamf Pro 11.30 or later. Enhanced log collection is an Apple feature for iOS, iPadOS, tvOS and macOS 27.0 and later. " +
	"Jamf Pro queues the command for any targeted device, but a device on an earlier OS cannot act on it."

// triggerTokenScopeNote records what Apple leaves open: whether one token is
// valid for more than one device.
const triggerTokenScopeNote = "\n\n" +
	"The action sends the same `apple_care_token` to every targeted device. Apple does not say whether one token is valid for more than one device. " +
	"If AppleCare issued the token for a single device, target only that device. The others reject the command on the device, so the invocation still succeeds and the failures appear in each device's management history."

// TriggerEnhancedLogCollectionAction starts an AppleCare enhanced log collection
// session. Jamf Pro refuses a missing or empty token and does not check the one
// it is given (wire-probed 2026-10-22).
type TriggerEnhancedLogCollectionAction struct {
	mdmAction
}

type TriggerEnhancedLogCollectionActionModel struct {
	ManagementIDs  types.List           `tfsdk:"management_ids"`
	SerialNumbers  types.List           `tfsdk:"serial_numbers"`
	AppleCareToken types.String         `tfsdk:"apple_care_token"`
	Timeouts       actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewTriggerEnhancedLogCollectionAction() action.Action {
	return &TriggerEnhancedLogCollectionAction{}
}

func (a *TriggerEnhancedLogCollectionAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_trigger_enhanced_log_collection"
}

func (a *TriggerEnhancedLogCollectionAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetListAttributes("device")
	attrs["apple_care_token"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The AppleCare token that authorises the session, issued by Apple with an AppleCare ticket. An interactive token makes the device ask its user for consent. A non-interactive token shows a notification and uploads in the background." + secretAttrNote,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Starts an AppleCare enhanced log collection session on one or more devices. The devices collect diagnostic logs and upload them to Apple for an AppleCare escalation. If a device already has this command waiting, Jamf Pro queues no second copy." +
			enhancedLogCollectionNote + triggerTokenScopeNote + batchNote + triggerEnhancedLogCollectionPrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *TriggerEnhancedLogCollectionAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *TriggerEnhancedLogCollectionAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configureWithFloor(ctx, req, resp, minJamfProVersionEnhancedLogCollection, "jamfplatform_pro_trigger_enhanced_log_collection")
}

func (a *TriggerEnhancedLogCollectionAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data TriggerEnhancedLogCollectionActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, triggerEnhancedLogCollectionCommand(data.AppleCareToken.ValueString()), "Trigger Enhanced Log Collection")
}

func triggerEnhancedLogCollectionCommand(token string) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeTriggerEnhancedLogCollection),
		TRIGGERENHANCEDLOGCOLLECTION: &pro.TriggerEnhancedLogCollectionCommand{
			CommandType:    pro.MDMCommandTypeTriggerEnhancedLogCollection,
			AppleCareToken: token,
		},
	}
}
