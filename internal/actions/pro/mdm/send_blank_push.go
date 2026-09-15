// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*SendBlankPushAction)(nil)
var _ action.ActionWithConfigure = (*SendBlankPushAction)(nil)
var _ action.ActionWithConfigValidators = (*SendBlankPushAction)(nil)

// SendBlankPushAction sends a blank push notification to one or more devices.
type SendBlankPushAction struct {
	mdmAction
}

type SendBlankPushActionModel struct {
	ManagementIDs types.List `tfsdk:"management_ids"`
	SerialNumbers types.List `tfsdk:"serial_numbers"`
}

func NewSendBlankPushAction() action.Action {
	return &SendBlankPushAction{}
}

func (a *SendBlankPushAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_send_blank_push"
}

func (a *SendBlankPushAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Sends a blank push notification to one or more devices to prompt them to check in." + blankPushBatchNote + sendBlankPushPrivileges,
		Attributes:          targetListAttributes("device"),
	}
}

func (a *SendBlankPushAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetListConfigValidators()
}

func (a *SendBlankPushAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *SendBlankPushAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data SendBlankPushActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ids, ok := a.resolveManagementIDs(ctx, resp, data.ManagementIDs, data.SerialNumbers)
	if !ok {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Sending blank push to %d device(s)", len(ids))})

	out, err := a.client.SendMdmBlankPushV2(ctx, &pro.BlankPushRequest{ClientManagementIds: ids})
	if err != nil {
		resp.Diagnostics.AddError(
			"Blank Push Failed",
			fmt.Sprintf("Unable to send blank push: %s", helpers.APIErrorDetail(err)),
		)
		return
	}

	if out != nil && len(out.ErrorUuids) > 0 {
		resp.Diagnostics.AddWarning(
			"Blank Push Partially Failed",
			fmt.Sprintf("Some devices did not accept the blank push: %v", out.ErrorUuids),
		)
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Blank push accepted for %d device(s)", len(ids))})
}
