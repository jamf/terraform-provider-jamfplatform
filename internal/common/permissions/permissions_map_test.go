// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package permissions

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// catalogue.go used to say of itself: "No test asserts what an entry SAYS.
// TestCatalogueCoversEverySDKCapability checks only that a required capability
// HAS a row, so a wrong section or a wrong permission name is invisible to it."
// This is that test, and the header no longer says it.
//
// It became possible because jamfplatform-go-sdk v0.21.0 committed a snapshot
// of the published permissions map and parses it as a privilege oracle, which
// established that the article's markdown rendering is machine-readable. The
// SDK reads only the Capability column — its parser discards both the
// Permission name and the "###" headings — so the two dimensions this file
// needs are the two nobody upstream consumes. jamfpro-cli reached the same
// conclusion independently and carries the sibling of this test; the two should
// move together.
//
// What this proves and what it does not: it removes drift between the
// transcription and the article, and it does NOT verify that the article
// matches what Jamf Account's picker actually prints. Nothing reachable from
// here can do the latter, which is why permissionsMapURL still appears in a
// rendered table with no recorded row.
//
// permissions-map.md beside this file is a verbatim copy fetched from
// permissionsMapURL. Refresh it with `make permissions-map` and read the diff:
// a name or a section moving is a real change to what an operator is told to
// look for, not housekeeping.
//
// The article is the source of truth, so its spelling is taken verbatim and
// there is deliberately no exception mechanism. A future divergence that is
// genuinely wanted gets a self-expiring exception map — an entry that stops
// disagreeing must fail — rather than a silent edit to catalogue.go.

// mapCapabilityRow matches a row of the article's capability tables:
// | Permission name | `slug:{actions}` | Endpoints |
var mapCapabilityRow = regexp.MustCompile("^\\|\\s*(.+?)\\s*\\|\\s*`([a-z0-9-]+):\\{([a-z,]+)\\}`\\s*\\|")

// The section of the article this file reads, bounded at both ends. The page
// also carries narrative tables — the privilege-collapse tables, the
// two-capability table — whose first column is an old privilege name rather
// than a permission name, and an "Endpoints with no permission" list. Reading
// either would attribute the wrong name to a slug.
const (
	mapSectionStart = "## Find the capability for an endpoint you already call"
	mapSectionEnd   = "## Endpoints with no permission"
)

// minDeclaredCapabilities guards against a snapshot that parsed to nothing — a
// redirect, a login page, or an upstream restructure that moved the tables. A
// parser that silently finds nothing reports perfect agreement, which is the
// one failure mode this test must not have. The article declared 125
// capabilities on 2026-09-03.
const minDeclaredCapabilities = 100

// publishedMapOmissions are capabilities the SDK requires and Jamf Account
// offers that the article does not yet declare. Each is self-expiring: the day
// the article declares the slug the entry must be deleted, so the article's own
// section and name take over and this file never shadows it.
//
// platform-environments: the gateway's authorization policy gates the six
// /organization/v1/{environments,tenants} routes on it (SDK v1.9.0,
// account-organization), and Jamf Account's integration form offers it as an
// Organization permission named "Platform Environments" with
// create/read/update/delete (SDK-recorded 2026-10-21). The article's 2026-10-22
// snapshot omits it.
var publishedMapOmissions = map[string]bool{
	"platform-environments": true,
}

// publishedMapActionOmissions are {capability}:{action} requirements the SDK
// declares on a capability the article does list, for an action the article's
// row for it leaves out. Self-expiring like publishedMapOmissions: the day the
// article's row carries the action the entry must go.
//
// recovery-lock:execute: the article lists recovery-lock as {r} only, over
// /view-recovery-lock-password. POST /pro/v2/mdm/commands enforces
// recovery-lock:execute for SET_RECOVERY_LOCK (SDK v1.9.0 WIRE-FACTS,
// 2026-10-22: a credential without it is refused 403 BAD_PERMISSIONS for that
// one command type and accepted for the other 34). The SDK reports it as
// enforced but unpublished, so the rendered table names it from the SDK and
// this entry records that the article cannot confirm it.
var publishedMapActionOmissions = map[string]bool{
	"recovery-lock:execute": true,
}

// publishedRow is one parsed article row.
type publishedRow struct {
	section string
	name    string
	actions map[string]bool
}

// mapActionCodes translates the article's one- and three-letter action codes
// into the slugs the SDK emits.
var mapActionCodes = map[string]string{
	"c":   "create",
	"r":   "read",
	"u":   "update",
	"d":   "delete",
	"dep": "deploy",
	"x":   "execute",
}

func parseActionCodes(t *testing.T, slug, codes string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for code := range strings.SplitSeq(codes, ",") {
		action, ok := mapActionCodes[code]
		if !ok {
			t.Errorf("%s declares action code %q, which mapActionCodes does not know", slug, code)
			continue
		}
		out[action] = true
	}
	return out
}

func parsePublishedMap(t *testing.T) map[string]publishedRow {
	t.Helper()

	raw, err := os.ReadFile("permissions-map.md")
	if err != nil {
		t.Fatalf("reading permissions-map.md: %v\n"+
			"This is the committed copy of %s. Refresh it with "+
			"`make permissions-map`; do not delete this guard.", err, permissionsMapURL)
	}
	body := string(raw)

	_, after, found := strings.Cut(body, mapSectionStart)
	if !found {
		t.Fatalf("permissions-map.md has no %q heading — the published article has been "+
			"restructured, so this parse cannot be trusted. Re-read it before adjusting "+
			"the bounds.", mapSectionStart)
	}
	before, _, found := strings.Cut(after, mapSectionEnd)
	if !found {
		t.Fatalf("permissions-map.md has no %q heading — see above.", mapSectionEnd)
	}

	rows := map[string]publishedRow{}
	var section string
	for line := range strings.SplitSeq(before, "\n") {
		if after0, ok := strings.CutPrefix(line, "### "); ok {
			section = strings.TrimSpace(after0)
			continue
		}
		m := mapCapabilityRow.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue // heading row, alignment row, or prose
		}
		if section == "" {
			t.Errorf("row %q appears before any ### heading, so it has no section", m[2])
			continue
		}
		// A capability is declared across as many rows as its resources need:
		// compliance-benchmarks has two, "Compliance Benchmarks" over
		// /benchmarks and "Compliance Benchmarks baseline rules" over
		// /baselines. The first wins, which is the row carrying the wider
		// action set and the one the catalogue transcribed.
		if _, seen := rows[m[2]]; !seen {
			rows[m[2]] = publishedRow{section: section, name: m[1], actions: parseActionCodes(t, m[2], m[3])}
		}
	}

	if len(rows) < minDeclaredCapabilities {
		t.Fatalf("parsed only %d capabilities from permissions-map.md — the row shape has "+
			"changed and this test would otherwise pass vacuously", len(rows))
	}
	return rows
}

// TestCatalogueMatchesThePublishedMap asserts every catalogue row's section and
// permission name against the committed copy of the article it transcribes.
func TestCatalogueMatchesThePublishedMap(t *testing.T) {
	published := parsePublishedMap(t)

	var onlyHere, onlyThere []string
	for slug := range catalogue {
		if _, ok := published[slug]; !ok && !publishedMapOmissions[slug] {
			onlyHere = append(onlyHere, slug)
		}
	}
	for slug := range publishedMapOmissions {
		if _, ok := published[slug]; ok {
			t.Errorf("publishedMapOmissions still excuses %q, which the article now declares — delete the entry", slug)
		}
		if _, ok := catalogue[slug]; !ok {
			t.Errorf("publishedMapOmissions excuses %q, which catalogue.go does not carry", slug)
		}
	}
	for slug := range published {
		if _, ok := catalogue[slug]; !ok {
			onlyThere = append(onlyThere, slug)
		}
	}
	sort.Strings(onlyHere)
	sort.Strings(onlyThere)
	if len(onlyHere) > 0 {
		t.Errorf("catalogue.go carries %d capabilities the article does not declare: %v\n"+
			"Either the article dropped them — in which case a rendered table now names a "+
			"permission Jamf Account no longer has — or the slug is misspelled here.",
			len(onlyHere), onlyHere)
	}
	if len(onlyThere) > 0 {
		t.Errorf("the article declares %d capabilities catalogue.go has no row for: %v\n"+
			"Add them; the file is deliberately a complete copy of the article, so that a "+
			"future revision stays diffable against it.", len(onlyThere), onlyThere)
	}

	for slug, want := range published {
		got, ok := catalogue[slug]
		if !ok {
			continue // already reported above
		}
		if got.category != want.section {
			t.Errorf("%s section = %q, article says %q\n"+
				"Jamf Account's picker groups by section, so a wrong one sends an operator "+
				"to the wrong part of the page.", slug, got.category, want.section)
		}
		if got.name != want.name {
			t.Errorf("%s name = %q, article says %q\n"+
				"The picker is searched BY NAME, so a name expanded or abbreviated here is a "+
				"name that finds nothing in the box.", slug, got.name, want.name)
		}
	}
}

// TestSDKActionsAreDeclaredByThePublishedMap asserts the action half of every
// SDK requirement against the article. TestCatalogueMatchesThePublishedMap
// compares section and name only, so a capability the article lists for read
// alone passes it while a rendered table tells an operator to tick Execute on a
// row whose picker has no such box.
func TestSDKActionsAreDeclaredByThePublishedMap(t *testing.T) {
	published := parsePublishedMap(t)

	required := map[string]string{}
	for pkgName, reg := range sdkRegistries() {
		for method, mp := range reg {
			for _, scoped := range mp.Scoped {
				if _, _, ok := splitScoped(scoped); !ok {
					continue
				}
				if _, dup := required[scoped]; !dup {
					required[scoped] = pkgName + "." + method
				}
			}
		}
	}

	for scoped, where := range required {
		capability, action, _ := splitScoped(scoped)
		row, ok := published[capability]
		if !ok {
			continue // publishedMapOmissions or the coverage test reports it
		}
		declared := row.actions[action]
		switch {
		case !declared && !publishedMapActionOmissions[scoped]:
			t.Errorf("%s requires %s, which the article's row for %q does not declare — "+
				"record it in publishedMapActionOmissions with the evidence, or the SDK is wrong",
				where, scoped, capability)
		case declared && publishedMapActionOmissions[scoped]:
			t.Errorf("publishedMapActionOmissions still excuses %q, which the article now declares — delete the entry", scoped)
		}
	}
	for scoped := range publishedMapActionOmissions {
		if _, ok := required[scoped]; !ok {
			t.Errorf("publishedMapActionOmissions excuses %q, which no SDK method requires — delete the entry", scoped)
		}
	}
}
