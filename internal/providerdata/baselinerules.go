// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"errors"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// BaselineRulesMaxConcurrent caps the baseline reads one configured provider instance runs at once.
// It is the Jamf API load budget every plan-time read in this provider keeps: a configuration with
// many benchmarks on different baselines would otherwise fan out at Terraform's full parallelism.
const BaselineRulesMaxConcurrent = 5

// BaselineRulesCache holds the rules and available OS versions of each compliance baseline one
// configured provider instance has read.
//
// A baseline's rules cannot change within one plan in any way the plan could use, and every benchmark
// built on the same baseline needs the same answer, so a configuration with N benchmarks on one
// baseline pays one request instead of N. The key is the baseline id: the endpoint takes nothing
// else, so the answer is whatever revision the platform currently serves for it. A benchmark pins
// its own sources when it is created, which is why the plan-time check reads the baseline as it
// stands now.
//
// A failed read is not stored, so the next caller retries it, the same rule this package applies to
// the Jamf Pro version fetch and aischemas to its vendor schemas. The map lock is never held across
// a request: callers for one baseline collapse onto a per-key sync.Once while callers for different
// baselines proceed independently up to BaselineRulesMaxConcurrent.
type BaselineRulesCache struct {
	client *compliancebenchmarks.Client
	slots  chan struct{}

	mu      sync.Mutex
	entries map[string]*baselineRulesEntry
}

// baselineRulesEntry is one baseline's read, shared by every caller asking for it.
type baselineRulesEntry struct {
	once  sync.Once
	rules *compliancebenchmarks.SourcedRules
	err   error
}

// NewBaselineRulesCache builds a cache over client.
func NewBaselineRulesCache(client *compliancebenchmarks.Client) *BaselineRulesCache {
	return &BaselineRulesCache{
		client:  client,
		slots:   make(chan struct{}, BaselineRulesMaxConcurrent),
		entries: map[string]*baselineRulesEntry{},
	}
}

// Rules returns the rules of baselineID, from the cache when an earlier caller already read it.
//
// Nil-receiver-safe: plan-time validation is advisory, so a resource that never received provider
// data must report that the read was unavailable rather than panic mid-plan.
func (c *BaselineRulesCache) Rules(ctx context.Context, baselineID string) (*compliancebenchmarks.SourcedRules, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("the provider is not configured, so the baseline cannot be read")
	}

	c.mu.Lock()
	entry, ok := c.entries[baselineID]
	if !ok {
		entry = &baselineRulesEntry{}
		c.entries[baselineID] = entry
	}
	c.mu.Unlock()

	entry.once.Do(func() {
		entry.rules, entry.err = c.fetch(ctx, baselineID)
		if entry.err == nil {
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.entries[baselineID] == entry {
			delete(c.entries, baselineID)
		}
	})
	return entry.rules, entry.err
}

// fetch performs one baseline read inside a concurrency slot.
func (c *BaselineRulesCache) fetch(ctx context.Context, baselineID string) (*compliancebenchmarks.SourcedRules, error) {
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.slots }()
	return c.client.GetBaselineRules(ctx, baselineID)
}

// BaselineRulesCache returns the baseline read cache for this configured provider instance, building
// it on first use so every benchmark receives the same one.
func (d *Data) BaselineRulesCache() *BaselineRulesCache {
	if d == nil || d.Client == nil {
		return nil
	}
	d.baselineRulesMu.Lock()
	defer d.baselineRulesMu.Unlock()
	if d.baselineRulesCache == nil {
		d.baselineRulesCache = NewBaselineRulesCache(compliancebenchmarks.New(d.Client))
	}
	return d.baselineRulesCache
}
