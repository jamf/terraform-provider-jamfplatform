// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// ConfirmAsyncDelete handles a classic Jamf Pro endpoint that ACCEPTS a DELETE
// but fronts it with a misleading non-2xx status and removes the record promptly
// thereafter — confirming via a GET-by-id poll. It is for /mobiledeviceapplications,
// whose DELETE returns HTTP 400 on an accepted delete but clears in ~2s and is
// NOT sensitive to being polled (wire-probed 2026-06-04).
//
// It is NOT suitable for endpoints where polling interferes with the delete:
//   - /ebooks is async AND GET-sensitive (polling holds it alive past 3.5min),
//     so its Delete is fire-and-trust — issue DELETE once, never GET, warn on the
//     accepted 4xx. See internal/resources/pro/ebook/crud.go.
//   - /macapplications is a clean synchronous DELETE (200), so it uses a plain
//     delete with a not-found-as-success branch.
//
// It issues the delete exactly ONCE via del. Re-issuing the DELETE while an async
// removal is pending can delay it, so this never re-issues — on a non-clean
// delete it only ever polls get (a read) until the resource reports not-found.
//
// Returns nil when removal is confirmed: a clean delete, an already-absent
// resource, or a poll that observes not-found. Returns an error only if the
// resource is still present when ctx expires — a genuine delete failure that the
// caller should surface (so the resource stays in state and self-heals on the
// next apply, rather than silently dropping a still-present record as drift).
//
// Deliberate property: ANY non-not-found del error is treated as an accepted
// async delete to be confirmed by polling — including, in principle, a genuine
// 5xx/403. For the classic apps endpoints this is correct, because an accepted
// delete is reported as a plain HTTP 400 that is status-indistinguishable from a
// real client error; the original del error is preserved in the returned
// message so a true failure still surfaces (just as a timeout rather than a
// fast failure). Use this only for endpoints with that accepted-async-behind-a-
// misleading-status behaviour, not as a general delete wrapper.
//
// pollInterval bounds how often get is retried; ctx (typically the Delete
// timeout) bounds the total wait.
func ConfirmAsyncDelete(ctx context.Context, pollInterval time.Duration, del, get func(context.Context) error) error {
	delErr := del(ctx)
	if delErr == nil || IsNotFoundError(delErr) {
		return nil
	}

	// del returned a non-not-found status — expected for the classic apps
	// endpoints, where a misleading HTTP 400 fronts an accepted async delete.
	// Confirm via the read poll rather than trusting or re-issuing the delete.
	pollErr := jamfplatform.PollUntil(ctx, pollInterval, func(pollCtx context.Context) (bool, error) {
		// A not-found on the read means the async delete has landed. Any other
		// outcome (still present, or a transient read error) is inconclusive —
		// keep polling; never abort the poll on a read error.
		if IsNotFoundError(get(pollCtx)) {
			return true, nil
		}
		return false, nil
	})
	if pollErr != nil {
		return fmt.Errorf("delete was accepted but the resource was still present when the delete timeout elapsed (last delete response: %v): %w", delErr, pollErr)
	}
	return nil
}
