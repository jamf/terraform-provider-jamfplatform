// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// The sweep must read whole policies, never subsets. The Classic API answers
// GET .../subset/PackageConfiguration with 200 and an empty body even when the
// policy installs packages, so a subset-based sweep would index no package
// references at all and every dependency alert for a package would report
// "no policy uses this" — a confident, wrong denial.
//
// This is a property of which endpoint the code calls, not of any tenant's data,
// so it is guarded here rather than against a live estate: an acceptance test can
// only observe "no package references", which on a tenant whose policies install
// nothing is simply the truth.

// policyServer serves the Classic policy surface with Jamf Pro's own asymmetry:
// the full read carries the package, the subset read returns 200 with an empty
// body. It records which shape was asked for.
type policyServer struct {
	fullReads   int
	subsetReads int
}

func (s *policyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/auth/token" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	switch {
	case strings.HasSuffix(r.URL.Path, "/policies"):
		_, _ = w.Write([]byte(`<policies><size>1</size>` +
			`<policy><id>17</id><name>Install Widget</name></policy></policies>`))
	case strings.Contains(r.URL.Path, "/subset/"):
		s.subsetReads++
		_, _ = w.Write([]byte(`<policy/>`))
	case strings.Contains(r.URL.Path, "/policies/id/"):
		s.fullReads++
		_, _ = w.Write([]byte(`<policy>` +
			`<general><id>17</id><name>Install Widget</name><enabled>true</enabled></general>` +
			`<scope><all_computers>true</all_computers></scope>` +
			`<package_configuration><packages><size>1</size>` +
			`<package><id>42</id><name>Widget.pkg</name><action>Install</action></package>` +
			`</packages></package_configuration>` +
			`<scripts><size>1</size><script><id>9</id><name>widget.sh</name></script></scripts>` +
			`</policy>`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// TestDependencyIndex_ReadsWholePoliciesNotSubsets is the regression guard for
// the reason the sweep reads whole policies.
func TestDependencyIndex_ReadsWholePoliciesNotSubsets(t *testing.T) {
	// testhelpers.NewMockClient is not usable here: it drags in internal/provider,
	// which reaches this package, and the cycle is fatal under the acceptance tag.
	handler := &policyServer{}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := jamfplatform.NewClient(server.URL, "test-id", "test-secret",
		jamfplatform.WithRetryPolicy(0, 0, 0))
	src := policyTenantSource{classic: proclassic.New(client)}

	idx, err := buildDependencyIndex(context.Background(), src)
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	if got := idx.uses[dependencyKey{DependencyPackage, "42"}]; len(got) != 1 {
		t.Errorf("package 42 uses = %d, want 1 — the sweep found no package reference, "+
			"which is what a subset read produces: 200 with an empty PackageConfiguration",
			len(got))
	}
	// The script reference is asserted alongside it so a wholly broken sweep is
	// told apart from one that reads policies but loses only the packages.
	if got := idx.uses[dependencyKey{DependencyScript, "9"}]; len(got) != 1 {
		t.Errorf("script 9 uses = %d, want 1 — the sweep indexed nothing at all", len(got))
	}

	if handler.fullReads != 1 || handler.subsetReads != 0 {
		t.Errorf("reads: full=%d subset=%d, want full=1 subset=0 — the sweep must never subset",
			handler.fullReads, handler.subsetReads)
	}
}
