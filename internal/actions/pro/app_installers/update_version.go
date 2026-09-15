// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*UpdateVersionAction)(nil)
var _ action.ActionWithConfigure = (*UpdateVersionAction)(nil)

// UpdateVersionAction moves a MANUAL App Installer deployment to a newer version
// of its title.
//
// This is an action rather than a writable attribute on
// jamfplatform_pro_app_installer because the operation is forward-only. Jamf Pro
// refuses a version that is not newer than the deployment's current one — the
// same version included — so an attribute over it could be advanced but never
// reverted, and a configuration naming an older version would fail every apply
// with no way back other than replacing the deployment.
type UpdateVersionAction struct {
	appInstallerAction
}

// UpdateVersionActionModel is the action's configuration.
type UpdateVersionActionModel struct {
	DeploymentID types.String `tfsdk:"deployment_id"`
	Version      types.String `tfsdk:"version"`
}

// NewUpdateVersionAction returns a new instance of UpdateVersionAction.
func NewUpdateVersionAction() action.Action {
	return &UpdateVersionAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *UpdateVersionAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_update_app_installer_version"
}

// Schema returns the action schema.
func (a *UpdateVersionAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Moves an App Installer deployment whose `update_behavior` is `MANUAL` on to a newer version of its title. " +
			"The operation is forward-only: Jamf Pro refuses any version that is not newer than the deployment's current `selected_version`, including that version itself. " +
			"That is why this is an action and not a writable attribute — a version cannot be rolled back, only replaced with the deployment. " +
			"A deployment on `AUTOMATIC` tracks the latest version already and needs no action." +
			updateVersionPrivileges,
		Attributes: map[string]actionschema.Attribute{
			"deployment_id": actionschema.StringAttribute{
				Required:            true,
				MarkdownDescription: "App Installer deployment ID to move. Must be a positive numeric string; Jamf Pro rejects anything else before it looks the deployment up.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"version": actionschema.StringAttribute{
				Required: true,
				MarkdownDescription: "Version of the title to move to, which must be newer than the deployment's current version. " +
					"Read the `versions` attribute of the `jamfplatform_pro_app_installer_title` data source for the versions Jamf Pro publishes.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the action.
func (a *UpdateVersionAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// Invoke moves the deployment on to the requested version.
//
// The request carries only the version; Jamf Pro decides whether the move is
// allowed. A forward-only refusal is a 400 whose description already names both
// the current and the requested version, so that error is surfaced as it stands
// and only the two preconditions are added. A deployment that does not exist is
// a 404 with an empty body — wire-verified 2026-09-03 — where those
// preconditions would be the whole message and none of them the cause, so it
// gets its own wording. helpers.IsNotFoundError is the right classifier here,
// unlike in the retry pair: it also matches the 400 INVALID_ID Jamf Pro raises
// for a malformed deployment ID, and a malformed ID and a missing one both point
// the operator at the same attribute.
func (a *UpdateVersionAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	if !a.ensureClient(resp) {
		return
	}

	var data UpdateVersionActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := data.DeploymentID.ValueString()
	version := data.Version.ValueString()

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("Moving App Installer deployment %s to version %s", id, version)})

	if err := a.client.UpdateAppInstallerDeploymentVersionV1(ctx, id, &pro.AppTitleVersion{Version: &version}); err != nil {
		detail := fmt.Sprintf("Unable to move App Installer deployment %s to version %s: %s. "+
			"The version must be newer than the deployment's current one, and the deployment's update_behavior must be MANUAL.", id, version, err)
		if helpers.IsNotFoundError(err) {
			detail = fmt.Sprintf("Jamf Pro found no App Installer deployment %s, so it was not moved to version %s. "+
				"Check the deployment ID. Jamf Pro reported: %s", id, version, err)
		}
		resp.Diagnostics.AddError("Update App Installer Version Failed", detail)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("App Installer deployment %s moved to version %s", id, version)})
}
