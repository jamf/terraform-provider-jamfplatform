// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package testhelpers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// NewMockClient creates a test HTTP server with automatic OAuth2 token handling
// and returns a configured *jamfplatform.Client pointed at the mock server.
// The provided handler receives all non-token requests.
// The server is automatically closed when the test completes.
func NewMockClient(t *testing.T, handler http.Handler) *jamfplatform.Client {
	t.Helper()

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		handler.ServeHTTP(w, r)
	})

	server := httptest.NewServer(wrapped)
	t.Cleanup(server.Close)

	// Disable the SDK's automatic retry-on-transient-failure: a handler that
	// deliberately keeps returning e.g. 500/502/503 to exercise a caller's
	// error-handling path would otherwise hang for the production 1s-60s
	// backoff window on every run.
	c := jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0))
	return c
}

// RespondJSON writes a JSON response with the given status code and body.
func RespondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}
