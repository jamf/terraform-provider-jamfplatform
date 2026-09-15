// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdmactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*RenewMdmProfileAction)(nil)
var _ action.ActionWithConfigure = (*RenewMdmProfileAction)(nil)

// RenewMdmProfileAction renews the MDM enrollment profile on one or more mobile devices.
type RenewMdmProfileAction struct {
	mdmAction
}

type RenewMdmProfileActionModel struct {
	Udids types.List `tfsdk:"udids"`
}

func NewRenewMdmProfileAction() action.Action {
	return &RenewMdmProfileAction{}
}

func (a *RenewMdmProfileAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_renew_mdm_profile"
}

func (a *RenewMdmProfileAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Renews the MDM enrollment profile on one or more mobile devices." + renewMdmProfilePrivileges,
		Attributes: map[string]actionschema.Attribute{
			"udids": actionschema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Hardware UDIDs of the mobile devices. Source these from the `jamfplatform_device` data source `hardware_udid` attribute.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
}

func (a *RenewMdmProfileAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *RenewMdmProfileAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data RenewMdmProfileActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var udids []string
	resp.Diagnostics.Append(data.Udids.ElementsAs(ctx, &udids, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Renewing MDM profile on %d mobile device(s)", len(udids))})

	out, err := a.client.RenewMdmProfileV1(ctx, &pro.Udids{Udids: &udids})
	if err != nil {
		resp.Diagnostics.AddError(
			"Renew MDM Profile Failed",
			fmt.Sprintf("Unable to renew the MDM profile: %s", helpers.APIErrorDetail(err)),
		)
		return
	}

	if out != nil && out.UdidsNotProcessed != nil && out.UdidsNotProcessed.Udids != nil && len(*out.UdidsNotProcessed.Udids) > 0 {
		resp.Diagnostics.AddWarning(
			"Renew MDM Profile Partially Failed",
			fmt.Sprintf("Some mobile devices were not processed: %v", *out.UdidsNotProcessed.Udids),
		)
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: "Renew MDM profile request accepted"})
}
