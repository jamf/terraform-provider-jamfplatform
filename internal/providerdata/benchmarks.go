// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
)

// BenchmarkLock serializes compliance benchmark creates and deletes within one configured provider
// instance.
//
// A benchmark is not one object: Jamf deploys it to Jamf Pro as about sixty (configuration
// profiles, policies, smart groups, extension attributes and scripts) and tears them down
// asynchronously after the delete is accepted. Wire-observed on the EU gateway 2026-10-18, deleting
// several benchmarks at once leaves some of those objects behind (two to eleven configuration
// profiles and policies per run) while every benchmark still reports a clean delete, and the same
// deletes run one at a time leave nothing. Terraform runs independent resources concurrently, so the
// resource holds this lock from the write until the benchmark has finished syncing or disappeared.
//
// It is a one-slot channel rather than a sync.Mutex so a caller waiting for it still honours
// cancellation and its timeout. It serializes one provider process only: two `terraform apply` runs
// against the same environment can still overlap.
type BenchmarkLock struct {
	slot chan struct{}
}

// Acquire blocks until the lock is free or ctx is done, and returns the function that releases it.
// A nil lock is always free, which is what a resource that never received provider data holds.
func (l *BenchmarkLock) Acquire(ctx context.Context) (func(), error) {
	if l == nil {
		return func() {}, nil
	}
	select {
	case l.slot <- struct{}{}:
		return func() { <-l.slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BenchmarkLock returns the benchmark write lock for this configured provider instance, building it
// on first use so every resource receives the same one.
func (d *Data) BenchmarkLock() *BenchmarkLock {
	if d == nil {
		return nil
	}
	d.benchmarkMu.Lock()
	defer d.benchmarkMu.Unlock()
	if d.benchmarkLock == nil {
		d.benchmarkLock = &BenchmarkLock{slot: make(chan struct{}, 1)}
	}
	return d.benchmarkLock
}
