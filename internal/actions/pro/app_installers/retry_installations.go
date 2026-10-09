// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*RetryInstallationsAction)(nil)
var _ action.ActionWithConfigure = (*RetryInstallationsAction)(nil)

// RetryInstallationsAction retries the failed installations of one App Installer
// deployment, optionally narrowed to specific computers.
type RetryInstallationsAction struct {
	appInstallerAction
}

// RetryInstallationsActionModel is the action's configuration.
type RetryInstallationsActionModel struct {
	DeploymentID types.String         `tfsdk:"deployment_id"`
	ComputerIDs  types.List           `tfsdk:"computer_ids"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

// NewRetryInstallationsAction returns a new instance of RetryInstallationsAction.
func NewRetryInstallationsAction() action.Action {
	return &RetryInstallationsAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *RetryInstallationsAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_retry_app_installer_installations"
}

// Schema returns the action schema.
func (a *RetryInstallationsAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Retries the failed installations of one App Installer deployment, optionally for specific computers. " +
			"Jamf Pro reports nothing when there is nothing to retry, so this action raises a warning rather than failing the apply — see the note below. " +
			"To retry every failed App Installer installation in the tenant instead, use `jamfplatform_pro_retry_all_app_installer_installations`." +
			retryInstallationsPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"deployment_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "App Installer deployment ID whose failed installations should be retried. Must be a positive numeric string; Jamf Pro rejects anything else before it looks the deployment up.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"computer_ids": actionschema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Jamf Pro computer IDs to retry, each retried individually. Omit to retry every failed installation in the deployment; an empty list is not a valid way to say that.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
		},
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

// Configure wires the Jamf Pro client into the action.
func (a *RetryInstallationsAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// Invoke retries the deployment's failed installations, per computer when
// computer_ids is supplied and for the whole deployment otherwise.
//
// Per-computer retry is one request each: Jamf Pro exposes no batch form. An
// empty 404 on one computer is that computer having nothing to retry, which
// must not abandon the rest of the list.
func (a *RetryInstallationsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data RetryInstallationsActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.DeploymentID.ValueString()

	var computerIDs []string
	if helpers.IsConfiguredValue(data.ComputerIDs) {
		resp.Diagnostics.Append(data.ComputerIDs.ElementsAs(ctx, &computerIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if len(computerIDs) == 0 {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retrying failed installations for App Installer deployment %s", id)})
		err := a.client.RetryAppInstallerDeploymentInstallationsV1(ctx, id)
		if isNothingToRetry(err) {
			summary, detail := nothingToRetryDiagnostic(fmt.Sprintf("deployment %s", id))
			resp.Diagnostics.AddWarning(summary, detail)
			return
		}
		if err != nil {
			resp.Diagnostics.AddError(
				"Retry App Installer Installations Failed",
				fmt.Sprintf("Unable to retry the failed installations of App Installer deployment %s: %s", id, helpers.APIErrorDetail(err)),
			)
			return
		}
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retry requested for App Installer deployment %s", id)})
		return
	}

	var retried, skipped int
	for _, computerID := range computerIDs {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retrying App Installer deployment %s on computer %s", id, computerID)})
		err := a.client.RetryAppInstallerDeploymentComputerInstallationV1(ctx, id, computerID)
		if isNothingToRetry(err) {
			skipped++
			continue
		}
		if err != nil {
			resp.Diagnostics.AddError(
				"Retry App Installer Installations Failed",
				fmt.Sprintf("Unable to retry App Installer deployment %s on computer %s: %s. %d of %d computers were retried before this failure.", id, computerID, helpers.APIErrorDetail(err), retried, len(computerIDs)),
			)
			return
		}
		retried++
	}

	if skipped > 0 {
		summary, detail := nothingToRetryDiagnostic(fmt.Sprintf("%d of the %d computers requested on deployment %s", skipped, len(computerIDs), id))
		resp.Diagnostics.AddWarning(summary, detail)
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Retry requested for %d of %d computers on App Installer deployment %s", retried, len(computerIDs), id)})
}
