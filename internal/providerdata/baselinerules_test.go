// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// newBaselineRulesServer serves the token exchange and delegates every other request to handler.
func newBaselineRulesServer(t *testing.T, handler http.HandlerFunc) *compliancebenchmarks.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return compliancebenchmarks.New(jamfplatform.NewClient(srv.URL, "id", "secret", jamfplatform.WithRetryPolicy(0, 0, 0), jamfplatform.WithMinRequestInterval(0)))
}

func writeBaselineRules(w http.ResponseWriter, ruleID string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"availableOsVersions":[{"osType":"MAC_OS","osVersion":26}],"rules":[{"id":%q}],"sources":[]}`, ruleID)
}

func TestBaselineRulesCache_CollapsesConcurrentReads(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	cache := NewBaselineRulesCache(newBaselineRulesServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		writeBaselineRules(w, r.URL.Query().Get("baseline-id"))
	}))

	var wg sync.WaitGroup
	results := make([]*compliancebenchmarks.SourcedRules, 8)
	for i := range results {
		wg.Go(func() {
			rules, err := cache.Rules(context.Background(), "cis_lvl1")
			if err != nil {
				t.Errorf("Rules: %v", err)
			}
			results[i] = rules
		})
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := hits.Load(); got != 1 {
		t.Errorf("requests for 8 concurrent callers on one baseline = %d, want 1", got)
	}
	for i, rules := range results {
		if rules == nil || len(rules.Rules) != 1 || rules.Rules[0].ID != "cis_lvl1" {
			t.Errorf("caller %d got %+v", i, rules)
		}
	}
}

func TestBaselineRulesCache_KeysByBaseline(t *testing.T) {
	var hits atomic.Int32
	cache := NewBaselineRulesCache(newBaselineRulesServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		writeBaselineRules(w, r.URL.Query().Get("baseline-id"))
	}))
	ctx := context.Background()

	for _, id := range []string{"cis_lvl1", "cis_lvl2", "cis_lvl1", "cis_lvl2"} {
		rules, err := cache.Rules(ctx, id)
		if err != nil {
			t.Fatalf("Rules(%q): %v", id, err)
		}
		if rules.Rules[0].ID != id {
			t.Errorf("Rules(%q) returned the answer for %q", id, rules.Rules[0].ID)
		}
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("requests for two baselines read twice each = %d, want 2", got)
	}
}

func TestBaselineRulesCache_DoesNotStoreFailures(t *testing.T) {
	var hits atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	cache := NewBaselineRulesCache(newBaselineRulesServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeBaselineRules(w, "cis_lvl1")
	}))
	ctx := context.Background()

	for range 2 {
		if _, err := cache.Rules(ctx, "cis_lvl1"); err == nil {
			t.Fatal("expected the read to fail")
		}
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("requests after two failed reads = %d, want 2: a failure must not be stored", got)
	}

	fail.Store(false)
	if _, err := cache.Rules(ctx, "cis_lvl1"); err != nil {
		t.Fatalf("read after recovery: %v", err)
	}
	if _, err := cache.Rules(ctx, "cis_lvl1"); err != nil {
		t.Fatalf("repeat read: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("requests after recovery and a repeat = %d, want 3", got)
	}
}

func TestBaselineRulesCache_CapsConcurrency(t *testing.T) {
	var inFlight, peak atomic.Int32
	cache := NewBaselineRulesCache(newBaselineRulesServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		writeBaselineRules(w, r.URL.Query().Get("baseline-id"))
	}))

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if _, err := cache.Rules(context.Background(), fmt.Sprintf("baseline_%d", i)); err != nil {
				t.Errorf("Rules %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	if got := peak.Load(); got > BaselineRulesMaxConcurrent {
		t.Errorf("peak concurrent requests = %d, want at most %d", got, BaselineRulesMaxConcurrent)
	}
	if got := peak.Load(); got < 2 {
		t.Errorf("peak concurrent requests = %d; distinct baselines should run in parallel", got)
	}
}

func TestBaselineRulesCache_NilReceiverReportsUnavailable(t *testing.T) {
	var cache *BaselineRulesCache
	if _, err := cache.Rules(context.Background(), "cis_lvl1"); err == nil {
		t.Fatal("a nil cache must report that the read is unavailable")
	}
	var data *Data
	if data.BaselineRulesCache() != nil {
		t.Error("a nil Data must yield a nil cache")
	}
}

func TestData_BaselineRulesCacheIsSharedPerInstance(t *testing.T) {
	d := &Data{Client: jamfplatform.NewClient("http://127.0.0.1:0", "id", "secret")}
	first, second := d.BaselineRulesCache(), d.BaselineRulesCache()
	if first == nil || first != second {
		t.Error("every benchmark in one provider instance must receive the same cache")
	}
}
