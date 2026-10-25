// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

const nameCollisionGuide = "../../templates/guides/name-collisions.md"

var (
	guideRowRe      = regexp.MustCompile("(?m)^\\|\\s*`(jamfplatform_[a-z0-9_]+)`\\s*\\|[^|]*\\|\\s*(Yes|No)\\s*\\|")
	enforcedLiteral = regexp.MustCompile(`\bEnforced:\s+(true|false)\b`)
)

type guideRow struct {
	enforced bool
}

func nameCollisionGuideRows(t *testing.T) map[string][]guideRow {
	t.Helper()
	body, err := os.ReadFile(nameCollisionGuide)
	if err != nil {
		t.Fatalf("reading %s: %v", nameCollisionGuide, err)
	}
	rows := map[string][]guideRow{}
	for _, m := range guideRowRe.FindAllStringSubmatch(string(body), -1) {
		rows[m[1]] = append(rows[m[1]], guideRow{enforced: m[2] == "Yes"})
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
// check without a row in the guide, or a row for a construct that no longer has
// one, fails here, and so does a row whose "Jamf refuses duplicates" cell
// disagrees with the package's own Enforced setting.
func TestNameCollisionGuide_ListsEveryCheckedConstruct(t *testing.T) {
	rows := nameCollisionGuideRows(t)
	if len(rows) < 10 {
		t.Fatalf("parsed %d constructs from the guide table; the parser is likely matching nothing", len(rows))
	}
	pkgs := collisionPackages(t)

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
		if !ok {
			continue
		}
		for _, want := range []bool{true, false} {
			if !slices.Contains(pkg.enforced, want) {
				continue
			}
			if !slices.ContainsFunc(guide, func(r guideRow) bool { return r.enforced == want }) {
				t.Errorf("%s sets Enforced: %v but no guide row for it says %s", construct, want, map[bool]string{true: "Yes", false: "No"}[want])
			}
		}
	}
}
