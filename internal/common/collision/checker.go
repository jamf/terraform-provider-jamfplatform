// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package collision detects, during `terraform plan`, a create or rename whose
// key already belongs to an object that exists in the tenant.
//
// Jamf refuses a duplicate name for most object types, and the refusal arrives
// halfway through an apply. For the types Jamf does not check, the apply
// succeeds and leaves two objects with one name. Both are findable at plan time
// by listing the type once per provider instance and comparing keys under the
// collation Jamf itself applies. The check runs from a resource's ModifyPlan, which
// Terraform repeats immediately before each apply, so an object created in the
// web interface between plan and apply is still caught.
//
// A check never blocks a plan it cannot evaluate. A listing that fails yields
// one warning per namespace and the plan continues.
package collision

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Mode selects what a detected collision does to the plan.
type Mode string

const (
	// ModeOff disables the check.
	ModeOff Mode = "off"
	// ModeWarn reports a collision as a warning and lets the plan proceed.
	ModeWarn Mode = "warn"
	// ModeError reports a collision as an error and fails the plan.
	ModeError Mode = "error"
)

// Modes lists the accepted values of the provider's mode setting.
func Modes() []string {
	return []string{string(ModeOff), string(ModeWarn), string(ModeError)}
}

// Config is the provider-level setting. IncludeUnenforced extends the check to
// types for which Jamf stores duplicates, so a second object of the same name
// is reported although Jamf would accept it.
type Config struct {
	Mode              Mode
	IncludeUnenforced bool
}

// Entry is one existing key in a namespace. A single object contributes several
// entries when its uniqueness key is a collection, such as DNS zone domains.
// ID is the identifier the Terraform resource imports by.
type Entry struct {
	ID  string
	Key string
}

// Source lists the existing objects of one type. Noun is the object name as the
// admin UI writes it ("static computer group"). ResourceType is the Terraform
// resource that adopts it ("jamfplatform_pro_static_computer_group"), empty when
// no resource in this provider manages the type.
type Source struct {
	Noun         string
	ResourceType string
	List         func(ctx context.Context) ([]Entry, error)
}

// Namespace is a set of object types that share one uniqueness rule. Computer
// groups are the case that needs more than one source: smart groups, static
// groups and Platform COMPUTER device groups draw on one pool of names.
//
// ID keys the per-provider cache and must be unique per rule. Product names the
// service in a diagnostic ("Jamf Pro"). KeyLabel names the compared attribute
// and defaults to "name". Enforced records whether Jamf refuses a duplicate.
type Namespace struct {
	ID        string
	Product   string
	KeyLabel  string
	Enforced  bool
	Collation Collation
	Sources   []Source
}

func (n Namespace) keyLabel() string {
	if n.KeyLabel == "" {
		return "name"
	}
	return n.KeyLabel
}

// sweepTimeout bounds one namespace listing. The listing is shared by every
// resource in the plan, so it runs detached from the context of whichever
// resource asked first.
const sweepTimeout = 2 * time.Minute

// sweepConcurrency bounds parallel namespace listings, matching the budget the
// impact alert sweeps keep to.
const sweepConcurrency = 5

type sweep struct {
	once    sync.Once
	entries []taggedEntry
	err     error
	notice  sync.Once
}

type taggedEntry struct {
	Entry
	source Source
}

// Checker holds the per-provider listing cache. A nil *Checker is valid and
// checks nothing, which is how ModeOff is represented, so resources need no flag
// check of their own.
type Checker struct {
	cfg Config
	sem chan struct{}

	mu     sync.Mutex
	sweeps map[string]*sweep
}

// New returns a Checker for the configuration, or nil when the mode is off.
func New(cfg Config) *Checker {
	if cfg.Mode == ModeOff || cfg.Mode == "" {
		return nil
	}
	return &Checker{
		cfg:    cfg,
		sem:    make(chan struct{}, sweepConcurrency),
		sweeps: make(map[string]*sweep),
	}
}

// Enabled reports whether the checker will report anything.
func (c *Checker) Enabled() bool { return c != nil }

// Config returns the configuration the checker was built with.
func (c *Checker) Config() Config {
	if c == nil {
		return Config{Mode: ModeOff}
	}
	return c.cfg
}

// applies reports whether a namespace is checked under this configuration.
func (c *Checker) applies(ns Namespace) bool {
	return c != nil && (ns.Enforced || c.cfg.IncludeUnenforced)
}

func (c *Checker) sweepFor(ns Namespace) *sweep {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.sweeps[ns.ID]
	if !ok {
		s = &sweep{}
		c.sweeps[ns.ID] = s
	}
	return s
}

func (c *Checker) load(ctx context.Context, ns Namespace) *sweep {
	s := c.sweepFor(ns)
	s.once.Do(func() {
		c.sem <- struct{}{}
		defer func() { <-c.sem }()

		listCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sweepTimeout)
		defer cancel()

		for _, src := range ns.Sources {
			entries, err := src.List(listCtx)
			if err != nil {
				s.err = fmt.Errorf("listing %ss: %w", src.Noun, err)
				s.entries = nil
				return
			}
			for _, e := range entries {
				s.entries = append(s.entries, taggedEntry{Entry: e, source: src})
			}
		}
	})
	return s
}

// Match is an existing object whose key collides with a planned one.
type Match struct {
	Entry
	Noun         string
	ResourceType string
}

// find returns the existing objects colliding with key. An entry is the object
// being planned, and is skipped, when it carries selfID and was listed by the
// source of the same resource type.
func (s *sweep) find(ns Namespace, key, selfID, resourceType string) []Match {
	want := ns.Collation.Fold(key)
	var out []Match
	for _, e := range s.entries {
		if selfID != "" && e.ID == selfID && e.source.ResourceType == resourceType {
			continue
		}
		if ns.Collation.Fold(e.Key) != want {
			continue
		}
		out = append(out, Match{Entry: e.Entry, Noun: e.source.Noun, ResourceType: e.source.ResourceType})
	}
	return out
}
