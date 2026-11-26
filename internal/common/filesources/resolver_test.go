// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

// fakeSource is a driver holding one object in memory. It records what it was
// asked, so a test can assert the Resolver passed the pin and the parsed
// reference through.
type fakeSource struct {
	body     string
	declared int64
	marker   string
	filename string

	statRef Ref
	openRef Ref
	openPin string
	closed  bool
}

func (f *fakeSource) Stat(_ context.Context, ref Ref) (Attributes, error) {
	f.statRef = ref
	return Attributes{Size: f.declared, Marker: f.marker}, nil
}

func (f *fakeSource) Open(_ context.Context, ref Ref, pin string, _ int64) (Opened, error) {
	f.openRef = ref
	f.openPin = pin
	return Opened{
		Body:     &closeRecorder{Reader: strings.NewReader(f.body), closed: &f.closed},
		Filename: f.filename,
		Size:     f.declared, Marker: f.marker,
	}, nil
}

type closeRecorder struct {
	io.Reader
	closed *bool
}

func (c *closeRecorder) Close() error {
	*c.closed = true
	return nil
}

func resolverWith(kind Kind, source Source) *Resolver {
	r := New(Config{}, nil)
	r.drivers[kind] = source
	return r
}

func TestResolverNative(t *testing.T) {
	var unconfigured *Resolver
	for _, r := range []*Resolver{unconfigured, New(Config{}, nil)} {
		for src, want := range map[string]bool{
			"s3://bucket/app.pkg":            true,
			"s3://bucket":                    true,
			"azblob://account/pkgs/app.pkg":  true,
			"https://example.com/app.pkg":    false,
			"/tmp/app.pkg":                   false,
			"gs://bucket/app.pkg":            false,
			"":                               false,
			"azblob://not an account/x/y":    true,
			"s3://bucket/app.pkg?versionId=": true,
		} {
			if got := r.Native(src); got != want {
				t.Errorf("Native(%q) = %v, want %v", src, got, want)
			}
		}
	}
}

func TestNilResolverRefusesANativeSource(t *testing.T) {
	var r *Resolver
	if _, err := r.Stat(context.Background(), "s3://bucket/app.pkg"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Stat: want ErrNotConfigured, got %v", err)
	}
	if _, err := r.Open(context.Background(), "azblob://account/pkgs/app.pkg", "", 0); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Open: want ErrNotConfigured, got %v", err)
	}
	if got := r.Unresolved(); got != nil {
		t.Errorf("Unresolved on a nil resolver = %v, want nil", got)
	}
}

func TestResolverRefusesWhatItDoesNotRead(t *testing.T) {
	r := New(Config{}, nil)
	for _, src := range []string{"/tmp/app.pkg", "https://example.com/app.pkg", "gs://bucket/app.pkg"} {
		if _, err := r.Stat(context.Background(), src); !errors.Is(err, ErrNotNative) {
			t.Errorf("Stat(%q): want ErrNotNative, got %v", src, err)
		}
	}
	_, err := r.Stat(context.Background(), "s3://bucket")
	if _, ok := errors.AsType[*SyntaxError](err); !ok {
		t.Errorf("a malformed native source must surface its syntax error, got %v", err)
	}
}

func TestResolverWithNoDriverNamesTheScheme(t *testing.T) {
	r := New(Config{}, nil)
	delete(r.drivers, KindAzureBlob)
	_, err := r.Open(context.Background(), "azblob://account/pkgs/app.pkg", "", 0)
	if !errors.Is(err, ErrNoDriver) {
		t.Fatalf("want ErrNoDriver, got %v", err)
	}
	if !strings.Contains(err.Error(), "azblob://") {
		t.Errorf("the error must name the scheme: %q", err)
	}
}

func TestResolverStatPassesTheParsedReference(t *testing.T) {
	source := &fakeSource{declared: 42, marker: "etag:\"abc\""}
	r := resolverWith(KindS3, source)

	got, err := r.Stat(context.Background(), "s3://bucket/releases/app.pkg?versionId=v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Size != 42 || got.Marker != "etag:\"abc\"" {
		t.Errorf("attributes = %+v", got)
	}
	if source.statRef.Bucket != "bucket" || source.statRef.Key != "releases/app.pkg" || source.statRef.VersionID != "v1" {
		t.Errorf("driver saw %+v", source.statRef)
	}
}

func TestResolverOpenPassesThePinAndFillsTheFilename(t *testing.T) {
	source := &fakeSource{body: "hello", declared: 5, marker: "etag:\"abc\""}
	r := resolverWith(KindAzureBlob, source)

	opened, err := r.Open(context.Background(), "azblob://account/pkgs/releases/app.pkg", "etag:\"abc\"", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = opened.Body.Close() }()

	if source.openPin != "etag:\"abc\"" {
		t.Errorf("driver saw pin %q", source.openPin)
	}
	if opened.Filename != "app.pkg" {
		t.Errorf("Filename = %q, want the blob's base name", opened.Filename)
	}
	body, _ := io.ReadAll(opened.Body)
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
}

func TestResolverOpenKeepsTheDriversFilename(t *testing.T) {
	source := &fakeSource{body: "x", declared: 1, filename: "served-name.pkg"}
	r := resolverWith(KindS3, source)

	opened, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = opened.Body.Close() }()
	if opened.Filename != "served-name.pkg" {
		t.Errorf("Filename = %q", opened.Filename)
	}
}

func TestResolverOpenRefusesAnObjectOverTheCap(t *testing.T) {
	source := &fakeSource{body: "0123456789", declared: 10}
	r := resolverWith(KindS3, source)

	_, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 9)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	if !source.closed {
		t.Error("the body of a refused object must be closed")
	}

	source.closed = false
	opened, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 10)
	if err != nil {
		t.Fatalf("an object exactly at the cap must open: %v", err)
	}
	_ = opened.Body.Close()
}

func TestResolverOpenAppliesTheDefaultCapToSizesPastFourGibibytes(t *testing.T) {
	const fiveGiB = int64(5) << 30
	const nineGiB = int64(9) << 30

	r := resolverWith(KindS3, &fakeSource{declared: fiveGiB})
	opened, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("5 GiB is under the default cap: %v", err)
	}
	if opened.Size != fiveGiB {
		t.Errorf("Size = %d, want %d", opened.Size, fiveGiB)
	}
	_ = opened.Body.Close()

	r = resolverWith(KindS3, &fakeSource{declared: nineGiB})
	if _, err := r.Open(context.Background(), "s3://bucket/app.pkg", "", 0); !errors.Is(err, ErrTooLarge) {
		t.Errorf("9 GiB is over the default cap: got %v", err)
	}
}

func TestResolverUnresolvedIsACopy(t *testing.T) {
	paths := []string{"file_sources.s3.region"}
	r := New(Config{}, paths)
	paths[0] = "changed"

	got := r.Unresolved()
	if len(got) != 1 || got[0] != "file_sources.s3.region" {
		t.Fatalf("Unresolved = %v", got)
	}
	got[0] = "changed"
	if r.Unresolved()[0] != "file_sources.s3.region" {
		t.Error("a caller must not be able to edit the resolver's list")
	}
}

func TestVersioned(t *testing.T) {
	for src, want := range map[string]bool{
		"s3://bucket/app.pkg?versionId=v1":         true,
		"s3://bucket/app.pkg":                      false,
		"s3://bucket/app.pkg?versionId=":           false,
		"azblob://account/pkgs/app.pkg":            false,
		"https://example.com/app.pkg?versionId=v1": false,
		"/tmp/app.pkg":                             false,
	} {
		if got := Versioned(src); got != want {
			t.Errorf("Versioned(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestUnresolvedForNamesOnlyWhatTheSourceDependsOn(t *testing.T) {
	r := New(Config{}, []string{
		"file_sources.s3.region",
		"file_sources.s3_lookalike",
		"file_sources.azure.client_id",
		"file_sources.https[0].bearer_token",
	})
	cases := map[string][]string{
		"s3://bucket/app.pkg":           {"file_sources.s3.region"},
		"azblob://account/pkgs/app.pkg": {"file_sources.azure.client_id"},
		"https://example.com/app.pkg":   nil,
		"/tmp/app.pkg":                  nil,
	}
	for src, want := range cases {
		if got := r.UnresolvedFor(src); !slices.Equal(got, want) {
			t.Errorf("UnresolvedFor(%q) = %v, want %v", src, got, want)
		}
	}

	whole := New(Config{}, []string{"file_sources"})
	for _, src := range []string{"s3://bucket/app.pkg", "azblob://account/pkgs/app.pkg"} {
		if got := whole.UnresolvedFor(src); !slices.Equal(got, []string{"file_sources"}) {
			t.Errorf("UnresolvedFor(%q) = %v, want the whole block", src, got)
		}
	}
	var unconfigured *Resolver
	if got := unconfigured.UnresolvedFor("s3://bucket/app.pkg"); got != nil {
		t.Errorf("a nil resolver has nothing unresolved, got %v", got)
	}
}
