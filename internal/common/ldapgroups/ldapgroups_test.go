// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldapgroups

import (
	"context"
	"errors"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// fakeSearcher returns a canned result (or error) for SearchLdapGroupsV1.
type fakeSearcher struct {
	results []pro.LdapGroup
	err     error
	gotQ    string
}

func (f *fakeSearcher) SearchLdapGroupsV1(_ context.Context, q string) (*pro.LdapGroupSearchResults, error) {
	f.gotQ = q
	if f.err != nil {
		return nil, f.err
	}
	return &pro.LdapGroupSearchResults{Results: f.results, TotalCount: len(f.results)}, nil
}

func sampleGroups() []pro.LdapGroup {
	// Mirrors the live wire shape: a contains-search for "Admin" returns
	// several groups, only one of which is an exact "Admins" match.
	return []pro.LdapGroup{
		{Name: "Admins", ID: "37158", LdapServerID: 31, DistinguishedName: "CN=Admins,OU=G,DC=x", UUID: "u1"},
		{Name: "Harbinger_Admins", ID: "37207", LdapServerID: 31, UUID: "u2"},
		{Name: "Admins", ID: "99", LdapServerID: 42, UUID: "u3"}, // same name, different server
	}
}

func TestResolve_ExactMatchScopedToServer(t *testing.T) {
	f := &fakeSearcher{results: sampleGroups()}
	g, err := Resolve(context.Background(), f, "Admins", 31)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.ID != "37158" {
		t.Errorf("ID = %q, want 37158", g.ID)
	}
	if g.LdapServerID != 31 {
		t.Errorf("LdapServerID = %d, want 31", g.LdapServerID)
	}
	if f.gotQ != "Admins" {
		t.Errorf("search query = %q, want Admins", f.gotQ)
	}
}

func TestResolve_ContainsMatchIsNotAccepted(t *testing.T) {
	// "Harbinger_Admins" contains "Admins" but is not an exact match.
	f := &fakeSearcher{results: []pro.LdapGroup{
		{Name: "Harbinger_Admins", ID: "37207", LdapServerID: 31},
	}}
	_, err := Resolve(context.Background(), f, "Admins", 31)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
}

func TestResolve_NotFoundOnServer(t *testing.T) {
	f := &fakeSearcher{results: sampleGroups()}
	_, err := Resolve(context.Background(), f, "Admins", 999)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound for unknown server, got %v", err)
	}
}

func TestResolve_Ambiguous(t *testing.T) {
	// Two exact matches on the same server.
	f := &fakeSearcher{results: []pro.LdapGroup{
		{Name: "Admins", ID: "1", LdapServerID: 31},
		{Name: "Admins", ID: "2", LdapServerID: 31},
	}}
	_, err := Resolve(context.Background(), f, "Admins", 31)
	if !errors.Is(err, ErrAmbiguousGroup) {
		t.Fatalf("expected ErrAmbiguousGroup, got %v", err)
	}
}

func TestResolve_EmptyName(t *testing.T) {
	f := &fakeSearcher{results: sampleGroups()}
	if _, err := Resolve(context.Background(), f, "", 31); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound for empty name, got %v", err)
	}
}

func TestResolve_SearchError(t *testing.T) {
	f := &fakeSearcher{err: errors.New("boom")}
	if _, err := Resolve(context.Background(), f, "Admins", 31); err == nil {
		t.Fatal("expected error to propagate from the searcher")
	}
}

func TestValidate(t *testing.T) {
	f := &fakeSearcher{results: sampleGroups()}
	if err := Validate(context.Background(), f, "Admins", 31, "37158"); err != nil {
		t.Errorf("consistent triple should validate, got %v", err)
	}
	err := Validate(context.Background(), f, "Admins", 31, "0000")
	if err == nil || errors.Is(err, ErrGroupNotFound) {
		t.Errorf("mismatched id should be a mismatch error, got %v", err)
	}
}

func TestResolveByName_ExactMatchesAcrossServers(t *testing.T) {
	// "Admins" exists on servers 31 and 42 — both valid for name-only
	// (server-agnostic) scope use; the contains-match "Harbinger_Admins" is not.
	f := &fakeSearcher{results: sampleGroups()}
	got, err := ResolveByName(context.Background(), f, "Admins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 exact matches across servers, got %d: %+v", len(got), got)
	}
	if f.gotQ != "Admins" {
		t.Errorf("search query = %q, want Admins", f.gotQ)
	}
}

func TestResolveByName_NoMatch(t *testing.T) {
	f := &fakeSearcher{results: sampleGroups()}
	got, err := ResolveByName(context.Background(), f, "Nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no matches, got %+v", got)
	}
}

func TestResolveByName_SearchError(t *testing.T) {
	f := &fakeSearcher{err: errors.New("boom")}
	if _, err := ResolveByName(context.Background(), f, "Admins"); err == nil {
		t.Fatal("expected error when search fails")
	}
}

func TestResolveByName_EmptyName(t *testing.T) {
	f := &fakeSearcher{results: sampleGroups()}
	got, err := ResolveByName(context.Background(), f, "")
	if err != nil || got != nil {
		t.Fatalf("empty name should return (nil, nil), got (%v, %v)", got, err)
	}
}
