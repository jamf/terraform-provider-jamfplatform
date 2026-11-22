// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filesources"
)

// objectStore is a local stand-in for S3 holding one object,
// s3://bucket/releases/app.pkg, whose body and ETag a test can change, and
// which counts the requests it receives by method.
type objectStore struct {
	mu      sync.Mutex
	body    string
	etag    string
	missing bool
	refuse  bool

	acceptAnyPin bool
	omitETag     bool
	heads        int
	gets         int
}

func (o *objectStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if r.Method == http.MethodHead {
		o.heads++
	} else {
		o.gets++
	}
	switch {
	case o.refuse:
		w.WriteHeader(http.StatusForbidden)
		return
	case o.missing || r.URL.Path != "/bucket/releases/app.pkg":
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if want := r.Header.Get("If-Match"); want != "" && want != o.etag && !o.acceptAnyPin {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if !o.omitETag {
		w.Header().Set("ETag", o.etag)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(o.body)))
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, o.body)
	}
}

func (o *objectStore) replace(body, etag string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.body, o.etag = body, etag
}

func (o *objectStore) requests() (heads, gets int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.heads, o.gets
}

// objectStoreResolver returns a cloud storage resolver whose S3 requests go to
// a local objectStore. The AWS environment is pointed at nothing so the
// developer's own credentials and profile play no part.
func objectStoreResolver(t *testing.T, body string) (*filesources.Resolver, *objectStore) {
	t.Helper()
	store := &objectStore{body: body, etag: `"abc"`}
	server := httptest.NewServer(store)
	t.Cleanup(server.Close)

	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, name := range []string{"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3"} {
		t.Setenv(name, "")
	}
	return filesources.New(filesources.Config{S3: filesources.S3Config{
		Region: "eu-west-1", AccessKey: "AKIAEXAMPLE", SecretKey: "secret", EndpointS3: server.URL, UsePathStyle: true,
	}}, nil), store
}

const objectSource = "s3://bucket/releases/app.pkg"

func TestOpenPackageSourceStagesACloudStorageObject(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")

	file, filename, marker, cleanup, err := openPackageSource(context.Background(), sources, objectSource, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(file)
	if string(body) != "package bytes" || filename != "app.pkg" || marker != `etag:"abc"` {
		t.Errorf("body=%q filename=%q marker=%q", body, filename, marker)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Errorf("the staged file must be seekable for a retried upload: %v", err)
	}
	cleanup()
	if _, err := os.Stat(file.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup must remove the staged file, stat gave %v", err)
	}
}

func TestOpenPackageSourceNamesAMissingObject(t *testing.T) {
	sources, _ := objectStoreResolver(t, "package bytes")

	_, _, _, cleanup, err := openPackageSource(context.Background(), sources, "s3://bucket/releases/other.pkg", "")
	cleanup()
	if !errors.Is(err, filesources.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if summary, _ := sourceProblem(err); summary != "Package file not found in cloud storage" {
		t.Errorf("summary = %q", summary)
	}
}

func TestOpenPackageSourcePinnedToAReplacedObjectFailsBeforeAnyByteIsRead(t *testing.T) {
	sources, store := objectStoreResolver(t, "planned bytes")
	store.replace("other bytes", `"def"`)

	_, _, _, cleanup, err := openPackageSource(context.Background(), sources, objectSource, `etag:"abc"`)
	cleanup()
	if !errors.Is(err, filesources.ErrChanged) {
		t.Fatalf("want ErrChanged, got %v", err)
	}
	summary, detail := sourceProblem(err)
	if summary != "Package file changed since the plan" || !strings.Contains(detail, "without sending the replacement") {
		t.Errorf("diagnostic = %q: %q", summary, detail)
	}

	file, _, marker, cleanup, err := openPackageSource(context.Background(), sources, objectSource, `etag:"def"`)
	defer cleanup()
	if err != nil || marker != `etag:"def"` {
		t.Fatalf("the current marker must open: marker=%q err=%v", marker, err)
	}
	body, _ := io.ReadAll(file)
	if string(body) != "other bytes" {
		t.Errorf("body = %q", body)
	}
}

func TestOpenPackageSourceWithoutAResolverDoesNotReadALocalPath(t *testing.T) {
	_, _, _, cleanup, err := openPackageSource(context.Background(), nil, objectSource, "")
	cleanup()
	if !errors.Is(err, filesources.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
	if summary, _ := sourceProblem(err); summary != "Cloud storage source cannot be read" {
		t.Errorf("summary = %q", summary)
	}
}

func TestOpenPackageSourceStillReadsALocalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.pkg")
	if err := os.WriteFile(path, []byte("local bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, filename, marker, cleanup, err := openPackageSource(context.Background(), nil, path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	body, _ := io.ReadAll(file)
	if string(body) != "local bytes" || filename != "local.pkg" || marker != "" {
		t.Errorf("body=%q filename=%q marker=%q", body, filename, marker)
	}
}

func TestSameAsRecorded(t *testing.T) {
	local := fileDigests{sha3: "aa", sha256: "bb", md5: "cc"}
	cases := []struct {
		name         string
		native       bool
		recorded     fileDigests
		recordedSize string
		want         bool
	}{
		{"cloud source, digest matches", true, fileDigests{sha3: "AA"}, "10", true},
		{"cloud source, digest differs", true, fileDigests{sha3: "ff"}, "10", false},
		{"cloud source, md5 only matches", true, fileDigests{md5: "cc"}, "10", true},
		{"cloud source, no digest and the same size", true, fileDigests{}, "10", false},
		{"cloud source, no digest and no size", true, fileDigests{}, "", false},
		{"local source, no digest and the same size", false, fileDigests{}, "10", true},
		{"local source, no digest and another size", false, fileDigests{}, "11", false},
		{"local source, digest differs", false, fileDigests{sha3: "ff"}, "10", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameAsRecorded(tc.native, tc.recorded, local, tc.recordedSize, 10); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDriftedOutsideTerraform(t *testing.T) {
	cases := []struct {
		name                      string
		recorded, current         fileDigests
		recordedSize, currentSize string
		want                      bool
	}{
		{"nothing changed", fileDigests{sha3: "aa", md5: "cc"}, fileDigests{sha3: "AA", md5: "cc"}, "10", "10", false},
		{"digest changed", fileDigests{sha3: "aa"}, fileDigests{sha3: "ff"}, "10", "10", true},
		{"digest changed, size blanked by a metadata update", fileDigests{sha3: "aa"}, fileDigests{sha3: "ff"}, "10", "", true},
		{"size blanked by a metadata update", fileDigests{sha3: "aa"}, fileDigests{sha3: "aa"}, "10", "", false},
		{"digest filled in after an upload", fileDigests{}, fileDigests{sha3: "aa"}, "10", "10", false},
		{"digest not yet recomputed", fileDigests{sha3: "aa"}, fileDigests{}, "10", "10", false},
		{"replaced by a file Jamf Pro does not hash", fileDigests{sha3: "aa"}, fileDigests{}, "10", "99", true},
		{"unhashed record, another size", fileDigests{}, fileDigests{}, "10", "99", true},
		{"unhashed record, same size", fileDigests{}, fileDigests{}, "10", "10", false},
		{"unhashed record, size blanked", fileDigests{}, fileDigests{}, "10", "", false},
		{"new record with nothing recorded", fileDigests{}, fileDigests{}, "", "10", false},
		{"weaker digest only, unchanged", fileDigests{md5: "cc"}, fileDigests{md5: "cc"}, "10", "10", false},
		{"weaker digest only, changed", fileDigests{md5: "cc"}, fileDigests{md5: "dd"}, "10", "10", true},
		{"strongest digest gone, weaker one changed, same size", fileDigests{sha3: "aa", md5: "cc"}, fileDigests{md5: "dd"}, "10", "10", true},
		{"strongest digest gone, weaker one unchanged, size blanked", fileDigests{sha3: "aa", md5: "cc"}, fileDigests{md5: "cc"}, "10", "", false},
		{"strongest unchanged, weaker one changed", fileDigests{sha3: "aa", md5: "cc"}, fileDigests{sha3: "aa", md5: "dd"}, "10", "10", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := driftedOutsideTerraform(tc.recorded, tc.current, tc.recordedSize, tc.currentSize); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSourceUnchanged(t *testing.T) {
	model := func(source, marker types.String) *PackageResourceModel {
		return &PackageResourceModel{PackageFileSource: source, PackageFileSourceVersion: marker}
	}
	s3 := types.StringValue(objectSource)
	kept := types.StringValue(`etag:"abc"`)
	cases := []struct {
		name        string
		plan, state *PackageResourceModel
		want        bool
	}{
		{"marker kept", model(s3, kept), model(s3, kept), true},
		{"marker replaced", model(s3, types.StringValue(`etag:"def"`)), model(s3, kept), false},
		{"marker deferred to apply", model(s3, types.StringUnknown()), model(s3, kept), false},
		{"no marker in state", model(s3, kept), model(s3, types.StringNull()), false},
		{"another object with the same etag", model(types.StringValue("s3://bucket/copy.pkg"), kept), model(s3, kept), false},
		{"local path", model(types.StringValue("/tmp/app.pkg"), types.StringNull()), model(types.StringValue("/tmp/app.pkg"), types.StringNull()), false},
		{"checksum that is not the recorded digest", withChecksum(model(s3, kept), "ff", ""), withChecksum(model(s3, kept), "", "aa"), false},
		{"checksum equal to the recorded digest", withChecksum(model(s3, kept), "AA", ""), withChecksum(model(s3, kept), "", "aa"), true},
		{"checksum on a record with no digest", withChecksum(model(s3, kept), "aa", ""), model(s3, kept), false},
		{"no source", model(types.StringNull(), types.StringNull()), model(types.StringNull(), types.StringNull()), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sourceUnchanged(tc.plan, tc.state); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSettleSourceVersion(t *testing.T) {
	cases := []struct {
		name     string
		planned  types.String
		recorded string
		want     types.String
	}{
		{"read replaces a planned marker", types.StringValue(`etag:"abc"`), `etag:"abc"`, types.StringValue(`etag:"abc"`)},
		{"read settles a deferred marker", types.StringUnknown(), `etag:"def"`, types.StringValue(`etag:"def"`)},
		{"nothing read keeps a kept marker", types.StringValue(`etag:"abc"`), "", types.StringValue(`etag:"abc"`)},
		{"nothing read and nothing planned", types.StringNull(), "", types.StringNull()},
		{"nothing read settles unknown to null", types.StringUnknown(), "", types.StringNull()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &PackageResourceModel{PackageFileSourceVersion: tc.planned}
			settleSourceVersion(m, tc.recorded)
			if !m.PackageFileSourceVersion.Equal(tc.want) {
				t.Errorf("got %s, want %s", m.PackageFileSourceVersion, tc.want)
			}
		})
	}
}

func withChecksum(m *PackageResourceModel, checksum, recordedSHA3 string) *PackageResourceModel {
	if checksum != "" {
		m.PackageFileSourceChecksum = types.StringValue(checksum)
	}
	if recordedSHA3 != "" {
		m.Sha3512 = types.StringValue(recordedSHA3)
	}
	return m
}

func TestOpenPackageSourcePinnedReturnsThePinNotTheStoresEcho(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")
	store.acceptAnyPin = true

	_, _, marker, cleanup, err := openPackageSource(context.Background(), sources, objectSource, `etag:W/"abc"`)
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if marker != `etag:W/"abc"` {
		t.Errorf("marker = %q, want the planned pin so state cannot disagree with the plan", marker)
	}
}

// credentialedServer is an https server that serves app.pkg only to a request
// carrying the bearer token, with a resolver that holds that token for it.
func credentialedServer(t *testing.T, body string) (*filesources.Resolver, string) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer download-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "https://")
	return filesources.New(filesources.Config{
		HTTPS:         []filesources.HTTPSCredential{{Host: host, BearerToken: "download-token"}},
		HTTPTransport: server.Client().Transport,
	}, nil), server.URL + "/releases/app.pkg"
}

func TestOpenPackageSourceSendsTheCredentialOfAnHTTPSHost(t *testing.T) {
	sources, src := credentialedServer(t, "package bytes")

	file, filename, marker, cleanup, err := openPackageSource(context.Background(), sources, src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	body, _ := io.ReadAll(file)
	if string(body) != "package bytes" || filename != "app.pkg" || marker != "" {
		t.Errorf("body=%q filename=%q marker=%q", body, filename, marker)
	}
}

func TestOpenPackageURLStreamSendsTheCredentialOfAnHTTPSHost(t *testing.T) {
	sources, src := credentialedServer(t, "package bytes")

	body, filename, size, marker, err := openPackageURLStream(context.Background(), sources, src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = body.Close() }()
	got, _ := io.ReadAll(body)
	if string(got) != "package bytes" || filename != "app.pkg" || size != 13 || marker != "" {
		t.Errorf("body=%q filename=%q size=%d marker=%q", got, filename, size, marker)
	}
}

func TestOpenPackageSourceNamesARefusedDownload(t *testing.T) {
	_, src := credentialedServer(t, "package bytes")
	host := strings.TrimPrefix(strings.TrimSuffix(src, "/releases/app.pkg"), "https://")
	wrong := filesources.New(filesources.Config{
		HTTPS:         []filesources.HTTPSCredential{{Host: host, BearerToken: "wrong-token"}},
		HTTPTransport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // local test server
	}, nil)

	_, _, _, cleanup, err := openPackageSource(context.Background(), wrong, src, "")
	cleanup()
	if !errors.Is(err, filesources.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
	summary, detail := sourceProblem(err)
	if summary != "Server refused the package file download" || strings.Contains(detail, "wrong-token") {
		t.Errorf("diagnostic = %q: %q", summary, detail)
	}
}

func TestOpenPackageSourceLeavesAURLWithNoCredentialToTheOldPath(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, "plain bytes")
	}))
	t.Cleanup(plain.Close)
	sources, _ := credentialedServer(t, "package bytes")

	file, _, _, cleanup, err := openPackageSource(context.Background(), sources, plain.URL+"/app.pkg", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	body, _ := io.ReadAll(file)
	if string(body) != "plain bytes" {
		t.Errorf("body = %q", body)
	}
}

func TestOpenPackageURLStreamReadsACloudStorageObjectPinned(t *testing.T) {
	sources, store := objectStoreResolver(t, "package bytes")

	body, filename, size, marker, err := openPackageURLStream(context.Background(), sources, objectSource, `etag:"abc"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if string(got) != "package bytes" || filename != "app.pkg" || size != 13 || marker != `etag:"abc"` {
		t.Errorf("body=%q filename=%q size=%d marker=%q", got, filename, size, marker)
	}

	store.replace("other bytes", `"def"`)
	if _, _, _, _, err := openPackageURLStream(context.Background(), sources, objectSource, `etag:"abc"`); !errors.Is(err, filesources.ErrChanged) {
		t.Errorf("a stream pinned to a replaced object must fail, got %v", err)
	}
}
