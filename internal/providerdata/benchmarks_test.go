// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBenchmarkLock_SharedAndExclusive(t *testing.T) {
	d := &Data{}
	a, b := d.BenchmarkLock(), d.BenchmarkLock()
	if a != b {
		t.Fatal("BenchmarkLock returned two different locks for one provider instance")
	}

	release, err := a.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := b.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Acquire while held = %v, want context.DeadlineExceeded", err)
	}

	release()
	release2, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire after release: %v", err)
	}
	release2()
}

func TestBenchmarkLock_NilIsFree(t *testing.T) {
	var d *Data
	release, err := d.BenchmarkLock().Acquire(context.Background())
	if err != nil {
		t.Fatalf("nil lock Acquire: %v", err)
	}
	release()
}
