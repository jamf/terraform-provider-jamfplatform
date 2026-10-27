// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jcdsupload"
)

type routeRecorder struct {
	mu         sync.Mutex
	apiUploads int
	initiates  int
	apiErr     error
	events     []string
	puts       []*pro.Package
	getFn      func(call int) pro.Package
	gets       int
}

func (r *routeRecorder) log(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *routeRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func (r *routeRecorder) count(prefix string) int {
	n := 0
	for _, e := range r.snapshot() {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func (r *routeRecorder) UploadPackageV1(_ context.Context, id, fileName string, file io.Reader) (*pro.HrefResponse, error) {
	_, _ = io.Copy(io.Discard, file)
	r.mu.Lock()
	r.apiUploads++
	r.mu.Unlock()
	r.log("upload-api:" + id + ":" + fileName)
	return &pro.HrefResponse{}, r.apiErr
}

func (r *routeRecorder) directUpload(_ context.Context, _ jcdsupload.API, fileName string, body io.Reader) error {
	_, _ = io.Copy(io.Discard, body)
	r.log("upload-jcds:" + fileName)
	return nil
}

func (r *routeRecorder) GetPackageV1(context.Context, string) (*pro.Package, error) {
	r.mu.Lock()
	r.gets++
	call := r.gets
	r.mu.Unlock()
	r.log("get")
	if r.getFn == nil {
		return &pro.Package{}, nil
	}
	rec := r.getFn(call)
	return &rec, nil
}

func (r *routeRecorder) RefreshCloudDistributionPointInventoryV1(context.Context, string) error {
	return nil
}

func (r *routeRecorder) UpdatePackageV1(_ context.Context, _ string, request *pro.Package) (*pro.Package, error) {
	r.mu.Lock()
	r.puts = append(r.puts, request)
	r.mu.Unlock()
	r.log("put:" + request.FileName)
	return request, nil
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
		if got := rec.snapshot(); len(got) != 1 || got[0] != "upload-api:7:a.pkg" {
			t.Fatalf("events = %v, want the API upload of id 7 and a.pkg", got)
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
