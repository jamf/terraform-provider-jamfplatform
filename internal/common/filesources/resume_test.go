// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package filesources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"testing"
	"testing/iotest"
	"time"
)

// cuttingSource is a driver over one object that drops each response after a
// set number of bytes, the way a cut connection does: as io.ErrUnexpectedEOF,
// or as a clean early end of the body. It honours the offset and the pin, and
// records every open.
type cuttingSource struct {
	content   func(offset, n int64) []byte
	size      int64
	marker    string
	cutAfter  []int64
	cleanEOF  bool
	failOpens int

	reopenErr    error
	reopenMarker string
	reopenSize   int64
	failAtEnd    error

	opens []cuttingOpen
}

type cuttingOpen struct {
	pin    string
	offset int64
}

func (c *cuttingSource) Stat(context.Context, Ref) (Attributes, error) {
	return Attributes{Size: c.size, Marker: c.marker}, nil
}

func (c *cuttingSource) Open(_ context.Context, _ Ref, pin string, offset int64) (Opened, error) {
	c.opens = append(c.opens, cuttingOpen{pin: pin, offset: offset})
	if pin != "" && pin != c.marker {
		return Opened{}, ErrChanged
	}
	if len(c.opens) > 1 && c.reopenErr != nil {
		return Opened{}, c.reopenErr
	}
	if len(c.opens) > 1 && c.failOpens > 0 {
		c.failOpens--
		return Opened{}, errors.New("connection refused")
	}
	remaining := c.size - offset
	var body io.Reader = &generatedReader{content: c.content, offset: offset, end: c.size}
	if i := len(c.opens) - 1; i < len(c.cutAfter) && c.cutAfter[i] < remaining {
		body = io.LimitReader(body, c.cutAfter[i])
		if !c.cleanEOF {
			body = io.MultiReader(body, iotest.ErrReader(io.ErrUnexpectedEOF))
		}
	}
	if c.failAtEnd != nil {
		body = io.MultiReader(body, iotest.ErrReader(c.failAtEnd))
	}
	opened := Opened{Body: io.NopCloser(body), Size: c.size, Marker: c.marker}
	if len(c.opens) > 1 && c.reopenMarker != "" {
		opened.Marker = c.reopenMarker
	}
	if len(c.opens) > 1 && c.reopenSize != 0 {
		opened.Size = c.reopenSize
	}
	return opened, nil
}

// generatedReader yields an object's bytes from a function of the offset, so a
// test can stand in for an object of several gibibytes without holding it.
type generatedReader struct {
	content func(offset, n int64) []byte
	offset  int64
	end     int64
}

func (g *generatedReader) Read(p []byte) (int, error) {
	if g.offset >= g.end {
		return 0, io.EOF
	}
	n := min(int64(len(p)), g.end-g.offset)
	copy(p, g.content(g.offset, n))
	g.offset += n
	return int(n), nil
}

func fromBytes(data []byte) func(offset, n int64) []byte {
	return func(offset, n int64) []byte { return data[offset : offset+n] }
}

func withoutResumeBackoff(t *testing.T) {
	t.Helper()
	previous := resumeBackoff
	resumeBackoff = 0
	t.Cleanup(func() { resumeBackoff = previous })
}

func patterned(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*7 + i/251)
	}
	return data
}

func TestResumedReadIsByteIdentical(t *testing.T) {
	withoutResumeBackoff(t)
	data := patterned(100_000)
	for name, cleanEOF := range map[string]bool{"cut reported as an error": false, "cut reported as an early end": true} {
		t.Run(name, func(t *testing.T) {
			source := &cuttingSource{content: fromBytes(data), size: int64(len(data)), marker: `etag:"abc"`, cutAfter: []int64{30_000, 1, 45_000, 10}, cleanEOF: cleanEOF}
			opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := io.ReadAll(opened.Body)
			if err != nil {
				t.Fatalf("a cut must be resumed, got %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("the resumed stream differs from the object: %d bytes read, %d stored", len(got), len(data))
			}
			wantOffsets := []int64{0, 30_000, 30_001, 75_001, 75_011}
			if len(source.opens) != len(wantOffsets) {
				t.Fatalf("opened %d times, want %d: %+v", len(source.opens), len(wantOffsets), source.opens)
			}
			for i, open := range source.opens {
				if open.offset != wantOffsets[i] {
					t.Errorf("open %d started at byte %d, want %d", i, open.offset, wantOffsets[i])
				}
				if i > 0 && open.pin != `etag:"abc"` {
					t.Errorf("open %d was pinned to %q; every reopen must be pinned to the marker the first response carried, though the caller pinned nothing", i, open.pin)
				}
			}
		})
	}
}

func TestResumedReadRefusesAnObjectReplacedMeanwhile(t *testing.T) {
	withoutResumeBackoff(t)
	data := patterned(1000)
	source := &cuttingSource{content: fromBytes(data), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", `etag:"abc"`, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	head := make([]byte, 400)
	if _, err := io.ReadFull(opened.Body, head); err != nil {
		t.Fatalf("reading up to the cut: %v", err)
	}
	source.marker = `etag:"replaced"`

	_, err = io.ReadAll(opened.Body)
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("bytes of two objects must never be spliced: want ErrChanged, got %v", err)
	}
}

func TestResumedReadGivesUpWhenItMakesNoProgress(t *testing.T) {
	withoutResumeBackoff(t)
	source := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}, failOpens: 100}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = io.ReadAll(opened.Body)
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("want ErrIncomplete, got %v", err)
	}
	if got := len(source.opens) - 1; got != maxResumeAttempts {
		t.Errorf("reopened %d times, want %d", got, maxResumeAttempts)
	}
}

func TestResumedReadCountsOnlyConsecutiveFailures(t *testing.T) {
	withoutResumeBackoff(t)
	data := patterned(4000)
	cuts := make([]int64, 20)
	for i := range cuts {
		cuts[i] = 100
	}
	source := &cuttingSource{content: fromBytes(data), size: 4000, marker: `etag:"abc"`, cutAfter: cuts}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := io.ReadAll(opened.Body)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("twenty cuts that each make progress must all be resumed: %d bytes, %v", len(got), err)
	}
}

func TestResumedReadStopsWhenItsContextEnds(t *testing.T) {
	previous := resumeBackoff
	resumeBackoff = time.Hour
	t.Cleanup(func() { resumeBackoff = previous })
	source := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	opened, err := resolverWith(KindS3, source).Open(ctx, "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = io.ReadAll(opened.Body)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrIncomplete) {
		t.Fatalf("want the context's error, got %v", err)
	}
	if len(source.opens) != 1 {
		t.Errorf("reopened %d times after the context ended", len(source.opens)-1)
	}
}

func TestReadWithNoMarkerIsNotResumed(t *testing.T) {
	withoutResumeBackoff(t)
	source := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, cutAfter: []int64{400}}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = io.ReadAll(opened.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("an object that cannot be pinned must not be resumed, got %v", err)
	}
	if len(source.opens) != 1 {
		t.Errorf("reopened %d times with nothing to pin to", len(source.opens)-1)
	}
}

func TestStageResumesACutRead(t *testing.T) {
	withoutResumeBackoff(t)
	data := patterned(50_000)
	source := &cuttingSource{content: fromBytes(data), size: 50_000, marker: `etag:"abc"`, cutAfter: []int64{20_000}}

	staged, cleanup, err := resolverWith(KindS3, source).Stage(context.Background(), "s3://bucket/app.pkg", "", 0)
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(staged.File)
	if !bytes.Equal(got, data) {
		t.Fatalf("the staged file differs from the object")
	}
}

func TestResumeAcrossTheFourGibibyteBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("reads five gibibytes of generated bytes")
	}
	withoutResumeBackoff(t)
	const size = int64(5) << 30
	const cut = int64(4)<<30 + 12345
	content := func(offset, n int64) []byte {
		chunk := make([]byte, n)
		for i := range chunk {
			chunk[i] = byte((offset + int64(i)) >> 20)
		}
		return chunk
	}
	source := &cuttingSource{content: content, size: size, marker: `etag:"abc"`, cutAfter: []int64{cut}}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opened.Size != size {
		t.Fatalf("Size = %d, want %d", opened.Size, size)
	}

	resumed := sha256.New()
	n, err := io.Copy(resumed, opened.Body)
	if err != nil || n != size {
		t.Fatalf("read %d of %d bytes: %v", n, size, err)
	}
	if len(source.opens) != 2 || source.opens[1].offset != cut {
		t.Fatalf("opens = %+v, want one reopen at byte %d", source.opens, cut)
	}
	straight := sha256.New()
	if _, err := io.Copy(straight, &generatedReader{content: content, end: size}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resumed.Sum(nil), straight.Sum(nil)) {
		t.Error("a read resumed past four gibibytes differs from a straight read")
	}
}

func TestErrorAfterTheLastByteIsReportedNotResumed(t *testing.T) {
	withoutResumeBackoff(t)
	checksum := errors.New("checksum did not match")
	source := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, failAtEnd: checksum}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := io.ReadAll(opened.Body)
	if !errors.Is(err, checksum) || len(got) != 1000 {
		t.Fatalf("want every byte and then the store's own error, got %d bytes and %v", len(got), err)
	}
	if len(source.opens) != 1 {
		t.Errorf("reopened %d times at the end of the object", len(source.opens)-1)
	}
}

func TestResumeStopsAtOnceOnARefusalRetryingCannotChange(t *testing.T) {
	withoutResumeBackoff(t)
	for name, refusal := range map[string]error{"not found": ErrNotFound, "access denied": ErrAccessDenied, "no credential": ErrCredentials} {
		t.Run(name, func(t *testing.T) {
			source := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}, reopenErr: refusal}
			opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_, err = io.ReadAll(opened.Body)
			if !errors.Is(err, refusal) {
				t.Fatalf("want %v, got %v", refusal, err)
			}
			if len(source.opens) != 2 {
				t.Errorf("reopened %d times, want once", len(source.opens)-1)
			}
		})
	}
}

func TestResumeRefusesAReopenedResponseThatIsNotTheSameObject(t *testing.T) {
	withoutResumeBackoff(t)
	cases := map[string]struct {
		source *cuttingSource
		want   error
	}{
		"another marker, from a store that ignores If-Match": {
			&cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}, reopenMarker: `etag:"other"`}, ErrChanged,
		},
		"another size": {
			&cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}, reopenSize: 2000}, ErrIncomplete,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opened, err := resolverWith(KindS3, tc.source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := io.ReadAll(opened.Body)
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if len(got) != 400 {
				t.Errorf("delivered %d bytes; nothing from the second object may follow the first's 400", len(got))
			}
		})
	}
}

func TestResumingReaderStaysFailedAndStaysClosed(t *testing.T) {
	withoutResumeBackoff(t)
	source := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`, cutAfter: []int64{400}, reopenErr: ErrChanged}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := io.ReadAll(opened.Body); !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged, got %v", err)
	}
	opens := len(source.opens)
	if n, err := opened.Body.Read(make([]byte, 10)); n != 0 || !errors.Is(err, ErrChanged) {
		t.Errorf("a failed read must stay failed, got %d bytes and %v", n, err)
	}
	if err := opened.Body.Close(); err != nil {
		t.Errorf("closing after a failed reopen: %v", err)
	}
	if err := opened.Body.Close(); err != nil {
		t.Errorf("closing twice: %v", err)
	}
	if len(source.opens) != opens {
		t.Errorf("a failed reader made %d more requests", len(source.opens)-opens)
	}

	healthy := &cuttingSource{content: fromBytes(patterned(1000)), size: 1000, marker: `etag:"abc"`}
	opened, err = resolverWith(KindS3, healthy).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = opened.Body.Close()
	if n, err := opened.Body.Read(make([]byte, 10)); n != 0 || err == nil {
		t.Errorf("a closed reader must read nothing, got %d bytes and %v", n, err)
	}
	if len(healthy.opens) != 1 {
		t.Errorf("a closed reader made %d more requests", len(healthy.opens)-1)
	}
}

func TestEmptyObjectReadsAsEmpty(t *testing.T) {
	source := &cuttingSource{content: fromBytes(nil), size: 0, marker: `etag:"abc"`}
	opened, err := resolverWith(KindS3, source).Open(context.Background(), "s3://bucket/app.pkg", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := io.ReadAll(opened.Body)
	if err != nil || len(got) != 0 || len(source.opens) != 1 {
		t.Errorf("got %d bytes, %v, %d opens", len(got), err, len(source.opens))
	}
}

func TestParseContentRange(t *testing.T) {
	cases := map[string]struct {
		start, total int64
		ok           bool
	}{
		"bytes 5-9/10":                           {5, 10, true},
		"bytes 0-0/8589934592":                   {0, 8589934592, true},
		"bytes 4294979641-5368709119/5368709120": {4294979641, 5368709120, true},
		"":                                       {0, 0, false},
		"bytes 5-9":                              {0, 0, false},
		"bytes 5-9/*":                            {0, 0, false},
		"bytes */10":                             {0, 0, false},
	}
	for header, want := range cases {
		start, total, ok := parseContentRange(header)
		if ok != want.ok || (ok && (start != want.start || total != want.total)) {
			t.Errorf("%q: got %d, %d, %v", header, start, total, ok)
		}
	}
}

func TestRangedSizeRefusesARangeTheStoreDidNotHonour(t *testing.T) {
	ref, _ := Parse("s3://bucket/app.pkg")
	if size, err := rangedSize(ref, "bytes 6-9/10", 6); err != nil || size != 10 {
		t.Errorf("got %d, %v", size, err)
	}
	if _, err := rangedSize(ref, "bytes 0-9/10", 6); !errors.Is(err, ErrIncomplete) {
		t.Errorf("a response that starts elsewhere must be refused, got %v", err)
	}
	if _, err := rangedSize(ref, "", 6); !errors.Is(err, ErrNoSize) {
		t.Errorf("a response with no Content-Range must be refused, got %v", err)
	}
}
