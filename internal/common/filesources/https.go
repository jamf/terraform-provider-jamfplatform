// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
)

// maxRedirects bounds the redirect chain an authenticated download follows,
// the same bound internal/common/files applies to an unauthenticated one.
const maxRedirects = 10

// ErrUnauthorized is returned when a server refuses an authenticated
// download with 401 or 403.
var ErrUnauthorized = errors.New("the server refused the credential")

// httpsOpener downloads from https servers that need a credential.
//
// A credential belongs to one host, matched on hostname and effective port,
// and is sent over https and nowhere else. Both halves are enforced on every
// request in a redirect chain, not only the first, because that is where they
// fail in practice: an artifact server answering with a redirect to a storage
// host, or to a plain-http mirror.
//
// Go's client already drops Authorization, Cookie and WWW-Authenticate when a
// redirect leaves the original domain, but it keeps them for a subdomain, and
// it forwards every other header, which is exactly where an API key in a
// custom header travels. So each redirected request is first stripped of
// everything this opener may have set, whatever the target, and then given the
// credential of the host it is going to, if that host has one of its own and
// the scheme is https. Nothing is carried from one host to the next.
//
// The Referer the client adds to a redirected request is removed as well. It
// holds the URL just left, query included, and a source that carries a token
// in its query would otherwise hand that token to the host it was redirected
// to.
type httpsOpener struct {
	credentials map[string]HTTPSCredential
	userAgent   string
	transport   http.RoundTripper
}

// newHTTPSOpener indexes the configured credentials by normalised host. An
// entry whose host does not normalise is left out: the provider schema has
// already refused it, so one can only arrive here from a caller that skipped
// validation, and guessing what it meant could send a credential somewhere it
// was not meant for.
func newHTTPSOpener(credentials []HTTPSCredential, userAgent string, transport http.RoundTripper) *httpsOpener {
	if transport == nil {
		transport = http.DefaultTransport
	}
	opener := &httpsOpener{
		credentials: map[string]HTTPSCredential{},
		userAgent:   userAgent,
		transport:   transport,
	}
	for _, credential := range credentials {
		key, err := NormalizeHost(credential.Host)
		if err != nil {
			continue
		}
		opener.credentials[key] = credential
	}
	return opener
}

// hostKey returns the key a request URL's credential is looked up under, the
// form NormalizeHost gives a configured host, or "" for a host that can have
// no credential.
//
// Only an ASCII hostname has a key. The transport does not dial the bytes it
// is given: it converts a non-ASCII name with IDNA first, and that conversion
// and plain lower-casing disagree. "f\u0130les.test" lower-cases to
// "files.test" and is dialled as a different registered domain, so matching
// on the lower-cased form would hand the credential of files.test to whoever
// holds the other name, and a redirect can name it (found in review by doing
// exactly that). A name that is not plain ASCII is therefore never matched,
// and the letters are folded by ASCII rule alone.
//
// A trailing dot is dropped from a hostname, which resolves the same with or
// without it, but not from an address: "127.0.0.1." is a name to look up, not
// the address it resembles.
func hostKey(u *url.URL) string {
	name := u.Hostname()
	for i := range len(name) {
		if name[i] >= 0x80 {
			return ""
		}
	}
	name = asciiLower(name)
	if address, err := netip.ParseAddr(name); err == nil {
		name = address.String()
	} else {
		name = strings.TrimSuffix(name, ".")
		if _, err := netip.ParseAddr(name); err == nil {
			return ""
		}
	}
	if name == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = defaultHTTPSPort
	}
	return net.JoinHostPort(name, port)
}

// asciiLower folds A to Z and leaves every other byte alone.
func asciiLower(s string) string {
	folded := []byte(s)
	for i, c := range folded {
		if c >= 'A' && c <= 'Z' {
			folded[i] = c + ('a' - 'A')
		}
	}
	return string(folded)
}

// credentialFor returns the credential to send with a request to u. There is
// none for any scheme but https, whatever the host.
func (o *httpsOpener) credentialFor(u *url.URL) (HTTPSCredential, bool) {
	if u.Scheme != SchemeHTTPS {
		return HTTPSCredential{}, false
	}
	credential, ok := o.credentials[hostKey(u)]
	return credential, ok
}

// authorize strips every header this opener can set from the request, then
// sets the credential of the host the request is going to, if it has one.
func (o *httpsOpener) authorize(req *http.Request) {
	req.Header.Del("Authorization")
	for _, credential := range o.credentials {
		if credential.HeaderName != "" {
			req.Header.Del(credential.HeaderName)
		}
	}
	credential, ok := o.credentialFor(req.URL)
	if !ok {
		return
	}
	switch {
	case credential.BearerToken != "":
		req.Header.Set("Authorization", "Bearer "+credential.BearerToken)
	case credential.HeaderName != "":
		req.Header.Set(credential.HeaderName, credential.HeaderValue)
	default:
		req.SetBasicAuth(credential.Username, credential.Password)
	}
}

// open issues the GET and returns the response body with the declared size,
// or -1 when the server declared none.
func (o *httpsOpener) open(ctx context.Context, src string) (Opened, error) {
	shown := files.RedactSource(src)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return Opened{}, fmt.Errorf("building the request for %s: %w", shown, files.RedactError(err))
	}
	if o.userAgent != "" {
		req.Header.Set("User-Agent", o.userAgent)
	}
	o.authorize(req)

	client := &http.Client{
		Transport: o.transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			next.Header.Del("Referer")
			o.authorize(next)
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return Opened{}, fmt.Errorf("fetching %s: %w", shown, files.RedactError(err))
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		_ = resp.Body.Close()
		return Opened{}, fmt.Errorf("%w: %s answered HTTP %d to the request for %s", ErrUnauthorized, resp.Request.URL.Host, resp.StatusCode, shown)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		_ = resp.Body.Close()
		return Opened{}, fmt.Errorf("fetching %s: the server answered HTTP %d", shown, resp.StatusCode)
	}

	size := resp.ContentLength
	if resp.Uncompressed {
		size = -1
	}
	filename := path.Base(resp.Request.URL.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}
	return Opened{Body: resp.Body, Filename: filename, Size: size}, nil
}

// Authenticated reports whether src is an https URL whose host has a
// credential in file_sources.https. Such a source is downloaded by OpenURL or
// StageURL; any other URL stays with internal/common/files, unchanged. An
// http URL is never authenticated, even to a host that has a credential: the
// credential is not sent in clear, and the download proceeds without it.
// Nil-receiver-safe.
func (r *Resolver) Authenticated(src string) bool {
	if r == nil || r.https == nil {
		return false
	}
	ref, _ := Parse(src)
	if ref.Kind != KindHTTPS || !ref.LowerCaseScheme() {
		return false
	}
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	_, ok := r.https.credentialFor(u)
	return ok
}

// OpenURL starts an authenticated download. The returned Size is -1 when the
// server declared none, which an upload that needs the size up front has to
// handle by staging; a declared size above maxBytes is refused before a byte
// is read, and the body is capped at maxBytes either way. There is no marker:
// a URL has no change marker this provider reads. The caller closes Body.
func (r *Resolver) OpenURL(ctx context.Context, src string, maxBytes int64) (Opened, error) {
	if !r.Authenticated(src) {
		return Opened{}, fmt.Errorf("%w: %s has no credential in file_sources.https", ErrNotConfigured, files.RedactSource(src))
	}
	if maxBytes <= 0 {
		maxBytes = files.DefaultMaxBytes
	}
	tflog.Debug(ctx, "downloading with a credential", map[string]any{"source": files.RedactSource(src)})
	opened, err := r.https.open(ctx, src)
	if err != nil {
		return Opened{}, err
	}
	if opened.Size > maxBytes {
		_ = opened.Body.Close()
		return Opened{}, fmt.Errorf("%w: %s declares %d bytes, over the %d byte limit", ErrTooLarge, files.RedactSource(src), opened.Size, maxBytes)
	}
	opened.Body = &cappedBody{Reader: io.LimitReader(opened.Body, maxBytes), body: opened.Body}
	return opened, nil
}

// cappedBody reads at most the cap from a body and closes the body itself.
type cappedBody struct {
	io.Reader
	body io.Closer
}

func (c *cappedBody) Close() error { return c.body.Close() }

// StageURL downloads an authenticated URL to a temporary file, as Stage does
// for a cloud storage object: the file is private to the user, named from a
// fixed pattern, rewound, and removed by the cleanup, which is never nil.
//
// When the server declared a size the byte count is held to it. When it did
// not, the cap is the only bound, and reaching it is an error: the download
// may have been cut off there, and a truncated file hashes consistently with
// itself.
func (r *Resolver) StageURL(ctx context.Context, src string, maxBytes int64) (Staged, func(), error) {
	noop := func() {}
	if maxBytes <= 0 {
		maxBytes = files.DefaultMaxBytes
	}
	opened, err := r.OpenURL(ctx, src, maxBytes+1)
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
	body := &readFailure{reader: opened.Body}
	written, err := io.Copy(tmp, body)
	switch {
	case err != nil && body.err != nil:
		cleanup()
		return Staged{}, noop, fmt.Errorf("%w: %w", ErrIncomplete, files.RedactError(err))
	case err != nil:
		cleanup()
		return Staged{}, noop, fmt.Errorf("writing the source to a temporary file: %w", err)
	case written > maxBytes:
		cleanup()
		return Staged{}, noop, fmt.Errorf("%w: %s is over the %d byte limit", ErrTooLarge, files.RedactSource(src), maxBytes)
	case opened.Size >= 0 && written != opened.Size:
		cleanup()
		return Staged{}, noop, fmt.Errorf("%w: the server declared %d bytes and sent %d", ErrIncomplete, opened.Size, written)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return Staged{}, noop, fmt.Errorf("rewinding the temporary file: %w", err)
	}
	opened.Size = written
	return Staged{File: tmp, Filename: opened.Filename, Attributes: opened.Attributes}, cleanup, nil
}
