// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
)

// ErrNotConfigured is returned for a native source when there is no Resolver,
// which is the state of a resource whose Configure never received provider
// data.
var ErrNotConfigured = errors.New("the provider is not configured, so a cloud storage source cannot be read")

// ErrNotNative is returned when a source that is not s3:// or azblob:// is
// handed to the Resolver. Local paths and http(s) URLs belong to
// internal/common/files.
var ErrNotNative = errors.New("not a cloud storage source")

// ErrNoDriver is returned for a recognised scheme that has no driver.
var ErrNoDriver = errors.New("this provider build cannot read the source's scheme")

// ErrChanged is returned by a pinned read when the object no longer carries
// the marker it was pinned to: it was replaced after the marker was taken.
var ErrChanged = errors.New("the object changed after it was last examined")

// ErrNotFound is returned when the store reports that the object does not
// exist.
var ErrNotFound = errors.New("the object does not exist")

// ErrAccessDenied is returned when the store refuses the read.
var ErrAccessDenied = errors.New("the store refused access to the object")

// ErrCredentials is returned when no usable credential could be built for the
// store, before any request for the object is made.
var ErrCredentials = errors.New("no usable credential for the store")

// ErrNoSize is returned when the store does not declare the object's size. An
// upload needs the size before the first byte, and a read cannot be checked
// for completeness without it.
var ErrNoSize = errors.New("the store did not declare the object's size")

// ErrIncomplete is returned by Stage when the store delivered fewer or more
// bytes than it declared.
var ErrIncomplete = errors.New("the read ended before the whole object arrived")

// ErrTooLarge is returned by Open when the object declares more bytes than the
// caller allows. Nothing has been read.
var ErrTooLarge = errors.New("the object is larger than the upload size limit")

// Resolver reads native sources with the credentials one provider instance
// was configured with. The zero value is not usable; build one with New. A
// nil Resolver is the unconfigured state: it answers Native truthfully and
// refuses every read with ErrNotConfigured.
type Resolver struct {
	config     Config
	unresolved []string
	drivers    map[Kind]Source
	https      *httpsOpener
}

// String keeps a Resolver out of logs. fmt does not call String on the
// unexported fields of a struct it prints, so the redaction on Config alone
// would not cover a Resolver printed whole.
func (*Resolver) String() string { return redacted }

// GoString keeps a Resolver out of %#v output.
func (*Resolver) GoString() string { return redacted }

// New builds the Resolver for one configured provider instance. unresolved
// lists the file_sources paths whose values were unknown when it was built,
// which happens during a plan when a value derives from another resource.
//
// It builds no client and makes no request. A driver authenticates on first
// use, because validating credentials costs a round trip and most
// configurations never name a native source.
func New(config Config, unresolved []string) *Resolver {
	return &Resolver{
		config:     config,
		unresolved: slices.Clone(unresolved),
		drivers: map[Kind]Source{
			KindS3:        newS3Driver(config.S3, config.UserAgent),
			KindAzureBlob: newAzureDriver(config.Azure, config.UserAgent, config.HTTPTransport),
		},
		https: newHTTPSOpener(config.HTTPS, config.UserAgent, config.HTTPTransport),
	}
}

// Unresolved returns the file_sources paths that were unknown when the
// Resolver was built, in the order the provider met them. Empty once every
// value is known, which is always the case during apply.
func (r *Resolver) Unresolved() []string {
	if r == nil {
		return nil
	}
	return slices.Clone(r.unresolved)
}

// Native reports whether src is a source this package reads. A malformed
// native source still reports true, so the caller routes it here and gets the
// syntax error from Stat or Open in place of a missing-file error from the
// local-path branch. Nil-receiver-safe.
func (r *Resolver) Native(src string) bool {
	return Native(src)
}

// Native reports whether src is an s3:// or azblob:// source, well formed or
// not. It needs no Resolver, so a caller deciding only which branch a source
// belongs to can ask without one.
func Native(src string) bool {
	ref, _ := Parse(src)
	return ref.Native()
}

// Versioned reports whether src names one version of an object, with a
// ?versionId= suffix. Such a source is immutable: the bytes behind it cannot
// change, so a marker recorded for it never goes stale.
func Versioned(src string) bool {
	ref, err := Parse(src)
	return err == nil && ref.VersionID != ""
}

// UnresolvedFor returns the unknown file_sources paths that reading src
// depends on: the whole block, or anything under the block of the source's
// own store. A value unknown under another store's block does not stop this
// source from being read. Empty for a source that is not native.
func (r *Resolver) UnresolvedFor(src string) []string {
	ref, _ := Parse(src)
	block := ""
	switch ref.Kind {
	case KindS3:
		block = unresolvedRoot + ".s3"
	case KindAzureBlob:
		block = unresolvedRoot + ".azure"
	default:
		return nil
	}
	var out []string
	for _, at := range r.Unresolved() {
		if at == unresolvedRoot || at == block || strings.HasPrefix(at, block+".") {
			out = append(out, at)
		}
	}
	return out
}

// unresolvedRoot is the provider attribute every unresolved path starts with.
const unresolvedRoot = "file_sources"

// Stat reports the object's size and marker without reading it.
func (r *Resolver) Stat(ctx context.Context, src string) (Attributes, error) {
	ref, driver, err := r.driver(src)
	if err != nil {
		return Attributes{}, err
	}
	tflog.Debug(ctx, "examining cloud storage object", map[string]any{"source": ref.String()})
	return driver.Stat(ctx, ref)
}

// Open starts reading the object from its first byte. A non-empty pin makes
// the read fail with ErrChanged unless the object still carries that marker.
// maxBytes caps the declared size, and zero or less means
// files.DefaultMaxBytes; an object over the cap is refused before a byte is
// read. The returned Body resumes by itself when the connection is cut
// (resumingBody), for as long as ctx lives, so ctx must outlast the read. The
// caller closes the returned Body.
func (r *Resolver) Open(ctx context.Context, src, pin string, maxBytes int64) (Opened, error) {
	ref, driver, err := r.driver(src)
	if err != nil {
		return Opened{}, err
	}
	if maxBytes <= 0 {
		maxBytes = files.DefaultMaxBytes
	}
	tflog.Debug(ctx, "reading cloud storage object", map[string]any{"source": ref.String(), "pinned": pin != ""})
	opened, err := driver.Open(ctx, ref, pin, 0)
	if err != nil {
		return Opened{}, err
	}
	if opened.Size > maxBytes {
		_ = opened.Body.Close()
		return Opened{}, fmt.Errorf("%w: %s declares %d bytes, over the %d byte limit", ErrTooLarge, ref, opened.Size, maxBytes)
	}
	if opened.Filename == "" {
		opened.Filename = ref.Filename()
	}
	opened.Body = &resumingBody{ctx: ctx, source: driver, ref: ref, pin: opened.Marker, size: opened.Size, body: opened.Body}
	return opened, nil
}

// driver parses src and returns the driver for its scheme.
func (r *Resolver) driver(src string) (Ref, Source, error) {
	ref, err := Parse(src)
	if err != nil {
		return ref, nil, err
	}
	if !ref.Native() {
		return ref, nil, ErrNotNative
	}
	if r == nil {
		return ref, nil, ErrNotConfigured
	}
	driver, ok := r.drivers[ref.Kind]
	if !ok {
		return ref, nil, fmt.Errorf("%w: %s://", ErrNoDriver, ref.Scheme)
	}
	return ref, driver, nil
}
