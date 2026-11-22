// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-azure-sdk/sdk/auth"
	"github.com/hashicorp/go-azure-sdk/sdk/environments"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// blobAPIVersion is the x-ms-version every Blob request declares. It is the
// version the marker, pin and Range behaviour were probed against on
// 2026-11-22, and a bearer token is refused without one.
const blobAPIVersion = "2023-11-03"

// managedIdentityTimeout bounds a token request to the managed identity
// endpoint. Off Azure that endpoint does not exist, and the credential module
// retries it for about 50 seconds before giving up (measured 2026-11-22), which
// a plan on a laptop with use_msi set would spend on every token request. The
// failure is then held like a failed build, so the wait is paid once. On Azure
// the cap can cut short a throttled endpoint that would have answered later;
// the error says so. A variable so tests can shorten it.
var managedIdentityTimeout = 10 * time.Second

// The authentication methods, in the order the credential module tries them.
const (
	azureMethodClientSecret = "client_secret"
	azureMethodOIDC         = "use_oidc"
	azureMethodMSI          = "use_msi"
	azureMethodCLI          = "use_cli"
)

// azureSettings is file_sources.azure with the ARM_* environment variables
// layered underneath: the values a credential is built from.
type azureSettings struct {
	environment string
	tenantID    string
	clientID    string

	clientSecret string

	useOIDC          bool
	oidcToken        string
	oidcRequestURL   string
	oidcRequestToken string

	useMSI      bool
	msiEndpoint string

	useCLI      bool
	cliExplicit bool

	unsupported string
	adoPipeline bool
}

// resolveAzureSettings layers the azurerm backend's environment variables
// under the configured values, as that backend does, so a machine already set
// up for the backend needs no file_sources.azure block.
//
// The variable names and the fallbacks (use_cli true, the rest false,
// environment public) are the backend's own, read from its source on
// 2026-11-22. Only ARM_* is read; the AZURE_* names other tools use are not.
// The request URL and request token also fall back to the variables GitHub
// Actions sets for a job with id-token: write, which is what makes use_oidc
// work there with nothing else configured.
//
// A configured value always wins, and a file path is read only when the value
// it would supply is not set directly. Values are trimmed, because the
// credential module trims before deciding whether a method is complete, and a
// file that is named but empty is an error: read as "no secret", it would let
// the choice fall through to the Azure CLI and read as somebody else.
//
// The backend also authenticates with a client certificate, AKS workload
// identity and an Azure DevOps service connection, none of which this provider
// supports. Their variables are noted so method can refuse them by name
// instead of quietly using the Azure CLI on a machine set up for one of them. getenv and readFile are parameters so
// the layering is testable without touching the process environment.
func resolveAzureSettings(config AzureConfig, getenv func(string) string, readFile func(string) ([]byte, error)) (azureSettings, error) {
	text := func(configured string, names ...string) string {
		if configured = strings.TrimSpace(configured); configured != "" {
			return configured
		}
		for _, name := range names {
			if v := strings.TrimSpace(getenv(name)); v != "" {
				return v
			}
		}
		return ""
	}
	var firstErr error
	flag := func(configured *bool, name string, fallback bool) bool {
		if configured != nil {
			return *configured
		}
		v := getenv(name)
		if v == "" {
			return fallback
		}
		parsed, err := strconv.ParseBool(v)
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("%w: %s is not a boolean", ErrCredentials, name)
		}
		return parsed
	}
	fromFile := func(value, path, what string) string {
		if value != "" || path == "" {
			return value
		}
		contents, err := readFile(path)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%w: reading the %s file: %w", ErrCredentials, what, err)
			}
			return ""
		}
		read := strings.TrimSpace(string(contents))
		if read == "" && firstErr == nil {
			firstErr = fmt.Errorf("%w: the %s file is empty", ErrCredentials, what)
		}
		return read
	}

	settings := azureSettings{
		environment:      text(config.Environment, "ARM_ENVIRONMENT"),
		tenantID:         text(config.TenantID, "ARM_TENANT_ID"),
		clientID:         text(config.ClientID, "ARM_CLIENT_ID"),
		useOIDC:          flag(config.UseOIDC, "ARM_USE_OIDC", false),
		oidcRequestURL:   text(config.OIDCRequestURL, "ARM_OIDC_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_URL"),
		oidcRequestToken: text(config.OIDCRequestToken, "ARM_OIDC_REQUEST_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
		useMSI:           flag(config.UseMSI, "ARM_USE_MSI", false),
		msiEndpoint:      text(config.MSIEndpoint, "ARM_MSI_ENDPOINT"),
		useCLI:           flag(config.UseCLI, "ARM_USE_CLI", true),
	}
	settings.clientSecret = fromFile(text(config.ClientSecret, "ARM_CLIENT_SECRET"), text(config.ClientSecretFile, "ARM_CLIENT_SECRET_FILE_PATH"), "client secret")
	settings.oidcToken = fromFile(text(config.OIDCToken, "ARM_OIDC_TOKEN"), text(config.OIDCTokenFile, "ARM_OIDC_TOKEN_FILE_PATH"), "OIDC token")
	if settings.environment == "" {
		settings.environment = AzureEnvironmentPublic
	}
	settings.cliExplicit = config.UseCLI != nil || strings.TrimSpace(getenv("ARM_USE_CLI")) != ""
	settings.adoPipeline = text("", "ARM_ADO_PIPELINE_SERVICE_CONNECTION_ID", "ARM_OIDC_AZURE_SERVICE_CONNECTION_ID") != ""
	var unsupported []string
	for _, name := range []string{"ARM_CLIENT_CERTIFICATE", "ARM_CLIENT_CERTIFICATE_PATH"} {
		if text("", name) != "" {
			unsupported = append(unsupported, name)
		}
	}
	if aks, _ := strconv.ParseBool(text("", "ARM_USE_AKS_WORKLOAD_IDENTITY")); aks {
		unsupported = append(unsupported, "ARM_USE_AKS_WORKLOAD_IDENTITY")
	}
	settings.unsupported = strings.Join(unsupported, ", ")
	return settings, firstErr
}

// method picks the one authentication method the settings enable, and refuses
// a method that is enabled but incomplete.
//
// The credential module walks the enabled methods in a fixed order and uses
// the first whose fields are all present. It skips an incomplete one without
// a word, and with nothing left reports only "no Authorizer could be
// configured", which does not say what was tried or what was missing (probed
// 2026-11-22). So the choice is made here, in the module's order, and an
// incomplete method is an error that names the missing value in place of a
// silent fall through to the next method. A client secret with no client ID
// must not quietly become an Azure CLI login as somebody else.
//
// A client secret is a method whenever one is present, as in the backend,
// where it has no switch.
func (s azureSettings) method() (string, error) {
	missing := func(method string, needs ...string) error {
		return fmt.Errorf("%w: file_sources.azure %s also needs %s, in the block or the matching ARM_* variable",
			ErrCredentials, method, strings.Join(needs, " and "))
	}
	var identity []string
	if s.tenantID == "" {
		identity = append(identity, "tenant_id")
	}
	if s.clientID == "" {
		identity = append(identity, "client_id")
	}
	switch {
	case s.clientSecret != "":
		if len(identity) > 0 {
			return "", missing(azureMethodClientSecret, identity...)
		}
		return azureMethodClientSecret, nil
	case s.useOIDC:
		if len(identity) > 0 {
			return "", missing(azureMethodOIDC, identity...)
		}
		if s.oidcToken == "" && s.adoPipeline {
			return "", fmt.Errorf("%w: an Azure DevOps service connection is configured in the environment, and file_sources.azure does not authenticate through one. Supply the token in oidc_token or oidc_token_file_path", ErrCredentials)
		}
		if s.oidcToken == "" && (s.oidcRequestURL == "" || s.oidcRequestToken == "") {
			return "", missing(azureMethodOIDC, "a token: oidc_token, oidc_token_file_path, or oidc_request_url with oidc_request_token")
		}
		return azureMethodOIDC, nil
	case s.useMSI:
		return azureMethodMSI, nil
	case s.useCLI:
		if s.unsupported != "" && !s.cliExplicit {
			return "", fmt.Errorf("%w: %s is set, and file_sources.azure does not support that authentication method. Use a client secret, use_oidc or use_msi, or set use_cli to true to read with the Azure CLI login instead", ErrCredentials, s.unsupported)
		}
		return azureMethodCLI, nil
	}
	return "", fmt.Errorf("%w: file_sources.azure enables no authentication method. use_cli is false, and neither a client secret, use_oidc nor use_msi is set", ErrCredentials)
}

// credentials turns the settings into the credential module's input with the
// one chosen method enabled and every other left off, so the module cannot
// choose differently.
func (s azureSettings) credentials(method string, environment environments.Environment) auth.Credentials {
	credentials := auth.Credentials{
		Environment: environment,
		TenantID:    s.tenantID,
		ClientID:    s.clientID,
	}
	switch method {
	case azureMethodClientSecret:
		credentials.EnableAuthenticatingUsingClientSecret = true
		credentials.ClientSecret = s.clientSecret
	case azureMethodOIDC:
		if s.oidcToken != "" {
			credentials.EnableAuthenticationUsingOIDC = true
			credentials.OIDCAssertionToken = s.oidcToken
		} else {
			credentials.EnableAuthenticationUsingGitHubOIDC = true
			credentials.OIDCTokenRequestURL = s.oidcRequestURL
			credentials.OIDCTokenRequestToken = s.oidcRequestToken
		}
	case azureMethodMSI:
		credentials.EnableAuthenticatingUsingManagedIdentity = true
		credentials.CustomManagedIdentityEndpoint = s.msiEndpoint
	case azureMethodCLI:
		credentials.EnableAuthenticatingUsingAzureCLI = true
	}
	return credentials
}

// azureToken returns a bearer token for Blob Storage. It is asked before every
// request, never once per read: the implementation caches and renews, and a
// read resumed after a cut must not reuse a token that may have expired while
// the first part was in flight. An Azure CLI token lasts about 26 minutes,
// less than a large upload.
type azureToken func(ctx context.Context) (string, error)

// azureDriver reads azblob:// sources with plain REST requests carrying a
// Microsoft Entra bearer token. There is no Blob SDK underneath: a HEAD and a
// ranged, conditional GET are the whole of it (probed 2026-11-22), and the
// SDK would add a dependency several times the size of this file.
//
// The credential is built on first use and once, as the S3 driver's is and for
// the same reasons; a failed build is held for buildFailureHold.
//
// Two limits come from the credential module and are not worked around. It
// runs the Azure CLI without a context, so a hung az ignores the caller's
// deadline, and while it runs the first time every other azblob read waits on
// this driver's lock. And a token supplied in oidc_token is exchanged again
// when the access token nears expiry, by which time a short-lived CI token
// has itself expired; an apply that outlasts the first access token needs the
// request-URL form, a client secret or a managed identity.
//
// The credential module writes a few lines through the standard log package
// (the Azure CLI command it runs, the managed identity URL). They are left
// where they go: Terraform shows a plugin's standard error only at debug
// level, and none of them carries a token (checked against a debug log of a
// real run).
type azureDriver struct {
	config    AzureConfig
	userAgent string
	transport http.RoundTripper

	getenv   func(string) string
	readFile func(string) ([]byte, error)
	endpoint func(account, suffix string) string
	newToken func(ctx context.Context, settings azureSettings, method string, environment *environments.Environment) (azureToken, error)

	mu          sync.Mutex
	token       azureToken
	suffix      string
	buildErr    error
	buildFailed time.Time
}

// newAzureDriver builds the driver. It makes no request.
func newAzureDriver(config AzureConfig, userAgent string, transport http.RoundTripper) *azureDriver {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &azureDriver{
		config:    config,
		userAgent: userAgent,
		transport: transport,
		getenv:    os.Getenv,
		readFile:  os.ReadFile,
		endpoint: func(account, suffix string) string {
			return "https://" + account + ".blob." + suffix
		},
		newToken: newAzureToken,
	}
}

// newAzureToken builds the credential module's authorizer for one method and
// wraps it as an azureToken.
func newAzureToken(ctx context.Context, settings azureSettings, method string, environment *environments.Environment) (azureToken, error) {
	authorizer, err := auth.NewAuthorizerFromCredentials(ctx, settings.credentials(method, *environment), environment.Storage)
	if err != nil {
		return nil, fmt.Errorf("%w: file_sources.azure %s: %w", ErrCredentials, method, err)
	}
	return func(caller context.Context) (string, error) {
		ctx := caller
		if method == azureMethodMSI {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(caller, managedIdentityTimeout)
			defer cancel()
		}
		token, err := authorizer.Token(ctx, nil)
		switch {
		case err != nil && caller.Err() != nil:
			return "", fmt.Errorf("requesting an Azure token: %w", caller.Err())
		case err != nil && method == azureMethodMSI && ctx.Err() != nil:
			return "", fmt.Errorf("%w: file_sources.azure use_msi: the managed identity endpoint gave no token within %s. Off Azure there is no such endpoint; on Azure it may be throttling, so try again", ErrCredentials, managedIdentityTimeout)
		case err != nil:
			return "", fmt.Errorf("%w: file_sources.azure %s: %w", ErrCredentials, method, err)
		case token == nil || token.AccessToken == "":
			return "", fmt.Errorf("%w: file_sources.azure %s: the token endpoint answered with no token", ErrCredentials, method)
		}
		return token.AccessToken, nil
	}, nil
}

// credential returns the token source and the storage domain suffix, building
// them on first use.
func (d *azureDriver) credential(ctx context.Context) (azureToken, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.token != nil {
		return d.token, d.suffix, nil
	}
	if d.buildErr != nil && time.Since(d.buildFailed) < buildFailureHold {
		return nil, "", d.buildErr
	}
	token, suffix, err := d.build(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", fmt.Errorf("building the Azure credential: %w", ctx.Err())
		}
		d.buildErr, d.buildFailed = err, time.Now()
		return nil, "", err
	}
	d.token, d.suffix, d.buildErr = token, suffix, nil
	return token, suffix, nil
}

// build resolves the settings, picks the method and builds its credential.
func (d *azureDriver) build(ctx context.Context) (azureToken, string, error) {
	settings, err := resolveAzureSettings(d.config, d.getenv, d.readFile)
	if err != nil {
		return nil, "", err
	}
	environment, err := environments.FromName(settings.environment)
	if err != nil {
		return nil, "", fmt.Errorf("%w: file_sources.azure environment %q is not one of %s", ErrCredentials, settings.environment, strings.Join(AzureEnvironments(), ", "))
	}
	suffix, ok := environment.Storage.DomainSuffix()
	if !ok || suffix == nil || *suffix == "" {
		return nil, "", fmt.Errorf("%w: the %s environment has no Blob Storage endpoint", ErrCredentials, settings.environment)
	}
	method, err := settings.method()
	if err != nil {
		return nil, "", err
	}
	tflog.Debug(ctx, "authenticating to Azure Blob Storage", map[string]any{"method": method, "environment": settings.environment})
	token, err := d.newToken(ctx, settings, method, environment)
	if err != nil {
		return nil, "", err
	}
	return token, *suffix, nil
}

// Stat implements Source with a HEAD on the blob.
func (d *azureDriver) Stat(ctx context.Context, ref Ref) (Attributes, error) {
	resp, err := d.request(ctx, http.MethodHead, ref, "", 0)
	if err != nil {
		return Attributes{}, err
	}
	_ = resp.Body.Close()
	if resp.ContentLength < 0 {
		return Attributes{}, fmt.Errorf("%w: %s", ErrNoSize, ref)
	}
	return Attributes{Size: resp.ContentLength, Marker: etagMarker(new(resp.Header.Get("ETag")))}, nil
}

// Open implements Source with a GET, pinned with If-Match and started at
// offset with Range.
func (d *azureDriver) Open(ctx context.Context, ref Ref, pin string, offset int64) (Opened, error) {
	etag := ""
	if pin != "" {
		var ok bool
		if etag, ok = strings.CutPrefix(pin, markerETag); !ok {
			return Opened{}, fmt.Errorf("the marker %q is not one an Azure Blob Storage source records", pin)
		}
	}
	resp, err := d.request(ctx, http.MethodGet, ref, etag, offset)
	if err != nil {
		return Opened{}, err
	}
	size := resp.ContentLength
	if offset > 0 {
		size, err = rangedSize(ref, resp.Header.Get("Content-Range"), offset)
	} else if resp.ContentLength < 0 {
		err = fmt.Errorf("%w: %s", ErrNoSize, ref)
	}
	if err != nil {
		_ = resp.Body.Close()
		return Opened{}, err
	}
	return Opened{
		Body: resp.Body,
		Size: size, Marker: etagMarker(new(resp.Header.Get("ETag"))),
	}, nil
}

// request sends one authorised request for the blob and returns the response
// for a 200 or a 206. Anything else is closed and classified.
//
// The request asks for the stored bytes as they are. Left to itself Go's
// client asks for gzip and decompresses a blob stored with that encoding,
// which discards the declared length: the plan-time HEAD would succeed and
// the apply's read would then fail for want of a size.
//
// A token request that fails for a credential reason is held like a failed
// build, and the credential is rebuilt after the hold. The methods that make
// no request while building (client secret, OIDC, managed identity) fail
// here, on the first token, and without the hold every package in a plan
// would pay for the same failure in turn.
//
// Redirects are not followed. Blob Storage does not issue them for these
// requests, and following one would mean deciding whether a bearer token may
// travel to wherever it points; an unexpected 3xx is reported as it is.
func (d *azureDriver) request(ctx context.Context, method string, ref Ref, etag string, offset int64) (*http.Response, error) {
	token, suffix, err := d.credential(ctx)
	if err != nil {
		return nil, err
	}
	bearer, err := token(ctx)
	if err != nil {
		if errors.Is(err, ErrCredentials) {
			d.mu.Lock()
			d.token, d.buildErr, d.buildFailed = nil, err, time.Now()
			d.mu.Unlock()
		}
		return nil, err
	}

	segments := strings.Split(ref.Blob, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	target := d.endpoint(ref.Account, suffix) + "/" + url.PathEscape(ref.Container) + "/" + strings.Join(segments, "/")
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request for %s: %w", ref, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("x-ms-version", blobAPIVersion)
	req.Header.Set("Accept-Encoding", "identity")
	if d.userAgent != "" {
		req.Header.Set("User-Agent", d.userAgent)
	}
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}

	client := &http.Client{
		Transport:     d.transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ref, err)
	}
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
		return resp, nil
	}
	_ = resp.Body.Close()
	return nil, classifyBlobStatus(ref, resp)
}

// classifyBlobStatus turns a refusal into an error this package's callers can
// act on. Blob Storage names the reason in the x-ms-error-code header, on a
// HEAD as well, so the text carries it.
func classifyBlobStatus(ref Ref, resp *http.Response) error {
	code := resp.Header.Get("x-ms-error-code")
	if code == "" {
		code = "no error code"
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s (%s)", ErrNotFound, ref, code)
	case http.StatusPreconditionFailed:
		return fmt.Errorf("%w: %s", ErrChanged, ref)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: Azure Blob Storage refused the token for %s (%s)", ErrCredentials, ref, code)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %s (%s). The identity needs the Storage Blob Data Reader role on the container or the account, and a new role assignment can take a minute to apply", ErrAccessDenied, ref, code)
	}
	return fmt.Errorf("reading %s: Azure Blob Storage answered HTTP %d (%s)", ref, resp.StatusCode, code)
}
