// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package uemconnectactions implements the fire-once Jamf Security Cloud UEM
// Connect actions:
//
//   - jamfplatform_security_cloud_uem_connect_synchronize — the admin UI's
//     *Actions → Synchronize* on a UEM Connect integration.
//   - jamfplatform_security_cloud_activation_profile_deploy — the *Deploy to Jamf
//     Pro* button under *UEM actions* on an activation profile's deployment page.
//
// Nothing else on the integration's Actions menu belongs here. "Disable" is the
// resource's `enabled` attribute and "Delete" is destroying the resource — both are
// state, and an action for either would be a second way to change something
// Terraform already manages. Cancelling a running sync
// (CancelUemConnectorSyncV1) is left out for a different reason: it needs the
// transaction ID of the run to cancel, which the trigger does not return, and
// Terraform has no shape for undoing an action that has already been invoked.
//
// The deploy is an action rather than a resource for a reason the wire settled
// rather than taste. Its scoping field only ever adds: deploying group 3 onto a
// profile scoped to groups 1 and 2 leaves it scoped to 1, 2 and 3, an empty list
// changes nothing, and omitting the field changes nothing either. There is no
// route that removes a deployment, and no route that reads or lists activation
// profiles. A resource would promise convergence none of that can deliver.
package uemconnectactions

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// uemConnectAction shares Configure logic across the UEM Connect actions.
type uemConnectAction struct {
	client *securitycloud.Client
}

// configure binds the provider-supplied Jamf Security Cloud client to the action.
//
// It goes through ConfigureSecurityCloud rather than ConfigurePro: Security Cloud
// has no customer-tenant version to gate on, and a tenant can hold it without
// holding Jamf Pro, so a Pro version fetch would be both meaningless and fatal.
func (a *uemConnectAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_uem_connect_synchronize")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.client = client
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *uemConnectAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Security Cloud client was not configured. Re-run terraform init/apply so the provider can "+
			"configure successfully.",
	)
	return false
}

// resolveIntegrationID returns the integration to act on: the configured ID when
// one is given, otherwise the tenant's only integration.
//
// A tenant holds at most one UEM Connect integration, so requiring its opaque
// identifier would be friction with nothing to disambiguate. The attribute stays
// available because naming the resource's ID is how a configuration makes the
// action depend on the integration existing.
func (a *uemConnectAction) resolveIntegrationID(ctx context.Context, configured string, diags *diag.Diagnostics) string {
	if configured != "" {
		return configured
	}

	page, err := a.client.ListUemConnectorsV1(ctx)
	if err != nil {
		if !appendInvokeDiagnostics(diags, err) {
			diags.AddError(
				"Could not find the UEM Connect integration to synchronize",
				"Reading this tenant's UEM Connect integration failed, so there is nothing to act on. Reported: "+
					helpers.APIErrorDetail(err),
			)
		}
		return ""
	}
	if page == nil || len(page.Results) == 0 {
		diags.AddError(
			"No UEM Connect integration on this tenant",
			"Jamf Security Cloud reports no UEM Connect integration for this tenant, so there is nothing to "+
				"synchronize. Set one up with jamfplatform_security_cloud_uem_connect, or name an integration "+
				"explicitly with `uem_connect_id`.",
		)
		return ""
	}
	return page.Results[0].ID
}

// appendInvokeDiagnostics turns a failure into the most specific diagnostic the
// error body supports, and reports whether it recognised one.
//
// CONNECTOR_DISABLED is the one worth translating: a disabled integration refuses
// a sync, and the raw message names the integration's identifier rather than the
// setting that has to change.
func appendInvokeDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeConnectorDisabled:
			diags.AddError(
				"UEM Connect is disabled, so it cannot synchronize",
				"Jamf Security Cloud refuses to synchronize a disabled integration. Enable it first — set "+
					"`enabled = true` on the jamfplatform_security_cloud_uem_connect resource, or enable it in "+
					"the Jamf Security Cloud admin UI. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeNotEntitled:
			addNotEntitled(diags, detail.Description)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// addNotEntitled reports a tenant that authenticated but does not hold the UEM
// Connect surface.
//
// Shared by both actions because entitlement is not authentication: a perfectly
// valid integration is still refused with NOT_ENTITLED, and the raw error says
// nothing about what to do next.
func addNotEntitled(diags *diag.Diagnostics, description string) {
	diags.AddError(
		"Tenant not entitled to Jamf Security Cloud UEM Connect",
		"The credentials authenticated successfully but this tenant does not have the UEM Connect surface "+
			"enabled. Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+description,
	)
}

// isNotFound reports whether an error is Jamf Security Cloud saying the named
// integration does not exist.
//
// Two 404s that no Jamf service produced are excluded: each reports that the
// request reached no Jamf service, so reporting the integration as absent would
// blame the operator for a live integration and a network fault they were never
// told about. They are the gateway's own unrouted reply
// (helpers.IsGatewayUnrouted) and an edge error page, which CloudFront serves
// with a 404 among other statuses (helpers.IsEdgeBlocked).
func isNotFound(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	if helpers.IsGatewayUnrouted(err) {
		return false
	}
	if helpers.IsEdgeBlocked(err) {
		return false
	}
	return apiErr.HasStatus(http.StatusNotFound)
}
