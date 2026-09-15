// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*FlushMdmCommandsAction)(nil)
var _ action.ActionWithConfigure = (*FlushMdmCommandsAction)(nil)

// FlushMdmCommandsAction cancels (flushes) pending or failed management commands for a device or group.
type FlushMdmCommandsAction struct {
	mdmAction
}

type FlushMdmCommandsActionModel struct {
	IDType types.String `tfsdk:"id_type"`
	ID     types.String `tfsdk:"id"`
	Status types.String `tfsdk:"status"`
}

func NewFlushMdmCommandsAction() action.Action {
	return &FlushMdmCommandsAction{}
}

func (a *FlushMdmCommandsAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_flush_mdm_commands"
}

func (a *FlushMdmCommandsAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Cancels (flushes) pending or failed management commands for a device or group." + flushMdmCommandsPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"id_type": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Type of target: `computers`, `computergroups`, `mobiledevices`, or `mobiledevicegroups`.",
				Validators: []validator.String{
					stringvalidator.OneOf("computers", "computergroups", "mobiledevices", "mobiledevicegroups"),
				},
			},
			"id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Jamf Pro ID of the device or group to flush commands for. Numeric.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^\d+$`),
						"must be a numeric Jamf Pro ID",
					),
				},
			},
			"status": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Which commands to flush: `Pending`, `Failed`, or `Pending+Failed`.",
				Validators: []validator.String{
					stringvalidator.OneOf(proclassic.CommandflushStatusValues()...),
				},
			},
		},
	}
}

func (a *FlushMdmCommandsAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *FlushMdmCommandsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClassicClient(resp) {
		return
	}

	var data FlushMdmCommandsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	idType := data.IDType.ValueString()
	id := data.ID.ValueString()
	status := data.Status.ValueString()

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Flushing %s commands for %s %s", status, idType, id)})

	if err := a.classic.DeleteCommandFlushByIDTypeIDStatus(ctx, idType, id, status); err != nil {
		resp.Diagnostics.AddError(
			"Flush Commands Failed",
			fmt.Sprintf("Unable to flush %s commands for %s %s: %s", status, idType, id, helpers.APIErrorDetail(err)),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Flushed %s commands for %s %s", status, idType, id)})
}
