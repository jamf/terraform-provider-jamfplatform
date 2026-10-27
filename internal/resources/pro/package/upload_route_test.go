// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

type routeRecorder struct {
	apiUploads int
	initiates  int
	apiErr     error
}

func (r *routeRecorder) UploadPackageV1(_ context.Context, _, _ string, _ io.Reader) (*pro.HrefResponse, error) {
	r.apiUploads++
	return &pro.HrefResponse{}, r.apiErr
}

func (r *routeRecorder) InitiateJCDSUploadV1(context.Context) (*pro.Credentials, error) {
	r.initiates++
	return nil, errors.New("stop after initiate")
}

func (r *routeRecorder) RenewJCDSCredentialsV1(context.Context) (*pro.Credentials, error) {
	return nil, errors.New("not expected")
}

func TestRouteForCdnTypeCoversEveryType(t *testing.T) {
	t.Parallel()

	for _, cdn := range pro.CloudDistributionPointCdnTypeValues() {
		want := uploadViaAPI
		if cdn == pro.CloudDistributionPointCdnTypeJamfCloud {
			want = uploadViaJCDS
		}
		if got := routeForCdnType(cdn); got != want {
			t.Errorf("routeForCdnType(%q) = %v, want %v", cdn, got, want)
		}
	}
	if got := routeForCdnType(""); got != uploadViaAPI {
		t.Errorf("routeForCdnType(\"\") = %v, want the API route", got)
	}
}

func TestUploadBinaryDispatchesByRoute(t *testing.T) {
	t.Parallel()

	t.Run("api route calls UploadPackageV1 only", func(t *testing.T) {
		t.Parallel()
		rec := &routeRecorder{}
		if err := uploadBinary(context.Background(), rec, uploadViaAPI, "7", "a.pkg", strings.NewReader("x")); err != nil {
			t.Fatalf("uploadBinary: %v", err)
		}
		if rec.apiUploads != 1 || rec.initiates != 0 {
			t.Fatalf("apiUploads=%d initiates=%d, want 1 and 0", rec.apiUploads, rec.initiates)
		}
	})

	t.Run("api route returns the upload error", func(t *testing.T) {
		t.Parallel()
		rec := &routeRecorder{apiErr: errors.New("boom")}
		if err := uploadBinary(context.Background(), rec, uploadViaAPI, "7", "a.pkg", strings.NewReader("x")); err == nil {
			t.Fatal("want the upload error, got nil")
		}
	})

	t.Run("jcds route initiates and never calls UploadPackageV1", func(t *testing.T) {
		t.Parallel()
		rec := &routeRecorder{}
		err := uploadBinary(context.Background(), rec, uploadViaJCDS, "7", "a.pkg", strings.NewReader("x"))
		if err == nil || !strings.Contains(err.Error(), "initiate") {
			t.Fatalf("want an initiate error, got %v", err)
		}
		if rec.apiUploads != 0 || rec.initiates != 1 {
			t.Fatalf("apiUploads=%d initiates=%d, want 0 and 1", rec.apiUploads, rec.initiates)
		}
	})
}
