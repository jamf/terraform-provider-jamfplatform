// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patchactions

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

var _ action.Action = (*RetryPatchPolicyLogsAction)(nil)
var _ action.ActionWithConfigure = (*RetryPatchPolicyLogsAction)(nil)

// RetryPatchPolicyLogsAction retries failed patch policy installation attempts.
type RetryPatchPolicyLogsAction struct {
	patchAction
}

type RetryPatchPolicyLogsActionModel struct {
	PatchPolicyID types.String `tfsdk:"patch_policy_id"`
	DeviceIDs     types.List   `tfsdk:"device_ids"`
}

func NewRetryPatchPolicyLogsAction() action.Action {
	return &RetryPatchPolicyLogsAction{}
}

func (a *RetryPatchPolicyLogsAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_retry_patch_policy_logs"
}

func (a *RetryPatchPolicyLogsAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Retries failed patch policy installation attempts, optionally for specific computers." + retryPatchPolicyLogsPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"patch_policy_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Jamf Pro patch policy ID.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"device_ids": actionschema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Jamf Pro computer IDs to retry. Omit to retry all failed devices; an empty list is not a valid way to say that.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
}

func (a *RetryPatchPolicyLogsAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *RetryPatchPolicyLogsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data RetryPatchPolicyLogsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.PatchPolicyID.ValueString()

	var ids []string
	if !data.DeviceIDs.IsNull() && !data.DeviceIDs.IsUnknown() {
		resp.Diagnostics.Append(data.DeviceIDs.ElementsAs(ctx, &ids, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if len(ids) > 0 {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retrying patch policy %s for %d device(s)", id, len(ids))})
		if err := a.client.RetryPatchPolicyLogsV2(ctx, id, &pro.PatchPolicyLogRetry{DeviceIds: &ids}); err != nil {
			resp.Diagnostics.AddError(
				"Retry Patch Policy Logs Failed",
				fmt.Sprintf("Unable to retry patch policy %s for the specified devices: %s", id, helpers.APIErrorDetail(err)),
			)
			return
		}
	} else {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retrying patch policy %s for all failed devices", id)})
		if err := a.client.RetryAllPatchPolicyLogsV2(ctx, id); err != nil {
			resp.Diagnostics.AddError(
				"Retry Patch Policy Logs Failed",
				fmt.Sprintf("Unable to retry patch policy %s: %s", id, helpers.APIErrorDetail(err)),
			)
			return
		}
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retry requested for patch policy %s", id)})
}
