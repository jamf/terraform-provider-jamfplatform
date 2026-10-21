// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

func shortReadBack(t *testing.T) {
	t.Helper()
	interval, bound := readBackInterval, readBackBound
	readBackInterval, readBackBound = time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { readBackInterval, readBackBound = interval, bound })
}

// TestReadBackAfterWrite_RetriesAStaleRead covers the Jamf Pro 11.32.0 shape:
// the first read after a write is one write behind, the second is current.
func TestReadBackAfterWrite_RetriesAStaleRead(t *testing.T) {
	shortReadBack(t)
	reads := []string{"before", "after"}
	n := 0
	got, err := ReadBackAfterWrite(context.Background(), func(context.Context) (string, error) {
		v := reads[min(n, len(reads)-1)]
		n++
		return v, nil
	}, func(v string) bool { return v == "after" })
	if err != nil || got != "after" || n != 2 {
		t.Fatalf("got %q, err %v after %d reads; want \"after\" after 2", got, err, n)
	}
}

// TestReadBackAfterWrite_FreshReadCostsNothing pins that a tenant reading fresh
// makes exactly one request.
func TestReadBackAfterWrite_FreshReadCostsNothing(t *testing.T) {
	shortReadBack(t)
	n := 0
	_, _ = ReadBackAfterWrite(context.Background(), func(context.Context) (string, error) { n++; return "after", nil }, func(v string) bool { return v == "after" })
	if n != 1 {
		t.Fatalf("a settled first read must not be repeated, got %d reads", n)
	}
}

// TestReadBackAfterWrite_ReturnsTheLastReadAtTheBound pins that a read which
// never settles is handed back, not turned into an error.
func TestReadBackAfterWrite_ReturnsTheLastReadAtTheBound(t *testing.T) {
	shortReadBack(t)
	got, err := ReadBackAfterWrite(context.Background(), func(context.Context) (string, error) { return "server value", nil }, func(string) bool { return false })
	if err != nil || got != "server value" {
		t.Fatalf("got %q, err %v; want the last read and no error", got, err)
	}
}

func TestReadBackAfterWrite_StopsOnAReadError(t *testing.T) {
	shortReadBack(t)
	n := 0
	boom := errors.New("boom")
	_, err := ReadBackAfterWrite(context.Background(), func(context.Context) (string, error) { n++; return "", boom }, func(string) bool { return false })
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("err %v after %d reads; want boom after 1", err, n)
	}
}

func TestPlannedValueReflected(t *testing.T) {
	if !PlannedValueReflected(types.StringUnknown(), types.StringValue("x")) {
		t.Error("an unknown planned value must count as reflected")
	}
	if !PlannedValueReflected(types.StringValue("x"), types.StringValue("x")) {
		t.Error("equal values must count as reflected")
	}
	if PlannedValueReflected(types.StringValue("x"), types.StringValue("y")) {
		t.Error("different values must not count as reflected")
	}
}

func TestWaitUntilGone_StopsAtNotFound(t *testing.T) {
	shortReadBack(t)
	n := 0
	WaitUntilGone(context.Background(), func(context.Context) error {
		n++
		if n < 2 {
			return nil
		}
		return &jamfplatform.APIResponseError{StatusCode: 404, Body: `{"httpStatus":404,"errors":[]}`}
	})
	if n != 2 {
		t.Fatalf("want a stale read then not-found, got %d reads", n)
	}
}

func TestWaitUntilGone_GivesUpAtTheBound(t *testing.T) {
	shortReadBack(t)
	WaitUntilGone(context.Background(), func(context.Context) error { return nil })
}
