// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package files

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"golang.org/x/sync/errgroup"
)

func TestURLSource(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://example.com/foo.pkg", true},
		{"http://example.com/foo.pkg", true},
		{"file:///tmp/foo.pkg", false},
		{"/tmp/foo.pkg", false},
		{"./relative.pkg", false},
		{"", false},
		{"HTTPS://example.com", false}, // case-sensitive on purpose, mirrors design doc
	}
	for _, c := range cases {
		if got := URLSource(c.in); got != c.want {
			t.Errorf("URLSource(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSanitizedTempPath(t *testing.T) {
	temp := os.TempDir()

	cases := []struct {
		name    string
		url     string
		wantErr bool
		wantSub string // expected basename substring on success
	}{
		{"plain pkg", "https://example.com/foo.pkg", false, "foo.pkg"},
		{"nested path", "https://example.com/dir/sub/foo.pkg", false, "foo.pkg"},
		{"missing basename", "https://example.com/", false, "download"},
		// path.Base strips the directory portion — "../../etc/passwd" reduces to
		// "passwd", which is a safe basename. The traversal defence guards
		// against pathological cases where the basename itself contains "..".
		{"basename collapses via path.Base", "https://example.com/../../etc/passwd", false, "passwd"},
		{"absolute basename ignored", "https://example.com/foo.pkg?x=y", false, "foo.pkg"},
		{"malformed url", "://not-a-url", true, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SanitizedTempPath(c.url)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got path %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasPrefix(got, temp) {
				t.Errorf("path %q is not under temp dir %q", got, temp)
			}
			if !strings.Contains(filepath.Base(got), c.wantSub) {
				t.Errorf("basename %q does not contain %q", filepath.Base(got), c.wantSub)
			}
		})
	}
}

func TestDownloadCapped_Success(t *testing.T) {
	body := []byte("hello-pkg-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	filename, n, err := DownloadCapped(context.Background(), srv.URL+"/something.pkg", &buf, 0)
	if err != nil {
		t.Fatalf("DownloadCapped: %v", err)
	}
	if n != int64(len(body)) {
		t.Errorf("wrote %d bytes, want %d", n, len(body))
	}
	if !bytes.Equal(buf.Bytes(), body) {
		t.Errorf("body mismatch: got %q want %q", buf.String(), string(body))
	}
	if filename != "something.pkg" {
		t.Errorf("filename %q, want %q", filename, "something.pkg")
	}
}

func TestDownloadCapped_RedirectsCapHonoured(t *testing.T) {
	// Build a chain of 12 redirects; client cap is 10 — expect failure.
	mux := http.NewServeMux()
	for i := range 12 {
		mux.HandleFunc("/hop"+itoa(i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/hop"+itoa(i+1), http.StatusFound)
		})
	}
	mux.HandleFunc("/hop12", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("end"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var buf bytes.Buffer
	_, _, err := DownloadCapped(context.Background(), srv.URL+"/hop0", &buf, 0)
	if err == nil {
		t.Fatalf("expected redirect-cap error, got nil")
	}
	if !strings.Contains(err.Error(), "redirects") {
		t.Errorf("error %v does not mention redirects", err)
	}
}

func TestDownloadCapped_CapExceeded(t *testing.T) {
	// Server writes 100 bytes, cap is 10.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), 100))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, _, err := DownloadCapped(context.Background(), srv.URL+"/big.pkg", &buf, 10)
	if !errors.Is(err, ErrDownloadCapExceeded) {
		t.Fatalf("expected ErrDownloadCapExceeded, got %v", err)
	}
}

func TestDownloadCapped_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, _, err := DownloadCapped(context.Background(), srv.URL+"/missing.pkg", &buf, 0)
	if err == nil {
		t.Fatalf("expected error on 404")
	}
}

func TestOpenUploadSource_LocalPath(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "src*.pkg")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	want := []byte("local-pkg-bytes")
	if _, err := tmp.Write(want); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	file, filename, cleanup, err := OpenUploadSource(context.Background(), tmp.Name(), DefaultMaxBytes)
	if err != nil {
		t.Fatalf("OpenUploadSource: %v", err)
	}
	defer cleanup()

	if filename != filepath.Base(tmp.Name()) {
		t.Errorf("filename %q, want %q", filename, filepath.Base(tmp.Name()))
	}
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes mismatch: got %q want %q", got, want)
	}
}

func TestOpenUploadSource_URL(t *testing.T) {
	want := []byte("url-pkg-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	file, filename, cleanup, err := OpenUploadSource(context.Background(), srv.URL+"/remote.pkg", DefaultMaxBytes)
	if err != nil {
		t.Fatalf("OpenUploadSource: %v", err)
	}

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("bytes mismatch: got %q want %q", got, want)
	}
	if filename != "remote.pkg" {
		t.Errorf("filename %q, want %q", filename, "remote.pkg")
	}

	tempPath := file.Name()
	cleanup()
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected temp file %q to be removed after cleanup, stat err=%v", tempPath, err)
	}
}

func TestOpenUploadSource_URLCapExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), 100))
	}))
	defer srv.Close()

	_, _, _, err := OpenUploadSource(context.Background(), srv.URL+"/big.pkg", 10)
	if !errors.Is(err, ErrDownloadCapExceeded) {
		t.Fatalf("expected ErrDownloadCapExceeded, got %v", err)
	}
}

func TestOpenUploadSource_URLConcurrentSameName(t *testing.T) {
	// Reproduces the parallel-plan race: N icon resources all download the same
	// URL (e.g. Apple's "512x512bb.png") concurrently; each must get a unique
	// tempfile. The start-gate releases all goroutines simultaneously to maximise
	// overlap, mirroring Terraform's default -parallelism=10 walk.
	const n = 20
	body := []byte("icon-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	url := srv.URL + "/512x512bb.png"

	var (
		mu    sync.Mutex
		paths []string
	)
	gate := make(chan struct{})

	var eg errgroup.Group
	for range n {
		eg.Go(func() error {
			<-gate
			file, _, cleanup, err := OpenUploadSource(context.Background(), url, DefaultMaxBytes)
			if err != nil {
				return err
			}
			defer cleanup()
			mu.Lock()
			paths = append(paths, file.Name())
			mu.Unlock()
			return nil
		})
	}

	close(gate)
	if err := eg.Wait(); err != nil {
		t.Fatalf("concurrent OpenUploadSource: %v", err)
	}

	if len(paths) != n {
		t.Fatalf("got %d paths, want %d", len(paths), n)
	}
	seen := make(map[string]bool, n)
	for _, p := range paths {
		if seen[p] {
			t.Errorf("duplicate temp path: %q", p)
		}
		seen[p] = true
	}
}

// itoa is a tiny local helper avoiding strconv import noise in the redirect
// test fixture.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func TestHashStreamSHA256MatchesComputeContentSHA256(t *testing.T) {
	content := []byte("icon bytes that also fit in memory")

	got, err := HashStreamSHA256(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("HashStreamSHA256: %v", err)
	}
	if want := ComputeContentSHA256(content); got != want {
		t.Fatalf("HashStreamSHA256 = %q, want %q; the streaming and buffering hashes must agree", got, want)
	}
	if !strings.HasPrefix(got, ContentSHA256Prefix) {
		t.Fatalf("HashStreamSHA256 = %q, want the %q prefix", got, ContentSHA256Prefix)
	}
}

func TestHashStreamSHA256ReadError(t *testing.T) {
	if _, err := HashStreamSHA256(iotest.ErrReader(errors.New("boom"))); err == nil {
		t.Fatal("HashStreamSHA256 returned no error for a reader that fails")
	}
}

func TestHashLocalSource(t *testing.T) {
	content := []byte("local icon bytes")
	p := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}

	got, err := HashLocalSource(context.Background(), p)
	if err != nil {
		t.Fatalf("HashLocalSource: %v", err)
	}
	if want := ComputeContentSHA256(content); got != want {
		t.Fatalf("HashLocalSource = %q, want %q", got, want)
	}
}

func TestHashLocalSourceMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.png")

	if _, err := HashLocalSource(context.Background(), missing); err == nil {
		t.Fatal("HashLocalSource returned no error for a path that does not exist")
	}
}

func TestOpenURLStream_DeclaredSize(t *testing.T) {
	body := []byte("hello-pkg-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	rc, name, size, err := OpenURLStream(context.Background(), srv.URL+"/a.pkg", 0)
	if err != nil {
		t.Fatalf("OpenURLStream: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if size != int64(len(body)) {
		t.Errorf("size %d, want %d", size, len(body))
	}
	if name != "a.pkg" {
		t.Errorf("filename %q, want a.pkg", name)
	}
}

func TestOpenURLStream_UnknownSizeIsMinusOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		_, _ = w.Write([]byte("part-one"))
		f.Flush()
		_, _ = w.Write([]byte("part-two"))
	}))
	defer srv.Close()

	rc, _, size, err := OpenURLStream(context.Background(), srv.URL+"/a.pkg", 0)
	if err != nil {
		t.Fatalf("OpenURLStream: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if size != -1 {
		t.Errorf("size %d, want -1 for a chunked response", size)
	}
}

func TestOpenURLStream_DeclaredSizeOverCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 64))
	}))
	defer srv.Close()

	if _, _, _, err := OpenURLStream(context.Background(), srv.URL+"/a.pkg", 16); err == nil {
		t.Fatal("expected a cap error for a declared length over maxBytes")
	}
}

func TestRedactSource(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/tmp/app.pkg", "/tmp/app.pkg"},
		{"./build/what?.pkg", "./build/what?.pkg"},
		{"", ""},
		{"https://example.com/app.pkg", "https://example.com/app.pkg"},
		{"https://example.com/app.pkg?X-Amz-Signature=secret&X-Amz-Expires=60", "https://example.com/app.pkg?<redacted>"},
		{"https://example.com/app.pkg#secret", "https://example.com/app.pkg?<redacted>"},
		{"https://example.com?sig=secret", "https://example.com?<redacted>"},
		{"https://user:secret@example.com/app.pkg", "https://example.com/app.pkg"},
		{"https://user:secret@example.com/a@b.pkg?sig=secret", "https://example.com/a@b.pkg?<redacted>"},
		{"HTTPS://example.com/app.pkg?sig=secret", "HTTPS://example.com/app.pkg?<redacted>"},
		{"https://example.com/%zz?sig=secret", "https://example.com/%zz?<redacted>"},
		{"s3://bucket/app.pkg?versionId=abc", "s3://bucket/app.pkg?<redacted>"},
		{" https://example.com/app.pkg?sig=secret", " https://example.com/app.pkg?<redacted>"},
		{"https://user:sec/ret@example.com/app.pkg", "https://<redacted>"},
		{"https://user:se?cret@example.com/app.pkg", "https://<redacted>"},
		{"https://user:se#cret@example.com/app.pkg", "https://<redacted>"},
		{"https://example.com:8443/team@example/app.pkg", "https://example.com:8443/team@example/app.pkg"},
		{"https://[2001:db8::1]:8443/a@b.pkg?sig=secret", "https://[2001:db8::1]:8443/a@b.pkg?<redacted>"},
		{"C://packages/app#1.pkg", "C://packages/app#1.pkg"},
	}
	for _, c := range cases {
		if got := RedactSource(c.in); got != c.want {
			t.Errorf("RedactSource(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestErrorsNeverCarryTheQueryString(t *testing.T) {
	const secret = "s3cr3t-signature"

	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer redirectTarget.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/big":
			w.Header().Set("Content-Length", "10")
			_, _ = w.Write([]byte("0123456789"))
		case "/loop":
			http.Redirect(w, r, "/loop?sig="+secret, http.StatusFound)
		case "/badlocation":
			w.Header().Set("Location", "https://user:"+secret+"@cdn.example.com/%zz/app.pkg?sig="+secret)
			w.WriteHeader(http.StatusFound)
		case "/badheader":
			hijacked, _, _ := w.(http.Hijacker).Hijack()
			_, _ = io.WriteString(hijacked, "HTTP/1.1 302 Found\r\nLocation: /x?sig="+secret+"\x7f\r\nContent-Length: 0\r\n\r\n")
			_ = hijacked.Close()
		case "/away":
			http.Redirect(w, r, redirectTarget.URL+"/signed?sig="+secret, http.StatusFound)
		}
	}))
	defer srv.Close()

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	sources := map[string]string{
		"non-2xx status":                     srv.URL + "/forbidden?sig=" + secret,
		"redirect loop":                      srv.URL + "/loop?sig=" + secret,
		"non-2xx after a redirect":           srv.URL + "/away?sig=" + secret,
		"connection refused":                 closedURL + "/app.pkg?sig=" + secret,
		"unparseable URL":                    "https://example.com/%zz?sig=" + secret,
		"password in the URL":                "https://user:" + secret + "@127.0.0.1:1/app.pkg",
		"redirect to an unparseable URL":     srv.URL + "/badlocation?sig=" + secret,
		"malformed response header":          srv.URL + "/badheader?sig=" + secret,
		"upper-case scheme read as a path":   "HTTPS://host.invalid/app.pkg?sig=" + secret,
		"leading space read as a path":       " https://host.invalid/app.pkg?sig=" + secret,
		"password holding a slash":           "https://user:" + secret + "/x@host.invalid/app.pkg",
		"password holding a question mark":   "https://user:pre?" + secret + "@host.invalid/app.pkg",
		"password in an unparseable address": "https://user:" + secret + "@host.invalid:port/app.pkg",
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			var errs []error
			_, _, err := DownloadCapped(context.Background(), src, io.Discard, 0)
			errs = append(errs, err)
			body, _, _, err := OpenURLStream(context.Background(), src, 0)
			if body != nil {
				_ = body.Close()
			}
			errs = append(errs, err)
			_, _, cleanup, err := OpenUploadSource(context.Background(), src, 0)
			cleanup()
			errs = append(errs, err)
			_, err = SanitizedTempPath(src)
			errs = append(errs, err)

			failed := 0
			for _, err := range errs {
				if err == nil {
					continue
				}
				failed++
				if strings.Contains(err.Error(), secret) {
					t.Errorf("error carries the secret: %v", err)
				}
			}
			if failed == 0 {
				t.Fatal("the source was expected to fail at least one call")
			}
		})
	}

	t.Run("over the cap", func(t *testing.T) {
		_, _, _, err := OpenURLStream(context.Background(), srv.URL+"/big?sig="+secret, 5)
		if err == nil {
			t.Fatal("want an over-cap error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error carries the secret: %v", err)
		}
	})
}

func TestRedactedErrorKeepsItsCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := DownloadCapped(ctx, "https://example.com/app.pkg?sig=secret", io.Discard, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the cause must survive redaction, got %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error carries the secret: %v", err)
	}
}
