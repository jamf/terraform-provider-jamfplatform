// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// newStubClient starts h behind an OAuth token endpoint and returns an SDK
// client pointed at it.
//
// The seam is the HTTP boundary because the resolution takes a concrete
// *proclassic.Client, and the stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package
// under the acceptance build tag and the provider registers this package —
// importing it from an in-package test makes that a cycle. Retries and the
// request interval are disabled so a deliberate 4xx/5xx is answered once rather
// than waited out.
func newStubClient(t *testing.T, h http.Handler) *jamfplatform.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return jamfplatform.NewClient(server.URL, "test-id", "test-secret",
		jamfplatform.WithRetryPolicy(0, 0, 0),
		jamfplatform.WithMinRequestInterval(0),
	)
}

// These tests cover the source_id resolution law, which is load-bearing in a way
// the rest of this package is not: the number it produces lands in source_id,
// which is Required and RequiresReplace, so a wrong number puts a
// destroy-and-recreate of a live patch title in the next plan. The arms are
// exercised over sourceIDFromCatalogues rather than through a client because the
// decision is pure — reading the catalogues is fetchPatchSourceCatalogues' job,
// tested separately below over a mock server.

// src builds a catalogue entry with both fields populated, the shape a healthy
// Jamf Pro catalogue reports.
func src(id int, name string) proclassic.IDName {
	return proclassic.IDName{ID: &id, Name: &name}
}

// srcNoID builds a catalogue entry carrying a name but no id. The classic
// payload makes both fields optional, so this is representable on the wire, and
// an entry like it must not be counted as a match — nothing downstream can use
// an absent id, and counting one would turn a unique match into an ambiguous
// refusal.
func srcNoID(name string) proclassic.IDName {
	return proclassic.IDName{Name: &name}
}

// srcNoName builds a catalogue entry carrying an id but no name — the other half
// of the same wire optionality.
func srcNoName(id int) proclassic.IDName {
	return proclassic.IDName{ID: &id}
}

// assertMessageNamesIDs fails unless every id appears as a standalone number in
// msg. Matching on word boundaries rather than the exact rendering keeps the
// assertion about the candidate ids being disclosed, not about how the list is
// formatted — a practitioner cannot disambiguate two sources without them.
func assertMessageNamesIDs(t *testing.T, msg string, ids ...int) {
	t.Helper()
	for _, id := range ids {
		re := regexp.MustCompile(`(^|[^0-9])` + strconv.Itoa(id) + `([^0-9]|$)`)
		if !re.MatchString(msg) {
			t.Errorf("error message does not name candidate id %d: %s", id, msg)
		}
	}
}

// TestSourceIDFromCatalogues pins every arm of the resolution law: exactly one
// match yields the id, and nothing else yields a number at all. The ambiguous
// arm is the mutation-critical one — deleting the single-match arm so an
// ambiguous name silently first-matches is invisible to every other test in this
// package, and produces a plausible-looking wrong id in a RequiresReplace
// attribute.
func TestSourceIDFromCatalogues(t *testing.T) {
	tests := []struct {
		name        string
		catalogues  providerdata.PatchSourceCatalogues
		query       string
		wantID      int64
		wantErr     bool
		errContains []string
		errNamesIDs []int
	}{
		{
			name: "internal catalogue only",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Jamf")},
				External: []proclassic.IDName{src(7, "Acme Titles")},
			},
			query:  "Jamf",
			wantID: 1,
		},
		{
			name: "external catalogue only",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Jamf")},
				External: []proclassic.IDName{src(7, "Acme Titles")},
			},
			query:  "Acme Titles",
			wantID: 7,
		},
		{
			name: "present in both catalogues is refused, naming both ids",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Shared Name")},
				External: []proclassic.IDName{src(7, "Shared Name")},
			},
			query:       "Shared Name",
			wantErr:     true,
			errContains: []string{"not unique", `"Shared Name"`},
			errNamesIDs: []int{1, 7},
		},
		{
			name: "duplicated within one catalogue is refused, naming both ids",
			catalogues: providerdata.PatchSourceCatalogues{
				External: []proclassic.IDName{src(4, "Acme Titles"), src(9, "Acme Titles")},
			},
			query:       "Acme Titles",
			wantErr:     true,
			errContains: []string{"not unique"},
			errNamesIDs: []int{4, 9},
		},
		{
			name: "present in neither catalogue names the source",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Jamf")},
				External: []proclassic.IDName{src(7, "Acme Titles")},
			},
			query:       "Renamed Since Create",
			wantErr:     true,
			errContains: []string{"no patch source named", `"Renamed Since Create"`},
		},
		{
			name: "empty catalogues yield the no-match error",
			// A tenant whose catalogues could not be populated, or a caller
			// handed the zero value: the answer is "no match", not a panic.
			catalogues:  providerdata.PatchSourceCatalogues{},
			query:       "Jamf",
			wantErr:     true,
			errContains: []string{"no patch source named"},
		},
		{
			name: "an empty name is refused as a missing name",
			// The v3 configuration carries no patch source name at all. That is
			// a property of the title, not of the client or the privileges, and
			// the message has to say so or it sends a practitioner after the
			// wrong thing.
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Jamf")},
			},
			query:       "",
			wantErr:     true,
			errContains: []string{"no patch source name"},
		},
		{
			name: "an entry with no id does not make a unique match ambiguous",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Jamf"), srcNoID("Jamf")},
				External: []proclassic.IDName{srcNoID("Jamf")},
			},
			query:  "Jamf",
			wantID: 1,
		},
		{
			name: "an entry with no name is not matched",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{srcNoName(1)},
			},
			query:       "Jamf",
			wantErr:     true,
			errContains: []string{"no patch source named"},
		},
		{
			name: "the match is case sensitive",
			catalogues: providerdata.PatchSourceCatalogues{
				Internal: []proclassic.IDName{src(1, "Jamf")},
			},
			query:       "jamf",
			wantErr:     true,
			errContains: []string{"no patch source named"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sourceIDFromCatalogues(tc.catalogues, tc.query)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got id %v", got)
				}
				if !got.IsNull() {
					t.Errorf("a refused resolution must yield a null id, got %v", got)
				}
				for _, want := range tc.errContains {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				assertMessageNamesIDs(t, err.Error(), tc.errNamesIDs...)
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.IsNull() || got.ValueInt64() != tc.wantID {
				t.Errorf("expected id %d, got %v", tc.wantID, got)
			}
		})
	}
}

// TestSourceIDFromCatalogues_EmptyNameDoesNotBlameTheClient pins the wording of
// the empty-name arm specifically. It is the one arm that fires without either
// catalogue being consulted, so a message mentioning the client or the
// catalogues would point a practitioner at privileges when the title itself is
// what reports no source.
func TestSourceIDFromCatalogues_EmptyNameDoesNotBlameTheClient(t *testing.T) {
	_, err := sourceIDFromCatalogues(providerdata.PatchSourceCatalogues{}, "")
	if err == nil {
		t.Fatal("expected an error for an empty patch source name")
	}
	for _, forbidden := range []string{"client", "catalogue"} {
		if strings.Contains(strings.ToLower(err.Error()), forbidden) {
			t.Errorf("the empty-name error must not blame %q: %s", forbidden, err)
		}
	}
}

// patchSourceServer serves the two classic catalogue endpoints, recording which
// were asked for so a test can assert the second read is not issued after the
// first fails. The classic surface answers XML — the SDK routes the decode off
// the /proclassic/ path, not off the response content type — so these bodies are
// the shapes ListPatchInternalSources / ListPatchExternalSources actually parse.
type patchSourceServer struct {
	internalReads int
	externalReads int
	internalCode  int
	externalCode  int
}

func (s *patchSourceServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	switch r.URL.Path {
	case "/proclassic/patchinternalsources":
		s.internalReads++
		if s.internalCode != 0 {
			w.WriteHeader(s.internalCode)
			return
		}
		_, _ = w.Write([]byte(`<patch_internal_sources><size>1</size>` +
			`<patch_internal_source><id>1</id><name>Jamf</name></patch_internal_source>` +
			`</patch_internal_sources>`))
	case "/proclassic/patchexternalsources":
		s.externalReads++
		if s.externalCode != 0 {
			w.WriteHeader(s.externalCode)
			return
		}
		_, _ = w.Write([]byte(`<patch_external_sources><size>2</size>` +
			`<patch_external_source><id>7</id><name>Acme Titles</name></patch_external_source>` +
			`<patch_external_source><id>8</id><name>Widget Titles</name></patch_external_source>` +
			`</patch_external_sources>`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// newPatchSourceClient wires a proclassic client onto the recording handler.
func newPatchSourceClient(t *testing.T, h *patchSourceServer) *proclassic.Client {
	t.Helper()
	return proclassic.New(newStubClient(t, h))
}

// TestFetchPatchSourceCatalogues_BothCatalogues pins that the snapshot carries
// both catalogues as read, and that both are always read — a name present in
// both is ambiguous, and only reading both can establish that, so stopping at
// the first hit would silently turn the ambiguous arm above into a first-match.
func TestFetchPatchSourceCatalogues_BothCatalogues(t *testing.T) {
	h := &patchSourceServer{}
	got, err := fetchPatchSourceCatalogues(context.Background(), newPatchSourceClient(t, h))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Internal) != 1 || got.Internal[0].ID == nil || *got.Internal[0].ID != 1 {
		t.Errorf("internal catalogue: expected one entry with id 1, got %+v", got.Internal)
	}
	if len(got.External) != 2 || got.External[1].Name == nil || *got.External[1].Name != "Widget Titles" {
		t.Errorf("external catalogue: expected two entries ending in Widget Titles, got %+v", got.External)
	}
	if h.internalReads != 1 || h.externalReads != 1 {
		t.Errorf("reads: internal=%d external=%d, want one of each", h.internalReads, h.externalReads)
	}

	// The snapshot is what the resolution decides over, so resolve through it
	// once to prove the two are wired to each other.
	id, err := sourceIDFromCatalogues(got, "Acme Titles")
	if err != nil {
		t.Fatalf("resolving over the fetched snapshot: %v", err)
	}
	if id.ValueInt64() != 7 {
		t.Errorf("expected id 7 from the fetched snapshot, got %v", id)
	}
}

// TestFetchPatchSourceCatalogues_InternalReadFails pins that a failed internal
// read is reported, names which catalogue failed, and short-circuits — the
// caller cannot decide anything from half a snapshot, so the second request is
// not worth issuing.
func TestFetchPatchSourceCatalogues_InternalReadFails(t *testing.T) {
	h := &patchSourceServer{internalCode: http.StatusForbidden}
	_, err := fetchPatchSourceCatalogues(context.Background(), newPatchSourceClient(t, h))
	if err == nil {
		t.Fatal("expected an error when the internal catalogue cannot be read")
	}
	if !strings.Contains(err.Error(), "listing internal patch sources") {
		t.Errorf("error does not name the internal catalogue read: %v", err)
	}
	if h.externalReads != 0 {
		t.Errorf("expected no external read after the internal one failed, got %d", h.externalReads)
	}
}

// TestFetchPatchSourceCatalogues_ExternalReadFails pins the other half: a
// tenant whose internal catalogue reads cleanly still fails, naming the external
// read. Nothing is asserted about the returned snapshot — it is partially filled
// here and every caller discards it on error, which is what keeps a half-read
// pair from resolving a name present in both catalogues to the internal id.
func TestFetchPatchSourceCatalogues_ExternalReadFails(t *testing.T) {
	h := &patchSourceServer{externalCode: http.StatusInternalServerError}
	_, err := fetchPatchSourceCatalogues(context.Background(), newPatchSourceClient(t, h))
	if err == nil {
		t.Fatal("expected an error when the external catalogue cannot be read")
	}
	if !strings.Contains(err.Error(), "listing external patch sources") {
		t.Errorf("error does not name the external catalogue read: %v", err)
	}
	if h.internalReads != 1 || h.externalReads != 1 {
		t.Errorf("reads: internal=%d external=%d, want one of each — the external catalogue must be read even when the internal one already matched", h.internalReads, h.externalReads)
	}
}

// TestFetchPatchSourceCatalogues_NilClient pins the unconfigured-client guard.
// Configure can legitimately leave a construct without a client (the framework
// calls it with nil ProviderData during early lifecycle), and resolution must
// report that rather than panic mid-plan.
func TestFetchPatchSourceCatalogues_NilClient(t *testing.T) {
	_, err := fetchPatchSourceCatalogues(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error for a nil client")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error does not say the client is not configured: %v", err)
	}
}

// TestResolveSourceID_ReadsThenDecides pins the uncached path the resource's
// import uses: it fetches both catalogues from the tenant and hands them to the
// same decision every other caller makes.
func TestResolveSourceID_ReadsThenDecides(t *testing.T) {
	h := &patchSourceServer{}
	got, err := resolveSourceID(context.Background(), newPatchSourceClient(t, h), "Jamf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ValueInt64() != 1 {
		t.Errorf("expected id 1, got %v", got)
	}
}

// TestResolveSourceID_WrapsFetchFailureWithTheName pins that an unreadable
// catalogue on the import path says which name was being resolved. Import is the
// one caller for which this failure is fatal, so the diagnostic has to identify
// the title's source rather than only the HTTP error.
func TestResolveSourceID_WrapsFetchFailureWithTheName(t *testing.T) {
	h := &patchSourceServer{internalCode: http.StatusForbidden}
	got, err := resolveSourceID(context.Background(), newPatchSourceClient(t, h), "Jamf")
	if err == nil {
		t.Fatal("expected an error when the catalogues cannot be read")
	}
	if !got.IsNull() {
		t.Errorf("a failed resolution must yield a null id, got %v", got)
	}
	for _, want := range []string{"patch source catalogues", `"Jamf"`, "listing internal patch sources"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
