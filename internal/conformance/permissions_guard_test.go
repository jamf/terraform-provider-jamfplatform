// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// Every construct package renders its required Jamf permissions into its
// schema description and carries a render guard asserting the rendering
// happened. The honest form of that guard is permissions.Renders(x,
// "capability:action"), which parses the Markdown table back and checks the row
// for that capability really ticks the named action. A strings.Contains on the
// capability cell is not equivalent: it passes a row that ticks the wrong
// boxes, which is the exact failure Renders' own doc comment says a substring
// match cannot see.
//
// This walks every permissions_test.go and fails a file that asserts a rendered
// table with strings.Contains on a *Privileges variable without also calling
// permissions.Renders on that same variable. It is a structural guard: the move
// to Jamf Account's vocabulary fanned out across 261 files, twenty-one of them
// left on the substring form for a release, and nothing detected it —
// registration_test.go checks construct type names and never reads a
// MarkdownDescription, so the next construct inherits whichever form its
// author's neighbour happens to use.
//
// The unit of the check is the variable, not the file: a construct whose
// resource requires permissions and whose data source requires none (login_page
// is the live example) legitimately holds both forms in one file.
func TestPermissionsTests_AssertTablesWithRenders(t *testing.T) {
	files := findPermissionsTestFiles(t)
	if len(files) == 0 {
		t.Fatal("found no permissions_test.go files to inspect")
	}

	boilerplate := noPrivilegeBlock()
	if boilerplate == "" {
		t.Fatal("permissions.Section rendered no no-privilege block; the detector cannot tell a header assertion from a table assertion")
	}

	var renderGuards int

	for _, path := range files {
		if reason, ok := permissionsGuardAllowlist[packageDir(path)]; ok {
			t.Logf("%s: allowlisted (%s)", relPath(path), reason)
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		substring := map[string]bool{}
		renders := map[string]bool{}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			switch {
			case isCall(call, "strings", "Contains"):
				name, ok := privilegesVar(call.Args[0])
				if !ok {
					return true
				}
				// Asserting the block heading or the "None — any authenticated
				// ..." sentinel says nothing about which boxes a row ticks, so
				// it is not a table assertion and Renders has nothing to check.
				if lit, ok := stringLiteral(call.Args[1]); ok && isBoilerplateAssertion(lit, boilerplate) {
					return true
				}
				substring[name] = true
			case isCall(call, "permissions", "Renders"):
				if name, ok := privilegesVar(call.Args[0]); ok {
					renders[name] = true
					renderGuards++
				}
			}
			return true
		})

		rel := relPath(path)
		var weak []string
		for name := range substring {
			if !renders[name] {
				weak = append(weak, name)
			}
		}
		sort.Strings(weak)
		for _, name := range weak {
			t.Errorf("%s: asserts the rendered permission table with strings.Contains(%s, ...) and never "+
				"calls permissions.Renders on %s; a substring match passes a row that ticks the wrong "+
				"actions — assert permissions.Renders(%s, \"capability:action\") for each capability the "+
				"construct's SDK methods require",
				rel, name, name, name)
		}
	}

	if renderGuards == 0 {
		t.Fatal("inspected permissions tests but found no permissions.Renders call — the detector has drifted from the code")
	}
	t.Logf("checked %d permissions tests, %d permissions.Renders assertions", len(files), renderGuards)
}

// permissionsGuardAllowlist names package directories whose substring
// assertion on a *Privileges variable is legitimate, keyed on the directory
// relative to internal/.
//
// It is empty, deliberately. The three constructs whose SDK methods carry no
// scoped permissions at all — pro/app_store_country_codes, pro/icon and
// pro/tenant_id — render the "None" sentinel block rather than a table, so
// there is no row for Renders to parse; but all three assert either the block
// heading or the word "None", which isBoilerplateAssertion already recognises
// as not-a-table-assertion. Classifying by what the assertion says beats
// exempting a package wholesale: an exemption would also hide the day one of
// those endpoints starts requiring a permission and its guard is left matching
// a capability cell. Add an entry only for a file the content test cannot
// classify, with the reason inline.
var permissionsGuardAllowlist = map[string]string{}

// findPermissionsTestFiles collects every permissions_test.go under internal.
func findPermissionsTestFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	root := ".."
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "permissions_test.go" {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

// noPrivilegeBlock renders the block Section emits for a construct whose SDK
// methods require no permission. It carries both fixed strings a render guard
// may legitimately match — the "Required Jamf permissions" heading and the
// "None — ..." sentinel — so deriving it here keeps the classification in step
// with the renderer instead of restating its wording.
func noPrivilegeBlock() string {
	const method = "MethodRequiringNoPrivileges"
	return permissions.Section(permissions.Registry{method: {}}, method)
}

// isBoilerplateAssertion reports whether the asserted literal is part of the
// fixed block wording rather than a table row, i.e. whether it holds no claim
// about which actions a capability's row ticks.
func isBoilerplateAssertion(literal, block string) bool {
	return literal != "" && strings.Contains(block, literal)
}

// privilegesVar returns the name of a rendered-permissions variable the
// expression refers to. The naming convention is the detector: every construct
// holds its rendered block in resourcePrivileges / dataSourcePrivileges or a
// sibling of that shape.
func privilegesVar(expr ast.Expr) (string, bool) {
	ident, ok := expr.(*ast.Ident)
	if !ok || !strings.HasSuffix(ident.Name, "Privileges") {
		return "", false
	}
	return ident.Name, true
}

// stringLiteral unquotes an untyped string literal argument.
func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// packageDir returns the package directory of a file, relative to internal/.
func packageDir(path string) string {
	return relPath(filepath.Dir(path))
}

// helperPackages are the shared helper packages whose SDK calls fail the
// calling construct outright, mapped to the Go identifier each exports. A
// construct importing one must pass it to permissions.SectionWith from its
// permissions.go, or its "Required Jamf permissions" table omits a privilege
// the construct really needs (ldap-servers:read for a directory group,
// devices:read for a management_id target).
//
// Not listed on purpose: criteria, whose group-name lookups swallow every error
// and degrade to the wire value; and impact, which is advisory and opt-in and
// is documented in the impact-alerts guide instead.
var helperPackages = map[string]string{
	"ldapgroups":     "ldapgroups.Permissions",
	"computertarget": "computertarget.Permissions",
}

// TestHelperPermissions_EveryImporterDeclaresThem fails when a construct
// package imports a helper in helperPackages but its permissions.go never
// references the helper's Permissions, which is how blueprints deploy and the
// directory-group search went undocumented.
func TestHelperPermissions_EveryImporterDeclaresThem(t *testing.T) {
	const modPrefix = "github.com/jamf/terraform-provider-jamfplatform/internal/common/"
	root := filepath.Join("..")
	var failures []string
	checked := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		rel := filepath.ToSlash(path)
		if strings.Contains(rel, "/common/") || strings.Contains(rel, "/testhelpers") {
			return nil
		}
		raw, readErr := os.ReadFile(filepath.Join(path, "permissions.go"))
		if readErr != nil {
			return nil
		}
		permSrc := string(raw)
		files, _ := filepath.Glob(filepath.Join(path, "*.go"))
		imported := map[string]bool{}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			af, parseErr := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if parseErr != nil {
				continue
			}
			for _, im := range af.Imports {
				p, _ := strconv.Unquote(im.Path.Value)
				if name, ok := strings.CutPrefix(p, modPrefix); ok {
					imported[name] = true
				}
			}
		}
		for pkg, ident := range helperPackages {
			if !imported[pkg] {
				continue
			}
			checked++
			if !strings.Contains(permSrc, ident) {
				failures = append(failures, rel+" imports "+pkg+" but permissions.go never references "+ident)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 10 {
		t.Fatalf("only %d importer/helper pairs found; the walk is not reaching the construct packages", checked)
	}
	sort.Strings(failures)
	for _, f := range failures {
		t.Error(f)
	}
}
