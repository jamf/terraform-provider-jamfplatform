// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/actionvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
)

var _ action.Action = (*EnableLostModeAction)(nil)
var _ action.ActionWithConfigure = (*EnableLostModeAction)(nil)
var _ action.ActionWithConfigValidators = (*EnableLostModeAction)(nil)

// hasVisibleText rejects a value made only of whitespace. Jamf Pro accepts a
// whitespace-only message (wire-probed 2026-10-22; only the empty string is
// refused), which would lock a device behind a screen that says nothing.
var hasVisibleText = stringvalidator.RegexMatches(regexp.MustCompile(`\S`), "must contain at least one visible character")

// EnableLostModeAction turns on Lost Mode on supervised mobile devices.
//
// message and phone_number map to lostModeMessage and lostModePhone, footnote to
// lostModeFootnote. Jamf Pro refuses the command unless at least one of message
// and phone_number is present: a footnote alone, nothing at all and an empty
// message each answer 400 "Either phone number or Lost Mode message must be
// entered" (wire-probed 2026-10-22). All three values are shared by every device
// in the batch, so the command batches safely.
type EnableLostModeAction struct {
	mdmAction
}

type EnableLostModeActionModel struct {
	ManagementIDs types.List           `tfsdk:"management_ids"`
	SerialNumbers types.List           `tfsdk:"serial_numbers"`
	Message       types.String         `tfsdk:"message"`
	PhoneNumber   types.String         `tfsdk:"phone_number"`
	Footnote      types.String         `tfsdk:"footnote"`
	Timeouts      actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewEnableLostModeAction() action.Action {
	return &EnableLostModeAction{}
}

func (a *EnableLostModeAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_enable_lost_mode"
}

func (a *EnableLostModeAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetListAttributes("mobile device")
	attrs["message"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Message shown on the Lock Screen. Set this, `phone_number`, or both.",
		Validators:          []validator.String{hasVisibleText},
	}
	attrs["phone_number"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Phone number shown on the Lock Screen. Set this, `message`, or both.",
		Validators:          []validator.String{hasVisibleText},
	}
	attrs["footnote"] = actionschema.StringAttribute{
		Optional:            true,
		MarkdownDescription: "Text shown at the bottom of the Lock Screen. Jamf Pro refuses a footnote on its own, so set `message` or `phone_number` too.",
		Validators:          []validator.String{hasVisibleText},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Turns on Lost Mode on one or more supervised iPhones and iPads. Each device locks and shows the message, phone number and footnote you set." +
			"\n\nIf a device already has this command waiting, for example because it is offline, Jamf Pro keeps that command and queues no other, so changes to the message or phone number do not reach the device." +
			batchNote + enableLostModePrivileges,
		Attributes: attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *EnableLostModeAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return append(deviceTargetListConfigValidators(),
		actionvalidator.AtLeastOneOf(
			path.MatchRoot("message"),
			path.MatchRoot("phone_number"),
		),
	)
}

func (a *EnableLostModeAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *EnableLostModeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data EnableLostModeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementIDs, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	a.sendCommandBatch(ctx, resp, managementIDs, enableLostModeCommand(data), "Enable Lost Mode")
}

// enableLostModeCommand builds the commandData for the configured text. An unset
// attribute stays a nil pointer, so the field is omitted from the request rather
// than sent empty, which Jamf Pro refuses for the message.
func enableLostModeCommand(data EnableLostModeActionModel) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeEnableLostMode),
		ENABLELOSTMODE: &pro.EnableLostModeCommand{
			CommandType:      pro.MDMCommandTypeEnableLostMode,
			LostModeMessage:  data.Message.ValueStringPointer(),
			LostModePhone:    data.PhoneNumber.ValueStringPointer(),
			LostModeFootnote: data.Footnote.ValueStringPointer(),
		},
	}
}
