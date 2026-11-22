// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Staged is a native source copied to a temporary file, rewound and ready to
// read. Attributes are the ones the store reported when the read began, so
// Marker describes the bytes in File.
type Staged struct {
	File     *os.File
	Filename string
	Attributes
}

// Stage copies the object to a temporary file and returns it rewound, with a
// cleanup that closes and removes it. The caller must run cleanup, which is
// never nil.
//
// An upload that hashes its source before sending it, and may send it twice
// on a retry, needs a seekable file; this is the cloud storage counterpart of
// files.OpenUploadSource for a URL. pin and maxBytes are as for Open.
//
// The file is created by os.CreateTemp, so it is readable by the current user
// alone, and its name comes from a fixed pattern, never from the object's
// name. A process that is killed before cleanup runs leaves it behind.
//
// The byte count is checked against the size the store declared. A read that
// stops early surfaces as an error from the body, and the check catches the
// rest: a store that ends the stream cleanly but short must not produce a
// truncated upload that hashes consistently with itself.
func (r *Resolver) Stage(ctx context.Context, src, pin string, maxBytes int64) (Staged, func(), error) {
	noop := func() {}
	opened, err := r.Open(ctx, src, pin, maxBytes)
	if err != nil {
		return Staged{}, noop, err
	}
	defer func() { _ = opened.Body.Close() }()

	tmp, err := os.CreateTemp("", "jamfplatform-source-*")
	if err != nil {
		return Staged{}, noop, fmt.Errorf("creating a temporary file for the source: %w", err)
	}
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}

	body := &readFailure{reader: io.LimitReader(opened.Body, opened.Size+1)}
	written, err := io.Copy(tmp, body)
	if err != nil {
		cleanup()
		if body.err != nil {
			return Staged{}, noop, fmt.Errorf("%w: %w", ErrIncomplete, err)
		}
		return Staged{}, noop, fmt.Errorf("writing the source to a temporary file: %w", err)
	}
	if written != opened.Size {
		cleanup()
		return Staged{}, noop, fmt.Errorf("%w: the store declared %d bytes and sent %d", ErrIncomplete, opened.Size, written)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return Staged{}, noop, fmt.Errorf("rewinding the temporary file: %w", err)
	}
	return Staged{File: tmp, Filename: opened.Filename, Attributes: opened.Attributes}, cleanup, nil
}

// readFailure remembers whether a read failed, so a copy that stops can be
// laid at the right door: the store that stopped sending, or the disk that
// stopped taking.
type readFailure struct {
	reader io.Reader
	err    error
}

func (r *readFailure) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}
