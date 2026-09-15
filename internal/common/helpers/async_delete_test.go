// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package helpers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

func notFoundErr() error { return &jamfplatform.APIResponseError{StatusCode: http.StatusNotFound} }

// misleadingErr mimics the classic apps' accepted-async delete: a non-not-found
// status (HTTP 400 without an INVALID_ID detail) fronting a delete that the
// server has actually accepted.
func misleadingErr() error { return &jamfplatform.APIResponseError{StatusCode: http.StatusBadRequest} }

func TestConfirmAsyncDelete_CleanDeleteSkipsPoll(t *testing.T) {
	gets := 0
	err := ConfirmAsyncDelete(context.Background(), time.Millisecond,
		func(context.Context) error { return nil },
		func(context.Context) error { gets++; return nil },
	)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if gets != 0 {
		t.Errorf("clean delete must not poll GET; got %d GETs", gets)
	}
}

func TestConfirmAsyncDelete_AlreadyAbsentSkipsPoll(t *testing.T) {
	gets := 0
	err := ConfirmAsyncDelete(context.Background(), time.Millisecond,
		func(context.Context) error { return notFoundErr() },
		func(context.Context) error { gets++; return nil },
	)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if gets != 0 {
		t.Errorf("already-absent delete must not poll GET; got %d GETs", gets)
	}
}

func TestConfirmAsyncDelete_MisleadingThenConfirmed(t *testing.T) {
	dels, gets := 0, 0
	err := ConfirmAsyncDelete(context.Background(), time.Millisecond,
		func(context.Context) error { dels++; return misleadingErr() },
		func(context.Context) error {
			gets++
			if gets >= 3 { // present twice, then gone
				return notFoundErr()
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected nil after async delete confirmed, got %v", err)
	}
	if dels != 1 {
		t.Errorf("DELETE must be issued exactly once (never re-issued); got %d", dels)
	}
	if gets != 3 {
		t.Errorf("expected 3 GETs (present, present, gone), got %d", gets)
	}
}

func TestConfirmAsyncDelete_StillPresentAtTimeoutErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	dels := 0
	err := ConfirmAsyncDelete(ctx, time.Millisecond,
		func(context.Context) error { dels++; return misleadingErr() },
		func(context.Context) error { return nil }, // never not-found
	)
	if err == nil {
		t.Fatal("expected an error when the resource is still present at timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected wrapped context.DeadlineExceeded, got %v", err)
	}
	if dels != 1 {
		t.Errorf("DELETE must be issued exactly once even while polling; got %d", dels)
	}
}
