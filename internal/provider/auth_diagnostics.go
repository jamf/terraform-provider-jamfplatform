// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/egressip"
	"golang.org/x/oauth2"
)

// egressIPLookupURL is named in the diagnostic as the command to run when the
// lookup itself could not reach the echo service.
const egressIPLookupURL = egressip.LookupURL

// egressIPLookup is the lookup used by authFailureDiagnostic, indirected so
// tests can exercise the blocked-request branch without network access. The
// implementation is shared with the resource diagnostics and caches, so a
// configuration failure and a later edge block cost one request between them.
//
// Calls through rather than copying the function value, for the reason given on
// the copy in internal/common/helpers: a package-initialisation copy cannot be
// stubbed from another package's test.
var egressIPLookup = func() string { return egressip.Lookup() }

// authFailureDiagnostic renders a failed credential validation as a Terraform
// diagnostic summary and detail.
//
// It exists to separate two failures that need opposite remedies and that the
// raw error does not distinguish:
//
//   - A rejected credential. Jamf's token service answers in JSON, including
//     when it refuses: 401 {"error":"invalid_client"}. Remedy: fix the secret.
//   - A request that never reached Jamf. A WAF, IP allowlist or captive proxy
//     answered instead, with an HTML error page or an empty body. Remedy: get
//     this host's egress IP allowed. Nothing is wrong with the credentials.
//   - A base URL that is not the gateway root. The token endpoint is always
//     {baseURL}/auth/token, so a base URL carrying a path prefix sends the
//     exchange somewhere Jamf does not serve and it comes back 404. Remedy: fix
//     base_url. Neither the credentials nor the network is involved.
//
// The SDK marks the second case with ErrUnexpectedResponse (v0.13.0+) precisely
// so consumers can branch rather than string-match, since the condition is
// inferred from the shape of the body rather than reported by Jamf. Without the
// branch both cases rendered as "please verify your credentials are correct",
// which sends the user hunting a secret that turns out to be fine — the failure
// mode the sentinel was added to end.
//
// The status code is deliberately not consulted: a block arrives as a 403 from
// nginx, as a 200 carrying a login or SPA shell, or as a bare 503, so no single
// status identifies it.
// The 404 is separated first because it arrives carrying the same sentinel as a
// network block — the body is not JSON either way — and the blocked-request
// wording is the expensive wrong answer for it: it sends the user to their
// network team and to Jamf Support over a base-URL typo. That typo stops being
// hypothetical at the Platform API GA, which retires {region}.apigw.jamf.com in
// favour of {region}.api.jamfcloud.com and drops the /api segment, so every
// existing configuration has to be edited and some will be edited wrong.
//
// The status code is read off the oauth2 error rather than matched in the
// message because the SDK already models it: annotateTokenError wraps the
// *oauth2.RetrieveError with %w, so it stays reachable through errors.As no
// matter how many layers wrap it, whereas the guidance text it appends is prose
// and free to be reworded.
func authFailureDiagnostic(baseURL string, err error) (summary, detail string) {
	if isTokenEndpointNotFound(err) {
		return "Jamf Platform Base URL Not Found", "The Jamf Platform API returned 404 for the authentication request. " +
			"The token endpoint is always `{base_url}/auth/token`, so a 404 there means `base_url` is not the " +
			"gateway root — most often because it carries a path prefix such as `/api`, which the GA gateway does " +
			"not use.\n\n" +
			"Set `base_url` to the regional gateway root, with no path:\n" +
			"  - https://us.api.jamfcloud.com\n" +
			"  - https://eu.api.jamfcloud.com\n" +
			"  - https://apac.api.jamfcloud.com\n\n" +
			"Configured base URL: " + baseURL + "\n\n" +
			"This is neither a credential problem nor a network block: the credentials were never assessed, " +
			"because the request did not reach an endpoint that assesses them.\n\n" +
			"Technical details: " + err.Error()
	}

	if !errors.Is(err, jamfplatform.ErrUnexpectedResponse) {
		return "Authentication Failed", fmt.Sprintf(
			"Unable to authenticate with Jamf Platform API. Please verify your credentials are correct.\n\nError: %s",
			err.Error(),
		)
	}

	// Support needs to find the block in gateway logs, which means the time it
	// happened and the address it came from. Both are gathered here rather than
	// asked for later, because by the time the user opens a ticket the timestamp
	// is gone and the egress IP may have changed.
	ipLine := egressIPLookup()
	if ipLine == "" {
		ipLine = "(unable to determine — run `curl -s " + egressIPLookupURL + "` on this host)"
	}

	return "Jamf Platform API Request Blocked", "The Jamf Platform API returned a non-JSON response to the authentication " +
		"request, which means something in front of Jamf answered instead of Jamf itself. " +
		"This is typically a security policy — a WAF or an IP allowlist — blocking requests " +
		"from this host's IP address. It is not a credential problem: a rejected client ID or " +
		"secret comes back as JSON and reports itself as such.\n\n" +
		"Contact Jamf Support and provide the following:\n" +
		"  - Timestamp:    " + time.Now().UTC().Format(time.RFC3339) + "\n" +
		"  - Base URL:     " + baseURL + "\n" +
		"  - Egress IP:    " + ipLine + "\n\n" +
		"Technical details: " + err.Error()
}

// betaGatewayHost is the host suffix of the pre-GA Jamf Platform API gateway,
// served regionally as {region}.apigw.jamf.com. It stopped serving the API at
// the Platform API GA; the host itself still resolves and still answers the
// token exchange, and will be switched off at some later point.
const betaGatewayHost = "apigw.jamf.com"

// betaGatewayError reports a base URL still pointing at the beta gateway, or
// ("", "") if there is nothing to say.
//
// This errors where baseURLPathWarning warns, and the asymmetry is the point: a
// path prefix beneath a Jamf host might be a customer reverse proxy the provider
// cannot distinguish, whereas this host is Jamf's own and serves no API
// namespace under any configuration. Nothing usable follows from continuing.
//
// It is checked before the token exchange rather than left to
// authFailureDiagnostic because the exchange *succeeds*: wire-probed 2026-09-08,
// {region}.apigw.jamf.com still proxies /auth/token to the same auth backend and
// answers a GA credential with a valid token, while every API namespace beneath
// it answers a bare 404. So the failure surfaces nowhere near authentication. It
// surfaces as every Read reporting the object gone, one refresh emptying the
// state file, and `terraform plan` printing a clean set of creates and exit
// status 0 — reported by a user who upgraded the provider without changing
// base_url. helpers.IsGatewayUnrouted stops that from deleting anything; this
// stops the run before it starts and names the cause.
//
// The hostname is matched with any trailing dot removed because url.Hostname
// preserves one, and the fully qualified form is a live host rather than a
// curiosity: wire-probed 2026-09-08, POST https://eu.apigw.jamf.com./auth/token
// returns a valid token, so leaving it unmatched would trade one named
// diagnostic for a 404 on every subsequent read.
func betaGatewayError(baseURL string) (summary, detail string) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host != betaGatewayHost && !strings.HasSuffix(host, "."+betaGatewayHost) {
		return "", ""
	}
	return "Base URL Names the Beta Gateway", "`base_url` is set to " + baseURL + ", the pre-GA beta gateway. It no " +
		"longer serves the Platform API: it still accepts your credentials, then returns 404 for every request " +
		"after that.\n\n" +
		"Set `base_url` to the regional gateway root, with no path:\n" +
		"  - https://us.api.jamfcloud.com\n" +
		"  - https://eu.api.jamfcloud.com\n" +
		"  - https://apac.api.jamfcloud.com\n\n" +
		"Beta API integration credentials stopped working at the same time. Register a replacement integration " +
		"in Jamf Account and use `environment_id` in place of `tenant_id`. See the `Upgrading to the Platform " +
		"API GA` guide.\n\n" +
		"On `v0.29.0` and `v0.30.0` this same setting produced a plan offering to create everything you already " +
		"manage, with no error in it, and an apply then emptied the state file and created nothing in Jamf. " +
		"This check runs before Terraform decides whether to refresh, so `terraform plan -refresh=false` will not " +
		"get past it: inspect `terraform show terraform.tfstate.backup` now to see whether an earlier apply emptied " +
		"your state, and restore that file if it did."
}

// jamfGatewayHosts are the domains Jamf serves its own gateways from. A path
// prefix is only wrong beneath these: the SDK builds every request as
// baseURL + path, so a caller fronting Jamf with their own reverse proxy that
// mounts the token endpoint and the namespaces under one prefix is a supported
// configuration and must not be warned about.
var jamfGatewayHosts = []string{".jamfcloud.com", ".jamf.com", ".jamfnebula.com"}

// baseURLPathWarning reports a Jamf base URL carrying a path prefix, or ("", "")
// if there is nothing to say.
//
// This is the same defect authFailureDiagnostic's 404 branch explains after the
// fact, caught before the token exchange so the message names the cause rather
// than the symptom. It warns rather than errors for the reason above — the
// provider cannot tell a mis-set base URL from a deliberate reverse proxy except
// by the host, and being wrong in the erroring direction would make a working
// configuration unusable.
//
// The hostname is matched with any trailing dot removed for the same reason
// betaGatewayError does it: url.Hostname preserves one, and a fully qualified
// Jamf host carrying a path prefix is the same misconfiguration written a
// second way.
func baseURLPathWarning(baseURL string) (summary, detail string) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return "", ""
	}
	if trimmed := strings.Trim(parsed.Path, "/"); trimmed == "" {
		return "", ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	isJamf := false
	for _, suffix := range jamfGatewayHosts {
		if strings.HasSuffix(host, suffix) {
			isJamf = true
			break
		}
	}
	if !isJamf {
		return "", ""
	}
	return "Base URL Carries a Path Prefix", "`base_url` is set to " + baseURL + ", which includes a path. The Jamf Platform " +
		"gateways serve the token endpoint and every API namespace at the host root, so authentication will be " +
		"attempted against " + strings.TrimRight(baseURL, "/") + "/auth/token and fail with a 404.\n\n" +
		"Set `base_url` to the host alone, for example https://eu.api.jamfcloud.com. The `/api` segment older " +
		"configurations carried is not used by the Jamf Platform API at GA."
}

// isTokenEndpointNotFound reports whether the token exchange failed with a 404,
// which identifies a base URL that is not the gateway root.
//
// It requires the sentinel as well as the status, so a 404 carrying a JSON body
// stays a credential failure. Jamf's token service answers refusals in JSON, and
// a JSON 404 from Jamf itself would be Jamf reporting something about the
// request rather than the gateway failing to route it.
func isTokenEndpointNotFound(err error) bool {
	if !errors.Is(err, jamfplatform.ErrUnexpectedResponse) {
		return false
	}
	var retrieve *oauth2.RetrieveError
	if !errors.As(err, &retrieve) {
		return false
	}
	return retrieve.Response != nil && retrieve.Response.StatusCode == http.StatusNotFound
}
