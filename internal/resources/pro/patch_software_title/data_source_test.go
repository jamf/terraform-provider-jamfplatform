// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// configurationsListPath is the single unpaginated v3 configurations list the
// data source's name lookup reads. It returns whole configuration objects rather
// than stubs, which is why a name match needs no follow-up get.
const configurationsListPath = "/pro/v3/patch-software-title-configurations"

// newListClient serves body verbatim from the configurations list endpoint and
// returns a Pro client pointed at it, over the local stub server
// patch_sources_test.go builds (see newStubClient for why it is local). status 0
// means 200.
func newListClient(t *testing.T, status int, body string) *pro.Client {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != configurationsListPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(body))
	})
	return pro.New(newStubClient(t, handler))
}

// TestLookupByName_SingleMatch pins the happy path: the matched element is the
// answer, carried back whole.
func TestLookupByName_SingleMatch(t *testing.T) {
	c := newListClient(t, 0, `[
	  {"id":"147","displayName":"8x8 Work","softwareTitleNameId":"285","patchSourceName":"Jamf"},
	  {"id":"148","displayName":"Firefox","softwareTitleNameId":"73","patchSourceName":"Jamf"}
	]`)

	got, err := lookupByName(context.Background(), c, "Firefox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != "148" || got.SoftwareTitleNameID != "73" {
		t.Errorf("expected the Firefox configuration (id 148), got %+v", got)
	}
}

// TestLookupByName_NoMatch pins that an absent name is reported and names the
// name asked for, since the caller supplied it and nothing else identifies the
// lookup that failed.
func TestLookupByName_NoMatch(t *testing.T) {
	c := newListClient(t, 0, `[{"id":"147","displayName":"8x8 Work"}]`)

	got, err := lookupByName(context.Background(), c, "Firefox")
	if err == nil {
		t.Fatalf("expected an error, got %+v", got)
	}
	if !strings.Contains(err.Error(), `"Firefox"`) {
		t.Errorf("error does not name the display name looked up: %v", err)
	}
	if got != nil {
		t.Errorf("a failed lookup must return no configuration, got %+v", got)
	}
}

// TestLookupByName_AmbiguousDisplayName is the mutation-critical case. name is a
// freeform Required field the practitioner sets at Create, so two titles can
// genuinely share a display name; the lookup must refuse and disclose both ids
// rather than silently return whichever came back last, which would read a
// different title than the configuration names.
func TestLookupByName_AmbiguousDisplayName(t *testing.T) {
	c := newListClient(t, 0, `[
	  {"id":"147","displayName":"8x8 Work","softwareTitleNameId":"285"},
	  {"id":"902","displayName":"8x8 Work","softwareTitleNameId":"285"}
	]`)

	got, err := lookupByName(context.Background(), c, "8x8 Work")
	if err == nil {
		t.Fatalf("expected an ambiguous-match error, got %+v", got)
	}
	if got != nil {
		t.Errorf("an ambiguous lookup must return no configuration, got %+v", got)
	}

	ambiguous, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err)
	if !ok {
		t.Fatalf("expected *jamfplatform.AmbiguousMatchError so the data source can render its own diagnostic, got %T: %v", err, err)
	}
	if ambiguous.Name != "8x8 Work" {
		t.Errorf("expected the error to carry the queried name, got %q", ambiguous.Name)
	}
	if len(ambiguous.Matches) != 2 || ambiguous.Matches[0] != "147" || ambiguous.Matches[1] != "902" {
		t.Errorf("expected both colliding ids in wire order, got %v", ambiguous.Matches)
	}
	for _, want := range []string{"147", "902"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("rendered error does not name candidate id %s: %v", want, err)
		}
	}
}

// TestLookupByName_IDLessEntryIsNotAMatch pins the guard the v3 rewrite dropped
// and the review restored: an entry whose id is empty is not a match. Nothing
// downstream can read a title by an empty id, so counting one would move the
// failure to the version read and have it blame the versions rather than the
// malformed entry.
func TestLookupByName_IDLessEntryIsNotAMatch(t *testing.T) {
	c := newListClient(t, 0, `[{"id":"","displayName":"8x8 Work","softwareTitleNameId":"285"}]`)

	got, err := lookupByName(context.Background(), c, "8x8 Work")
	if err == nil {
		t.Fatalf("expected an error, got %+v", got)
	}
	if _, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err); ok {
		t.Errorf("an id-less entry is no match at all, not an ambiguity: %v", err)
	}
	if !strings.Contains(err.Error(), "no patch software title named") {
		t.Errorf("expected the no-match error, got %v", err)
	}
}

// TestLookupByName_IDLessEntryDoesNotMakeAMatchAmbiguous is the same guard from
// the other side: an id-less entry sharing the name with one real match must
// leave that match unique, not turn the lookup into a refusal.
func TestLookupByName_IDLessEntryDoesNotMakeAMatchAmbiguous(t *testing.T) {
	c := newListClient(t, 0, `[
	  {"id":"","displayName":"8x8 Work"},
	  {"id":"147","displayName":"8x8 Work","softwareTitleNameId":"285"}
	]`)

	got, err := lookupByName(context.Background(), c, "8x8 Work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != "147" {
		t.Errorf("expected the one readable configuration (id 147), got %+v", got)
	}
}

// TestLookupByName_ListFailureIsReported pins that a failed list says it was
// resolving a name, so the diagnostic distinguishes "the lookup could not run"
// from "the name matched nothing".
func TestLookupByName_ListFailureIsReported(t *testing.T) {
	c := newListClient(t, http.StatusForbidden, "")

	_, err := lookupByName(context.Background(), c, "8x8 Work")
	if err == nil {
		t.Fatal("expected an error when the configurations list cannot be read")
	}
	for _, want := range []string{"list patch software titles", `"8x8 Work"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

// TestLookupByName_EmptyListIsANoMatch pins that a tenant with no patch titles
// at all is a legitimate no-match rather than a special case: the configurations
// list returns a plain slice, in which nil and empty are indistinguishable.
func TestLookupByName_EmptyListIsANoMatch(t *testing.T) {
	c := newListClient(t, 0, `[]`)

	if _, err := lookupByName(context.Background(), c, "8x8 Work"); err == nil {
		t.Fatal("expected a no-match error against an empty configurations list")
	}
}
