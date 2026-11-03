// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"sync"
)

// writeGate is a one-slot semaphore built on first use, so a Data assembled as a
// struct literal in a test needs no constructor call to get a working gate.
type writeGate struct {
	once sync.Once
	slot chan struct{}
}

func (g *writeGate) acquire(ctx context.Context) (func(), error) {
	g.once.Do(func() { g.slot = make(chan struct{}, 1) })
	select {
	case g.slot <- struct{}{}:
		var released sync.Once
		return func() { released.Do(func() { <-g.slot }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// AcquireDeviceGroupWrite admits one device group create or delete at a time per
// configured provider instance and returns the function that releases the slot.
// It blocks until the slot is free or ctx ends, so a caller passes Terraform's
// own context and starts any operation deadline only after this returns: time
// spent queued is not time the API spent working.
//
// The Platform device group API answers a create of a large smart group by
// creating it and then failing the request, and the failure rate and the
// tenant-wide damage rise with the number of writes in flight: three at once
// produced 504s with duplicate groups, and four at once was followed by a 502
// storm on the groups API and, per the tenant's operator, an instance restart
// (observed 2026-11-03, one shared EU tenant, 22,000-member groups). One at a
// time does not stop the individual failures, which the create reconciliation
// absorbs; it stops this provider adding load to a service already struggling.
// Terraform's -parallelism flag cannot do it: it caps every resource at once and
// has no per-type setting.
//
// Only a single provider process is serialised. Two concurrent terraform runs
// against one tenant still overlap.
//
// A nil receiver admits immediately, so a resource built without provider data in
// a unit test does not block.
func (d *Data) AcquireDeviceGroupWrite(ctx context.Context) (func(), error) {
	if d == nil {
		return func() {}, nil
	}
	return d.deviceGroupWrites.acquire(ctx)
}
