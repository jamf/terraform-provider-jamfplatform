// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	jamfprovider "github.com/jamf/terraform-provider-jamfplatform/internal/provider"
)

var constructRe = regexp.MustCompile("`(jamfplatform_[a-z0-9_]+)`")

const nameCollisionGuide = "../../templates/guides/name-collisions.md"

var (
	guideRowRe      = regexp.MustCompile("(?m)^\\|\\s*`(jamfplatform_[a-z0-9_]+)`\\s*\\|([^|]*)\\|([^|]*)\\|")
	enforcedLiteral = regexp.MustCompile(`\bEnforced:\s+(true|false)\b`)
)

type guideRow struct {
	hasDefault bool
	hasStrict  bool
}

func keysPresent(cell string) bool {
	cell = strings.TrimSpace(cell)
	return cell != "" && cell != "—"
}

func guideSection(t *testing.T, heading string) string {
	t.Helper()
	body, err := os.ReadFile(nameCollisionGuide)
	if err != nil {
		t.Fatalf("reading %s: %v", nameCollisionGuide, err)
	}
	text := string(body)
	start := strings.Index(text, "## "+heading)
	if start < 0 {
		t.Fatalf("%s has no %q section", nameCollisionGuide, heading)
	}
	rest := text[start+len("## "+heading):]
	if end := strings.Index(rest, "\n## "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func nameCollisionGuideRows(t *testing.T) map[string][]guideRow {
	t.Helper()
	rows := map[string][]guideRow{}
	for _, m := range guideRowRe.FindAllStringSubmatch(guideSection(t, "Checked object types"), -1) {
		rows[m[1]] = append(rows[m[1]], guideRow{hasDefault: keysPresent(m[2]), hasStrict: keysPresent(m[3])})
	}
	return rows
}

type collisionPackage struct {
	enforced []bool
}

func collisionPackages(t *testing.T) map[string]collisionPackage {
	t.Helper()
	root := "../resources"
	out := map[string]collisionPackage{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "collision.go" {
			return nil
		}
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		construct := "jamfplatform_" + strings.ReplaceAll(filepath.ToSlash(rel), "/", "_")

		pkg := collisionPackage{}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return err
			}
			for _, m := range enforcedLiteral.FindAllStringSubmatch(string(src), -1) {
				pkg.enforced = append(pkg.enforced, m[1] == "true")
			}
		}
		out[construct] = pkg
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(out) == 0 {
		t.Fatal("found no collision.go files; the guide cannot be checked against nothing")
	}
	return out
}

// TestNameCollisionGuide_ListsEveryCheckedConstruct keeps the user guide's table
// of checked types in step with the code. A package that wires the collision
// check without a row in the guide, a row for a construct that no longer has
// one, or a second row for one construct, fails here, and so does a row whose
// key columns disagree with the package's own Enforced settings.
func TestNameCollisionGuide_ListsEveryCheckedConstruct(t *testing.T) {
	rows := nameCollisionGuideRows(t)
	if len(rows) < 10 {
		t.Fatalf("parsed %d constructs from the guide table; the parser is likely matching nothing", len(rows))
	}
	pkgs := collisionPackages(t)

	var duplicated []string
	for construct, guide := range rows {
		if len(guide) > 1 {
			duplicated = append(duplicated, construct)
		}
	}
	sort.Strings(duplicated)
	if len(duplicated) > 0 {
		t.Errorf("constructs with more than one row in %s, which keeps one row per construct: %v", nameCollisionGuide, duplicated)
	}

	var missing, stale []string
	for construct := range pkgs {
		if _, ok := rows[construct]; !ok {
			missing = append(missing, construct)
		}
	}
	for construct := range rows {
		if _, ok := pkgs[construct]; !ok {
			stale = append(stale, construct)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("constructs with a collision check but no row in %s: %v", nameCollisionGuide, missing)
	}
	if len(stale) > 0 {
		t.Errorf("rows in %s for constructs with no collision.go: %v", nameCollisionGuide, stale)
	}

	for construct, pkg := range pkgs {
		guide, ok := rows[construct]
		if !ok || len(guide) == 0 {
			continue
		}
		if slices.Contains(pkg.enforced, true) && !guide[0].hasDefault {
			t.Errorf("%s sets Enforced: true but its guide row lists no key under \"Checked by default\"", construct)
		}
		if slices.Contains(pkg.enforced, false) && !guide[0].hasStrict {
			t.Errorf("%s sets Enforced: false but its guide row lists no key under \"Checked with include_unenforced\"", construct)
		}
	}
}

func guideUncheckedConstructs(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for line := range strings.SplitSeq(guideSection(t, "Object types that are not checked"), "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		for _, m := range constructRe.FindAllStringSubmatch(cells[1], -1) {
			out[m[1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatal("parsed no constructs from the not-checked table; the parser is likely matching nothing")
	}
	return out
}

func registeredResourceTypeNames(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	p := jamfprovider.New("test")()
	var names []string
	for _, newResource := range p.Resources(ctx) {
		var resp resource.MetadataResponse
		newResource().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
		names = append(names, resp.TypeName)
	}
	return names
}

// TestNameCollisionGuide_CoversEveryRegisteredResource closes the loop the other
// two tests leave open. Every resource the provider registers must be either
// checked, with a row in the checked table, or listed in the not-checked table
// with its reason. A new construct that is neither fails here, so the collision
// question is asked of it once and the answer is written down. A construct in
// both tables, or listed but no longer registered, fails too.
func TestNameCollisionGuide_CoversEveryRegisteredResource(t *testing.T) {
	checked := nameCollisionGuideRows(t)
	unchecked := guideUncheckedConstructs(t)
	registered := registeredResourceTypeNames(t)

	registeredSet := map[string]bool{}
	var unaccounted []string
	for _, name := range registered {
		registeredSet[name] = true
		_, isChecked := checked[name]
		if !isChecked && !unchecked[name] {
			unaccounted = append(unaccounted, name)
		}
	}
	sort.Strings(unaccounted)
	if len(unaccounted) > 0 {
		t.Errorf("registered resources in neither guide table. Wire the collision check (STYLE_GUIDE §Plan-time name collision check) or list them under %q: %v", "Object types that are not checked", unaccounted)
	}

	var both, stale []string
	for name := range unchecked {
		if _, ok := checked[name]; ok {
			both = append(both, name)
		}
		if !registeredSet[name] {
			stale = append(stale, name)
		}
	}
	for name := range checked {
		if !registeredSet[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(both)
	sort.Strings(stale)
	if len(both) > 0 {
		t.Errorf("constructs in both guide tables: %v", both)
	}
	if len(stale) > 0 {
		t.Errorf("guide lists constructs the provider does not register: %v", stale)
	}
}

// TestNameCollisionGuide_UncheckedTypesHaveNoCheck keeps the not-checked list
// honest: a construct named there that later gains a collision.go must move to
// the checked table.
func TestNameCollisionGuide_UncheckedTypesHaveNoCheck(t *testing.T) {
	pkgs := collisionPackages(t)
	for name := range guideUncheckedConstructs(t) {
		if _, ok := pkgs[name]; ok {
			t.Errorf("%s is listed as not checked but has a collision.go", name)
		}
	}
}
