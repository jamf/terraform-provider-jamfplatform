// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Read-back pacing for ReadBackAfterWrite. Variables rather than constants so a
// unit test can run the loop without waiting on the clock.
var (
	readBackInterval = 250 * time.Millisecond
	readBackBound    = 10 * time.Second
)

// ReadBackAfterWrite reads a group back after a write until settled reports that
// the read reflects that write, and returns the last read either way.
//
// Jamf Pro 11.32.0 applies a PUT to /pro/v3/computer-groups/{smart,static}-groups
// after answering 202: a GET issued straight after the write returns the group
// as it was before it, and the next GET, about 150–300 ms later, returns the
// write (wire-verified 2026-10-20, eight consecutive writes, every first read
// one write behind). 11.33.0 answers the same 202 and reads fresh at once. A
// state builder fed the stale read hands Terraform the previous value and
// fails the apply with "Provider produced inconsistent result after apply".
//
// The loop is bounded by readBackBound and by ctx, and a read that never
// settles is returned rather than refused: a server that genuinely stores a
// different value must still reach Terraform's own consistency check, which
// names the attribute, instead of a timeout that names nothing. On a tenant
// that reads fresh the first read settles and no extra request is made. A read
// error ends the loop at once and is returned to the caller.
func ReadBackAfterWrite[T any](ctx context.Context, read func(context.Context) (T, error), settled func(T) bool) (T, error) {
	deadline := time.Now().Add(readBackBound)
	attempts := 0
	for {
		got, err := read(ctx)
		attempts++
		if err != nil || settled(got) {
			if attempts > 1 && err == nil {
				tflog.Debug(ctx, "Jamf Pro group read-back settled after a stale read", map[string]any{"reads": attempts})
			}
			return got, err
		}
		if !time.Now().Add(readBackInterval).Before(deadline) {
			tflog.Warn(ctx, "Jamf Pro group read-back did not reflect the write before the bound; using the last read", map[string]any{"reads": attempts})
			return got, nil
		}
		select {
		case <-ctx.Done():
			return got, nil
		case <-time.After(readBackInterval):
		}
	}
}

// PlannedValueReflected reports whether a value read back from Jamf Pro matches
// the planned one, treating an unknown planned value as matching, since
// Terraform accepts whatever the provider returns for it. It is the
// per-attribute test a settled predicate for ReadBackAfterWrite is built from.
func PlannedValueReflected(planned, read attr.Value) bool {
	return planned.IsUnknown() || planned.Equal(read)
}

// WaitUntilGone reads a group after a successful DELETE until the read reports
// not-found, for the same Jamf Pro 11.32.0 lag ReadBackAfterWrite outlasts: the
// DELETE answers 204 and a GET straight after it still returns the group, for
// about 150–250 ms (wire-verified 2026-10-20). Without it a destroy reports
// success while the group is still readable, so a create of the same name or a
// destroy check that follows at once sees it.
//
// It never fails the delete. The DELETE itself succeeded, so a group that is
// still readable at the bound, or a read error other than not-found, is logged
// and the delete reported as done.
func WaitUntilGone(ctx context.Context, read func(context.Context) error) {
	deadline := time.Now().Add(readBackBound)
	for {
		err := read(ctx)
		if helpers.IsNotFoundError(err) {
			return
		}
		if err != nil {
			tflog.Warn(ctx, "Jamf Pro group read after delete failed; treating the delete as done", map[string]any{"error": err.Error()})
			return
		}
		if !time.Now().Add(readBackInterval).Before(deadline) {
			tflog.Warn(ctx, "Jamf Pro group still readable after delete at the bound; treating the delete as done")
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(readBackInterval):
		}
	}
}
