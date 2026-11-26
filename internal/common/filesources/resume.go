// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// maxResumeAttempts is how many times in a row a read may be reopened without
// delivering a byte before it is given up. A reopen that makes progress
// resets the count, so a long transfer over a poor link is not capped at five
// cuts in total.
const maxResumeAttempts = 5

// resumeBackoff is the wait before the first reopen, doubled for each
// consecutive failure. A variable so tests do not wait.
var resumeBackoff = time.Second

// resumingBody reads a cloud storage object and survives the connection being
// cut partway.
//
// A package is gigabytes, and a transfer that long is cut often enough to
// matter. Both stores report a cut the same way, as an error or an early end
// of the body and never as a short success (probed 2026-11-22), so the reader
// can tell a cut from the end of the object: it has not yet read the size the
// store declared. It then opens the object again from the byte it has reached
// and carries on, and the caller sees one uninterrupted stream.
//
// Two rules make that safe. The reopened read is pinned to the marker the
// first response carried, whether or not the caller pinned the first read, so
// bytes from two different objects are never spliced: an object replaced in
// the meantime fails with ErrChanged. And the reopened response must describe
// the same object from the right place, the same total size starting at the
// offset asked for, which is the store's word that the offset was honoured.
//
// The driver is asked afresh for every reopen and so authenticates afresh. A
// credential that expired while the first part was in flight must not be
// reused; the AWS SDK and the Azure token source both renew on demand.
//
// An object whose store reported no marker cannot be pinned, so its read is
// not resumed and a cut surfaces as it is.
type resumingBody struct {
	ctx    context.Context
	source Source
	ref    Ref
	pin    string
	size   int64

	body     io.ReadCloser
	offset   int64
	failures int
	cut      error
	failed   error
	closed   bool
}

// Read implements io.Reader.
//
// An error that arrives once every declared byte has been delivered is not a
// cut: there is nothing left to resume, and reopening at the end of the
// object would only draw a refusal that hides the real error. It is returned
// as it is. A terminal error is kept and returned by every later Read, and a
// closed reader reads nothing, so neither can start a new request.
func (b *resumingBody) Read(p []byte) (int, error) {
	for {
		switch {
		case b.closed:
			return 0, errors.New("read of a closed cloud storage object")
		case b.failed != nil:
			return 0, b.failed
		}
		if b.cut != nil {
			if err := b.reopen(); err != nil {
				b.failed = err
				return 0, err
			}
		}
		n, err := b.body.Read(p)
		b.offset += int64(n)
		if n > 0 {
			b.failures = 0
		}
		switch {
		case err == nil:
			return n, nil
		case errors.Is(err, io.EOF) && b.offset >= b.size:
			return n, io.EOF
		case errors.Is(err, io.EOF):
			err = io.ErrUnexpectedEOF
		case b.offset >= b.size:
			b.failed = err
			return n, err
		}
		if b.pin == "" {
			b.failed = err
			return n, err
		}
		b.cut = err
		if n > 0 {
			return n, nil
		}
	}
}

// reopen replaces the cut body with a new read from the current offset.
//
// A refusal that another attempt cannot change ends the read at once: the
// object is gone, access is denied, or there is no credential. Only a failure
// that looks like the network is tried again. The reopened response must
// carry the marker the read is pinned to. A store that honours If-Match
// cannot send another, but an S3-compatible one that ignores the header
// could, and its bytes would otherwise be joined to the first object's.
func (b *resumingBody) reopen() error {
	cause := b.cut
	_ = b.body.Close()
	b.body = nil
	for {
		if err := b.ctx.Err(); err != nil {
			return fmt.Errorf("%w: %w", ErrIncomplete, err)
		}
		b.failures++
		if b.failures > maxResumeAttempts {
			return fmt.Errorf("%w: %s was cut at byte %d of %d and could not be resumed in %d attempts: %w",
				ErrIncomplete, b.ref, b.offset, b.size, maxResumeAttempts, cause)
		}
		select {
		case <-b.ctx.Done():
			return fmt.Errorf("%w: %w", ErrIncomplete, b.ctx.Err())
		case <-time.After(resumeBackoff << (b.failures - 1)):
		}
		tflog.Debug(b.ctx, "resuming a cut cloud storage read", map[string]any{
			"source": b.ref.String(), "offset": b.offset, "size": b.size, "attempt": b.failures, "cause": cause.Error(),
		})
		opened, err := b.source.Open(b.ctx, b.ref, b.pin, b.offset)
		switch {
		case errors.Is(err, ErrChanged):
			return fmt.Errorf("%w while it was being read, at byte %d of %d", err, b.offset, b.size)
		case errors.Is(err, ErrNotFound), errors.Is(err, ErrAccessDenied), errors.Is(err, ErrCredentials):
			return fmt.Errorf("resuming at byte %d of %d: %w", b.offset, b.size, err)
		case err != nil:
			cause = err
			continue
		case opened.Marker != "" && opened.Marker != b.pin:
			_ = opened.Body.Close()
			return fmt.Errorf("%w while it was being read, at byte %d of %d: %s", ErrChanged, b.offset, b.size, b.ref)
		case opened.Size != b.size:
			_ = opened.Body.Close()
			return fmt.Errorf("%w: resuming %s at byte %d, the store now reports %d bytes where it first reported %d", ErrIncomplete, b.ref, b.offset, opened.Size, b.size)
		}
		b.body, b.cut = opened.Body, nil
		return nil
	}
}

// Close implements io.Closer. It is safe to call more than once, and after a
// reopen that failed and left no body open.
func (b *resumingBody) Close() error {
	b.closed = true
	if b.body == nil {
		return nil
	}
	body := b.body
	b.body = nil
	return body.Close()
}

// parseContentRange reads the first byte and the total size out of a
// Content-Range header ("bytes 5-9/10"). A ranged response declares the
// length of the range in Content-Length, so the object's size has to come
// from here, and the first byte is the store's confirmation of where the
// range starts. ok is false for a header that gives no total ("bytes 5-9/*")
// or does not parse.
func parseContentRange(header string) (start, total int64, ok bool) {
	span, size, found := strings.Cut(strings.TrimPrefix(header, "bytes "), "/")
	if !found {
		return 0, 0, false
	}
	first, _, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, false
	}
	start, startErr := strconv.ParseInt(first, 10, 64)
	total, totalErr := strconv.ParseInt(size, 10, 64)
	return start, total, startErr == nil && totalErr == nil
}

// rangedSize returns the object's whole size for a read that started at
// offset, checked against the Content-Range the store sent back. A store that
// ignores the Range and sends the object from its first byte would otherwise
// be spliced onto what was already read.
func rangedSize(ref Ref, header string, offset int64) (int64, error) {
	start, total, ok := parseContentRange(header)
	switch {
	case !ok:
		return 0, fmt.Errorf("%w: %s", ErrNoSize, ref)
	case start != offset:
		return 0, fmt.Errorf("%w: asked for %s from byte %d and the store answered from byte %d", ErrIncomplete, ref, offset, start)
	}
	return total, nil
}
