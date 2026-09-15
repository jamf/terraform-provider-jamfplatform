// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package managed_software_updates implements the fire-once Jamf Pro Managed Software
// Updates actions: jamfplatform_pro_managed_software_update_plan (submit a group update
// plan) and jamfplatform_pro_managed_software_update_abandon_feature_toggle (break-glass
// force-stop of a stuck feature-toggle process).
package managed_software_updates

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by these actions.
// Empty: the Managed Software Updates endpoints are present at the provider's overall floor.
const minJamfProVersion = ""

// msuAction shares Configure logic across the Managed Software Updates actions.
type msuAction struct {
	client *pro.Client
}

// configure binds the provider-supplied Jamf Pro client to the action via the shared
// providerdata.ConfigurePro helper.
func (a *msuAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "managed_software_updates")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.client = client
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *msuAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Pro client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}
