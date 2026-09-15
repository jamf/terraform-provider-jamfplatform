// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"bytes"
	"context"
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// pollInterval is the cadence at which the verification poll nudges the
// cloud distribution point and re-reads the package record. Lifted from
// jamf-cli; package-level constant so the user controls only the upper
// bound via the `timeouts` block.
const pollInterval = 10 * time.Second

// Hash algorithm tags as the server reports them post-JCDS upload.
const (
	hashTypeSHA3512  = "SHA3_512"
	cloudStatusReady = "READY"
)

// errLocalChecksumMismatch is returned when the user-supplied
// `package_file_source_checksum` does not match the locally-computed SHA-3
// digest. The upload is aborted; nothing reaches the server.
var errLocalChecksumMismatch = errors.New("package_file_source_checksum did not match locally computed SHA-3-512")

// errCorruption is returned when the verification poll observes a non-empty
// server hash that matches neither the expected upload hash nor the
// pre-upload `previousHash`. This indicates the file uploaded differs from
// what we hashed locally.
var errCorruption = errors.New("server-computed package hash did not match locally computed SHA-3-512")

// errUploadFailed is returned when the verification poll observes a
// server-reported `size` of "0" in the same tick the hash first changes away
// from `previousHash`. JCDS occasionally starts recomputing a package's
// metadata for an upload that never actually landed the binary; size=0 is
// its signal for that failure, distinct from the transient "" while size
// catches up (continue) or a hash that matches neither expectation
// (errCorruption). Mirrors the `size_is_zero` check in jamf-cli/jamf-upload's
// JCDS poll loop. uploadAndPoll retries the upload when it sees this error.
var errUploadFailed = errors.New("server reported size=0 immediately after the uploaded package's hash changed")

// errVerificationTimeout is returned when the verification poll's context
// deadline fires before convergence.
var errVerificationTimeout = errors.New("verification poll timed out waiting for JCDS hash convergence")

// HashFileSHA3 streams the local file through sha3.New512 once, returning
// the hex digest and byte count. The provider needs SHA-3-512 for the
// convergence check and the optional `package_file_source_checksum`
// integrity hint; MD5/SHA-256 are server-computed by JCDS.
func HashFileSHA3(path string) (string, int64, error) {
	f, err := os.Open(path) //nolint:gosec // user-supplied path is intentional
	if err != nil {
		return "", 0, fmt.Errorf("opening %q for hashing: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return HashStreamSHA3(f)
}

// HashStreamSHA3 streams r through sha3.New512 once. Lets callers that
// already hold an open *os.File hash the bytes without reopening the
// file — critical for URL-source uploads where reopening would force a
// second download.
func HashStreamSHA3(r io.Reader) (string, int64, error) {
	h := sha3.New512()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", n, fmt.Errorf("hashing stream: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// PollPackageVerification nudges the cloud distribution point and polls
// the package record until JCDS has finished committing the uploaded
// binary. Convergence requires server-reported `hashValue`, `size`, AND
// `cloudTransferStatus=READY` to all line up with the locally-computed
// expectations. The size gate is critical: acceptance traces showed JCDS
// briefly returning a transient `hashValue` matching the new bytes while
// `size` was still the old binary's, then reverting — a hash-only
// convergence check would falsely declare success on that transient view.
//
// Returns errUploadFailed as soon as a server-reported `size` of "0" shows
// up alongside a hash change — a definitive failed-upload signal that would
// otherwise stall out the full poll budget before timing out. uploadAndPoll
// retries the whole upload when it sees this error.
//
// expectSha3 / expectSize describe the locally-computed digest + byte
// count of the file we just uploaded. previousHash is the SHA-3 digest
// stored before the upload so a "still-recomputing" view does not trip
// the corruption check.
//
// Each tick performs:
//
//  1. RefreshCloudDistributionPointInventoryV1(fileName) — slow (~4s) on
//     first call, fast (~200ms) afterwards.
//  2. GetPackageV1(id) — observe convergence.
//
// Returns the converged *pro.Package on success.
func PollPackageVerification(ctx context.Context, client *pro.Client, id, fileName, expectSha3 string, expectSize int64, previousHash string) (*pro.Package, error) {
	if client == nil {
		return nil, errors.New("PollPackageVerification: nil client")
	}
	expectLower := strings.ToLower(expectSha3)
	previousLower := strings.ToLower(previousHash)

	// First iteration runs immediately — no initial wait. Subsequent ticks
	// honour pollInterval.
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	tick := 0
	for {
		tick++
		// Honour ctx cancellation before each tick.
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, errVerificationTimeout
			}
			return nil, err
		}

		// Nudge the CDP. fileName drives a targeted recompute; ignore the
		// error and treat the subsequent GET as authoritative — if the
		// refresh fails transiently, the next tick will retry.
		_ = client.RefreshCloudDistributionPointInventoryV1(ctx, fileName)

		pkg, err := client.GetPackageV1(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("polling package %s: %w", id, err)
		}

		cur := strings.ToLower(helpers.DerefString(pkg.HashValue))
		hashType := helpers.DerefString(pkg.HashType)
		status := helpers.DerefString(pkg.CloudTransferStatus)
		sizeStr := helpers.DerefString(pkg.Size)

		tflog.Info(ctx, "package poll tick", map[string]any{
			"tick":           tick,
			"id":             id,
			"cur_hash":       cur,
			"expect_hash":    expectLower,
			"previous_hash":  previousLower,
			"hash_type":      hashType,
			"transfer_state": status,
			"cur_size":       sizeStr,
			"expect_size":    expectSize,
		})

		decision := classifyPollTick(cur, expectLower, previousLower, hashType, status, sizeStr, expectSize)
		switch decision {
		case pollDecisionConverged:
			return pkg, nil
		case pollDecisionUploadFailed:
			return nil, fmt.Errorf("%w: package %s", errUploadFailed, id)
		case pollDecisionCorruption:
			return nil, fmt.Errorf("%w: expected %s, server reported %s", errCorruption, expectLower, cur)
		}

		// Wait for the next tick. The first iteration's refresh+GET has
		// already fired above; subsequent iterations wait for the ticker
		// to honour pollInterval before retrying.
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, errVerificationTimeout
			}
			return nil, ctx.Err()
		case <-ticker.C:
			// next iteration
		}
	}
}

// sizeRestorePollBudget bounds the post-PUT poll that re-derives the
// server-managed `size`. The binary is already committed to the cloud
// distribution point, so refresh-inventory repopulates size within a tick
// or two; a longer wait means the CDP is busy, and we proceed with whatever
// the server reports rather than failing an otherwise-successful apply.
const sizeRestorePollBudget = 2 * time.Minute

// finalReadRestoringSize performs the final post-write GET, re-deriving the
// server-managed `size` for cloud-distribution-point-backed packages.
//
// `size` is read-only on /v1/packages and is computed only from CDP
// inventory: wire-confirmed (platform-nmartin, 2026-06-25) that the server
// drops a user-supplied size on POST and that ANY metadata PUT — whether it
// echoes size back or omits it — blanks the server-managed size to "". For a
// package whose binary lives on the CDP (cloudTransferStatus non-empty) this
// nudges refresh-inventory and polls until size repopulates, then returns the
// refreshed record. For metadata-only / FSDP packages (no CDP binary) size is
// legitimately empty and would never repopulate, so the first GET is returned
// unchanged. A poll timeout is non-fatal: the write itself succeeded and the
// next refresh (or read) will populate size — failing the apply over a
// derived field would be the worse outcome.
func finalReadRestoringSize(ctx context.Context, client *pro.Client, id, fileName string) (*pro.Package, error) {
	got, err := client.GetPackageV1(ctx, id)
	if err != nil {
		return nil, err
	}
	// No CDP binary (metadata-only / FSDP), size already populated (e.g.
	// Create, where no PUT blanked it), or no file name to target a refresh:
	// nothing to restore.
	if helpers.DerefString(got.CloudTransferStatus) == "" || helpers.DerefString(got.Size) != "" || fileName == "" {
		return got, nil
	}

	pollCtx, cancel := context.WithTimeout(ctx, sizeRestorePollBudget)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		// Nudge the CDP, then re-read. The first iteration runs immediately;
		// in practice size returns on this first refresh.
		_ = client.RefreshCloudDistributionPointInventoryV1(pollCtx, fileName)
		if refreshed, getErr := client.GetPackageV1(pollCtx, id); getErr == nil {
			got = refreshed
			if helpers.DerefString(got.Size) != "" {
				return got, nil
			}
		}
		select {
		case <-pollCtx.Done():
			// Non-fatal — return the latest record (size still empty) rather
			// than failing the apply over a server-derived field.
			tflog.Warn(ctx, "package size did not repopulate after write; proceeding with empty size", map[string]any{
				"id":        id,
				"file_name": fileName,
			})
			return got, nil
		case <-ticker.C:
		}
	}
}

// ManifestBodiesEqual compares the manifest body stored in state with the
// content found at the supplied manifest file source. Returns true when
// stored already matches the file source content — re-upload is unnecessary.
//
// The src parameter accepts either a local path or an `http(s)://` URL.
// URL sources are downloaded into a sanitised tempfile via the common
// files helper (with the standard 8 GiB cap) and the tempfile is cleaned
// up before this function returns.
func ManifestBodiesEqual(ctx context.Context, stored, src string) (bool, error) {
	if src == "" {
		return stored == "", nil
	}

	data, err := readSourceBytes(ctx, src)
	if err != nil {
		return false, err
	}

	return bytes.Equal([]byte(stored), data), nil
}

// readSourceBytes loads the full contents of a manifest source into memory.
// Manifests are small plists — not worth streaming. Reuses
// files.OpenUploadSource for the URL-or-path branch.
func readSourceBytes(ctx context.Context, src string) ([]byte, error) {
	f, _, cleanup, err := files.OpenUploadSource(ctx, src, files.DefaultMaxBytes)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading manifest source %q: %w", src, err)
	}
	return data, nil
}

// retryableUploadFailure reports whether uploadAndPoll / streamURLUploadOnce
// should re-run the whole upload after a failed poll: errUploadFailed
// (server size=0) with attempts remaining. Every other poll outcome —
// corruption, timeout, or errUploadFailed with the budget exhausted — is
// terminal. Pure-function extraction so the retry boundary is unit-tested
// without spinning up a Pro client.
func retryableUploadFailure(err error, attempt, maxAttempts int) bool {
	return errors.Is(err, errUploadFailed) && attempt < maxAttempts
}

// pollDecision summarises the convergence-check outcome for a single
// poll tick. Pure-function extraction lets the decision logic be
// unit-tested without spinning up a Pro client.
type pollDecision int

const (
	pollDecisionContinue pollDecision = iota
	pollDecisionConverged
	pollDecisionUploadFailed
	pollDecisionCorruption
)

// isZeroSize reports whether curSize is the server's literal "0" — the
// signal for a package binary that never actually landed on the cloud
// distribution point, as opposed to "" (size not yet computed; not a
// failure). An empty or unparseable curSize is not zero.
func isZeroSize(curSize string) bool {
	if curSize == "" {
		return false
	}
	n, err := strconv.ParseInt(curSize, 10, 64)
	return err == nil && n == 0
}

// classifyPollTick decides what to do with the server's response on a
// single verification poll tick. expectHash + expectSize describe the
// locally-known bytes; previousHash is the digest stored before the
// upload (used to swallow the "still recomputing" transient view).
//
//   - UploadFailed: the hash just changed away from previousHash AND size
//     reads back "0" while the upload itself is NOT actually zero bytes.
//     Checked before anything else — a zero-byte binary is a definitive
//     failed-upload signal regardless of what the hash looks like, and
//     waiting out the rest of the poll budget only delays the retry.
//     Mirrors jamf-cli/jamf-upload's `size_is_zero` check. Gated on
//     expectSize != 0 so a genuinely empty `package_file_source` (size "0"
//     really is correct) still converges normally below instead of being
//     misread as a failed upload.
//   - Converged: hash matches expected, hashType is SHA3_512, status is
//     READY, AND size matches expected. The size gate catches the
//     transient JCDS window where hash flips to new while size still
//     shows the old binary — also covers the "package being updated"
//     case, where a lingering size only matches the previous package's
//     byte count, never the new upload's.
//   - Corruption: server reported a SHA3_512 hash that is neither
//     expected nor previousHash AND size matches expected (corrupt bytes
//     would still report their own size). Skip the corruption branch
//     when size has not yet caught up — that is the transient window,
//     not a real mismatch.
//   - Continue: everything else.
func classifyPollTick(cur, expectHash, previousHash, hashType, status, curSize string, expectSize int64) pollDecision {
	hashChanged := cur != "" && cur != previousHash
	if hashChanged && expectSize != 0 && isZeroSize(curSize) {
		return pollDecisionUploadFailed
	}

	sizeMatch := curSize != "" && curSize == strconv.FormatInt(expectSize, 10)
	hashMatch := cur != "" && cur == expectHash && hashType == hashTypeSHA3512 && status == cloudStatusReady
	if hashMatch && sizeMatch {
		return pollDecisionConverged
	}
	if cur != "" && cur != expectHash && cur != previousHash && hashType == hashTypeSHA3512 && sizeMatch {
		return pollDecisionCorruption
	}
	return pollDecisionContinue
}
