// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

type scriptedVerifier struct {
	mu      sync.Mutex
	records []pro.Package
	gets    int
}

func (v *scriptedVerifier) RefreshCloudDistributionPointInventoryV1(context.Context, string) error {
	return nil
}

func (v *scriptedVerifier) GetPackageV1(context.Context, string) (*pro.Package, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	i := v.gets
	if i >= len(v.records) {
		i = len(v.records) - 1
	}
	v.gets++
	rec := v.records[i]
	return &rec, nil
}

func record(status, size, sha3, md5 string) pro.Package {
	p := pro.Package{CloudTransferStatus: &status, Size: &size}
	if sha3 != "" {
		p.Sha3512 = &sha3
	}
	if md5 != "" {
		p.Md5 = &md5
	}
	return p
}

func TestUnverifiableTick(t *testing.T) {
	t.Parallel()

	const size = int64(2147483648)
	sizeText := strconv.FormatInt(size, 10)
	cases := []struct {
		name    string
		server  fileDigests
		status  string
		curSize string
		expect  int64
		want    bool
	}{
		{"ready at the right size with no hash", fileDigests{}, cloudStatusReady, sizeText, size, true},
		{"hash present", fileDigests{md5: "aa"}, cloudStatusReady, sizeText, size, false},
		{"stale hash present", fileDigests{sha3: "bb"}, cloudStatusReady, sizeText, size, false},
		{"still transferring", fileDigests{}, "PENDING", sizeText, size, false},
		{"size not caught up", fileDigests{}, cloudStatusReady, "1024", size, false},
		{"size empty", fileDigests{}, cloudStatusReady, "", size, false},
		{"zero byte upload keeps the existing path", fileDigests{}, cloudStatusReady, "0", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := unverifiableTick(tc.server, tc.status, tc.curSize, tc.expect); got != tc.want {
				t.Fatalf("unverifiableTick = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFileDigests_SameContent(t *testing.T) {
	t.Parallel()

	local := fileDigests{sha3: "aaaa", md5: "1111"}
	cases := []struct {
		name         string
		recorded     fileDigests
		recordedSize string
		localSize    int64
		want         bool
	}{
		{"matching digest", fileDigests{sha3: "AAAA"}, "10", 10, true},
		{"differing digest ignores equal size", fileDigests{sha3: "bbbb"}, "10", 10, false},
		{"no digest, identical size", fileDigests{}, "2147483648", 2147483648, true},
		{"no digest, different size", fileDigests{}, "1024", 2147483648, false},
		{"no digest, no size is a metadata-only record", fileDigests{}, "", 2147483648, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.recorded.sameContent(local, tc.recordedSize, tc.localSize); got != tc.want {
				t.Fatalf("sameContent = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewUnverifiedError_Wording(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		previous     fileDigests
		previousSize string
		want         string
	}{
		{"create", fileDigests{}, "", "computed no hash for it"},
		{"replace, previous hash cleared", fileDigests{md5: "aa"}, "314572800", "previous hash was cleared"},
		{"replace, identical size", fileDigests{}, "2147483648", "same size"},
		{"replace, identical size with a previous hash", fileDigests{md5: "aa"}, "2147483648", "same size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := newUnverifiedError("big.pkg", 2147483648, tc.previous, tc.previousSize)
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
			if !strings.Contains(err.Error(), "not integrity-verified") {
				t.Errorf("error %q must say the upload is not verified", err.Error())
			}
			if strings.ContainsRune(err.Error(), '—') {
				t.Errorf("error %q contains an em dash", err.Error())
			}
		})
	}
}

func TestPollPackageVerification_UnverifiedAfterGrace(t *testing.T) {
	t.Parallel()

	const size = int64(2147483648)
	sizeText := strconv.FormatInt(size, 10)
	local := fileDigests{sha3: "aaaa", sha256: "ssss", md5: "1111"}

	cases := []struct {
		name         string
		records      []pro.Package
		previous     fileDigests
		previousSize string
		want         string
	}{
		{
			name:    "create, never hashed",
			records: []pro.Package{record(cloudStatusReady, sizeText, "", "")},
			want:    "computed no hash for it",
		},
		{
			name: "replace of a smaller package, hash cleared once the size flips",
			records: []pro.Package{
				record(cloudStatusReady, "314572800", "", "9999"),
				record(cloudStatusReady, sizeText, "", ""),
			},
			previous:     fileDigests{md5: "9999"},
			previousSize: "314572800",
			want:         "previous hash was cleared",
		},
		{
			name:         "replace with a file of identical size",
			records:      []pro.Package{record(cloudStatusReady, sizeText, "", "")},
			previousSize: sizeText,
			want:         "same size",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := &scriptedVerifier{records: tc.records}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			start := time.Now()
			_, err := pollPackageVerification(ctx, v, 2*time.Millisecond, 25*time.Millisecond, "7", "big.pkg", local, size, tc.previous, tc.previousSize)
			var un *unverifiedError
			if !errors.As(err, &un) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want *unverifiedError containing %q", err, tc.want)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("took %v: the grace must end the wait, not the context deadline", elapsed)
			}
		})
	}
}

func TestPollPackageVerification_KeepsWaiting(t *testing.T) {
	t.Parallel()

	const size = int64(1000)
	local := fileDigests{sha3: "aaaa", md5: "1111"}

	cases := []struct {
		name     string
		records  []pro.Package
		previous fileDigests
		wantErr  error
	}{
		{
			name:    "still transferring is not unverified",
			records: []pro.Package{record("PENDING", "1000", "", "")},
			wantErr: errVerificationTimeout,
		},
		{
			name:    "size not caught up is not unverified",
			records: []pro.Package{record(cloudStatusReady, "500", "", "")},
			wantErr: errVerificationTimeout,
		},
		{
			name:     "a stale previous hash at the right size keeps waiting",
			records:  []pro.Package{record(cloudStatusReady, "1000", "", "9999")},
			previous: fileDigests{md5: "9999"},
			wantErr:  errVerificationTimeout,
		},
		{
			name:    "a hash that matches neither side is still corruption",
			records: []pro.Package{record(cloudStatusReady, "1000", "", "dead")},
			wantErr: errCorruption,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := &scriptedVerifier{records: tc.records}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()

			_, err := pollPackageVerification(ctx, v, 2*time.Millisecond, 20*time.Millisecond, "7", "x.pkg", local, size, tc.previous, "")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if _, ok := errors.AsType[*unverifiedError](err); ok {
				t.Fatalf("a poll that must keep waiting reported unverified: %v", err)
			}
		})
	}
}

func TestPollPackageVerification_HashArrivingInsideTheGraceConverges(t *testing.T) {
	t.Parallel()

	const size = int64(1000)
	local := fileDigests{sha3: "aaaa", md5: "1111"}
	v := &scriptedVerifier{records: []pro.Package{
		record(cloudStatusReady, "1000", "", ""),
		record(cloudStatusReady, "1000", "", ""),
		record(cloudStatusReady, "1000", "aaaa", ""),
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := pollPackageVerification(ctx, v, 2*time.Millisecond, 5*time.Second, "7", "x.pkg", local, size, fileDigests{}, "")
	if err != nil || got == nil {
		t.Fatalf("poll = (%v, %v), want convergence", got, err)
	}
}

func TestClassifyUploadPollError_UnverifiedIsAWarning(t *testing.T) {
	t.Parallel()

	diags := classifyUploadPollError(newUnverifiedError("big.pkg", 2147483648, fileDigests{}, ""), 1)
	if diags.HasError() {
		t.Fatalf("an unverified upload must not fail the apply: %v", diags)
	}
	if diags.WarningsCount() != 1 || diags[0].Summary() != "Package upload not verified" {
		t.Fatalf("want one \"Package upload not verified\" warning, got %v", diags)
	}
}
