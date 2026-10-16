// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"errors"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/appstore"
)

// AppStoreMaxConcurrent caps the App Store lookups one configured provider instance runs at once.
// Apple publishes no rate limit and drew no 429 at 44 requests per second (2026-10-16), so this is
// not a measured ceiling: it matches the Jamf API load budget the rest of the provider keeps,
// because an undocumented limit is still a limit and a for_each over a large app list would
// otherwise fan out at Terraform's full parallelism.
const AppStoreMaxConcurrent = 5

// AppStoreCache holds the App Store listings one configured provider instance has looked up.
//
// A listing cannot change within one plan in any way the plan could use, and a configuration that
// looks up the same app twice (once per target, or from two modules) should pay one request. Two
// outcomes are stored: a listing, and a definitive miss (*appstore.NotFoundError or
// *appstore.StorefrontMismatchError), because Apple answers both with a 200 and asking again
// returns the same answer. A transport or status failure is not stored, so the next caller retries
// it, the same rule this package applies to the Jamf Pro version fetch.
//
// The map lock is never held across a request: concurrent callers for one query collapse onto a
// per-key sync.Once, as in aischemas.Cache, while callers for different queries proceed
// independently up to AppStoreMaxConcurrent.
type AppStoreCache struct {
	client *appstore.Client
	slots  chan struct{}

	mu      sync.Mutex
	entries map[appstore.Query]*appStoreEntry
}

// appStoreEntry is one query's lookup, shared by every caller asking for it.
type appStoreEntry struct {
	once sync.Once
	app  *appstore.App
	err  error
}

// NewAppStoreCache builds a cache over client.
func NewAppStoreCache(client *appstore.Client) *AppStoreCache {
	if client == nil {
		client = appstore.New()
	}
	return &AppStoreCache{
		client:  client,
		slots:   make(chan struct{}, AppStoreMaxConcurrent),
		entries: map[appstore.Query]*appStoreEntry{},
	}
}

// Lookup returns the listing q names, from the cache when an earlier caller already asked.
//
// Nil-receiver-safe for the same reason AppTitleCatalogCache is: a data source that never received
// provider data must report the failure rather than panic.
func (c *AppStoreCache) Lookup(ctx context.Context, q appstore.Query) (*appstore.App, error) {
	if c == nil {
		return nil, errors.New("the provider is not configured, so the App Store cannot be read")
	}

	c.mu.Lock()
	entry, ok := c.entries[q]
	if !ok {
		entry = &appStoreEntry{}
		c.entries[q] = entry
	}
	c.mu.Unlock()

	entry.once.Do(func() {
		entry.app, entry.err = c.fetch(ctx, q)
		if entry.err == nil || definitiveAppStoreAnswer(entry.err) {
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.entries[q] == entry {
			delete(c.entries, q)
		}
	})
	return entry.app, entry.err
}

// fetch performs one lookup inside a concurrency slot.
func (c *AppStoreCache) fetch(ctx context.Context, q appstore.Query) (*appstore.App, error) {
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.slots }()
	return c.client.Lookup(ctx, q)
}

// definitiveAppStoreAnswer reports whether err is an answer Apple would give again unchanged.
func definitiveAppStoreAnswer(err error) bool {
	var notFound *appstore.NotFoundError
	var mismatch *appstore.StorefrontMismatchError
	return errors.As(err, &notFound) || errors.As(err, &mismatch) || errors.Is(err, appstore.ErrInvalidQuery)
}

// SetAppStoreClient installs the App Store client the provider built in Configure, carrying its
// User-Agent and, under TF_LOG=debug, its request logger. Call it before any construct runs.
func (d *Data) SetAppStoreClient(client *appstore.Client) {
	if d == nil {
		return
	}
	d.appStoreMu.Lock()
	defer d.appStoreMu.Unlock()
	d.appStoreCache = NewAppStoreCache(client)
}

// AppStoreCache returns the App Store lookup cache for this configured provider instance, building
// one over a default client when Configure installed none.
func (d *Data) AppStoreCache() *AppStoreCache {
	if d == nil {
		return nil
	}
	d.appStoreMu.Lock()
	defer d.appStoreMu.Unlock()
	if d.appStoreCache == nil {
		d.appStoreCache = NewAppStoreCache(nil)
	}
	return d.appStoreCache
}
