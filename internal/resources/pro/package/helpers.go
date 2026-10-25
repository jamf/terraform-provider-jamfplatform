// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // MD5 is an integrity value the server reports, not a security control
	"crypto/sha256"
	"crypto/sha3"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
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

// The cloudTransferStatus values the verification poll acts on. Any other
// status is still in flight.
var (
	cloudStatusReady = string(pro.CloudDistributionPointInventoryFileInfoStatusReady)
	cloudStatusError = string(pro.CloudDistributionPointInventoryFileInfoStatusError)
)

// errLocalChecksumMismatch is returned when the user-supplied
// `package_file_source_checksum` does not match the locally-computed SHA-3
// digest. The upload is aborted; nothing reaches the server.
var errLocalChecksumMismatch = errors.New("package_file_source_checksum did not match locally computed SHA-3-512")

// errCorruption is returned when the verification poll observes a non-empty
// server digest that matches neither the locally computed digest nor the
// pre-upload one. This indicates the file uploaded differs from what we
// hashed locally.
var errCorruption = errors.New("server-computed package hash did not match the locally computed digest")

// errTransferFailed is returned when the distribution point reports the
// transfer of the uploaded file as ERROR. Nothing further will arrive, so
// the poll fails rather than waiting out its budget.
var errTransferFailed = errors.New("distribution point reported the package transfer as ERROR")

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
var errVerificationTimeout = errors.New("verification poll timed out waiting for the distribution point to compute a hash")

// verifyGrace is how long a package that is READY at the uploaded file's size
// gets to show a hash before the upload is reported as unverified. Jamf Pro
// computes no hash for some large packages, so waiting out the full create or
// update timeout would only delay an outcome that cannot change. It is a
// variable so tests can shorten it.
var verifyGrace = 20 * time.Second

// unverifiedError is a package that uploaded and is READY at the right size
// but could not be integrity-checked because Jamf Pro computed no hash for it.
// It is a warning, not a failure: the binary is on the distribution point.
type unverifiedError struct{ reason string }

func (e *unverifiedError) Error() string { return e.reason }

// packageVerifier is the pair of SDK calls the verification poll makes, named as
// an interface so the loop is unit-testable without a live tenant. *pro.Client
// satisfies it.
type packageVerifier interface {
	RefreshCloudDistributionPointInventoryV1(ctx context.Context, fileName string) error
	GetPackageV1(ctx context.Context, id string) (*pro.Package, error)
}

// fileDigests holds the digests of one package file. Locally all three are
// computed. On the server each is filled only by the distribution points that
// compute it: JCDS fills all three, an Amazon S3 distribution point MD5 only.
type fileDigests struct {
	sha3, sha256, md5 string
}

// digestPair is one algorithm's digest, labelled for diagnostics.
type digestPair struct {
	label, value string
}

// pairs returns the digests strongest first. Index i means the same algorithm
// in every fileDigests, so a server value and a local value at one index are
// comparable.
func (d fileDigests) pairs() []digestPair {
	return []digestPair{{"SHA3-512", d.sha3}, {"SHA-256", d.sha256}, {"MD5", d.md5}}
}

// strongest returns the index into pairs of the first populated digest, or -1
// when none is populated.
func (d fileDigests) strongest() int {
	for i, p := range d.pairs() {
		if p.value != "" {
			return i
		}
	}
	return -1
}

// matches reports whether the strongest digest recorded in d equals the same
// algorithm's digest of local. An empty record matches nothing.
func (d fileDigests) matches(local fileDigests) bool {
	i := d.strongest()
	if i < 0 {
		return false
	}
	return strings.EqualFold(d.pairs()[i].value, local.pairs()[i].value)
}

// recordedDigests reads the digests the package record holds, lowercased.
func recordedDigests(p *pro.Package) fileDigests {
	return fileDigests{
		sha3:   strings.ToLower(helpers.DerefString(p.Sha3512)),
		sha256: strings.ToLower(helpers.DerefString(p.Sha256)),
		md5:    strings.ToLower(helpers.DerefString(p.Md5)),
	}
}

// recordedDigestsFromState reads the digests held in state, lowercased.
func recordedDigestsFromState(m *PackageResourceModel) fileDigests {
	return fileDigests{
		sha3:   strings.ToLower(m.Sha3512.ValueString()),
		sha256: strings.ToLower(m.Sha256.ValueString()),
		md5:    strings.ToLower(m.Md5.ValueString()),
	}
}

// sameContent reports whether the record holds the bytes local describes. With
// a recorded digest the strongest one decides. A record with none, which is what
// Jamf Pro keeps for a large package it never hashes, can only be judged by
// size: an identical byte count is taken as unchanged, because nothing else is
// available to say otherwise, and a record with no size is never a match.
func (d fileDigests) sameContent(local fileDigests, recordedSize string, localSize int64) bool {
	if d.strongest() >= 0 {
		return d.matches(local)
	}
	return recordedSize != "" && recordedSize == strconv.FormatInt(localSize, 10)
}

// unverifiableTick reports whether the record looks like a package Jamf Pro will
// not hash: the transfer is READY at the uploaded file's size and no digest is
// populated. A digest that is populated but stale is not matched here, because
// a hash still arriving is indistinguishable from one that never will, and the
// poll keeps waiting for it. A zero-byte upload is excluded so the existing
// empty-package convergence is untouched.
func unverifiableTick(server fileDigests, status, curSize string, expectSize int64) bool {
	return expectSize > 0 &&
		server.strongest() < 0 &&
		status == cloudStatusReady &&
		curSize == strconv.FormatInt(expectSize, 10)
}

// newUnverifiedError explains why a READY package could not be verified, telling
// a create, a replace that cleared the previous hash, and a replace of identical
// size apart. previous and previousSize describe the record before the upload.
func newUnverifiedError(fileName string, size int64, previous fileDigests, previousSize string) *unverifiedError {
	sizeText := strconv.FormatInt(size, 10)
	var why string
	switch {
	case previousSize == sizeText:
		why = fmt.Sprintf("Jamf Pro replaced %s with a file of the same size (%s bytes), so the size cannot confirm the replace, and computed no new hash.", fileName, sizeText)
	case previous.strongest() >= 0:
		why = fmt.Sprintf("Jamf Pro replaced %s and reports it as ready at the new size (%s bytes) but computed no new hash. The previous hash was cleared.", fileName, sizeText)
	default:
		why = fmt.Sprintf("Jamf Pro reports %s as ready at the uploaded size (%s bytes) but computed no hash for it.", fileName, sizeText)
	}
	return &unverifiedError{reason: why + " The upload is not integrity-verified. Jamf Pro does not hash some large packages. Compare the file against your source before relying on it."}
}

// digester computes all three digests of a stream in one pass.
type digester struct {
	sha3, sha256, md5 hash.Hash
}

func newDigester() digester {
	return digester{sha3: sha3.New512(), sha256: sha256.New(), md5: md5.New()} //nolint:gosec // see the md5 import
}

// writer returns a writer that feeds every digest.
func (d digester) writer() io.Writer {
	return io.MultiWriter(d.sha3, d.sha256, d.md5)
}

// sum returns the digests of everything written so far.
func (d digester) sum() fileDigests {
	return fileDigests{
		sha3:   hex.EncodeToString(d.sha3.Sum(nil)),
		sha256: hex.EncodeToString(d.sha256.Sum(nil)),
		md5:    hex.EncodeToString(d.md5.Sum(nil)),
	}
}

// HashFileSHA3 returns the SHA-3-512 hex digest and byte count of a local file.
// Exported for the acceptance fixtures, which compare against the server's
// SHA-3-512 on Jamf Cloud.
func HashFileSHA3(path string) (string, int64, error) {
	d, n, err := hashFile(path)
	return d.sha3, n, err
}

// HashFileDigests returns the SHA-3-512 and MD5 hex digests and byte count of
// a local file. Exported for the acceptance fixtures, which check the digest a
// distribution point reports for the file it holds.
func HashFileDigests(path string) (string, string, int64, error) {
	d, n, err := hashFile(path)
	return d.sha3, d.md5, n, err
}

// hashFile streams the local file once, returning every digest and the byte
// count.
func hashFile(path string) (fileDigests, int64, error) {
	f, err := os.Open(path) //nolint:gosec // user-supplied path is intentional
	if err != nil {
		return fileDigests{}, 0, fmt.Errorf("opening %q for hashing: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return hashStream(f)
}

// hashStream streams r once through every digest. Lets callers that already
// hold an open *os.File hash the bytes without reopening the file — critical
// for URL-source uploads where reopening would force a second download.
func hashStream(r io.Reader) (fileDigests, int64, error) {
	d := newDigester()
	n, err := io.Copy(d.writer(), r)
	if err != nil {
		return fileDigests{}, n, fmt.Errorf("hashing stream: %w", err)
	}
	return d.sum(), n, nil
}

// PollPackageVerification nudges the cloud distribution point and polls the
// package record until the distribution point has finished committing the
// uploaded binary. Convergence requires the strongest digest the server holds
// to equal the locally computed one, with `size` and `cloudTransferStatus=READY`
// lining up too. The size gate is critical: acceptance traces showed JCDS
// briefly returning a transient hash matching the new bytes while `size` was
// still the old binary's, then reverting — a hash-only convergence check would
// falsely declare success on that transient view.
//
// Which digests exist depends on the distribution point: JCDS fills SHA3-512,
// an Amazon S3 distribution point MD5 only. The check compares the strongest
// one populated, so it never waits for a digest that will not arrive.
//
// Returns errUploadFailed as soon as a server-reported `size` of "0" shows
// up alongside a digest change, and errTransferFailed as soon as the transfer
// reports ERROR. uploadAndPoll retries the whole upload when it sees the former.
//
// local is the digest set of the file we just uploaded; previous is the digest
// set stored before the upload, so a "still-recomputing" view does not trip the
// corruption check. previousSize is the record's size before the upload, empty
// on Create.
//
// Jamf Pro computes no hash for some large packages even once they are READY.
// When the transfer is READY at the uploaded size and the record still holds no
// digest, the poll allows verifyGrace and then returns an *unverifiedError
// instead of waiting out the caller's timeout. A digest that is present but
// wrong is still errCorruption, and a digest that is stale keeps the poll waiting.
//
// Each tick performs:
//
//  1. RefreshCloudDistributionPointInventoryV1(fileName) — slow (~4s) on
//     first call, fast (~200ms) afterwards.
//  2. GetPackageV1(id) — observe convergence.
//
// Returns the converged *pro.Package on success.
func PollPackageVerification(ctx context.Context, client *pro.Client, id, fileName string, local fileDigests, expectSize int64, previous fileDigests, previousSize string) (*pro.Package, error) {
	if client == nil {
		return nil, errors.New("PollPackageVerification: nil client")
	}
	return pollPackageVerification(ctx, client, pollInterval, verifyGrace, id, fileName, local, expectSize, previous, previousSize)
}

// pollPackageVerification is PollPackageVerification with the cadence and the
// unverified grace passed in, so the loop is testable against a fake verifier.
func pollPackageVerification(ctx context.Context, client packageVerifier, interval, grace time.Duration, id, fileName string, local fileDigests, expectSize int64, previous fileDigests, previousSize string) (*pro.Package, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var graceStart time.Time
	tick := 0
	for {
		tick++
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, errVerificationTimeout
			}
			return nil, err
		}

		_ = client.RefreshCloudDistributionPointInventoryV1(ctx, fileName)

		pkg, err := client.GetPackageV1(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("polling package %s: %w", id, err)
		}

		server := recordedDigests(pkg)
		status := helpers.DerefString(pkg.CloudTransferStatus)
		sizeStr := helpers.DerefString(pkg.Size)

		tflog.Info(ctx, "package poll tick", map[string]any{
			"tick":           tick,
			"id":             id,
			"cur_sha3":       server.sha3,
			"cur_sha256":     server.sha256,
			"cur_md5":        server.md5,
			"expect_sha3":    local.sha3,
			"previous_sha3":  previous.sha3,
			"transfer_state": status,
			"cur_size":       sizeStr,
			"expect_size":    expectSize,
		})

		decision := classifyPollTick(server, local, previous, status, sizeStr, expectSize)
		switch decision {
		case pollDecisionConverged:
			return pkg, nil
		case pollDecisionUploadFailed:
			return nil, fmt.Errorf("%w: package %s", errUploadFailed, id)
		case pollDecisionTransferFailed:
			return nil, fmt.Errorf("%w: package %s", errTransferFailed, id)
		case pollDecisionCorruption:
			return nil, fmt.Errorf("%w: expected %s, server reported %s", errCorruption, local.sha3, server.sha3)
		}

		if unverifiableTick(server, status, sizeStr, expectSize) {
			if graceStart.IsZero() {
				graceStart = time.Now()
			}
			if time.Since(graceStart) >= grace {
				return nil, newUnverifiedError(fileName, expectSize, previous, previousSize)
			}
		} else {
			graceStart = time.Time{}
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, errVerificationTimeout
			}
			return nil, ctx.Err()
		case <-ticker.C:
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
	pollDecisionTransferFailed
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
// single verification poll tick. server is the digests the package record
// holds, local those of the bytes uploaded, previous those stored before the
// upload; curSize is the record's size and expectSize the local byte count.
//
// The comparison uses the strongest digest the server populated, at the same
// algorithm on both sides, so a distribution point that computes only MD5 is
// judged on MD5.
//
//   - TransferFailed: status is ERROR. Checked first; nothing further arrives.
//   - UploadFailed: the digest just changed away from previous AND size reads
//     back "0" while the upload itself is NOT actually zero bytes. Checked
//     before the match branches — a zero-byte binary is a definitive
//     failed-upload signal regardless of the digest, and waiting out the
//     rest of the poll budget only delays the retry. Gated on expectSize != 0
//     so a genuinely empty package_file_source (size "0" really is correct)
//     still converges normally below.
//   - Converged: digest matches local, status is READY, AND size matches
//     expected. The size gate catches the transient JCDS window where the
//     digest flips to new while size still shows the old binary — also covers
//     the "package being updated" case, where a lingering size only matches
//     the previous package's byte count, never the new upload's.
//   - Corruption: server reported a digest that is neither local nor previous
//     AND size matches expected (corrupt bytes would still report their own
//     size). Skip when size has not yet caught up — that is the transient
//     window, not a real mismatch.
//   - Continue: everything else, including no digest populated yet.
func classifyPollTick(server, local, previous fileDigests, status, curSize string, expectSize int64) pollDecision {
	if status == cloudStatusError {
		return pollDecisionTransferFailed
	}
	i := server.strongest()
	if i < 0 {
		return pollDecisionContinue
	}
	cur := server.pairs()[i].value
	localVal := local.pairs()[i].value
	prevVal := previous.pairs()[i].value

	if cur != prevVal && expectSize != 0 && isZeroSize(curSize) {
		return pollDecisionUploadFailed
	}

	sizeMatch := curSize != "" && curSize == strconv.FormatInt(expectSize, 10)
	if cur == localVal && status == cloudStatusReady && sizeMatch {
		return pollDecisionConverged
	}
	if cur != localVal && cur != prevVal && sizeMatch {
		return pollDecisionCorruption
	}
	return pollDecisionContinue
}
