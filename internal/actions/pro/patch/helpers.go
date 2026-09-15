// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package patchactions implements the fire-once Jamf Pro patch actions:
// jamfplatform_pro_retry_patch_policy_logs (retry failed patch policy
// installation attempts).
package patchactions

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty: the patch policy log endpoints are present at the
// provider's overall floor.
const minJamfProVersion = ""

// patchAction shares Configure logic across the patch actions.
type patchAction struct {
	client *pro.Client
}

// configure binds the provider-supplied Jamf Pro client to the action via the shared
// providerdata.ConfigurePro helper.
func (a *patchAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "patch")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.client = client
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *patchAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Pro client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}
