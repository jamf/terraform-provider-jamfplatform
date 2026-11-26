// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
)

func TestStageCopiesTheObjectToARewoundTemporaryFile(t *testing.T) {
	source := &fakeSource{body: "package bytes", declared: 13, marker: `etag:"abc"`}
	r := resolverWith(KindS3, source)

	staged, cleanup, err := r.Stage(context.Background(), "s3://bucket/releases/app.pkg", `etag:"abc"`, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(staged.File)
	if string(body) != "package bytes" {
		t.Errorf("file holds %q", body)
	}
	if staged.Filename != "app.pkg" || staged.Size != 13 || staged.Marker != `etag:"abc"` {
		t.Errorf("staged = %+v", staged)
	}
	if source.openPin != `etag:"abc"` {
		t.Errorf("the pin did not reach the driver: %q", source.openPin)
	}
	info, err := os.Stat(staged.File.Name())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want the file private to the user", info.Mode().Perm())
	}
	if strings.Contains(staged.File.Name(), "app.pkg") {
		t.Errorf("the temporary name must not come from the object: %s", staged.File.Name())
	}

	cleanup()
	if _, err := os.Stat(staged.File.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup must remove the file, stat gave %v", err)
	}
	if !source.closed {
		t.Error("the object's body must be closed")
	}
}

func TestStageRefusesAShortOrLongRead(t *testing.T) {
	for name, source := range map[string]*fakeSource{
		"short": {body: "12345", declared: 10},
		"long":  {body: "1234567890-and-more", declared: 10},
	} {
		t.Run(name, func(t *testing.T) {
			before := temporaryFiles(t)
			_, cleanup, err := resolverWith(KindS3, source).Stage(context.Background(), "s3://bucket/app.pkg", "", 0)
			cleanup()
			if !errors.Is(err, ErrIncomplete) {
				t.Fatalf("want ErrIncomplete, got %v", err)
			}
			if after := temporaryFiles(t); after != before {
				t.Errorf("a failed stage left %d temporary file(s) behind", after-before)
			}
		})
	}
}

// cutSource is a driver whose body fails partway, as a dropped connection
// does.
type cutSource struct{}

func (cutSource) Stat(context.Context, Ref) (Attributes, error) { return Attributes{}, nil }

func (cutSource) Open(context.Context, Ref, string, int64) (Opened, error) {
	body := io.MultiReader(strings.NewReader("12345"), iotest.ErrReader(io.ErrUnexpectedEOF))
	return Opened{Body: io.NopCloser(body), Size: 10}, nil
}

func TestStageReportsACutConnection(t *testing.T) {
	before := temporaryFiles(t)
	_, cleanup, err := resolverWith(KindS3, cutSource{}).Stage(context.Background(), "s3://bucket/app.pkg", "", 0)
	cleanup()
	if !errors.Is(err, ErrIncomplete) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want ErrIncomplete wrapping the cause, got %v", err)
	}
	if after := temporaryFiles(t); after != before {
		t.Errorf("a failed stage left %d temporary file(s) behind", after-before)
	}
}

func TestStageOnANilResolverIsRefusedWithAUsableCleanup(t *testing.T) {
	var r *Resolver
	_, cleanup, err := r.Stage(context.Background(), "s3://bucket/app.pkg", "", 0)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
	cleanup()
}

func temporaryFiles(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("reading the temporary directory: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "jamfplatform-source-") {
			count++
		}
	}
	return count
}
