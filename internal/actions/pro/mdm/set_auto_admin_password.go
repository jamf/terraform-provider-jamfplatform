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

var _ action.Action = (*SetAutoAdminPasswordAction)(nil)
var _ action.ActionWithConfigure = (*SetAutoAdminPasswordAction)(nil)
var _ action.ActionWithConfigValidators = (*SetAutoAdminPasswordAction)(nil)

// SetAutoAdminPasswordAction sets the automatic administrator password on a Mac.
//
// The guid names an account on one computer, and commandData is shared across a
// batch, so the action takes one device. Jamf Pro refuses the command without
// both fields and names what is missing (wire-probed 2026-10-22).
type SetAutoAdminPasswordAction struct {
	mdmAction
}

type SetAutoAdminPasswordActionModel struct {
	ManagementID types.String         `tfsdk:"management_id"`
	SerialNumber types.String         `tfsdk:"serial_number"`
	Guid         types.String         `tfsdk:"guid"`
	Password     types.String         `tfsdk:"password"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

func NewSetAutoAdminPasswordAction() action.Action {
	return &SetAutoAdminPasswordAction{}
}

func (a *SetAutoAdminPasswordAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_set_auto_admin_password"
}

func (a *SetAutoAdminPasswordAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	attrs := targetAttributes("computer")
	attrs["guid"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "GUID of the local administrator account whose password you are setting.",
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	attrs["password"] = actionschema.StringAttribute{
		Required:            true,
		MarkdownDescription: "The new password. Jamf Pro never returns it." + secretAttrNote,
		Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
	}
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Sets the automatic administrator password on a Mac. If the computer already has this command waiting, Jamf Pro keeps that command and queues no other, so a password you change in the meantime does not reach the computer." + singleTargetNote + setAutoAdminPasswordPrivileges,
		Attributes:          attrs,
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *SetAutoAdminPasswordAction) ConfigValidators(ctx context.Context) []action.ConfigValidator {
	return deviceTargetConfigValidators()
}

func (a *SetAutoAdminPasswordAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *SetAutoAdminPasswordAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data SetAutoAdminPasswordActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementID, ok := a.resolveManagementID(ctx, resp, data.ManagementID, data.SerialNumber)
	if !ok {
		return
	}

	a.sendCommand(ctx, resp, managementID, setAutoAdminPasswordCommand(data), "Set Auto Admin Password")
}

func setAutoAdminPasswordCommand(data SetAutoAdminPasswordActionModel) *pro.MDMCommandRequestCommandData {
	return &pro.MDMCommandRequestCommandData{
		CommandType: string(pro.MDMCommandTypeSetAutoAdminPassword),
		SETAUTOADMINPASSWORD: &pro.SetAutoAdminPasswordCommand{
			CommandType: pro.MDMCommandTypeSetAutoAdminPassword,
			Guid:        data.Guid.ValueStringPointer(),
			Password:    data.Password.ValueStringPointer(),
		},
	}
}
