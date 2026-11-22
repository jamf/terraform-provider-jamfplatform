// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"io"
)

// Source is one store's driver.
//
// Stat is a metadata-only request: it must not read the object. Open starts a
// read at offset. A non-empty pin is the marker a previous Stat or Open
// returned, and the driver must fail with ErrChanged when the object no longer
// carries it, so the bytes read are the bytes that marker described.
type Source interface {
	Stat(ctx context.Context, ref Ref) (Attributes, error)
	Open(ctx context.Context, ref Ref, pin string, offset int64) (Opened, error)
}

// Attributes is what a store reports about an object without reading it.
//
// Marker is the store's change marker, prefixed with its kind. It is compared
// as an opaque string and is never a content digest. Digests holds any
// full-object digest the store exposes, keyed by algorithm; a store that
// exposes none leaves it nil.
type Attributes struct {
	Size    int64
	Marker  string
	Digests map[string]string
}

// Opened is an object being read. The caller closes Body.
type Opened struct {
	Body     io.ReadCloser
	Filename string
	Attributes
}
