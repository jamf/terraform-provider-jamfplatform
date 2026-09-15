// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// stubTitleCatalog serves a fixed catalog snapshot, counting the reads it is
// asked for. The snapshot is the WHOLE catalog, which is what the resolver now
// sees: there is no server-side filter left to narrow it, so every candidate
// comes back and the resolver must do all the narrowing itself.
type stubTitleCatalog struct {
	titles []pro.AppTitle
	err    error
	calls  int
}

func (s *stubTitleCatalog) Titles(ctx context.Context) ([]pro.AppTitle, error) {
	s.calls++
	return s.titles, s.err
}

func TestResolveTitleIDByName_ExactMatch(t *testing.T) {
	l := &stubTitleCatalog{titles: []pro.AppTitle{{ID: "Composer", TitleName: "Jamf Composer"}}}
	id, err := resolveTitleIDByName(context.Background(), l, "Jamf Composer")
	if err != nil {
		t.Fatalf("expected the exact name to resolve, got %v", err)
	}
	if id != "Composer" {
		t.Errorf("id = %q, want Composer", id)
	}
}

// Jamf Pro's titleName filter matches case-insensitively, which is why the
// resolver reads the whole catalog and decides the match itself. Accepting an
// off-casing name would store the user's spelling and then have Read rewrite it to
// the canonical name — a perpetual diff. The resolver must reject it instead.
func TestResolveTitleIDByName_RejectsCaseMismatch(t *testing.T) {
	l := &stubTitleCatalog{titles: []pro.AppTitle{{ID: "Composer", TitleName: "Jamf Composer"}}}
	if _, err := resolveTitleIDByName(context.Background(), l, "jamf composer"); !errors.Is(err, errTitleNotInCatalog) {
		t.Fatalf("expected errTitleNotInCatalog for an off-casing name, got %v", err)
	}
}

// A partial name must not resolve either. Over the full catalog there is no glob
// to over-match, but the exact check is what makes a prefix — or the `Jamf*` a
// user might carry over from Jamf Pro's own filter syntax — fail cleanly instead
// of picking one of the titles it would have matched.
func TestResolveTitleIDByName_RejectsPartialName(t *testing.T) {
	l := &stubTitleCatalog{titles: []pro.AppTitle{
		{ID: "7F0", TitleName: "Jamf Sync"},
		{ID: "Composer", TitleName: "Jamf Composer"},
	}}
	for _, name := range []string{"Jamf", "Jamf*", "Jamf Comp"} {
		if _, err := resolveTitleIDByName(context.Background(), l, name); !errors.Is(err, errTitleNotInCatalog) {
			t.Errorf("expected errTitleNotInCatalog for partial name %q, got %v", name, err)
		}
	}
}

func TestResolveTitleIDByName_EmptyCatalogIsNotFound(t *testing.T) {
	l := &stubTitleCatalog{titles: []pro.AppTitle{}}
	if _, err := resolveTitleIDByName(context.Background(), l, "No Such App"); !errors.Is(err, errTitleNotInCatalog) {
		t.Fatalf("expected errTitleNotInCatalog, got %v", err)
	}
}

// Two titles sharing one exact name must be an ambiguity error, not an
// arbitrary pick — the chosen ID would flip between plans.
func TestResolveTitleIDByName_AmbiguousIsError(t *testing.T) {
	l := &stubTitleCatalog{titles: []pro.AppTitle{
		{ID: "A", TitleName: "Duplicated"},
		{ID: "B", TitleName: "Duplicated"},
	}}
	_, err := resolveTitleIDByName(context.Background(), l, "Duplicated")
	if err == nil {
		t.Fatal("expected an error for two titles sharing a name")
	}
	if errors.Is(err, errTitleNotInCatalog) {
		t.Errorf("ambiguity must not be reported as not-found: %v", err)
	}
	if !strings.Contains(err.Error(), "A") || !strings.Contains(err.Error(), "B") {
		t.Errorf("the ambiguity error must name the candidate IDs, got %q", err)
	}
}

func TestResolveTitleIDByName_TransportErrorPropagates(t *testing.T) {
	want := errors.New("connection refused")
	l := &stubTitleCatalog{err: want}
	if _, err := resolveTitleIDByName(context.Background(), l, "Jamf Composer"); !errors.Is(err, want) {
		t.Fatalf("expected the transport error to propagate, got %v", err)
	}
}

func TestResolveTitleIDByName_NoLookupForEmptyNameOrNilCatalog(t *testing.T) {
	l := &stubTitleCatalog{titles: []pro.AppTitle{{ID: "Composer", TitleName: "Jamf Composer"}}}
	if _, err := resolveTitleIDByName(context.Background(), l, ""); !errors.Is(err, errTitleNotInCatalog) {
		t.Errorf("empty name must be not-found, got %v", err)
	}
	if l.calls != 0 {
		t.Errorf("empty name must not reach the catalog, got %d reads", l.calls)
	}
	if _, err := resolveTitleIDByName(context.Background(), nil, "Jamf Composer"); err == nil {
		t.Error("nil catalog must error")
	}
}

// The finding this cache answers is a 3x request multiplication on a no-op plan:
// every App Installer instance resolved its own title name and reverse-resolved
// its own title id, so a 50-resource workspace paid 100 catalog requests on top of
// its 50 deployment reads. One provider-instance snapshot serves the lot, in both
// directions, however many lookups run against it.
func TestAppTitleCatalog_ReadOncePerProviderInstance(t *testing.T) {
	catalog := &stubTitleCatalog{titles: []pro.AppTitle{
		{ID: "Composer", TitleName: "Jamf Composer"},
		{ID: "7F0", TitleName: "Jamf Sync"},
	}}
	cache := providerdata.ConfigureAppTitleCatalog(
		providerdata.New(jamfplatform.NewClient("http://127.0.0.1:1", "test-id", "test-secret")),
		func(ctx context.Context, _ *pro.Client) ([]pro.AppTitle, error) { return catalog.Titles(ctx) },
	)

	for i := range 50 {
		id, err := resolveTitleIDByName(context.Background(), cache, "Jamf Composer")
		if err != nil || id != "Composer" {
			t.Fatalf("lookup %d: id = %q, err = %v", i, id, err)
		}
		name, ok := titleNameForID(context.Background(), cache, "7F0")
		if !ok || name != "Jamf Sync" {
			t.Fatalf("reverse lookup %d: name = %q ok = %v", i, name, ok)
		}
	}

	if catalog.calls != 1 {
		t.Errorf("expected exactly 1 catalog read for 100 lookups, got %d", catalog.calls)
	}
}

// stubDeploymentLister mirrors stubTitleLister for the deployment resolver.
type stubDeploymentLister struct {
	deployments []pro.AppTitleDeploymentSummary
	err         error
	filter      string
}

func (s *stubDeploymentLister) ListAppInstallerDeploymentsV1(ctx context.Context, sort []string, filter string) ([]pro.AppTitleDeploymentSummary, error) {
	s.filter = filter
	return s.deployments, s.err
}

func TestResolveDeploymentIDByName_ExactMatch(t *testing.T) {
	l := &stubDeploymentLister{deployments: []pro.AppTitleDeploymentSummary{{ID: "9", Name: "tf-acc-composer"}}}
	id, err := resolveDeploymentIDByName(context.Background(), l, "tf-acc-composer")
	if err != nil {
		t.Fatalf("expected the exact name to resolve, got %v", err)
	}
	if id != "9" {
		t.Errorf("id = %q, want 9", id)
	}
	if l.filter != `name=="tf-acc-composer"` {
		t.Errorf("filter = %q", l.filter)
	}
}

// Wire-verified: `name=="TF-PROBE-APPINST-SC"` returns the deployment named
// `tf-probe-appinst-sc`, so the resolver cannot take the server's word for it.
func TestResolveDeploymentIDByName_RejectsCaseMismatch(t *testing.T) {
	l := &stubDeploymentLister{deployments: []pro.AppTitleDeploymentSummary{{ID: "9", Name: "tf-acc-composer"}}}
	if _, err := resolveDeploymentIDByName(context.Background(), l, "TF-ACC-COMPOSER"); !errors.Is(err, errDeploymentNotFound) {
		t.Fatalf("expected errDeploymentNotFound for an off-casing name, got %v", err)
	}
}

func TestResolveDeploymentIDByName_AmbiguousIsError(t *testing.T) {
	l := &stubDeploymentLister{deployments: []pro.AppTitleDeploymentSummary{
		{ID: "9", Name: "dup"},
		{ID: "10", Name: "dup"},
	}}
	_, err := resolveDeploymentIDByName(context.Background(), l, "dup")
	if err == nil {
		t.Fatal("expected an error for two deployments sharing a name")
	}
	if errors.Is(err, errDeploymentNotFound) {
		t.Errorf("ambiguity must not be reported as not-found: %v", err)
	}
}

func TestEscapeRSQLString(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`Jamf Composer`, `Jamf Composer`},
		{`say "hi"`, `say \"hi\"`},
		{`back\slash`, `back\\slash`},
		{`both\ and "`, `both\\ and \"`},
		{`Jamf*`, `Jamf*`}, // an asterisk is left alone: the exact check narrows it
	} {
		if got := escapeRSQLString(tc.in); got != tc.want {
			t.Errorf("escapeRSQLString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
