// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	securitycloud.TriggerUemConnectorSyncV1
//	securitycloud.ListUemConnectorsV1 (resolving the tenant's only integration)
//
// Status: current. Last reviewed 2026-08-28.
package uemconnectactions

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var (
	_ action.Action              = (*SynchronizeAction)(nil)
	_ action.ActionWithConfigure = (*SynchronizeAction)(nil)
)

// SynchronizeAction starts a UEM Connect sync run.
type SynchronizeAction struct {
	uemConnectAction
}

// SynchronizeActionModel is the action's configuration.
type SynchronizeActionModel struct {
	UEMConnectID types.String         `tfsdk:"uem_connect_id"`
	Timeouts     actiontimeouts.Value `tfsdk:"timeouts"`
}

// NewSynchronizeAction returns a new instance of SynchronizeAction.
func NewSynchronizeAction() action.Action {
	return &SynchronizeAction{}
}

// Metadata sets the action type name for the Terraform provider.
func (a *SynchronizeAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_uem_connect_synchronize"
}

// Schema returns the action schema.
func (a *SynchronizeAction) Schema(ctx context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "**\"Synchronize\"** on the Jamf Security Cloud UEM Connect **Actions** menu. Starts a sync " +
			"run immediately, rather than waiting for the next scheduled one.\n\n" +
			"Jamf Security Cloud accepts the request and runs the sync in the background, so this returns as soon " +
			"as the run has started and does not wait for it or report what it did. Read the outcome from the " +
			"`latest_sync` attribute of the `jamfplatform_security_cloud_uem_connect` data source, or from the " +
			"sync logs in the admin UI.\n\n" +
			"Synchronizing a disabled integration is refused." +
			synchronizePrivileges,
		Attributes: map[string]actionschema.Attribute{
			"uem_connect_id": actionschema.StringAttribute{
				MarkdownDescription: "The UEM Connect integration to synchronize. A tenant holds at most one, so " +
					"this can be omitted and the integration found automatically. Set it to the `id` of your " +
					"`jamfplatform_security_cloud_uem_connect` resource to make the synchronize wait until that " +
					"integration exists.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

// Configure wires the Jamf Security Cloud client into the action.
func (a *SynchronizeAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

// Invoke starts the sync run.
//
// The request is accepted rather than completed, so there is nothing to poll and
// nothing to report beyond "started". A sync of a settled tenant finishes in about
// a second, but that is not a contract and waiting on it would turn a fire-once
// action into an operation that can time out for reasons the caller cannot act on.
//
// Repeated invocations are safe: two triggers back to back are both accepted
// (wire-verified 2026-08-28).
func (a *SynchronizeAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	var data SynchronizeActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := a.resolveIntegrationID(ctx, data.UEMConnectID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Starting a UEM Connect sync for integration %s", id),
	})

	if err := a.client.TriggerUemConnectorSyncV1(ctx, id); err != nil {
		if isNotFound(err) {
			resp.Diagnostics.AddError(
				"UEM Connect integration not found",
				fmt.Sprintf("Jamf Security Cloud has no UEM Connect integration %s, so there is nothing to "+
					"synchronize. Check `uem_connect_id`, or omit it to use the tenant's own integration.", id),
			)
			return
		}
		if !appendInvokeDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError(
				"UEM Connect Synchronize Failed",
				fmt.Sprintf("Unable to start a sync for UEM Connect integration %s: %s", id, helpers.APIErrorDetail(err)),
			)
		}
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{
		Message: fmt.Sprintf("Sync started for UEM Connect integration %s; it runs in the background", id),
	})
}
