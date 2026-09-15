// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// stickySessionStub serves a token endpoint and two API reads, sets Jamf Cloud's
// session cookie on the first response, and records what each request arrived
// with.
func stickySessionStub(t *testing.T) (baseURL string, seen func() []*http.Request) {
	t.Helper()

	var mu sync.Mutex
	var requests []*http.Request

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/token", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Clone(context.Background()))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-abc123",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}); err != nil {
			t.Errorf("writing token response: %v", err)
		}
	})
	mux.HandleFunc("/api/pro/v1/buildings", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Clone(context.Background()))
		mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "jpro-ingress", Value: "node-7", Path: "/"})
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"totalCount":0,"results":[]}`)); err != nil {
			t.Errorf("writing api response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL, func() []*http.Request {
		mu.Lock()
		defer mu.Unlock()
		return requests
	}
}

// TestCustomHeadersKeepSessionPinning pins the guarantee the Cookie refusal in
// resolveCustomHeaders rests on: configuring custom headers must not cost the
// session cookie Jamf Cloud uses to keep this client on one application node.
//
// The client rebuilds its transport chain when custom headers are configured,
// and the cookie jar hangs off the client being rebuilt rather than off the
// transport. Nothing upstream covers the jar, so a refactor there could drop it
// while every header assertion stayed green — and the symptom would be a read
// racing a write on some unrelated resource, never traced back to here. That is
// also the argument for refusing a Cookie custom header outright rather than
// warning about it, so the two belong in one place.
func TestCustomHeadersKeepSessionPinning(t *testing.T) {
	t.Parallel()

	baseURL, seen := stickySessionStub(t)

	client := jamfplatform.NewClient(baseURL, "cid", "csecret",
		jamfplatform.WithMinRequestInterval(0),
		jamfplatform.WithTenantID("00000000-0000-0000-0000-000000000000"),
		jamfplatform.WithHeaders(http.Header{"X-Proxy-Route": {"eu-west"}}),
		jamfplatform.WithAuthorizationHeaderName("X-Jamf-Authorization"),
	)

	ctx := context.Background()
	for i := range 2 {
		var out map[string]any
		if err := client.Transport().Do(ctx, http.MethodGet, "/api/pro/v1/buildings", nil, &out); err != nil {
			t.Fatalf("read %d: %v", i+1, err)
		}
	}

	requests := seen()
	var apiReads []*http.Request
	for _, r := range requests {
		if r.URL.Path == "/api/pro/v1/buildings" {
			apiReads = append(apiReads, r)
		}
	}
	if len(apiReads) != 2 {
		t.Fatalf("api reads = %d, want 2", len(apiReads))
	}

	if got := apiReads[0].Header.Get("Cookie"); got != "" {
		t.Errorf("first read Cookie = %q, want none — nothing has set a session cookie yet", got)
	}
	if _, err := apiReads[1].Cookie("jpro-ingress"); err != nil {
		t.Errorf("second read carried no jpro-ingress cookie (%v) — configuring custom headers has cost "+
			"session pinning, so a read taken straight after a write can land on a node that has not "+
			"caught up", err)
	}

	if got := apiReads[1].Header.Get("X-Proxy-Route"); got != "eu-west" {
		t.Errorf("second read X-Proxy-Route = %q, want eu-west", got)
	}
	if got := apiReads[1].Header.Get("X-Jamf-Authorization"); got != "Bearer tok-abc123" {
		t.Errorf("second read X-Jamf-Authorization = %q, want the relocated credential", got)
	}
	if got := apiReads[1].Header.Get("Authorization"); got != "" {
		t.Errorf("second read Authorization = %q, want it vacated for the proxy's own credential", got)
	}
	if got := apiReads[1].Header.Get("X-Tenant-Id"); got != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("second read X-Tenant-Id = %q, want the configured scope", got)
	}
}
