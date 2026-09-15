// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// waitForBenchmarkSync polls until the benchmark reaches a terminal state
// (SYNCED or FAILED) or the provided context is canceled.
func waitForBenchmarkSync(ctx context.Context, c *compliancebenchmarks.Client, id string, interval time.Duration) (*compliancebenchmarks.BenchmarkV2, error) {
	var synced *compliancebenchmarks.BenchmarkV2
	err := jamfplatform.PollUntil(ctx, interval, func(pollCtx context.Context) (bool, error) {
		benchmarks, err := c.ListBenchmarks(pollCtx)
		if err != nil {
			tflog.Debug(pollCtx, "polling benchmarks failed", map[string]any{"error": err.Error()})
			return false, fmt.Errorf("failed to poll benchmarks: %w", err)
		}
		for _, b := range benchmarks.Benchmarks {
			if b.ID != id {
				continue
			}
			benchCopy := b
			tflog.Debug(pollCtx, "benchmark syncState", map[string]any{"benchmark_id": id, "sync_state": benchCopy.SyncState})
			switch benchCopy.SyncState {
			case compliancebenchmarks.BenchmarkV2SyncStatePending:
				return false, nil
			case compliancebenchmarks.BenchmarkV2SyncStateSynced:
				synced = &benchCopy
				return true, nil
			case compliancebenchmarks.BenchmarkV2SyncStateFailed:
				return false, fmt.Errorf("benchmark %s in FAILED state", id)
			default:
				return false, fmt.Errorf("unexpected syncState for benchmark %s: %s", id, benchCopy.SyncState)
			}
		}
		tflog.Debug(pollCtx, "benchmark not present yet", map[string]any{"benchmark_id": id})
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return synced, nil
}

// waitForBenchmarkDeletion polls until the benchmark is no longer present or
// the context is canceled. Every 20 seconds it re-issues the delete command
// to unstick benchmarks that remain in DELETING state.
func waitForBenchmarkDeletion(ctx context.Context, c *compliancebenchmarks.Client, id string, interval time.Duration) error {
	lastDelete := time.Now()
	return jamfplatform.PollUntil(ctx, interval, func(pollCtx context.Context) (bool, error) {
		benchmarks, err := c.ListBenchmarks(pollCtx)
		if err != nil {
			tflog.Debug(pollCtx, "polling benchmarks failed", map[string]any{"error": err.Error()})
			return false, fmt.Errorf("failed to poll benchmarks: %w", err)
		}
		for _, b := range benchmarks.Benchmarks {
			if b.ID != id {
				continue
			}
			tflog.Debug(pollCtx, "benchmark still present during deletion poll", map[string]any{
				"benchmark_id": b.ID,
				"sync_state":   b.SyncState,
			})
			switch b.SyncState {
			case compliancebenchmarks.BenchmarkV2SyncStateDeleting:
				if time.Since(lastDelete) > 20*time.Second {
					lastDelete = time.Now()
					tflog.Debug(pollCtx, "retrying delete for stuck benchmark", map[string]any{"benchmark_id": id})
					_ = c.DeleteBenchmark(pollCtx, id)
				}
				return false, nil
			case compliancebenchmarks.BenchmarkV2SyncStateDeleteFailed:
				return false, fmt.Errorf("benchmark %s deletion failed: syncState=DELETE_FAILED", id)
			default:
				return false, fmt.Errorf("benchmark %s still present after delete, syncState=%s", id, b.SyncState)
			}
		}
		tflog.Debug(pollCtx, "benchmark absent after delete", map[string]any{"benchmark_id": id})
		return true, nil
	})
}
