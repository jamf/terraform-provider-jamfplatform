// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package files provides shared upload-source plumbing used by resources that
// stream binary payloads to Jamf Pro (package binaries, package manifests,
// icons, ...). The Layer 1 primitives detect URL vs local-path inputs,
// safely write URL downloads into the OS tempdir, and hand the caller an
// *os.File suitable for direct streaming to the SDK's Upload* helpers.
//
// Package-binary-specific orchestration (hash convergence, refresh-nudge
// polling) lives in the consuming resource, NOT here — only universal
// primitives belong in this package.
package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultMaxBytes caps the size of a URL-sourced download. 8 GiB lifts the
// upper bound well above realistic .pkg payloads while still defending
// against runaway responses.
const DefaultMaxBytes int64 = 8 * 1024 * 1024 * 1024

// maxRedirects bounds the HTTP redirect chain a URL download will follow.
const maxRedirects = 10

// urlPattern matches a leading http:// or https:// scheme. Used to decide
// whether an upload source string is a URL or a local filesystem path.
var urlPattern = regexp.MustCompile(`^https?://`)

// ErrDownloadCapExceeded is returned by DownloadCapped when a response body
// would exceed the supplied byte limit. The caller MUST treat the partial
// destination as untrusted and discard it.
var ErrDownloadCapExceeded = errors.New("files: download exceeded byte cap")

// ErrUnsafeTempPath is returned by SanitizedTempPath when the candidate
// basename would escape os.TempDir() (path traversal, absolute path, ...).
var ErrUnsafeTempPath = errors.New("files: refusing unsafe URL-derived temp path")

// URLSource reports whether src is an http:// or https:// URL. Anything else
// is treated as a local filesystem path by OpenUploadSource.
func URLSource(src string) bool {
	return urlPattern.MatchString(src)
}

// SanitizedTempPath returns a safe absolute path under os.TempDir() derived
// from the basename of rawURL's path. It rejects "..", absolute paths, and
// any candidate whose computed parent is not the tempdir itself. The
// returned filename does NOT exist on disk yet — the caller is responsible
// for creating it.
func SanitizedTempPath(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("files: parsing URL %q: %w", rawURL, err)
	}

	base := path.Base(parsed.Path)
	// path.Base returns "." for an empty path and "/" for the root.
	if base == "" || base == "." || base == "/" {
		base = "download"
	}

	// Reject path traversal attempts and absolute references.
	if strings.Contains(base, "..") || strings.ContainsRune(base, os.PathSeparator) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeTempPath, base)
	}
	if filepath.IsAbs(base) {
		return "", fmt.Errorf("%w: absolute path %q", ErrUnsafeTempPath, base)
	}

	tempDir := os.TempDir()
	candidate := filepath.Join(tempDir, base)

	// Re-verify the joined path lives under tempDir. filepath.Clean inside Join
	// would resolve "../" components — defence in depth against a basename we
	// did not anticipate.
	cleanParent := filepath.Clean(filepath.Dir(candidate))
	cleanTempDir := filepath.Clean(tempDir)
	if cleanParent != cleanTempDir {
		return "", fmt.Errorf("%w: parent %q != tempdir %q", ErrUnsafeTempPath, cleanParent, cleanTempDir)
	}

	return candidate, nil
}

// DownloadCapped streams src into dst with the supplied byte cap applied via
// io.LimitReader. It follows up to 10 redirects, then returns the basename
// resolved post-redirect (server-suggested filename). Errors with
// ErrDownloadCapExceeded if the response body exceeds maxBytes; the caller
// must discard whatever bytes landed in dst.
func DownloadCapped(ctx context.Context, src string, dst io.Writer, maxBytes int64) (string, int64, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	redirects := 0
	httpClient := &http.Client{
		Timeout: 0, // bound by ctx
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			redirects = len(via)
			if redirects > maxRedirects {
				return fmt.Errorf("files: exceeded %d redirects", maxRedirects)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return "", 0, fmt.Errorf("files: building request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("files: fetching %q: %w", src, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("files: GET %q returned HTTP %d", src, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxBytes)
	written, err := io.Copy(dst, limited)
	if err != nil {
		return "", written, fmt.Errorf("files: copying body: %w", err)
	}

	// Detect a body that would have exceeded the cap. After LimitReader is
	// exhausted, one additional byte from resp.Body proves the source had more
	// data than allowed.
	overflow := make([]byte, 1)
	n, _ := resp.Body.Read(overflow)
	if n > 0 {
		return "", written, ErrDownloadCapExceeded
	}

	// Filename: use the final URL's basename after redirects.
	filename := path.Base(resp.Request.URL.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	return filename, written, nil
}

// OpenURLStream issues a GET against src with the standard 10-redirect cap
// and returns the response body wrapped in an io.LimitReader so the caller
// cannot read past maxBytes. The final resolved filename (post-redirect)
// is returned alongside.
//
// Unlike DownloadCapped this function does NOT consume the body — it
// returns it for the caller to stream directly into the next stage of a
// pipeline (e.g. a multipart upload writer). The caller MUST Close() the
// returned io.ReadCloser when finished.
//
// Note: the cap-exceeded check is impossible to perform up-front because
// the body has not been read. Callers that want enforcement must wrap
// the returned reader in their own counted-copy and compare against
// maxBytes after the stream completes.
//
// The returned int64 is the response's declared Content-Length, or -1 when
// the origin sent none (chunked, or transparently decompressed). The SDK
// refuses to upload a body of unknown size, so a caller that needs to
// stream must fall back to OpenUploadSource on -1. A declared length above
// maxBytes is rejected here, before any byte is read.
func OpenURLStream(ctx context.Context, src string, maxBytes int64) (io.ReadCloser, string, int64, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	redirects := 0
	httpClient := &http.Client{
		Timeout: 0,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			redirects = len(via)
			if redirects > maxRedirects {
				return fmt.Errorf("files: exceeded %d redirects", maxRedirects)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", -1, fmt.Errorf("files: building request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", -1, fmt.Errorf("files: fetching %q: %w", src, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, "", -1, fmt.Errorf("files: GET %q returned HTTP %d", src, resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		_ = resp.Body.Close()
		return nil, "", -1, fmt.Errorf("files: %q declares %d bytes, over the %d byte cap", src, resp.ContentLength, maxBytes)
	}

	filename := path.Base(resp.Request.URL.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	limited := io.LimitReader(resp.Body, maxBytes)
	size := resp.ContentLength
	if resp.Uncompressed {
		size = -1
	}
	return &limitedBodyCloser{Reader: limited, body: resp.Body}, filename, size, nil
}

// limitedBodyCloser pairs a LimitReader view with the underlying body so
// Close cleans up the upstream connection.
type limitedBodyCloser struct {
	io.Reader
	body io.ReadCloser
}

func (l *limitedBodyCloser) Close() error { return l.body.Close() }

// OpenUploadSource resolves src into an *os.File suitable for streaming to
// an SDK Upload* method. For URL sources the body is downloaded into a
// sanitised tempfile (with the byte cap applied); cleanup os.Removes it.
// For local paths the file is opened read-only and cleanup is file.Close.
//
// Callers MUST defer the returned cleanup closure. The returned *os.File
// remains owned by the caller until cleanup runs.
func OpenUploadSource(ctx context.Context, src string, maxBytes int64) (*os.File, string, func(), error) {
	noop := func() {}

	if !URLSource(src) {
		f, err := os.Open(src) //nolint:gosec // user-supplied path is intentional
		if err != nil {
			return nil, "", noop, fmt.Errorf("files: opening local source %q: %w", src, err)
		}
		filename := filepath.Base(src)
		cleanup := func() { _ = f.Close() }
		return f, filename, cleanup, nil
	}

	tempPath, err := SanitizedTempPath(src)
	if err != nil {
		return nil, "", noop, err
	}

	// Use os.CreateTemp for an atomic unique path — avoids the TOCTOU race
	// when multiple resources download the same URL in parallel during plan.
	dir := filepath.Dir(tempPath)
	base := filepath.Base(tempPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	tmp, err := os.CreateTemp(dir, stem+"-*"+ext)
	if err != nil {
		return nil, "", noop, fmt.Errorf("files: creating tempfile: %w", err)
	}

	filename, _, err := DownloadCapped(ctx, src, tmp, maxBytes)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, "", noop, err
	}

	// Rewind so the SDK reads from the start of the file.
	if _, seekErr := tmp.Seek(0, io.SeekStart); seekErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, "", noop, fmt.Errorf("files: rewinding tempfile: %w", seekErr)
	}

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	return tmp, filename, cleanup, nil
}

// ContentSHA256Prefix is the algorithm tag prepended to every value returned
// by ComputeContentSHA256. Resources that surface the hash in Terraform state
// use this prefix so callers can recognise the hash format across files.
const ContentSHA256Prefix = "sha256:"

// ComputeContentSHA256 returns the canonical hash string for the supplied
// bytes: the ContentSHA256Prefix followed by the lowercase hex SHA-256.
// Centralised so resources that store an uploaded file's hash in state (icon,
// enrollment customization image) compute the value identically.
func ComputeContentSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return ContentSHA256Prefix + hex.EncodeToString(sum[:])
}

// HashStreamSHA256 streams r through SHA-256 once and returns the canonical
// hash string: ContentSHA256Prefix followed by the lowercase hex digest. It
// buffers nothing, so a caller holding an io.Seeker can hash a multi-gigabyte
// source and rewind it for the upload rather than materialising it in the heap.
//
// The reader is consumed. A caller that also uploads the bytes must Seek back
// to the start before handing the same source to the SDK.
func HashStreamSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("files: hashing source: %w", err)
	}
	return ContentSHA256Prefix + hex.EncodeToString(h.Sum(nil)), nil
}

// HashLocalSource opens a local file and returns the canonical hash of its
// bytes, streaming rather than buffering. Only local paths belong here: a URL
// is read during apply, where the bytes that are hashed are the bytes uploaded.
//
// Resources that decide at plan time whether an upload source has changed call
// this, so the hash a plan compares and the hash an apply commits are computed
// the same way.
func HashLocalSource(ctx context.Context, src string) (string, error) {
	file, _, cleanup, err := OpenUploadSource(ctx, src, DefaultMaxBytes)
	if err != nil {
		return "", err
	}
	defer cleanup()

	return HashStreamSHA256(file)
}
