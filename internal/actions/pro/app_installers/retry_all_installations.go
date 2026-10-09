// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*RetryAllInstallationsAction)(nil)
var _ action.ActionWithConfigure = (*RetryAllInstallationsAction)(nil)

// RetryAllInstallationsAction retries every failed App Installer installation in
// the tenant.
type RetryAllInstallationsAction struct {
	appInstallerAction
}

// NewRetryAllInstallationsAction returns a new instance of RetryAllInstallationsAction.
func NewRetryAllInstallationsAction() action.Action {
	return &RetryAllInstallationsAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *RetryAllInstallationsAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_retry_all_app_installer_installations"
}

// Schema returns the action schema. The action takes no arguments: its scope is
// the whole tenant, which is why it is a separate action from
// jamfplatform_pro_retry_app_installer_installations rather than that action with
// its deployment ID left out.
func (a *RetryAllInstallationsAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Retries every failed App Installer installation in the tenant, across all deployments. " +
			"Takes no arguments — to retry one deployment, use `jamfplatform_pro_retry_app_installer_installations`. " +
			"Jamf Pro reports nothing when there is nothing to retry, so this action raises a warning rather than failing the apply. " +
			"A retry re-attempts an install the deployment already intends, so it deploys nothing new." +
			retryAllInstallationsPrivileges,
		Attributes: map[string]actionschema.Attribute{},
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

// Configure wires the Jamf Pro client into the action.
func (a *RetryAllInstallationsAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// Invoke retries every failed installation in the tenant. An empty 404 means no
// deployment had a failed installation, which is a warning; every other failure
// fails the apply.
func (a *RetryAllInstallationsAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: "Retrying every failed App Installer installation in the tenant"})

	err := a.client.RetryAppInstallerInstallationsV1(ctx)
	if isNothingToRetry(err) {
		resp.Diagnostics.AddWarning(
			"No Failed Installations Retried",
			"Jamf Pro answered 404 with an empty error body, which for this endpoint means no App Installer deployment has a failed installation to retry. Nothing was retried.",
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Retry App Installer Installations Failed",
			"Unable to retry the tenant's failed App Installer installations: "+helpers.APIErrorDetail(err),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: "Retry requested for every failed App Installer installation"})
}
