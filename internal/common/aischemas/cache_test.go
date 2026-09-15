// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package aischemas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// schemaServer stubs the tool-schema route. The seam is the HTTP boundary rather than an injected
// interface, matching the rest of this repo's cache tests: the Cache holds a concrete SDK client,
// and an interface introduced only for a test would be a bigger change than the behaviour it pins.
//
// handle is called for every schema request with the request's URL path, and returns the body to
// write; blocking inside it holds that one fetch open, which is how the concurrency test proves two
// keys are in flight at once.
func schemaServer(t *testing.T, handle func(path string) (int, string)) *Cache {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		status, body := handle(r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return NewCache(jamfplatform.NewClient(server.URL, "test-id", "test-secret",
		jamfplatform.WithRetryPolicy(0, 0, 0),
		jamfplatform.WithMinRequestInterval(0),
	))
}

// schemaBody renders a tool-schema response carrying a minimal usable schema.
func schemaBody(toolID, schemaVersion string) string {
	body, err := json.Marshal(map[string]any{
		"toolId":        toolID,
		"schemaVersion": schemaVersion,
		"schema":        json.RawMessage(`{"type":"object","properties":{"verbose":{"type":"boolean"}}}`),
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

// TestDocumentSharesOneFetchPerKey pins the memoisation the type exists for: the Claude Code schema
// is 184 KB, and a plan holding several policies on one schema version must pay for it once.
func TestDocumentSharesOneFetchPerKey(t *testing.T) {
	var fetches atomic.Int64
	cache := schemaServer(t, func(path string) (int, string) {
		fetches.Add(1)
		return http.StatusOK, schemaBody("com.example.tool", "2026-01-01")
	})

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := cache.Document(context.Background(), "com.example.tool", "2026-01-01"); err != nil {
				t.Errorf("Document: %v", err)
			}
		})
	}
	wg.Wait()

	if _, err := cache.Document(context.Background(), "com.example.tool", "2026-01-01"); err != nil {
		t.Fatalf("Document: %v", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Errorf("fetched %d times, want 1", got)
	}
}

// TestDocumentFetchesDistinctKeysConcurrently pins that the lock covers the map and not the fetch.
// ModifyPlan runs concurrently for independent resource instances at Terraform's default
// parallelism of ten, so a lock held across the round-trip makes every unrelated policy queue
// behind the first one's 184 KB decode. With one mutex held across the fetch this test times out:
// the second request never reaches the server.
func TestDocumentFetchesDistinctKeysConcurrently(t *testing.T) {
	arrived := make(chan string, 2)
	release := make(chan struct{})
	cache := schemaServer(t, func(path string) (int, string) {
		arrived <- path
		<-release
		return http.StatusOK, schemaBody("com.example.tool", "2026-01-01")
	})

	var wg sync.WaitGroup
	for _, version := range []string{"2026-01-01", "2026-05-19"} {
		wg.Go(func() {
			if _, err := cache.Document(context.Background(), "com.example.tool", version); err != nil {
				t.Errorf("Document(%s): %v", version, err)
			}
		})
	}

	seen := map[string]bool{}
	for range 2 {
		select {
		case path := <-arrived:
			seen[path] = true
		case <-timeout():
			close(release)
			wg.Wait()
			t.Fatalf("only %d of 2 distinct schema fetches reached the server: one mutex is serialising unrelated keys", len(seen))
		}
	}
	close(release)
	wg.Wait()

	if len(seen) != 2 {
		t.Errorf("both fetches must be for distinct paths, saw %v", seen)
	}
}

// TestDocumentDoesNotCacheAFailure pins the invariant the type's doc states: a transient blip in the
// first policy's plan must not silence validation for every later one, so the next caller retries.
func TestDocumentDoesNotCacheAFailure(t *testing.T) {
	var fetches atomic.Int64
	cache := schemaServer(t, func(path string) (int, string) {
		if fetches.Add(1) == 1 {
			return http.StatusInternalServerError, `{"message":"transient"}`
		}
		return http.StatusOK, schemaBody("com.example.tool", "2026-01-01")
	})

	if _, err := cache.Document(context.Background(), "com.example.tool", "2026-01-01"); err == nil {
		t.Fatal("the first read must surface the failure")
	}
	document, err := cache.Document(context.Background(), "com.example.tool", "2026-01-01")
	if err != nil {
		t.Fatalf("the retry must succeed, got: %v", err)
	}
	if document == nil {
		t.Fatal("the retry must return a document")
	}
	if got := fetches.Load(); got != 2 {
		t.Errorf("fetched %d times, want 2 — a failure must not be cached", got)
	}
}

// TestDocumentReportsAnUnusableSchema pins that a served body this package cannot read reaches the
// caller as an error naming the parse, not as a Document that accepts everything.
func TestDocumentReportsAnUnusableSchema(t *testing.T) {
	cache := schemaServer(t, func(path string) (int, string) {
		return http.StatusOK, `{"toolId":"com.example.tool","schemaVersion":"2026-01-01","schema":false}`
	})

	_, err := cache.Document(context.Background(), "com.example.tool", "2026-01-01")
	if err == nil {
		t.Fatal("a `false` schema document must reach the caller as an error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("the error must name the parse: %v", err)
	}
}

// TestNoticeOnceFiresExactlyOnce pins the once-per-plan notice latch. A configuration holding twenty
// policies must report an unreadable catalogue once, not twenty times.
func TestNoticeOnceFiresExactlyOnce(t *testing.T) {
	cache := &Cache{}
	if !cache.NoticeOnce() {
		t.Fatal("the first notice must fire")
	}
	for range 3 {
		if cache.NoticeOnce() {
			t.Fatal("the notice must fire only once per cache")
		}
	}

	var nilCache *Cache
	if nilCache.NoticeOnce() {
		t.Error("a nil cache has nothing to report")
	}
}
