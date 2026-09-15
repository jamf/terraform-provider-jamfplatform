// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package jamfprotectactions implements the fire-once Jamf Pro Jamf Protect
// actions: jamfplatform_pro_jamf_protect_plans_sync (trigger an on-demand sync
// of the Jamf Protect plans catalog into Jamf Pro).
package jamfprotectactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty: the Jamf Protect integration endpoints are a
// long-standing part of the Pro API, present at the provider's overall floor
// (matches the jamfplatform_pro_jamf_protect resource).
const minJamfProVersion = ""

// jamfProtectAction shares Configure logic across the Jamf Protect actions.
type jamfProtectAction struct {
	client *pro.Client
}

// configure binds the provider-supplied Jamf Pro client to the action via the
// shared providerdata.ConfigurePro helper.
func (a *jamfProtectAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamf_protect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.client = client
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *jamfProtectAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Pro client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}
