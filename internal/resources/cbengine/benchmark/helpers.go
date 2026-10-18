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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
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

// waitForBenchmarkDeletion polls the benchmark by id until it answers 404, or
// the context is canceled. A 404 is the only completion signal.
//
// Neither the list's sync state nor the benchmark's own deleted flag says the
// teardown has finished. Wire-observed on the EU gateway 2026-10-18: within two
// seconds of the delete, GET reports deleted=true while the list still shows
// SYNCED or FAILED; the list then moves to DELETING, can return to SYNCED after
// GET has begun answering 404, and drops the benchmark about half a second
// after that. Issue #485 adds DELETE_FAILED to the states a benchmark reports on
// its way out. So any answer other than 404 means "still being deleted".
//
// The delete is re-issued every redeleteAfter while the benchmark still exists,
// which clears one that stalls (observed for over five minutes until a second
// delete removed it in eight seconds). A re-issued delete answering 404 means
// the benchmark is gone. On timeout the error names the last deleted flag seen.
//
// A Jamf Pro object that refers to one of the benchmark's generated smart
// groups blocks the teardown outright. Wire-observed on the EU gateway
// 2026-10-18 with a smart group whose "member of" criterion named one: the
// benchmark sat at DELETING with deleted=true for twelve minutes, 26 re-issued
// deletes were accepted without effect, and removing the referring group did
// not resume it. The first delete sent after the removal finished in seven
// seconds and left no Jamf Pro objects behind. So the timeout error points at
// that reference, and a re-issued delete clears the benchmark once the
// operator removes it mid-wait.
func waitForBenchmarkDeletion(ctx context.Context, c *compliancebenchmarks.Client, id string, interval, redeleteAfter time.Duration) error {
	lastDelete := time.Now()
	seen, lastDeleted := false, false
	err := jamfplatform.PollUntil(ctx, interval, func(pollCtx context.Context) (bool, error) {
		bench, err := c.GetBenchmark(pollCtx, id)
		if err != nil {
			if helpers.IsNotFoundError(err) {
				tflog.Debug(pollCtx, "benchmark gone after delete", map[string]any{"benchmark_id": id})
				return true, nil
			}
			tflog.Debug(pollCtx, "polling benchmark failed", map[string]any{"benchmark_id": id, "error": err.Error()})
			return false, fmt.Errorf("failed to poll benchmark: %w", err)
		}
		seen, lastDeleted = true, bench.Deleted
		tflog.Debug(pollCtx, "benchmark still present during deletion poll", map[string]any{
			"benchmark_id": id,
			"deleted":      bench.Deleted,
		})
		if time.Since(lastDelete) < redeleteAfter {
			return false, nil
		}
		lastDelete = time.Now()
		tflog.Debug(pollCtx, "re-issuing delete for benchmark", map[string]any{"benchmark_id": id})
		if err := c.DeleteBenchmark(pollCtx, id); err != nil {
			if helpers.IsNotFoundError(err) {
				return true, nil
			}
			tflog.Debug(pollCtx, "re-issued delete failed", map[string]any{"benchmark_id": id, "error": err.Error()})
		}
		return false, nil
	})
	if err != nil && seen && ctx.Err() != nil {
		return fmt.Errorf("benchmark %s still existed when the delete timeout expired (deleted=%t): %w. "+
			"Jamf cannot finish deleting a benchmark while a Jamf Pro object refers to one of the smart groups the benchmark created, "+
			"for example a smart group with a \"member of\" criterion naming one. "+
			"Remove that reference in Jamf Pro and run the destroy again: Jamf does not resume the delete by itself", id, lastDeleted, err)
	}
	return err
}
