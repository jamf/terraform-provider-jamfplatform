// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireDeviceGroupWrite_SerialisesHolders(t *testing.T) {
	d := &Data{}
	var inFlight, peak atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			release, err := d.AcquireDeviceGroupWrite(context.Background())
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			n := inFlight.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			inFlight.Add(-1)
			release()
		})
	}
	wg.Wait()
	if got := peak.Load(); got != 1 {
		t.Fatalf("peak holders = %d, want 1", got)
	}
}

func TestAcquireDeviceGroupWrite_WaiterHonoursContext(t *testing.T) {
	d := &Data{}
	release, err := d.AcquireDeviceGroupWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := d.AcquireDeviceGroupWrite(ctx); err == nil {
		t.Fatal("a waiter behind a held slot must fail when its context ends")
	}
}

func TestAcquireDeviceGroupWrite_ReleaseIsIdempotent(t *testing.T) {
	d := &Data{}
	release, err := d.AcquireDeviceGroupWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
	release()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	again, err := d.AcquireDeviceGroupWrite(ctx)
	if err != nil {
		t.Fatalf("slot not free after release: %v", err)
	}
	again()
}

func TestAcquireDeviceGroupWrite_NilDataAdmits(t *testing.T) {
	var d *Data
	release, err := d.AcquireDeviceGroupWrite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}
