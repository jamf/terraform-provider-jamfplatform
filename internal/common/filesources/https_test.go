// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

// seenRequest is what one server received.
type seenRequest struct {
	server        string
	path          string
	authorization string
	apiKey        string
	userAgent     string
	referer       string
}

// testNetwork is a set of local servers reachable under made-up names, so a
// test can redirect between hosts, ports and schemes. Every request any of
// them receives is recorded.
type testNetwork struct {
	t        *testing.T
	mu       sync.Mutex
	seen     []seenRequest
	addrs    map[string]string
	contents string
}

// newTestNetwork starts the servers:
//
//	files.test:443       https, the host the tests download from
//	files.test:8443      https, the same hostname on another port
//	sub.files.test:443   https, a subdomain of it
//	cdn.test:443         https, an unrelated host
//	files.test:80        plain http
//
// Each serves /app.pkg, and the redirect paths the handler lists.
func newTestNetwork(t *testing.T) *testNetwork {
	t.Helper()
	n := &testNetwork{t: t, addrs: map[string]string{}, contents: "package bytes"}
	for name, secure := range map[string]bool{
		"files.test:443": true, "files.test:8443": true, "sub.files.test:443": true, "cdn.test:443": true, "files.test:80": false,
		"xn--files-8fd.test:443": true,
	} {
		server := httptest.NewUnstartedServer(n.handler(name))
		if secure {
			server.StartTLS()
		} else {
			server.Start()
		}
		t.Cleanup(server.Close)
		n.addrs[name] = server.Listener.Addr().String()
	}
	return n
}

func (n *testNetwork) handler(name string) http.Handler {
	redirects := map[string]string{
		"/to-self":      "/app.pkg",
		"/to-cdn":       "https://cdn.test/app.pkg?X-Signature=signed-secret",
		"/to-subdomain": "https://sub.files.test/app.pkg",
		"/to-port":      "https://files.test:8443/app.pkg",
		"/to-http":      "http://files.test/app.pkg",
		"/to-cdn-back":  "https://cdn.test/back",
		"/back":         "https://files.test/app.pkg",
		"/loop":         "/loop",
		"/to-lookalike": "https://f%C4%B0les.test/app.pkg",
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.mu.Lock()
		n.seen = append(n.seen, seenRequest{
			server: name, path: r.URL.Path, authorization: r.Header.Get("Authorization"),
			apiKey: r.Header.Get("X-Api-Key"), userAgent: r.Header.Get("User-Agent"), referer: r.Header.Get("Referer"),
		})
		n.mu.Unlock()
		if target, ok := redirects[r.URL.Path]; ok {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		switch r.URL.Path {
		case "/app.pkg":
			_, _ = io.WriteString(w, n.contents)
		case "/chunked.pkg":
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, n.contents)
		case "/denied.pkg":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// transport dials the made-up names at the local servers and accepts their
// test certificates.
func (n *testNetwork) transport() http.RoundTripper {
	return &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local test servers with a test certificate
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			target, ok := n.addrs[addr]
			if !ok {
				return nil, errors.New("no test server at " + addr)
			}
			return (&net.Dialer{}).DialContext(ctx, network, target)
		},
	}
}

func (n *testNetwork) resolver(credentials ...HTTPSCredential) *Resolver {
	return New(Config{HTTPS: credentials, UserAgent: "terraform-provider-jamfplatform/test", HTTPTransport: n.transport()}, nil)
}

// last returns the final request the network saw, which is the one that
// served the file after any redirects.
func (n *testNetwork) last() seenRequest {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.seen) == 0 {
		n.t.Fatal("no request reached any server")
	}
	return n.seen[len(n.seen)-1]
}

func (n *testNetwork) all() []seenRequest {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]seenRequest(nil), n.seen...)
}

func download(t *testing.T, r *Resolver, src string) string {
	t.Helper()
	opened, err := r.OpenURL(context.Background(), src, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = opened.Body.Close() }()
	body, err := io.ReadAll(opened.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return string(body)
}

// encodedBasicForFiles is basicForFiles as it travels in an Authorization
// header. It is computed, not written out, so the secret scanner does not
// take the encoded form for a real credential.
var encodedBasicForFiles = base64.StdEncoding.EncodeToString([]byte("ci:files-password"))

var (
	bearerForFiles = HTTPSCredential{Host: "files.test", BearerToken: "files-token"}
	headerForFiles = HTTPSCredential{Host: "files.test", HeaderName: "X-Api-Key", HeaderValue: "files-key"}
	basicForFiles  = HTTPSCredential{Host: "FILES.test:443", Username: "ci", Password: "files-password"}
	bearerForCDN   = HTTPSCredential{Host: "cdn.test", BearerToken: "cdn-token"}
)

func TestAuthenticated(t *testing.T) {
	r := New(Config{HTTPS: []HTTPSCredential{bearerForFiles, {Host: "[2001:db8::1]:8443", BearerToken: "t"}}}, nil)
	cases := map[string]bool{
		"https://files.test/app.pkg":           true,
		"https://FILES.test/app.pkg":           true,
		"https://files.test:443/app.pkg":       true,
		"https://files.test./app.pkg":          true,
		"https://files.test/app.pkg?sig=abc":   true,
		"https://[2001:DB8::1]:8443/app.pkg":   true,
		"https://[2001:db8::1]/app.pkg":        false,
		"https://files.test:8443/app.pkg":      false,
		"https://sub.files.test/app.pkg":       false,
		"https://files.test.evil.test/app.pkg": false,
		"https://evil.test/files.test/app.pkg": false,
		"https://files.test@evil.test/app.pkg": false,
		"http://files.test/app.pkg":            false,
		"http://files.test:443/app.pkg":        false,
		"HTTPS://files.test/app.pkg":           false,
		"s3://files.test/app.pkg":              false,
		"/tmp/files.test/app.pkg":              false,
		"":                                     false,
		"https://f\u0130les.test/app.pkg":      false,
		"https://f%C4%B0les.test/app.pkg":      false,
		"https://F%C4%B0LES.TEST/app.pkg":      false,
		"https://\uff46iles.test/app.pkg":      false,
		"https://files.test:0443/app.pkg":      false,
	}
	for src, want := range cases {
		if got := r.Authenticated(src); got != want {
			t.Errorf("Authenticated(%q) = %v, want %v", src, got, want)
		}
	}
	var unconfigured *Resolver
	if unconfigured.Authenticated("https://files.test/app.pkg") {
		t.Error("a nil resolver has no credentials")
	}
	if New(Config{}, nil).Authenticated("https://files.test/app.pkg") {
		t.Error("a resolver with no https entries has no credentials")
	}
}

func TestOpenURLSendsEachKindOfCredentialToItsHost(t *testing.T) {
	cases := map[string]struct {
		credential    HTTPSCredential
		authorization string
		apiKey        string
	}{
		"bearer token": {bearerForFiles, "Bearer files-token", ""},
		"basic":        {basicForFiles, "Basic " + encodedBasicForFiles, ""},
		"header":       {headerForFiles, "", "files-key"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			network := newTestNetwork(t)
			body := download(t, network.resolver(tc.credential), "https://files.test/app.pkg")

			got := network.last()
			if body != "package bytes" || got.server != "files.test:443" {
				t.Fatalf("body=%q served by %s", body, got.server)
			}
			if got.authorization != tc.authorization || got.apiKey != tc.apiKey {
				t.Errorf("Authorization=%q X-Api-Key=%q", got.authorization, got.apiKey)
			}
			if got.userAgent != "terraform-provider-jamfplatform/test" {
				t.Errorf("User-Agent = %q", got.userAgent)
			}
		})
	}
}

func TestOpenURLRefusesAHostWithNoCredential(t *testing.T) {
	network := newTestNetwork(t)
	r := network.resolver(bearerForFiles)

	for _, src := range []string{"https://cdn.test/app.pkg", "https://files.test:8443/app.pkg", "http://files.test/app.pkg"} {
		if _, err := r.OpenURL(context.Background(), src, 0); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("OpenURL(%q): want ErrNotConfigured, got %v", src, err)
		}
	}
	if len(network.all()) != 0 {
		t.Error("nothing may be requested from a host that has no credential")
	}
}

func TestRedirectsNeverCarryACredentialToAnotherHost(t *testing.T) {
	cases := map[string]struct {
		path        string
		credentials []HTTPSCredential
		servedBy    string
		wantAuth    string
		wantAPIKey  string
	}{
		"same host keeps it":                                         {"/to-self", []HTTPSCredential{bearerForFiles}, "files.test:443", "Bearer files-token", ""},
		"another host loses the bearer token":                        {"/to-cdn", []HTTPSCredential{bearerForFiles}, "cdn.test:443", "", ""},
		"another host loses the custom header":                       {"/to-cdn", []HTTPSCredential{headerForFiles}, "cdn.test:443", "", ""},
		"another host loses basic":                                   {"/to-cdn", []HTTPSCredential{basicForFiles}, "cdn.test:443", "", ""},
		"a subdomain is another host":                                {"/to-subdomain", []HTTPSCredential{bearerForFiles}, "sub.files.test:443", "", ""},
		"a subdomain loses the custom header":                        {"/to-subdomain", []HTTPSCredential{headerForFiles}, "sub.files.test:443", "", ""},
		"another port is another host":                               {"/to-port", []HTTPSCredential{bearerForFiles}, "files.test:8443", "", ""},
		"plain http loses the bearer token":                          {"/to-http", []HTTPSCredential{bearerForFiles}, "files.test:80", "", ""},
		"plain http loses the custom header":                         {"/to-http", []HTTPSCredential{headerForFiles}, "files.test:80", "", ""},
		"a host with its own entry gets its own":                     {"/to-cdn", []HTTPSCredential{bearerForFiles, bearerForCDN}, "cdn.test:443", "Bearer cdn-token", ""},
		"its own and not the first host's header":                    {"/to-cdn", []HTTPSCredential{headerForFiles, bearerForCDN}, "cdn.test:443", "Bearer cdn-token", ""},
		"back to the first host it is sent again":                    {"/to-cdn-back", []HTTPSCredential{bearerForFiles}, "files.test:443", "Bearer files-token", ""},
		"back to the first host with the header set":                 {"/to-cdn-back", []HTTPSCredential{headerForFiles}, "files.test:443", "", "files-key"},
		"a name that lower-cases to the host loses the bearer token": {"/to-lookalike", []HTTPSCredential{bearerForFiles}, "xn--files-8fd.test:443", "", ""},
		"a name that lower-cases to the host loses the header":       {"/to-lookalike", []HTTPSCredential{headerForFiles}, "xn--files-8fd.test:443", "", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			network := newTestNetwork(t)
			body := download(t, network.resolver(tc.credentials...), "https://files.test"+tc.path)

			got := network.last()
			if body != "package bytes" || got.server != tc.servedBy {
				t.Fatalf("body=%q served by %s, want %s", body, got.server, tc.servedBy)
			}
			if got.authorization != tc.wantAuth || got.apiKey != tc.wantAPIKey {
				t.Errorf("the serving host received Authorization=%q X-Api-Key=%q, want %q and %q",
					got.authorization, got.apiKey, tc.wantAuth, tc.wantAPIKey)
			}
			for _, seen := range network.all() {
				if seen.server == "files.test:443" {
					continue
				}
				for _, secret := range []string{"files-token", "files-key", encodedBasicForFiles} {
					if strings.Contains(seen.authorization, secret) || strings.Contains(seen.apiKey, secret) {
						t.Errorf("%s received the credential of files.test", seen.server)
					}
				}
			}
		})
	}
}

func TestOpenURLStopsARedirectLoopWithoutLeakingTheSource(t *testing.T) {
	network := newTestNetwork(t)

	_, err := network.resolver(bearerForFiles).OpenURL(context.Background(), "https://files.test/loop?token=query-secret", 0)
	if err == nil {
		t.Fatal("want an error")
	}
	if got := len(network.all()); got > maxRedirects+2 {
		t.Errorf("followed %d requests", got)
	}
	for _, secret := range []string{"query-secret", "files-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the error carries %q: %v", secret, err)
		}
	}
}

func TestOpenURLNamesARefusedCredential(t *testing.T) {
	network := newTestNetwork(t)

	_, err := network.resolver(bearerForFiles).OpenURL(context.Background(), "https://files.test/denied.pkg?sig=query-secret", 0)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	if !strings.Contains(err.Error(), "files.test") {
		t.Errorf("the error must name the host that refused: %v", err)
	}
	for _, secret := range []string{"query-secret", "files-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("the error carries %q: %v", secret, err)
		}
	}

	_, err = network.resolver(bearerForFiles).OpenURL(context.Background(), "https://files.test/absent.pkg", 0)
	if err == nil || errors.Is(err, ErrUnauthorized) || !strings.Contains(err.Error(), "404") {
		t.Errorf("want the status reported, got %v", err)
	}
}

func TestOpenURLReportsTheSizeAndFilename(t *testing.T) {
	network := newTestNetwork(t)
	r := network.resolver(bearerForFiles)

	opened, err := r.OpenURL(context.Background(), "https://files.test/app.pkg", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = opened.Body.Close()
	if opened.Size != 13 || opened.Filename != "app.pkg" || opened.Marker != "" {
		t.Errorf("opened = %+v", opened.Attributes)
	}

	opened, err = r.OpenURL(context.Background(), "https://files.test/chunked.pkg", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = opened.Body.Close()
	if opened.Size != -1 {
		t.Errorf("Size = %d, want -1 for a response that declares none", opened.Size)
	}

	opened, err = r.OpenURL(context.Background(), "https://files.test/to-cdn", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = opened.Body.Close()
	if opened.Filename != "app.pkg" {
		t.Errorf("Filename = %q, want the name the redirect ended at", opened.Filename)
	}

	if _, err := r.OpenURL(context.Background(), "https://files.test/app.pkg", 12); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a declared size over the cap must be refused, got %v", err)
	}
}

func TestStageURL(t *testing.T) {
	network := newTestNetwork(t)
	r := network.resolver(bearerForFiles)

	for _, path := range []string{"/app.pkg", "/chunked.pkg"} {
		staged, cleanup, err := r.StageURL(context.Background(), "https://files.test"+path, 0)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", path, err)
		}
		body, _ := io.ReadAll(staged.File)
		if string(body) != "package bytes" || staged.Size != 13 {
			t.Errorf("%s: body=%q size=%d", path, body, staged.Size)
		}
		cleanup()
		if _, err := os.Stat(staged.File.Name()); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: cleanup must remove the file", path)
		}
	}

	before := temporaryFiles(t)
	for _, path := range []string{"/app.pkg", "/chunked.pkg"} {
		_, cleanup, err := r.StageURL(context.Background(), "https://files.test"+path, 12)
		cleanup()
		if !errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: a download over the cap must be refused, got %v", path, err)
		}
	}
	staged, cleanup, err := r.StageURL(context.Background(), "https://files.test/chunked.pkg", 13)
	if err != nil {
		t.Fatalf("a download exactly at the cap must stage: %v", err)
	}
	_ = staged
	cleanup()
	if after := temporaryFiles(t); after != before {
		t.Errorf("%d temporary file(s) left behind", after-before)
	}
}

func TestRedirectDoesNotTellTheNextHostTheURLItCameFrom(t *testing.T) {
	network := newTestNetwork(t)
	download(t, network.resolver(bearerForFiles), "https://files.test/to-cdn?token=query-secret")

	for _, seen := range network.all() {
		if seen.referer != "" {
			t.Errorf("%s received Referer %q", seen.server, seen.referer)
		}
	}
}

func TestHostKey(t *testing.T) {
	cases := map[string]string{
		"https://files.test/a":                "files.test:443",
		"https://FILES.TEST./a":               "files.test:443",
		"https://files.test:8443/a":           "files.test:8443",
		"https://[2001:DB8::1]/a":             "[2001:db8::1]:443",
		"https://10.0.0.5/a":                  "10.0.0.5:443",
		"https://10.0.0.5./a":                 "",
		"https://f\u0130les.test/a":           "",
		"https://f%C4%B0les.test/a":           "",
		"https://files.test\u3002evil.test/a": "",
	}
	for src, want := range cases {
		u, err := url.Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		if got := hostKey(u); got != want {
			t.Errorf("hostKey(%s) = %q, want %q", src, got, want)
		}
	}
}
