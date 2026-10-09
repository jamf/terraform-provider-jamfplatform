// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.SyncJamfProtectPlansV1   (POST /v1/jamf-protect/plans/sync — fire-and-forget 204)
//
// Status: current. Last reviewed 2026-07-03.
//
// Triggers an on-demand sync of the Jamf Protect plans catalog into Jamf Pro
// (Settings → Jamf apps → Jamf Protect → Sync Plans). The
// jamfplatform_pro_jamf_protect resource already fires this sync as a side
// effect on every register / settings write; this action exposes the same
// operation standalone so operators can re-sync without re-applying the
// registration. Requires the tenant to be registered with a Jamf Protect
// instance. No input, no state.

package jamfprotectactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	actionschema "github.com/hashicorp/terraform-plugin-framework/action/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/actiontimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ action.Action = (*SyncPlansAction)(nil)
var _ action.ActionWithConfigure = (*SyncPlansAction)(nil)

// SyncPlansAction triggers an on-demand Jamf Protect plans sync.
type SyncPlansAction struct {
	jamfProtectAction
}

// NewSyncPlansAction constructs the action.
func NewSyncPlansAction() action.Action {
	return &SyncPlansAction{}
}

func (a *SyncPlansAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_protect_plans_sync"
}

func (a *SyncPlansAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = actionschema.Schema{
		MarkdownDescription: "Triggers an on-demand sync of the Jamf Protect plans catalog into Jamf Pro (Settings → Jamf apps → Jamf Protect → Sync Plans). " +
			"The `jamfplatform_pro_jamf_protect` resource already runs this sync automatically whenever the registration is created or updated; use this action to re-sync on demand without re-applying the registration. Requires the tenant to be registered with a Jamf Protect instance. Takes no input." +
			syncPlansPrivileges,
		Attributes: map[string]actionschema.Attribute{},
	}
	resp.Schema.Attributes = actiontimeouts.Add(ctx, resp.Schema.Attributes)
}

func (a *SyncPlansAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.configure(ctx, req, resp)
}

func (a *SyncPlansAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	ctx, cancel := actiontimeouts.Bound(ctx, req.Config, &resp.Diagnostics, actiontimeouts.DefaultInvoke)
	defer cancel()
	if resp.Diagnostics.HasError() {
		return
	}

	if !a.ensureClient(resp) {
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: "Requesting Jamf Protect plans sync"})

	if err := a.client.SyncJamfProtectPlansV1(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Jamf Protect Plans Sync Failed",
			"Unable to trigger the Jamf Protect plans sync. Ensure the tenant is registered with a Jamf Protect instance "+
				"(jamfplatform_pro_jamf_protect). Original error: "+helpers.APIErrorDetail(err),
		)
		return
	}

	resp.SendProgress(action.InvokeProgressEvent{Message: "Jamf Protect plans sync requested"})
}
