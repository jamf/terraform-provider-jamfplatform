// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/go-azure-sdk/sdk/environments"
)

func envFrom(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func noFiles(string) ([]byte, error) { return nil, os.ErrNotExist }

func TestResolveAzureSettingsLayersTheEnvironmentUnderTheBlock(t *testing.T) {
	yes, no := true, false
	environment := envFrom(map[string]string{
		"ARM_ENVIRONMENT":                "china",
		"ARM_TENANT_ID":                  "env-tenant",
		"ARM_CLIENT_ID":                  "env-client",
		"ARM_CLIENT_SECRET":              "env-secret",
		"ARM_USE_OIDC":                   "true",
		"ARM_OIDC_TOKEN":                 "env-oidc-token",
		"ARM_OIDC_REQUEST_URL":           "https://arm.example/request",
		"ACTIONS_ID_TOKEN_REQUEST_URL":   "https://github.example/request",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "github-request-token",
		"ARM_USE_MSI":                    "1",
		"ARM_MSI_ENDPOINT":               "http://msi.example",
		"ARM_USE_CLI":                    "false",
	})

	got, err := resolveAzureSettings(AzureConfig{}, environment, noFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := azureSettings{
		environment: "china", tenantID: "env-tenant", clientID: "env-client", clientSecret: "env-secret",
		useOIDC: true, oidcToken: "env-oidc-token", oidcRequestURL: "https://arm.example/request", oidcRequestToken: "github-request-token",
		useMSI: true, msiEndpoint: "http://msi.example", useCLI: false, cliExplicit: true,
	}
	if got != want {
		t.Errorf("from the environment alone:\n got %+v\nwant %+v", got, want)
	}

	got, err = resolveAzureSettings(AzureConfig{
		Environment: "usgovernment", TenantID: "block-tenant", ClientID: "block-client", ClientSecret: "block-secret",
		UseOIDC: &no, OIDCToken: "block-oidc-token", OIDCRequestURL: "https://block.example/request", OIDCRequestToken: "block-request-token",
		UseMSI: &no, MSIEndpoint: "http://block-msi.example", UseCLI: &yes,
	}, environment, noFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = azureSettings{
		environment: "usgovernment", tenantID: "block-tenant", clientID: "block-client", clientSecret: "block-secret",
		useOIDC: false, oidcToken: "block-oidc-token", oidcRequestURL: "https://block.example/request", oidcRequestToken: "block-request-token",
		useMSI: false, msiEndpoint: "http://block-msi.example", useCLI: true, cliExplicit: true,
	}
	if got != want {
		t.Errorf("the block must win over the environment:\n got %+v\nwant %+v", got, want)
	}
}

func TestResolveAzureSettingsDefaults(t *testing.T) {
	got, err := resolveAzureSettings(AzureConfig{}, envFrom(nil), noFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := (azureSettings{environment: "public", useCLI: true}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if method, err := got.method(); err != nil || method != azureMethodCLI {
		t.Errorf("with nothing set the Azure CLI is the method, got %q and %v", method, err)
	}
}

func TestResolveAzureSettingsDoesNotReadAZUREVariables(t *testing.T) {
	got, err := resolveAzureSettings(AzureConfig{}, envFrom(map[string]string{
		"AZURE_TENANT_ID": "t", "AZURE_CLIENT_ID": "c", "AZURE_CLIENT_SECRET": "s",
	}), noFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.tenantID != "" || got.clientID != "" || got.clientSecret != "" {
		t.Errorf("AZURE_* variables were read: %+v", got)
	}
}

func TestResolveAzureSettingsReadsSecretsFromFiles(t *testing.T) {
	files := map[string]string{"/run/secret": "file-secret\n", "/run/token": "  file-token  "}
	read := func(path string) ([]byte, error) {
		contents, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(contents), nil
	}

	got, err := resolveAzureSettings(AzureConfig{ClientSecretFile: "/run/secret"}, envFrom(map[string]string{"ARM_OIDC_TOKEN_FILE_PATH": "/run/token"}), read)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.clientSecret != "file-secret" || got.oidcToken != "file-token" {
		t.Errorf("secret=%q token=%q, want both read and trimmed", got.clientSecret, got.oidcToken)
	}

	got, err = resolveAzureSettings(AzureConfig{ClientSecret: "direct", ClientSecretFile: "/run/absent"}, envFrom(nil), read)
	if err != nil || got.clientSecret != "direct" {
		t.Errorf("a value given directly must win and its file must not be read: %q, %v", got.clientSecret, err)
	}

	_, err = resolveAzureSettings(AzureConfig{ClientSecretFile: "/run/absent"}, envFrom(nil), read)
	if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), "client secret") {
		t.Errorf("an unreadable file must be a named credential error, got %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "file-secret") {
		t.Errorf("the error carries a secret: %v", err)
	}
}

func TestResolveAzureSettingsRefusesABooleanItCannotRead(t *testing.T) {
	_, err := resolveAzureSettings(AzureConfig{}, envFrom(map[string]string{"ARM_USE_OIDC": "yes please"}), noFiles)
	if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), "ARM_USE_OIDC") {
		t.Errorf("want a credential error naming the variable, got %v", err)
	}
}

func TestAzureMethodIsChosenInTheModulesOrderAndNeverFallsThrough(t *testing.T) {
	cases := []struct {
		name     string
		settings azureSettings
		method   string
		missing  string
	}{
		{"client secret", azureSettings{tenantID: "t", clientID: "c", clientSecret: "s", useOIDC: true, useMSI: true, useCLI: true}, azureMethodClientSecret, ""},
		{"client secret without a client id does not become the cli", azureSettings{tenantID: "t", clientSecret: "s", useCLI: true}, "", "client_id"},
		{"client secret without a tenant", azureSettings{clientID: "c", clientSecret: "s", useCLI: true}, "", "tenant_id"},
		{"oidc with a token", azureSettings{tenantID: "t", clientID: "c", useOIDC: true, oidcToken: "jwt", useMSI: true, useCLI: true}, azureMethodOIDC, ""},
		{"oidc with a request url and token", azureSettings{tenantID: "t", clientID: "c", useOIDC: true, oidcRequestURL: "u", oidcRequestToken: "r"}, azureMethodOIDC, ""},
		{"oidc with nothing to exchange does not become the cli", azureSettings{tenantID: "t", clientID: "c", useOIDC: true, useCLI: true}, "", "oidc_token"},
		{"oidc with a request url alone", azureSettings{tenantID: "t", clientID: "c", useOIDC: true, oidcRequestURL: "u"}, "", "oidc_request_token"},
		{"oidc without a client id", azureSettings{tenantID: "t", useOIDC: true, oidcToken: "jwt"}, "", "client_id"},
		{"managed identity", azureSettings{useMSI: true, useCLI: true}, azureMethodMSI, ""},
		{"azure cli", azureSettings{useCLI: true}, azureMethodCLI, ""},
		{"nothing enabled", azureSettings{}, "", "enables no authentication method"},
		{"a certificate in the environment does not become the cli", azureSettings{useCLI: true, unsupported: "ARM_CLIENT_CERTIFICATE_PATH"}, "", "ARM_CLIENT_CERTIFICATE_PATH"},
		{"the cli asked for by name is used whatever else is set", azureSettings{useCLI: true, cliExplicit: true, unsupported: "ARM_CLIENT_CERTIFICATE_PATH"}, azureMethodCLI, ""},
		{"a certificate beside a client secret is ignored", azureSettings{tenantID: "t", clientID: "c", clientSecret: "s", unsupported: "ARM_CLIENT_CERTIFICATE_PATH"}, azureMethodClientSecret, ""},
		{"an azure devops connection is refused by name", azureSettings{tenantID: "t", clientID: "c", useOIDC: true, oidcRequestURL: "u", oidcRequestToken: "r", adoPipeline: true}, "", "Azure DevOps"},
		{"an azure devops connection beside a supplied token", azureSettings{tenantID: "t", clientID: "c", useOIDC: true, oidcToken: "jwt", adoPipeline: true}, azureMethodOIDC, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, err := tc.settings.method()
			if tc.missing == "" {
				if err != nil || method != tc.method {
					t.Fatalf("got %q and %v, want %q", method, err, tc.method)
				}
				return
			}
			if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), tc.missing) {
				t.Fatalf("want a credential error naming %q, got %q and %v", tc.missing, method, err)
			}
		})
	}
}

func TestAzureCredentialsEnableOnlyTheChosenMethod(t *testing.T) {
	settings := azureSettings{
		tenantID: "t", clientID: "c", clientSecret: "s", useOIDC: true, oidcToken: "jwt",
		oidcRequestURL: "u", oidcRequestToken: "r", useMSI: true, msiEndpoint: "m", useCLI: true,
	}
	environment := *environments.AzurePublic()
	enabled := func(method string) []string {
		c := settings.credentials(method, environment)
		var on []string
		for name, flag := range map[string]bool{
			"secret": c.EnableAuthenticatingUsingClientSecret, "oidc": c.EnableAuthenticationUsingOIDC,
			"github": c.EnableAuthenticationUsingGitHubOIDC, "ado": c.EnableAuthenticationUsingADOPipelineOIDC,
			"msi": c.EnableAuthenticatingUsingManagedIdentity, "cli": c.EnableAuthenticatingUsingAzureCLI,
			"certificate": c.EnableAuthenticatingUsingClientCertificate,
		} {
			if flag {
				on = append(on, name)
			}
		}
		return on
	}
	for method, want := range map[string]string{
		azureMethodClientSecret: "secret", azureMethodOIDC: "oidc", azureMethodMSI: "msi", azureMethodCLI: "cli",
	} {
		if got := enabled(method); len(got) != 1 || got[0] != want {
			t.Errorf("%s enables %v, want only %s", method, got, want)
		}
	}
	settings.oidcToken = ""
	if got := enabled(azureMethodOIDC); len(got) != 1 || got[0] != "github" {
		t.Errorf("use_oidc with no token enables %v, want only the request-URL exchange", got)
	}
	if c := settings.credentials(azureMethodCLI, environment); c.ClientSecret != "" || c.OIDCAssertionToken != "" {
		t.Error("a method that does not use a secret must not be handed one")
	}
}

// fakeBlobStore is a Blob Storage endpoint holding blobs in memory, keyed by
// "container/blob". It answers as the service was probed to on 2026-11-22: a
// request with no x-ms-version or no bearer token is refused, a stale
// If-Match draws 412 ConditionNotMet, and a Range draws a 206 with a
// Content-Range.
type fakeBlobStore struct {
	mu       sync.Mutex
	blobs    map[string]fakeObject
	requests []*http.Request
}

func (f *fakeBlobStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(context.Background()))
	f.mu.Unlock()

	fail := func(status int, code string) {
		w.Header().Set("x-ms-error-code", code)
		w.WriteHeader(status)
	}
	if r.Header.Get("x-ms-version") == "" {
		fail(http.StatusBadRequest, "MissingRequiredHeader")
		return
	}
	switch r.Header.Get("Authorization") {
	case "Bearer good-token":
	case "Bearer reader-of-nothing":
		fail(http.StatusForbidden, "AuthorizationPermissionMismatch")
		return
	default:
		fail(http.StatusUnauthorized, "InvalidAuthenticationInfo")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "pkgs/redirected.pkg" {
		w.Header().Set("Location", "https://elsewhere.example/app.pkg")
		w.WriteHeader(http.StatusFound)
		return
	}
	blob, ok := f.blobs[name]
	if !ok {
		fail(http.StatusNotFound, "BlobNotFound")
		return
	}
	if want := r.Header.Get("If-Match"); want != "" && want != blob.etag {
		fail(http.StatusPreconditionFailed, "ConditionNotMet")
		return
	}
	body, status := blob.body, http.StatusOK
	if spec := r.Header.Get("Range"); spec != "" {
		start, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(spec, "bytes="), "-"))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
		body, status = body[start:], http.StatusPartialContent
	}
	w.Header().Set("ETag", blob.etag)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, body)
	}
}

func (f *fakeBlobStore) last() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[len(f.requests)-1]
}

// azureResolver returns a Resolver whose Azure driver talks to store and gets
// its token from a stand-in for the credential module, which counts how often
// it is built and asked.
func azureResolver(t *testing.T, store *fakeBlobStore, bearer string) (*Resolver, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	server := httptest.NewServer(store)
	t.Cleanup(server.Close)
	var builds, asks atomic.Int64

	r := New(Config{UserAgent: "terraform-provider-jamfplatform/test"}, nil)
	driver := r.drivers[KindAzureBlob].(*azureDriver)
	driver.getenv = envFrom(nil)
	driver.endpoint = func(account, suffix string) string {
		if account != "account" || suffix != "core.windows.net" {
			t.Errorf("endpoint asked for %s.blob.%s", account, suffix)
		}
		return server.URL
	}
	driver.newToken = func(_ context.Context, _ azureSettings, method string, _ *environments.Environment) (azureToken, error) {
		builds.Add(1)
		if method != azureMethodCLI {
			t.Errorf("method = %s", method)
		}
		return func(context.Context) (string, error) {
			asks.Add(1)
			return bearer, nil
		}, nil
	}
	return r, &builds, &asks
}

func TestAzureStatReportsSizeAndETagMarker(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/releases/My App.pkg": {body: "0123456789", etag: `"0x8DE1234"`}}}
	r, _, _ := azureResolver(t, store, "good-token")

	got, err := r.Stat(context.Background(), "azblob://account/pkgs/releases/My App.pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Size != 10 || got.Marker != `etag:"0x8DE1234"` {
		t.Errorf("attributes = %+v", got)
	}
	req := store.last()
	if req.Method != http.MethodHead || req.URL.EscapedPath() != "/pkgs/releases/My%20App.pkg" {
		t.Errorf("request = %s %s", req.Method, req.URL.EscapedPath())
	}
	if req.Header.Get("x-ms-version") != blobAPIVersion || req.Header.Get("User-Agent") != "terraform-provider-jamfplatform/test" {
		t.Errorf("headers = %v", req.Header)
	}
}

func TestAzureOpenPinsRangesAndReportsTheWholeSize(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "0123456789", etag: `"new"`}}}
	r, _, _ := azureResolver(t, store, "good-token")

	opened, err := r.Open(context.Background(), "azblob://account/pkgs/app.pkg", `etag:"new"`, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(opened.Body)
	_ = opened.Body.Close()
	if string(body) != "0123456789" || opened.Size != 10 || opened.Marker != `etag:"new"` || opened.Filename != "app.pkg" {
		t.Errorf("body=%q size=%d marker=%q filename=%q", body, opened.Size, opened.Marker, opened.Filename)
	}
	if got := store.last().Header.Get("If-Match"); got != `"new"` {
		t.Errorf("If-Match = %q", got)
	}

	if _, err := r.Open(context.Background(), "azblob://account/pkgs/app.pkg", `etag:"old"`, 0); !errors.Is(err, ErrChanged) {
		t.Errorf("a replaced blob must fail with ErrChanged, got %v", err)
	}
	if _, err := r.Open(context.Background(), "azblob://account/pkgs/app.pkg", "version:1", 0); err == nil {
		t.Error("a marker that is not an ETag must be refused")
	}

	driver := r.drivers[KindAzureBlob].(*azureDriver)
	ref, _ := Parse("azblob://account/pkgs/app.pkg")
	ranged, err := driver.Open(context.Background(), ref, `etag:"new"`, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tail, _ := io.ReadAll(ranged.Body)
	_ = ranged.Body.Close()
	if string(tail) != "6789" || ranged.Size != 10 {
		t.Errorf("tail=%q size=%d, want the bytes from the offset and the whole size", tail, ranged.Size)
	}
	if req := store.last(); req.Header.Get("Range") != "bytes=6-" || req.Header.Get("If-Match") != `"new"` {
		t.Errorf("a ranged read must carry Range and stay pinned: %v", req.Header)
	}
}

func TestAzureNamesEachRefusal(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "x", etag: `"e"`}}}

	r, _, _ := azureResolver(t, store, "good-token")
	_, err := r.Stat(context.Background(), "azblob://account/pkgs/absent.pkg")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "BlobNotFound") {
		t.Errorf("want ErrNotFound with the service's code, got %v", err)
	}

	r, _, _ = azureResolver(t, store, "reader-of-nothing")
	_, err = r.Stat(context.Background(), "azblob://account/pkgs/app.pkg")
	if !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "Storage Blob Data Reader") {
		t.Errorf("want ErrAccessDenied naming the role, got %v", err)
	}

	r, _, _ = azureResolver(t, store, "expired-token")
	_, err = r.Open(context.Background(), "azblob://account/pkgs/app.pkg", "", 0)
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("want ErrCredentials for a refused token, got %v", err)
	}
	if strings.Contains(err.Error(), "expired-token") {
		t.Errorf("the error carries the token: %v", err)
	}
}

func TestAzureDoesNotFollowARedirectWithItsToken(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{}}
	r, _, _ := azureResolver(t, store, "good-token")

	_, err := r.Stat(context.Background(), "azblob://account/pkgs/redirected.pkg")
	if err == nil || !strings.Contains(err.Error(), "302") {
		t.Fatalf("want the redirect reported, got %v", err)
	}
	if len(store.requests) != 1 {
		t.Errorf("made %d requests, want the redirect left unfollowed", len(store.requests))
	}
}

func TestAzureBuildsItsCredentialOnceAndAsksForATokenOnEveryRequest(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "x", etag: `"e"`}}}
	r, builds, asks := azureResolver(t, store, "good-token")

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := r.Stat(context.Background(), "azblob://account/pkgs/app.pkg"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()
	if builds.Load() != 1 {
		t.Errorf("the credential was built %d times, want once", builds.Load())
	}
	if asks.Load() != 8 {
		t.Errorf("a token was asked for %d times, want once per request so a resumed read never reuses an old one", asks.Load())
	}
}

func TestAzureHoldsAFailedCredentialBuildAndNamesIt(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "x", etag: `"e"`}}}
	r, _, _ := azureResolver(t, store, "good-token")
	driver := r.drivers[KindAzureBlob].(*azureDriver)
	driver.getenv = envFrom(map[string]string{"ARM_CLIENT_SECRET": "s", "ARM_TENANT_ID": "t"})

	_, err := r.Stat(context.Background(), "azblob://account/pkgs/app.pkg")
	if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), "client_id") {
		t.Fatalf("want a credential error naming client_id, got %v", err)
	}
	if len(store.requests) != 0 {
		t.Error("nothing may be requested without a credential")
	}

	driver.getenv = envFrom(nil)
	if _, err := r.Stat(context.Background(), "azblob://account/pkgs/app.pkg"); !errors.Is(err, ErrCredentials) {
		t.Errorf("a failed build is held for the callers queued behind it, got %v", err)
	}
	shortenBuildFailureHold(t)
	if _, err := r.Stat(context.Background(), "azblob://account/pkgs/app.pkg"); err != nil {
		t.Errorf("a failed build must not be kept past the hold: %v", err)
	}
}

func TestAzureUsesTheSovereignCloudEndpoint(t *testing.T) {
	for name, suffix := range map[string]string{"china": "core.chinacloudapi.cn", "usgovernment": "core.usgovcloudapi.net", "public": "core.windows.net"} {
		store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "x", etag: `"e"`}}}
		server := httptest.NewServer(store)
		r := New(Config{Azure: AzureConfig{Environment: name}}, nil)
		driver := r.drivers[KindAzureBlob].(*azureDriver)
		driver.getenv = envFrom(nil)
		var asked string
		driver.endpoint = func(account, got string) string {
			asked = "https://" + account + ".blob." + got
			return server.URL
		}
		driver.newToken = func(context.Context, azureSettings, string, *environments.Environment) (azureToken, error) {
			return func(context.Context) (string, error) { return "good-token", nil }, nil
		}
		if _, err := r.Stat(context.Background(), "azblob://account/pkgs/app.pkg"); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
		if want := "https://account.blob." + suffix; asked != want {
			t.Errorf("%s: endpoint = %s, want %s", name, asked, want)
		}
		server.Close()
	}
}

func TestResolveAzureSettingsTrimsAndRefusesAnEmptyFile(t *testing.T) {
	got, err := resolveAzureSettings(AzureConfig{TenantID: "  t  "}, envFrom(map[string]string{"ARM_CLIENT_ID": " c ", "ARM_CLIENT_SECRET": "   "}), noFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.tenantID != "t" || got.clientID != "c" || got.clientSecret != "" {
		t.Errorf("values must be trimmed as the credential module trims them: %+v", got)
	}
	if method, _ := got.method(); method != azureMethodCLI {
		t.Errorf("a blank secret is no secret, got method %q", method)
	}

	empty := func(string) ([]byte, error) { return []byte("  \n"), nil }
	_, err = resolveAzureSettings(AzureConfig{ClientSecretFile: "/run/secret"}, envFrom(nil), empty)
	if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), "empty") {
		t.Errorf("an empty secret file must be an error, not a fall through to the Azure CLI: %v", err)
	}
}

func TestResolveAzureSettingsNotesTheBackendMethodsItDoesNotSupport(t *testing.T) {
	got, err := resolveAzureSettings(AzureConfig{}, envFrom(map[string]string{
		"ARM_CLIENT_CERTIFICATE_PATH": "/cert.pfx", "ARM_USE_AKS_WORKLOAD_IDENTITY": "true",
		"ARM_OIDC_AZURE_SERVICE_CONNECTION_ID": "abc",
	}), noFiles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.unsupported != "ARM_CLIENT_CERTIFICATE_PATH, ARM_USE_AKS_WORKLOAD_IDENTITY" || !got.adoPipeline || got.cliExplicit {
		t.Errorf("got %+v", got)
	}
	if _, err := got.method(); !errors.Is(err, ErrCredentials) {
		t.Errorf("want the unsupported method refused, got %v", err)
	}
}

func TestAzureAsksForTheStoredBytesUncompressed(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "x", etag: `"e"`}}}
	r, _, _ := azureResolver(t, store, "good-token")

	opened, err := r.Open(context.Background(), "azblob://account/pkgs/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = opened.Body.Close()
	if got := store.last().Header.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("Accept-Encoding = %q, want identity so a compressed blob keeps its declared length", got)
	}
}

func TestAzureEscapesEveryCharacterOfABlobName(t *testing.T) {
	name := "dir with space/a#b%41+c;d=e&f/\u00e9t\u00e9.pkg"
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/" + name: {body: "x", etag: `"e"`}}}
	r, _, _ := azureResolver(t, store, "good-token")

	if _, err := r.Stat(context.Background(), "azblob://account/pkgs/"+name); err != nil {
		t.Fatalf("the blob name did not arrive as written: %v", err)
	}
}

func TestAzureHoldsATokenFailureSoEachPackageDoesNotPayForIt(t *testing.T) {
	store := &fakeBlobStore{blobs: map[string]fakeObject{"pkgs/app.pkg": {body: "x", etag: `"e"`}}}
	r, builds, _ := azureResolver(t, store, "good-token")
	driver := r.drivers[KindAzureBlob].(*azureDriver)
	var asks atomic.Int64
	driver.newToken = func(context.Context, azureSettings, string, *environments.Environment) (azureToken, error) {
		builds.Add(1)
		return func(context.Context) (string, error) {
			asks.Add(1)
			return "", fmt.Errorf("%w: the endpoint gave no token", ErrCredentials)
		}, nil
	}

	for range 3 {
		if _, err := r.Stat(context.Background(), "azblob://account/pkgs/app.pkg"); !errors.Is(err, ErrCredentials) {
			t.Fatalf("want ErrCredentials, got %v", err)
		}
	}
	if asks.Load() != 1 {
		t.Errorf("a token was asked for %d times, want the failure held after the first", asks.Load())
	}
}

func TestAzureTokenRequestCutShortByItsCallerIsNotACredentialFailure(t *testing.T) {
	hang := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(hang.Close)
	settings := azureSettings{environment: "public", useMSI: true, msiEndpoint: hang.URL}
	token, err := newAzureToken(context.Background(), settings, azureMethodMSI, environments.AzurePublic())
	if err != nil {
		t.Fatalf("building the managed identity credential: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = token(ctx)
	if err == nil || errors.Is(err, ErrCredentials) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a caller's deadline is not a credential failure, got %v", err)
	}

	previous := managedIdentityTimeout
	managedIdentityTimeout = 200 * time.Millisecond
	t.Cleanup(func() { managedIdentityTimeout = previous })
	_, err = token(context.Background())
	if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), "managed identity endpoint") {
		t.Fatalf("the driver's own deadline is the managed identity failure, got %v", err)
	}
}
