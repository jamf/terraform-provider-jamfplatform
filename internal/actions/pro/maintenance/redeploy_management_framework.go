// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package maintenanceactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*RedeployManagementFrameworkAction)(nil)
var _ action.ActionWithConfigure = (*RedeployManagementFrameworkAction)(nil)
var _ action.ActionWithConfigValidators = (*RedeployManagementFrameworkAction)(nil)

// RedeployManagementFrameworkAction redeploys the Jamf management framework to a computer.
type RedeployManagementFrameworkAction struct {
	maintenanceAction
}

type RedeployManagementFrameworkActionModel struct {
	ManagementID types.String         `tfsdk:"management_id"`
	SerialNumber types.String         `tfsdk:"serial_number"`
	UDID         types.String         `tfsdk:"udid"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewRedeployManagementFrameworkAction() action.Action {
	return &RedeployManagementFrameworkAction{}
}

func (a *RedeployManagementFrameworkAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_redeploy_management_framework"
}

func (a *RedeployManagementFrameworkAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Redeploys the Jamf management framework (binary and MDM management profile) to a computer." + redeployManagementFrameworkPrivileges,
		Attributes:          computerTargetAttributes(),
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *RedeployManagementFrameworkAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return computerTargetConfigValidators()
}

func (a *RedeployManagementFrameworkAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *RedeployManagementFrameworkAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data RedeployManagementFrameworkActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	computerID, ok := a.resolveComputerID(ctx, resp, data.ManagementID, data.SerialNumber, data.UDID)
	if !ok {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Requesting management framework redeploy for computer %s", computerID)})

	out, err := a.client.RedeployJamfManagementFrameworkV1(ctx, computerID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Redeploy Management Framework Failed",
			fmt.Sprintf("Unable to redeploy the management framework to computer %s: %s", computerID, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Redeploy requested for computer %s (command %s)", computerID, out.CommandUUID)})
}
