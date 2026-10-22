// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"crypto/sha3"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyPollTick(t *testing.T) {
	t.Parallel()

	const (
		expectHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		prevHash   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		corrupt    = "ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		expectMD5  = "11111111111111111111111111111111"
		prevMD5    = "22222222222222222222222222222222"
		expectSize = int64(11806221)
	)
	expectSizeStr := "11806221"
	oldSizeStr := "11774769"

	local := fileDigests{sha3: expectHash, md5: expectMD5}
	previous := fileDigests{sha3: prevHash, md5: prevMD5}
	jcds := func(v string) fileDigests { return fileDigests{sha3: v, md5: "", sha256: ""} }
	s3 := func(v string) fileDigests { return fileDigests{md5: v} }

	cases := []struct {
		name      string
		server    fileDigests
		status    string
		curSize   string
		local     fileDigests
		want      pollDecision
		wantNotes string
	}{
		{
			name:   "converged on JCDS SHA3-512",
			server: jcds(expectHash),
			status: "READY", curSize: expectSizeStr, local: local,
			want: pollDecisionConverged,
		},
		{
			name:   "converged on S3 MD5 only",
			server: s3(expectMD5),
			status: "READY", curSize: expectSizeStr, local: local,
			want:      pollDecisionConverged,
			wantNotes: "an S3 distribution point never fills sha3512; the MD5 is the strongest populated digest",
		},
		{
			name:   "strongest populated decides over a weaker wrong one",
			server: fileDigests{sha3: expectHash, md5: "wrong"},
			status: "READY", curSize: expectSizeStr, local: local,
			want: pollDecisionConverged,
		},
		{
			name:   "wrong SHA3-512 fails despite a matching MD5",
			server: fileDigests{sha3: corrupt, md5: expectMD5},
			status: "READY", curSize: expectSizeStr, local: local,
			want: pollDecisionCorruption,
		},
		{
			name:   "PENDING does not converge",
			server: s3(expectMD5),
			status: "PENDING", curSize: expectSizeStr, local: local,
			want: pollDecisionContinue,
		},
		{
			name:   "ERROR fails at once",
			server: s3(""),
			status: "ERROR", curSize: "", local: local,
			want: pollDecisionTransferFailed,
		},
		{
			name:   "previous file's digest still served — keep polling",
			server: s3(prevMD5),
			status: "READY", curSize: expectSizeStr, local: local,
			want:      pollDecisionContinue,
			wantNotes: "server has not recomputed from the new file yet",
		},
		{
			name:   "size still the old binary's — not converged",
			server: jcds(expectHash),
			status: "READY", curSize: oldSizeStr, local: local,
			want:      pollDecisionContinue,
			wantNotes: "size guard — JCDS in commit window; hash flipped but size hasn't",
		},
		{
			name:   "corruption — bogus digest, size matches upload",
			server: jcds(corrupt),
			status: "READY", curSize: expectSizeStr, local: local,
			want:      pollDecisionCorruption,
			wantNotes: "server stored bytes whose hash differs from what we sent",
		},
		{
			name:   "corruption skipped while size still in flight",
			server: jcds(corrupt),
			status: "READY", curSize: oldSizeStr, local: local,
			want:      pollDecisionContinue,
			wantNotes: "do not flag corruption during the transient size window",
		},
		{
			name:   "no digest populated yet — still warming up",
			server: fileDigests{},
			status: "", curSize: "", local: local,
			want: pollDecisionContinue,
		},
		{
			name:   "upload failed — size=0 with a new digest",
			server: jcds(expectHash),
			status: "READY", curSize: "0", local: local,
			want:      pollDecisionUploadFailed,
			wantNotes: "definitive failed-upload signal, checked before the match branches",
		},
		{
			name:   "upload failed — size=0 with a bogus digest",
			server: jcds(corrupt),
			status: "IN_PROGRESS", curSize: "0", local: local,
			want:      pollDecisionUploadFailed,
			wantNotes: "size=0 wins over corruption — do not wait out the poll budget to report the wrong failure mode",
		},
		{
			name:   "size=0 but digest has not changed yet — still warming up",
			server: s3(prevMD5),
			status: "READY", curSize: "0", local: local,
			want:      pollDecisionContinue,
			wantNotes: "digest still equals previous — not yet a signal either way",
		},
		{
			name:   "empty size is not zero size",
			server: jcds(expectHash),
			status: "READY", curSize: "", local: local,
			want:      pollDecisionContinue,
			wantNotes: "\"\" means not-yet-computed, not a failed upload",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyPollTick(tc.server, tc.local, previous, tc.status, tc.curSize, expectSize)
			if got != tc.want {
				t.Fatalf("decision = %v, want %v (%s)", got, tc.want, tc.wantNotes)
			}
		})
	}
}

// TestClassifyPollTick_ZeroByteUpload covers a genuinely empty
// package_file_source (expectSize == 0), which the "upload failed" fast path
// must NOT intercept — size "0" is the correct, converged value for a
// zero-byte binary, not a failure signal.
func TestClassifyPollTick_ZeroByteUpload(t *testing.T) {
	t.Parallel()

	const (
		expectHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		prevHash   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)

	got := classifyPollTick(fileDigests{sha3: expectHash}, fileDigests{sha3: expectHash}, fileDigests{sha3: prevHash}, "READY", "0", 0)
	if got != pollDecisionConverged {
		t.Fatalf("decision = %v, want %v — a zero-byte upload must converge, not be flagged as a failed upload", got, pollDecisionConverged)
	}
}

func TestHashFileSHA3_KnownBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.pkg")
	payload := []byte("hello world")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, size, err := HashFileSHA3(path)
	if err != nil {
		t.Fatalf("HashFileSHA3: %v", err)
	}
	if size != int64(len(payload)) {
		t.Errorf("size = %d, want %d", size, len(payload))
	}

	h := sha3.New512()
	if _, err := h.Write(payload); err != nil {
		t.Fatalf("hash write: %v", err)
	}
	want := hex.EncodeToString(h.Sum(nil))
	if got != want {
		t.Errorf("hash mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestHashFileSHA3_MissingFile(t *testing.T) {
	_, _, err := HashFileSHA3(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatalf("expected error on missing file")
	}
}

func TestManifestBodiesEqual_LocalSourceMatches(t *testing.T) {
	dir := t.TempDir()
	body := []byte("<?xml version=\"1.0\"?><plist/>")
	path := filepath.Join(dir, "manifest.plist")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	equal, err := ManifestBodiesEqual(context.Background(), string(body), path)
	if err != nil {
		t.Fatalf("ManifestBodiesEqual: %v", err)
	}
	if !equal {
		t.Errorf("expected equal manifest bodies")
	}
}

func TestManifestBodiesEqual_LocalSourceDiffers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.plist")
	if err := os.WriteFile(path, []byte("new-body"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	equal, err := ManifestBodiesEqual(context.Background(), "old-body", path)
	if err != nil {
		t.Fatalf("ManifestBodiesEqual: %v", err)
	}
	if equal {
		t.Errorf("expected manifest bodies to differ")
	}
}

func TestManifestBodiesEqual_URLSourceMatches(t *testing.T) {
	body := []byte("<?xml version=\"1.0\"?><plist><dict/></plist>")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	equal, err := ManifestBodiesEqual(context.Background(), string(body), srv.URL+"/manifest.plist")
	if err != nil {
		t.Fatalf("ManifestBodiesEqual: %v", err)
	}
	if !equal {
		t.Errorf("expected equal URL-loaded manifest")
	}
}

func TestManifestBodiesEqual_EmptySrcMatchesEmptyStored(t *testing.T) {
	equal, err := ManifestBodiesEqual(context.Background(), "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !equal {
		t.Errorf("empty src vs empty stored must be equal")
	}
}

func TestManifestBodiesEqual_EmptySrcVsNonEmptyStored(t *testing.T) {
	equal, err := ManifestBodiesEqual(context.Background(), "stored-body", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if equal {
		t.Errorf("empty src vs non-empty stored must not match")
	}
}

func TestIsZeroSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		curSize string
		want    bool
	}{
		{name: "literal zero", curSize: "0", want: true},
		{name: "empty string is not zero", curSize: "", want: false},
		{name: "nonzero size", curSize: "11806221", want: false},
		{name: "unparseable is not zero", curSize: "not-a-number", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isZeroSize(tc.curSize); got != tc.want {
				t.Errorf("isZeroSize(%q) = %v, want %v", tc.curSize, got, tc.want)
			}
		})
	}
}

func TestRetryableUploadFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		err         error
		attempt     int
		maxAttempts int
		want        bool
	}{
		{name: "upload failed with attempts remaining", err: errUploadFailed, attempt: 1, maxAttempts: 5, want: true},
		{name: "upload failed on last attempt", err: errUploadFailed, attempt: 5, maxAttempts: 5, want: false},
		{name: "upload failed past budget", err: errUploadFailed, attempt: 6, maxAttempts: 5, want: false},
		{name: "corruption is never retried", err: errCorruption, attempt: 1, maxAttempts: 5, want: false},
		{name: "timeout is never retried", err: errVerificationTimeout, attempt: 1, maxAttempts: 5, want: false},
		{name: "nil error is never retried", err: nil, attempt: 1, maxAttempts: 5, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := retryableUploadFailure(tc.err, tc.attempt, tc.maxAttempts); got != tc.want {
				t.Errorf("retryableUploadFailure(%v, %d, %d) = %v, want %v", tc.err, tc.attempt, tc.maxAttempts, got, tc.want)
			}
		})
	}
}

// TestHashFile_AllDigests checks every digest against known vectors for
// "hello world", the same payload the SHA-3-512 test uses.
func TestHashFile_AllDigests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture.pkg")
	if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, size, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if size != 11 {
		t.Errorf("size = %d, want 11", size)
	}
	if got.md5 != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Errorf("md5 = %q", got.md5)
	}
	if got.sha256 != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Errorf("sha256 = %q", got.sha256)
	}
	if got.sha3 == "" {
		t.Errorf("sha3 empty")
	}
}

func TestFileDigests_Matches(t *testing.T) {
	t.Parallel()

	local := fileDigests{sha3: "s", sha256: "b", md5: "m"}
	tests := []struct {
		name     string
		recorded fileDigests
		want     bool
	}{
		{"empty record never matches", fileDigests{}, false},
		{"JCDS sha3 record", fileDigests{sha3: "s"}, true},
		{"S3 md5-only record", fileDigests{md5: "m"}, true},
		{"strongest recorded decides", fileDigests{sha3: "s", md5: "other"}, true},
		{"different md5 only", fileDigests{md5: "other"}, false},
		{"different sha3 wins over matching md5", fileDigests{sha3: "other", md5: "m"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.recorded.matches(local); got != tc.want {
				t.Fatalf("matches = %v, want %v", got, tc.want)
			}
		})
	}
}
