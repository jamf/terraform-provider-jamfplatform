// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package aischemas

import (
	"context"
	"fmt"
	"sync"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
)

// Cache holds the product catalogue and the vendor schemas for one configured provider instance.
//
// Both are read-only server data that cannot change during a plan, and the Claude Code schema alone
// is 184 KB, so fetching per resource per plan would be waste an operator pays for. Failures are not
// cached: a transient blip in the first resource's plan must not silence validation for every later
// one, which is the same rule providerdata applies to the Jamf Pro version fetch.
type Cache struct {
	client *aigovernance.Client

	toolsMu sync.Mutex
	tools   []aigovernance.ToolSummary

	schemaMu sync.Mutex
	schemas  map[string]*Document
	inflight map[string]*schemaFetch

	noticeMu    sync.Mutex
	noticeFired bool
}

// schemaFetch is one in-progress fetch of one (tool, schemaVersion) pair. It exists so concurrent
// callers wanting the same schema share a single round-trip without callers wanting a different one
// having to wait for it, which one mutex held across the fetch would impose.
type schemaFetch struct {
	once     sync.Once
	document *Document
	err      error
}

// NewCache builds a cache over an authenticated client.
func NewCache(base *jamfplatform.Client) *Cache {
	return &Cache{client: aigovernance.New(base)}
}

// Tools returns the product catalogue, fetching it once.
func (c *Cache) Tools(ctx context.Context) ([]aigovernance.ToolSummary, error) {
	if c == nil {
		return nil, nil
	}
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()

	if c.tools != nil {
		return c.tools, nil
	}
	response, err := c.client.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	c.tools = response.Results
	return c.tools, nil
}

// Tool returns one product from the catalogue, and whether it is in it.
func (c *Cache) Tool(ctx context.Context, toolID string) (aigovernance.ToolSummary, bool, error) {
	tools, err := c.Tools(ctx)
	if err != nil {
		return aigovernance.ToolSummary{}, false, err
	}
	for _, tool := range tools {
		if tool.ID == toolID {
			return tool, true, nil
		}
	}
	return aigovernance.ToolSummary{}, false, nil
}

// Document returns the parsed vendor schema for one product at one schema version, fetching and
// parsing it once.
//
// The mutex covers the map reads and writes only, never the fetch: a plan runs ModifyPlan
// concurrently for independent resource instances, and Terraform's default parallelism is ten, so a
// lock held across the round-trip and the decode of a 184 KB body would serialise policies that
// share no schema at all. Concurrent callers wanting the same pair still collapse to one fetch,
// through a per-key sync.Once rather than a shared lock. A failed fetch is discarded rather than
// stored, so the next caller retries it — the invariant this type's doc states.
func (c *Cache) Document(ctx context.Context, toolID, schemaVersion string) (*Document, error) {
	if c == nil {
		return nil, nil
	}
	key := toolID + "@" + schemaVersion

	c.schemaMu.Lock()
	if document, ok := c.schemas[key]; ok {
		c.schemaMu.Unlock()
		return document, nil
	}
	if c.inflight == nil {
		c.inflight = map[string]*schemaFetch{}
	}
	fetch, ok := c.inflight[key]
	if !ok {
		fetch = &schemaFetch{}
		c.inflight[key] = fetch
	}
	c.schemaMu.Unlock()

	fetch.once.Do(func() {
		fetch.document, fetch.err = c.fetchDocument(ctx, toolID, schemaVersion)

		c.schemaMu.Lock()
		defer c.schemaMu.Unlock()
		delete(c.inflight, key)
		if fetch.err != nil {
			return
		}
		if c.schemas == nil {
			c.schemas = map[string]*Document{}
		}
		c.schemas[key] = fetch.document
	})
	return fetch.document, fetch.err
}

// fetchDocument reads and parses one vendor schema, holding no lock.
func (c *Cache) fetchDocument(ctx context.Context, toolID, schemaVersion string) (*Document, error) {
	response, err := c.client.GetToolSchema(ctx, toolID, schemaVersion)
	if err != nil {
		return nil, err
	}
	document, err := Parse(toolID, schemaVersion, response.Schema)
	if err != nil {
		return nil, fmt.Errorf("parse %s schema %s: %w", toolID, schemaVersion, err)
	}
	return document, nil
}

// NoticeOnce reports whether the caller should emit the "validation unavailable" notice. It returns
// true exactly once per Cache, so a plan whose catalogue or schema read failed says so once rather
// than once per policy in the configuration. Mirrors impact.Cache's notice, and the rule it states:
// data that cannot be read yields one notice and never fails a plan.
func (c *Cache) NoticeOnce() bool {
	if c == nil {
		return false
	}
	c.noticeMu.Lock()
	defer c.noticeMu.Unlock()
	if c.noticeFired {
		return false
	}
	c.noticeFired = true
	return true
}
