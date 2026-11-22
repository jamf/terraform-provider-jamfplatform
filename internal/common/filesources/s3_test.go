// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

// fakeObject is one object in the fake store.
type fakeObject struct {
	body   string
	etag   string
	region string
}

// fakeS3 is a path-style S3 endpoint holding objects in memory. It answers
// the requests the driver makes the way S3 was probed to on 2026-11-22: a
// request signed for the wrong Region draws a 301 naming the right one, a
// stale If-Match draws a 412, and a Range draws a 206 with a Content-Range.
type fakeS3 struct {
	objects     map[string]fakeObject
	omitRegion  bool
	requests    atomic.Int64
	mu          sync.Mutex
	userAgents  []string
	ifMatches   []string
	ranges      []string
	versionIDs  []string
	signedFor   []string
	credentials []string
}

var credentialScope = regexp.MustCompile(`Credential=([^/]+)/[0-9]+/([^/]+)/s3/aws4_request`)

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests.Add(1)
	scope := credentialScope.FindStringSubmatch(r.Header.Get("Authorization"))
	accessKey, region := "", ""
	if scope != nil {
		accessKey, region = scope[1], scope[2]
	}
	f.mu.Lock()
	f.userAgents = append(f.userAgents, r.Header.Get("User-Agent"))
	f.ifMatches = append(f.ifMatches, r.Header.Get("If-Match"))
	f.ranges = append(f.ranges, r.Header.Get("Range"))
	f.versionIDs = append(f.versionIDs, r.URL.Query().Get("versionId"))
	f.signedFor = append(f.signedFor, region)
	f.credentials = append(f.credentials, accessKey)
	f.mu.Unlock()

	name := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasSuffix(name, "/forbidden.pkg") {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	object, ok := f.objects[name]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if object.region != region {
		if !f.omitRegion {
			w.Header().Set(bucketRegionHeader, object.region)
		}
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}
	if want := r.Header.Get("If-Match"); want != "" && want != object.etag {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	body, status := object.body, http.StatusOK
	if spec := r.Header.Get("Range"); spec != "" {
		start, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(spec, "bytes="), "-"))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
		body, status = body[start:], http.StatusPartialContent
	}
	w.Header().Set("ETag", object.etag)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, body)
	}
}

// isolateAWSEnvironment points every ambient AWS setting at nothing, so a
// test reads the configuration it was given and not the developer's.
func isolateAWSEnvironment(t *testing.T) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	for _, name := range []string{
		"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
	} {
		t.Setenv(name, "")
	}
}

func startFakeS3(t *testing.T, store *fakeS3) string {
	t.Helper()
	server := httptest.NewServer(store)
	t.Cleanup(server.Close)
	return server.URL
}

// compatibleStoreResolver builds a Resolver the way a MinIO user configures
// one: a custom endpoint, path-style addressing and static keys.
func compatibleStoreResolver(t *testing.T, endpoint string) *Resolver {
	t.Helper()
	isolateAWSEnvironment(t)
	return New(Config{
		UserAgent: "terraform-provider-jamfplatform/test",
		S3: S3Config{
			Region:       "eu-west-1",
			AccessKey:    "AKIAEXAMPLE",
			SecretKey:    "secret",
			EndpointS3:   endpoint,
			UsePathStyle: true,
		},
	}, nil)
}

// awsDriver builds a driver that behaves as it does against AWS itself, with
// no custom endpoint in its configuration, while its requests go to the fake.
// Region discovery is switched off under a custom endpoint, so this is the
// only way to exercise it without AWS.
func awsDriver(endpoint, region string) *s3Driver {
	driver := newS3Driver(S3Config{UsePathStyle: true}, "")
	driver.base = &aws.Config{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider("AKIAEXAMPLE", "secret", ""),
		BaseEndpoint: aws.String(endpoint),
	}
	driver.lookupRegion = func(context.Context, string) (string, error) {
		return "", errors.New("the region lookup must not be needed")
	}
	return driver
}

func TestS3StatReportsSizeAndETagMarker(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/releases/app.pkg": {body: "0123456789", etag: `"abc-3"`, region: "eu-west-1"},
	}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	got, err := r.Stat(context.Background(), "s3://bucket/releases/app.pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Size != 10 || got.Marker != `etag:"abc-3"` {
		t.Errorf("attributes = %+v", got)
	}
	if !strings.Contains(store.userAgents[0], "terraform-provider-jamfplatform/test") {
		t.Errorf("the request must carry the provider's user agent, got %q", store.userAgents[0])
	}
	if store.credentials[0] != "AKIAEXAMPLE" {
		t.Errorf("the request was signed with %q", store.credentials[0])
	}
}

func TestS3OpenStreamsTheObject(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "0123456789", etag: `"abc"`, region: "eu-west-1"},
	}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	opened, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = opened.Body.Close() }()
	body, _ := io.ReadAll(opened.Body)
	if string(body) != "0123456789" || opened.Size != 10 || opened.Marker != `etag:"abc"` || opened.Filename != "app.pkg" {
		t.Errorf("body=%q size=%d marker=%q filename=%q", body, opened.Size, opened.Marker, opened.Filename)
	}
}

func TestS3OpenPinsWithIfMatch(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "new bytes", etag: `"new"`, region: "eu-west-1"},
	}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	_, err := r.Open(context.Background(), "s3://bucket/app.pkg", `etag:"old"`, 0)
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("a replaced object must fail with ErrChanged, got %v", err)
	}
	if got := store.ifMatches[len(store.ifMatches)-1]; got != `"old"` {
		t.Errorf("If-Match = %q, want the ETag exactly as recorded", got)
	}

	opened, err := r.Open(context.Background(), "s3://bucket/app.pkg", `etag:"new"`, 0)
	if err != nil {
		t.Fatalf("the current marker must open: %v", err)
	}
	_ = opened.Body.Close()
}

func TestS3OpenRefusesAMarkerOfAnotherKind(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	if _, err := r.Open(context.Background(), "s3://bucket/app.pkg", "version:abc", 0); err == nil {
		t.Fatal("a marker that is not an ETag must be refused")
	}
	if store.requests.Load() != 0 {
		t.Error("nothing may be requested with a marker the driver cannot send")
	}
}

func TestS3OpenAtAnOffsetReportsTheWholeSize(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "0123456789", etag: `"abc"`, region: "eu-west-1"},
	}}
	driver := newS3Driver(S3Config{Region: "eu-west-1", AccessKey: "AKIAEXAMPLE", SecretKey: "secret", EndpointS3: startFakeS3(t, store), UsePathStyle: true}, "")
	isolateAWSEnvironment(t)

	ref, _ := Parse("s3://bucket/app.pkg")
	opened, err := driver.Open(context.Background(), ref, `etag:"abc"`, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = opened.Body.Close() }()
	body, _ := io.ReadAll(opened.Body)
	if string(body) != "6789" {
		t.Errorf("body = %q, want the bytes from the offset", body)
	}
	if opened.Size != 10 {
		t.Errorf("Size = %d, want the size of the whole object", opened.Size)
	}
	if got := store.ranges[len(store.ranges)-1]; got != "bytes=6-" {
		t.Errorf("Range = %q", got)
	}
	if got := store.ifMatches[len(store.ifMatches)-1]; got != `"abc"` {
		t.Errorf("a ranged read must stay pinned, If-Match = %q", got)
	}
}

func TestS3ReadsTheNamedVersion(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg?versionId=v1.abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.versionIDs[0]; got != "v1.abc" {
		t.Errorf("versionId = %q", got)
	}
	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.versionIDs[1]; got != "" {
		t.Errorf("a source with no version must send none, got %q", got)
	}
}

func TestS3NamesAMissingAndARefusedObject(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	_, err := r.Stat(context.Background(), "s3://bucket/missing.pkg")
	if !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "s3://bucket/missing.pkg") {
		t.Errorf("want ErrNotFound naming the source, got %v", err)
	}
	_, err = r.Open(context.Background(), "s3://bucket/forbidden.pkg", "", 0)
	if !errors.Is(err, ErrAccessDenied) || !strings.Contains(err.Error(), "s3:ListBucket") {
		t.Errorf("want ErrAccessDenied explaining the permissions, got %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("the error must not carry the secret key: %v", err)
	}
}

func TestS3FollowsABucketToItsRegion(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "0123456789", etag: `"abc"`, region: "eu-west-1"},
	}}
	driver := awsDriver(startFakeS3(t, store), "eu-north-1")
	ref, _ := Parse("s3://bucket/app.pkg")

	got, err := driver.Stat(context.Background(), ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Size != 10 {
		t.Errorf("Size = %d", got.Size)
	}
	if want := []string{"eu-north-1", "eu-west-1"}; fmt.Sprint(store.signedFor) != fmt.Sprint(want) {
		t.Errorf("requests were signed for %v, want %v", store.signedFor, want)
	}

	opened, err := driver.Open(context.Background(), ref, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = opened.Body.Close()
	if store.requests.Load() != 3 || store.signedFor[2] != "eu-west-1" {
		t.Errorf("the bucket's Region must be remembered: %d requests signed for %v", store.requests.Load(), store.signedFor)
	}
}

func TestS3LooksTheRegionUpWhenTheRedirectDoesNotNameIt(t *testing.T) {
	store := &fakeS3{omitRegion: true, objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	driver := awsDriver(startFakeS3(t, store), "eu-north-1")
	lookups := 0
	driver.lookupRegion = func(_ context.Context, bucket string) (string, error) {
		lookups++
		if bucket != "bucket" {
			t.Errorf("looked up %q", bucket)
		}
		return "eu-west-1", nil
	}
	ref, _ := Parse("s3://bucket/app.pkg")

	for range 2 {
		if _, err := driver.Stat(context.Background(), ref); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if lookups != 1 {
		t.Errorf("the Region was looked up %d times, want once", lookups)
	}
}

func TestS3ReportsTheRedirectWhenTheRegionCannotBeFound(t *testing.T) {
	store := &fakeS3{omitRegion: true, objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	driver := awsDriver(startFakeS3(t, store), "eu-north-1")
	driver.lookupRegion = func(context.Context, string) (string, error) {
		return "", errors.New("no route to host")
	}
	ref, _ := Parse("s3://bucket/app.pkg")

	_, err := driver.Stat(context.Background(), ref)
	if err == nil || statusOf(err) != http.StatusMovedPermanently {
		t.Fatalf("want the 301 reported, got %v", err)
	}
	if store.requests.Load() != 1 {
		t.Errorf("made %d requests, want no retry without a Region to retry in", store.requests.Load())
	}
}

func TestS3DoesNotChaseARegionOnACompatibleStore(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "us-east-1"},
	}}
	r := compatibleStoreResolver(t, startFakeS3(t, store))

	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); err == nil {
		t.Fatal("want the store's answer reported")
	}
	if store.requests.Load() != 1 {
		t.Errorf("made %d requests; a custom endpoint has one Region", store.requests.Load())
	}
}

func TestS3BuildsItsClientOnceUnderConcurrentUse(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	var stsCalls atomic.Int64
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stsCalls.Add(1)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>arn:aws:iam::123456789012:user/ci</Arn><UserId>AIDAEXAMPLE</UserId><Account>123456789012</Account></GetCallerIdentityResult><ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></GetCallerIdentityResponse>`)
	}))
	t.Cleanup(sts.Close)
	isolateAWSEnvironment(t)
	r := New(Config{S3: S3Config{
		Region: "eu-west-1", AccessKey: "AKIAEXAMPLE", SecretKey: "secret",
		EndpointS3: startFakeS3(t, store), EndpointSTS: sts.URL, UsePathStyle: true,
	}}, nil)

	var wg sync.WaitGroup
	failures := make(chan error, 16)
	for range 16 {
		wg.Go(func() {
			if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); err != nil {
				failures <- err
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Errorf("unexpected error: %v", err)
	}
	if got := stsCalls.Load(); got != 1 {
		t.Errorf("credentials were validated %d times, want once for all sixteen reads", got)
	}
}

func TestS3ReportsACredentialFailureAndTriesAgain(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	var refuse atomic.Bool
	var stsCalls atomic.Int64
	refuse.Store(true)
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		stsCalls.Add(1)
		w.Header().Set("Content-Type", "text/xml")
		if refuse.Load() {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>SignatureDoesNotMatch</Code><Message>The request signature we calculated does not match the signature you provided.</Message></Error><RequestId>1</RequestId></ErrorResponse>`)
			return
		}
		_, _ = io.WriteString(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>arn:aws:iam::123456789012:user/ci</Arn><UserId>AIDAEXAMPLE</UserId><Account>123456789012</Account></GetCallerIdentityResult><ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></GetCallerIdentityResponse>`)
	}))
	t.Cleanup(sts.Close)
	isolateAWSEnvironment(t)
	r := New(Config{S3: S3Config{
		Region: "eu-west-1", AccessKey: "AKIAEXAMPLE", SecretKey: "wrong-secret",
		EndpointS3: startFakeS3(t, store), EndpointSTS: sts.URL, UsePathStyle: true,
	}}, nil)

	_, err := r.Stat(context.Background(), "s3://bucket/app.pkg")
	if !errors.Is(err, ErrCredentials) || !strings.Contains(err.Error(), "SignatureDoesNotMatch") {
		t.Fatalf("want ErrCredentials carrying the cause, got %v", err)
	}
	if strings.Contains(err.Error(), "wrong-secret") {
		t.Errorf("the error must not carry the secret key: %v", err)
	}
	if store.requests.Load() != 0 {
		t.Error("the object must not be requested with a credential that failed validation")
	}

	refuse.Store(false)
	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); !errors.Is(err, ErrCredentials) {
		t.Fatalf("a failed build is held for the callers queued behind it, got %v", err)
	}
	if got := stsCalls.Load(); got != 1 {
		t.Errorf("AWS STS was called %d times, want the failure reported without a second build", got)
	}

	shortenBuildFailureHold(t)
	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); err != nil {
		t.Fatalf("a failed build must not be kept past the hold: %v", err)
	}
}

func shortenBuildFailureHold(t *testing.T) {
	t.Helper()
	previous := buildFailureHold
	buildFailureHold = 0
	t.Cleanup(func() { buildFailureHold = previous })
}

func TestS3BuildCutShortByItsCallerIsNotACredentialFailureAndIsNotHeld(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	release := make(chan struct{})
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, callerIdentityResponse)
	}))
	t.Cleanup(sts.Close)
	isolateAWSEnvironment(t)
	r := New(Config{S3: S3Config{
		Region: "eu-west-1", AccessKey: "AKIAEXAMPLE", SecretKey: "secret",
		EndpointS3: startFakeS3(t, store), EndpointSTS: sts.URL, UsePathStyle: true,
	}}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := r.Stat(ctx, "s3://bucket/app.pkg")
	if err == nil || errors.Is(err, ErrCredentials) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want a deadline error that is not a credential failure, got %v", err)
	}

	close(release)
	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); err != nil {
		t.Fatalf("a build its caller abandoned must be tried again at once: %v", err)
	}
}

func TestS3StoreThatDeclaresNoSizeIsRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		if r.Method == http.MethodGet {
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, "chunked body")
		}
	}))
	t.Cleanup(server.Close)
	r := compatibleStoreResolver(t, server.URL)

	if _, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 0); !errors.Is(err, ErrNoSize) {
		t.Fatalf("want ErrNoSize, got %v", err)
	}
}

func TestS3CustomEndpointFromTheEnvironmentCountsAsOne(t *testing.T) {
	isolateAWSEnvironment(t)
	driver := newS3Driver(S3Config{}, "")
	if driver.customEndpoint() {
		t.Fatal("no endpoint is configured")
	}
	for _, name := range []string{"AWS_ENDPOINT_URL_S3", "AWS_ENDPOINT_URL"} {
		t.Setenv(name, "https://minio.internal:9000")
		if !driver.customEndpoint() {
			t.Errorf("%s must count as a custom endpoint", name)
		}
		t.Setenv(name, "")
	}
	if !newS3Driver(S3Config{EndpointS3: "https://minio.internal:9000"}, "").customEndpoint() {
		t.Error("endpoints.s3 must count as a custom endpoint")
	}
}

func TestS3RegionLookupStaysInsideTheStandardPartition(t *testing.T) {
	for region, want := range map[string]bool{
		"eu-west-1": true, "us-east-1": true, "cn-north-1": false, "us-gov-west-1": false, "us-iso-east-1": false,
	} {
		if got := inCommercialPartition(region); got != want {
			t.Errorf("%s: got %v, want %v", region, got, want)
		}
	}

	store := &fakeS3{omitRegion: true, objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "cn-northwest-1"},
	}}
	driver := awsDriver(startFakeS3(t, store), "cn-north-1")
	driver.lookupRegion = func(context.Context, string) (string, error) {
		t.Error("a bucket outside the standard partition must not be looked up in it")
		return "", nil
	}
	ref, _ := Parse("s3://bucket/app.pkg")
	if _, err := driver.Stat(context.Background(), ref); err == nil {
		t.Fatal("want the redirect reported")
	}
}

func TestS3NamesACredentialThatFailsAtRequestTime(t *testing.T) {
	driver := newS3Driver(S3Config{}, "")
	ref, _ := Parse("s3://bucket/app.pkg")
	err := driver.classify(ref, errors.New("operation error S3: HeadObject, get identity: get credentials: failed to refresh cached credentials, token expired"))
	if !errors.Is(err, ErrCredentials) {
		t.Errorf("want ErrCredentials, got %v", err)
	}
	if err := driver.classify(ref, errors.New("dial tcp: no route to host")); errors.Is(err, ErrCredentials) {
		t.Errorf("a network failure is not a credential failure: %v", err)
	}
}

func TestS3CredentialExchangeIsNotLogged(t *testing.T) {
	const token = "eyJhbGciOiJSUzI1NiJ9.web-identity-token.signature"
	const session = "FQoGZXIvYXdzSESSIONTOKENVALUE"
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: "eu-west-1"},
	}}
	sts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "text/xml")
		if r.Form.Get("Action") == "AssumeRoleWithWebIdentity" {
			_, _ = io.WriteString(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>ASIAEXAMPLEEXAMPLE00</AccessKeyId><SecretAccessKey>wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY00</SecretAccessKey><SessionToken>`+session+`</SessionToken><Expiration>2099-01-01T00:00:00Z</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/pkg-read/ci</Arn><AssumedRoleId>AROAEXAMPLE:ci</AssumedRoleId></AssumedRoleUser></AssumeRoleWithWebIdentityResult><ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></AssumeRoleWithWebIdentityResponse>`)
			return
		}
		_, _ = io.WriteString(w, callerIdentityResponse)
	}))
	t.Cleanup(sts.Close)
	isolateAWSEnvironment(t)
	r := New(Config{S3: S3Config{
		Region:       "eu-west-1",
		EndpointS3:   startFakeS3(t, store),
		EndpointSTS:  sts.URL,
		UsePathStyle: true,
		AssumeRoleWithWebIdentity: &S3AssumeRoleWithWebIdentity{
			RoleARN: "arn:aws:iam::123456789012:role/pkg-read", SessionName: "ci", WebIdentityToken: token,
		},
	}}, nil)

	var logged bytes.Buffer
	ctx := tflogtest.RootLogger(context.Background(), &logged)
	if _, err := r.Stat(ctx, "s3://bucket/app.pkg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.credentials[0] != "ASIAEXAMPLEEXAMPLE00" {
		t.Fatalf("the role was not assumed: signed with %q", store.credentials[0])
	}
	if !strings.Contains(logged.String(), "examining cloud storage object") {
		t.Fatal("the test captured no provider log output, so it proves nothing")
	}
	for name, secret := range map[string]string{"web identity token": token, "session token": session} {
		if strings.Contains(logged.String(), secret) {
			t.Errorf("the %s reached the log", name)
		}
	}
}

const callerIdentityResponse = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>arn:aws:iam::123456789012:user/ci</Arn><UserId>AIDAEXAMPLE</UserId><Account>123456789012</Account></GetCallerIdentityResult><ResponseMetadata><RequestId>1</RequestId></ResponseMetadata></GetCallerIdentityResponse>`

func TestS3FallsBackToARegionWhenNoneIsConfigured(t *testing.T) {
	store := &fakeS3{objects: map[string]fakeObject{
		"bucket/app.pkg": {body: "x", etag: `"abc"`, region: fallbackRegion},
	}}
	isolateAWSEnvironment(t)
	r := New(Config{S3: S3Config{
		AccessKey: "AKIAEXAMPLE", SecretKey: "secret", EndpointS3: startFakeS3(t, store), UsePathStyle: true,
	}}, nil)

	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); err != nil {
		t.Fatalf("a client with no Region configured must still start: %v", err)
	}
	if store.signedFor[0] != fallbackRegion {
		t.Errorf("signed for %q, want %q", store.signedFor[0], fallbackRegion)
	}
}
