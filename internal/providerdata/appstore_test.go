// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/appstore"
)

// appStoreBody is a minimal lookup answer for id from the US storefront.
func appStoreBody(id string) string {
	return fmt.Sprintf(`{"meta":{"storefront":{"cc":"US"},"language":{"tag":"en-us"}},"results":{%q:{"id":%q,"bundleId":"com.example.app%s","name":"App %s","offers":[{"price":0,"version":{"display":"1.0"}}]}}}`, id, id, id, id)
}

const appStoreMiss = `{"meta":{"storefront":{"cc":"US"}},"results":{}}`

func TestAppStoreCache_CollapsesConcurrentLookups(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		_, _ = w.Write([]byte(appStoreBody(r.URL.Query().Get("id"))))
	}))
	t.Cleanup(srv.Close)

	cache := NewAppStoreCache(appstore.New(appstore.WithEndpoint(srv.URL)))
	q := appstore.Query{ID: "1", Platform: appstore.PlatformMac, Country: "US"}

	var wg sync.WaitGroup
	results := make([]*appstore.App, 8)
	for i := range results {
		wg.Go(func() {
			app, err := cache.Lookup(context.Background(), q)
			if err != nil {
				t.Errorf("Lookup: %v", err)
			}
			results[i] = app
		})
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := hits.Load(); got != 1 {
		t.Errorf("requests = %d, want 1 for eight callers asking for one query", got)
	}
	for i, app := range results {
		if app == nil || app.Name != "App 1" {
			t.Errorf("caller %d got %+v", i, app)
		}
	}
}

func TestAppStoreCache_StoresMissesButNotFailures(t *testing.T) {
	var hits atomic.Int32
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(appStoreMiss))
	}))
	t.Cleanup(srv.Close)
	cache := NewAppStoreCache(appstore.New(appstore.WithEndpoint(srv.URL)))
	ctx := context.Background()

	miss := appstore.Query{ID: "2", Platform: appstore.PlatformMac, Country: "US"}
	for range 3 {
		var nf *appstore.NotFoundError
		if _, err := cache.Lookup(ctx, miss); !errors.As(err, &nf) {
			t.Fatalf("err = %v, want *NotFoundError", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("requests for a repeated miss = %d, want 1: a miss is a definitive answer", got)
	}

	fail.Store(true)
	broken := appstore.Query{ID: "3", Platform: appstore.PlatformMac, Country: "US"}
	for range 2 {
		var se *appstore.StatusError
		if _, err := cache.Lookup(ctx, broken); !errors.As(err, &se) {
			t.Fatalf("err = %v, want *StatusError", err)
		}
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("requests after two failed lookups = %d, want 3: a failure must not be stored", got)
	}
}

func TestAppStoreCache_CapsConcurrency(t *testing.T) {
	var inFlight, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
		_, _ = w.Write([]byte(appStoreBody(r.URL.Query().Get("id"))))
	}))
	t.Cleanup(srv.Close)
	cache := NewAppStoreCache(appstore.New(appstore.WithEndpoint(srv.URL)))

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			q := appstore.Query{ID: fmt.Sprint(100 + i), Platform: appstore.PlatformMobile, Country: "US"}
			if _, err := cache.Lookup(context.Background(), q); err != nil {
				t.Errorf("Lookup %d: %v", i, err)
			}
		})
	}
	wg.Wait()
	if got := peak.Load(); got > AppStoreMaxConcurrent {
		t.Errorf("peak concurrent requests = %d, want at most %d", got, AppStoreMaxConcurrent)
	}
	if got := peak.Load(); got < 2 {
		t.Errorf("peak concurrent requests = %d; distinct queries should run in parallel", got)
	}
}

func TestAppStoreCache_WaitingForASlotHonoursContext(t *testing.T) {
	cache := NewAppStoreCache(appstore.New(appstore.WithEndpoint("http://127.0.0.1:0")))
	for range AppStoreMaxConcurrent {
		cache.slots <- struct{}{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := cache.Lookup(ctx, appstore.Query{ID: "1", Platform: appstore.PlatformMac, Country: "US"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded while every slot is held", err)
	}
	if _, ok := cache.entries[appstore.Query{ID: "1", Platform: appstore.PlatformMac, Country: "US"}]; ok {
		t.Error("a context failure was stored")
	}
}

func TestAppStoreCache_NilSafe(t *testing.T) {
	var c *AppStoreCache
	if _, err := c.Lookup(context.Background(), appstore.Query{}); err == nil {
		t.Error("nil cache returned no error")
	}
	var d *Data
	if d.AppStoreCache() != nil {
		t.Error("nil Data returned a cache")
	}
	d.SetAppStoreClient(nil)
}

func TestData_AppStoreCacheIsPerInstance(t *testing.T) {
	d := New(nil)
	first := d.AppStoreCache()
	if first == nil || d.AppStoreCache() != first {
		t.Error("AppStoreCache is not stable across calls")
	}
	d.SetAppStoreClient(appstore.New())
	if d.AppStoreCache() == first {
		t.Error("SetAppStoreClient did not replace the cache")
	}
	if New(nil).AppStoreCache() == d.AppStoreCache() {
		t.Error("two provider instances share one cache")
	}
}
