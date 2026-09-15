// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package appinstalleractions implements the fire-once Jamf Pro App Installer
// actions: jamfplatform_pro_retry_app_installer_installations (retry the failed
// installations of one deployment),
// jamfplatform_pro_retry_all_app_installer_installations (retry every failed
// App Installer installation in the tenant) and
// jamfplatform_pro_update_app_installer_version (move a MANUAL deployment to a
// newer version of its title).
//
// The retry pair is deliberately two actions rather than one with an optional
// deployment ID. Omitting an attribute would make the tenant-wide retry the
// accident you get by forgetting the deployment, and unlike the patch-policy
// retry this action is modelled on, the unbounded form is not scoped by a named
// object. The breadth is in the name instead.
package appinstalleractions

import (
	"context"
	"errors"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/action"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty: the App Installer endpoints predate the provider's
// overall floor.
const minJamfProVersion = ""

// appInstallerClient is the subset of *pro.Client the App Installer actions use.
// Declaring it as an interface keeps Invoke unit-testable without a live client,
// which matters most for the retry pair: whether a response is "nothing to
// retry" or a genuine failure is decided entirely from the error the client
// returns, so a fake error is the only way to cover both outcomes.
type appInstallerClient interface {
	RetryAppInstallerInstallationsV1(ctx context.Context) error
	RetryAppInstallerDeploymentInstallationsV1(ctx context.Context, id string) error
	RetryAppInstallerDeploymentComputerInstallationV1(ctx context.Context, id string, computerID string) error
	UpdateAppInstallerDeploymentVersionV1(ctx context.Context, id string, request *pro.AppTitleVersion) error
}

// appInstallerAction shares Configure logic across the App Installer actions.
type appInstallerAction struct {
	client appInstallerClient
}

// configure binds the provider-supplied Jamf Pro client to the action via the shared
// providerdata.ConfigurePro helper. A nil client is left unbound rather than
// stored, so a failed configure cannot present itself to Invoke as a usable
// client through the interface field.
func (a *appInstallerAction) configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "app_installers")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	a.client = client
}

// ensureClient guarantees Configure completed successfully before Invoke.
func (a *appInstallerAction) ensureClient(resp *action.InvokeResponse) bool {
	if a.client != nil {
		return true
	}

	resp.Diagnostics.AddError(
		"Provider Not Configured",
		"The Jamf Pro client was not configured. Re-run terraform init/apply so the provider can configure successfully.",
	)
	return false
}

// isNothingToRetry reports whether err is a retry endpoint saying there was
// nothing to retry.
//
// Wire-verified 2026-09-03: the tell is a `404` carrying an EMPTY `errors`
// array, and nothing else. The shared helpers.IsNotFoundError is deliberately
// NOT used here, because it also matches `400` with an `INVALID_ID` detail —
// which on these endpoints is Jamf Pro refusing a MALFORMED deployment ID
// ("id field must be string of positive numeric value or -1") before it looks
// anything up. That is an operator mistake and must fail the apply, not be
// downgraded to a warning saying nothing needed retrying.
//
// Two 404s that no Jamf service produced are excluded, because reading either as
// "nothing needed retrying" succeeds the action for a request that never reached
// Jamf Pro: the operator takes the failed installations to be clear and no retry
// is ever issued. They are the gateway's own unrouted reply
// (helpers.IsGatewayUnrouted) and an edge error page, which CloudFront serves
// with a 404 among other statuses (helpers.IsEdgeBlocked).
//
// The edge term is defence rather than a live fix, and the distinction is worth
// recording because a future reader will find no test that fails without it.
// SDK v1.0.0 condenses every edge page into one synthetic error detail, so such a
// reply carries a non-empty `errors` array and the emptiness test already rejects
// it. That coupling is the SDK's to change, not this package's, and the cost of
// depending on it is an action that reports success for a blocked request.
func isNothingToRetry(err error) bool {
	apiErr, ok := errors.AsType[*jamfplatform.APIResponseError](err)
	if !ok || helpers.IsGatewayUnrouted(err) || helpers.IsEdgeBlocked(err) {
		return false
	}
	return apiErr.HasStatus(http.StatusNotFound) && len(apiErr.Details()) == 0
}

// nothingToRetryDiagnostic explains an empty 404 from either retry endpoint —
// the response isNothingToRetry matches.
//
// Wire-verified 2026-09-03: both answer `404` with an EMPTY `errors` array when
// there is nothing to retry, and the same empty 404 for a deployment that does
// not exist — the bodies are byte-identical, so the two cases cannot be told
// apart from here. Jamf Pro's unrouted tell in this namespace is
// `403 BAD_PERMISSIONS`, so a 404 is a routed request refused on state. A
// malformed deployment ID is a different response — `400 INVALID_ID`, naming
// the field — and surfaces as an error rather than reaching this diagnostic.
//
// It is a warning, not an error. A retry wired to a lifecycle action_trigger
// fires after every apply, and a healthy fleet with no failed installations is
// the normal case — failing the apply for it would break the workspace whenever
// nothing is wrong. The warning names both possible causes so the outcome is
// never silent.
func nothingToRetryDiagnostic(subject string) (string, string) {
	return "No Failed Installations Retried",
		"Jamf Pro answered 404 for " + subject + ". Either there are no failed installations to retry, " +
			"or the deployment does not exist — Jamf Pro returns the same empty response for both, so this " +
			"action cannot distinguish them. Nothing was retried. Check the deployment ID if you expected failures to be retried."
}
