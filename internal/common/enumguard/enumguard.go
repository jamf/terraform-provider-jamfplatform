// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package enumguard is the recurrence guard for restating a value the SDK
// already generates.
//
// Enum values and machine-readable error codes must come from the SDK's
// generated constants, never from a string literal — see STYLE_GUIDE.md
// §"Enum values and error codes come from the SDK, not from literals". That
// rule failed twice by review, so every package declaring such values pins it
// with a test rather than trusting a reader to notice.
//
// The check parses the package's own source rather than taking a hand-written
// list, so it cannot drift. It fires in both directions: on a literal that the
// SDK covers today, and — through [Check]'s Absent map — on a literal that a
// future SDK release promotes into an enum.
//
// It walks const *and* var declarations, including the elements and keys of
// slice and map composite literals, because a restated enum is far more often a
// `var validFoos = []string{...}` feeding a OneOf validator than a lone const.
// It also reaches inside function bodies for three shapes. Two are the framework
// calls that declare an accepted set in all but name — stringvalidator.OneOf
// (and OneOfCaseInsensitive) and stringdefault.StaticString — since a schema
// that inlines its vocabulary is restating it just as surely as a
// package-level var would. The third is a short
// variable declaration, `v := "install"`, which is how a builder names the one
// wire value it is about to send. That third shape was missing until this
// package's own review found five shipped packages whose only restated enum had
// it, leaving those guards unable to protect their own source.
//
// It reads every non-test file in the package directory, so moving a
// declaration between files cannot slip past it.
//
// What it deliberately does not cover: a literal that is not declared but merely
// used — compared against in ordinary logic (`if cdnType == "AMAZON_S3"`, `case
// "PENDING":`), returned (`return "PENDING"`), passed as an argument to anything
// but the two framework calls above, or assigned to a field of a struct literal
// built inside a function. Reaching those would mean treating every string in
// the package as an enum reference, which fires on prose and on unrelated
// vocabularies that happen to share a spelling. Those are an audit's job, not a
// guard's. The line the walker draws is declaration versus use: a short variable
// declaration is collected because the name it binds is the value the code goes
// on to send, whereas the same literal one line later in a `case` is only being
// recognised.
//
// A package adopts the guard with an enum_literals_test.go that calls [Check]
// and fails on [Findings.Problems]. The plain shape is
// internal/resources/pro/ebook/enum_literals_test.go; the shape with per-value
// exemptions is internal/resources/pro/macos_configuration_profile/enum_literals_test.go.
// [Params] documents the two exemption maps and why they are checked differently.
package enumguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Literal is one string literal found in a declaration the package makes — a
// const, a var, an inlined framework set or a func-body short variable
// declaration — tagged with that declaration so a failure names something a
// reader can grep for.
type Literal struct {
	// Decl names the declaration the literal was found under: a const or var
	// name, or "func -> name" for a literal declared inside a function body.
	Decl string
	// Value is the literal's unquoted content.
	Value string
	// Pos is "file:line", relative to the package directory.
	Pos string
}

// Params configures a [Check].
type Params struct {
	// Dir is the package directory to parse. Empty means ".".
	Dir string

	// Covered is every value the SDK generates for the vocabularies this
	// package writes — the concatenation of the relevant *Values() helpers.
	// A literal appearing here is a violation unless Absent excuses it.
	//
	// Leaving it empty is itself reported as [Findings.Vacuous]: a caller that
	// names no vocabulary asserts nothing, so a package with no vocabulary left
	// to restate must record the gap and drop its test rather than keep a test
	// that cannot fail.
	Covered []string

	// Absent maps a value this package must restate to the reason the SDK
	// carries no constant for it. Stating the reason per value is the point:
	// a comment claiming "the SDK has none of these" is the exact shape of
	// the defect this guard exists to catch, because it is usually true of
	// some values and false of others.
	//
	// An entry that Covered turns out to contain is itself a failure — that
	// is how the guard reports an SDK release promoting a literal into an
	// enum.
	Absent map[string]string

	// Ignore lists literal values that are not members of any vocabulary in
	// Covered, even though they spell one. Two shapes: something that is not
	// an enum value at all — an attribute name, a map key, a sentinel, an
	// English noun — and a genuine member of a *different* vocabulary that
	// the SDK does not generate. Each entry needs a reason for the same
	// reason Absent does.
	//
	// The distinction from Absent matters: Absent asserts the SDK carries no
	// constant for the value and is therefore checked against Covered, so an
	// SDK release that starts generating it fails. Ignore asserts the value
	// belongs to a different set, which a foreign vocabulary gaining or
	// losing the spelling says nothing about, so it is not checked. Putting a
	// cross-vocabulary collision in Absent reports a spurious promotion.
	Ignore map[string]string
}

// Findings is what [Check] returns: violations to fail on, plus the count of
// literals actually examined so a caller can refuse a vacuous pass.
type Findings struct {
	// Restated are literals the SDK already provides a constant for.
	Restated []string
	// Promoted are Absent entries the SDK has since started generating.
	Promoted []string
	// Stale are Absent or Ignore entries no literal in the package uses.
	Stale []string
	// Vacuous is set when no literal could ever be reported, for either of the
	// two reasons that happens: Covered is empty, or every value in it is
	// exempted. A guard that cannot fail is worse than none — it reads as
	// coverage while asserting nothing — so this is a failure. The empty case
	// is the one an SDK release causes on its own, by withdrawing the last
	// vocabulary a package restated, and it must not pass silently.
	Vacuous string
	// Examined is how many string literals were parsed.
	Examined int
}

// Check parses the package in p.Dir and reports every string literal the
// package declares — see the package doc for exactly which shapes count as a
// declaration — that the SDK already covers.
func Check(p Params) (Findings, error) {
	dir := p.Dir
	if dir == "" {
		dir = "."
	}

	lits, err := collect(dir)
	if err != nil {
		return Findings{}, err
	}

	covered := make(map[string]struct{}, len(p.Covered))
	for _, v := range p.Covered {
		covered[v] = struct{}{}
	}

	var out Findings
	out.Examined = len(lits)
	used := map[string]struct{}{}

	for _, l := range lits {
		_, excused := p.Absent[l.Value]
		_, ignored := p.Ignore[l.Value]
		if excused || ignored {
			used[l.Value] = struct{}{}
		}
		if _, ok := covered[l.Value]; !ok || excused || ignored {
			continue
		}
		out.Restated = append(out.Restated, fmt.Sprintf(
			"%s: %s is the literal %q, which the SDK generates a constant for — "+
				"alias the constant so the two cannot drift",
			l.Pos, l.Decl, l.Value,
		))
	}

	for v, why := range p.Absent {
		if _, ok := covered[v]; ok {
			out.Promoted = append(out.Promoted, fmt.Sprintf(
				"%q is listed as absent from the SDK (%s), but the SDK now generates it — "+
					"alias the constant and drop the exemption",
				v, why,
			))
		}
	}

	for _, m := range []map[string]string{p.Absent, p.Ignore} {
		for v := range m {
			if _, ok := used[v]; !ok {
				out.Stale = append(out.Stale, fmt.Sprintf(
					"%q is exempted but no literal in the package uses it — drop the exemption", v,
				))
			}
		}
	}

	live := 0
	for v := range covered {
		_, excused := p.Absent[v]
		_, ignored := p.Ignore[v]
		if !excused && !ignored {
			live++
		}
	}
	if live == 0 {
		cause := "every value in Covered is exempted"
		if len(covered) == 0 {
			cause = "Covered is empty"
		}
		out.Vacuous = cause + ", so this guard cannot fail — " +
			"name a vocabulary the package could actually restate, or drop the test and " +
			"record the gap instead"
	}

	sort.Strings(out.Restated)
	sort.Strings(out.Promoted)
	sort.Strings(out.Stale)
	return out, nil
}

// collect returns every string literal the package's non-test files declare: in
// a package-level const or var, in an inlined framework set, or on the
// right-hand side of a func-body short variable declaration.
func collect(dir string) ([]Literal, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	fset := token.NewFileSet()
	var out []Literal

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				out = append(out, inlineSets(fset, name, fn)...)
				out = append(out, localDefines(fset, name, fn)...)
				continue
			}
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) == 0 {
					continue
				}
				declName := value.Names[0].Name
				for _, expr := range value.Values {
					for _, lit := range stringLits(expr) {
						unquoted, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						pos := fset.Position(lit.Pos())
						out = append(out, Literal{
							Decl:  declName,
							Value: unquoted,
							Pos:   fmt.Sprintf("%s:%d", name, pos.Line),
						})
					}
				}
			}
		}
	}
	return out, nil
}

// stringLits returns every string BasicLit reachable from expr without
// descending into a call, a comparison or a function body — a literal that is
// only an argument or an operand is not a declared enum value, and pulling those
// in would make the guard fire on prose.
func stringLits(expr ast.Expr) []*ast.BasicLit {
	var out []*ast.BasicLit
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			out = append(out, e)
		}
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				out = append(out, stringLits(kv.Key)...)
				out = append(out, stringLits(kv.Value)...)
				continue
			}
			out = append(out, stringLits(elt)...)
		}
	case *ast.ParenExpr:
		out = append(out, stringLits(e.X)...)
	}
	return out
}

// Union concatenates SDK *Values() results into the single Covered slice
// [Check] takes, dropping duplicates so a value two vocabularies share is
// reported once.
func Union(sets ...[]string) []string {
	var out []string
	for _, s := range sets {
		for _, v := range s {
			if !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	return out
}

// Problems returns every failure in one slice, so a caller can report them
// without having to know which of the three kinds each one is.
func (f Findings) Problems() []string {
	out := make([]string, 0, len(f.Restated)+len(f.Promoted)+len(f.Stale)+1)
	out = append(out, f.Restated...)
	out = append(out, f.Promoted...)
	out = append(out, f.Stale...)
	if f.Vacuous != "" {
		out = append(out, f.Vacuous)
	}
	return out
}

// declaringCalls are the framework calls whose string arguments *are* an enum
// declaration: the set an attribute accepts, and the value it defaults to.
var declaringCalls = map[string]struct{}{
	"OneOf":                {},
	"OneOfCaseInsensitive": {},
	"StaticString":         {},
}

// inlineSets returns the string literals a function body passes to a
// declaringCalls call, attributed to the function so a failure points at the
// schema that inlined its vocabulary.
func inlineSets(fset *token.FileSet, file string, fn *ast.FuncDecl) []Literal {
	if fn.Body == nil {
		return nil
	}
	var out []Literal
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, ok := declaringCalls[sel.Sel.Name]; !ok {
			return true
		}
		for _, arg := range call.Args {
			for _, lit := range stringLits(arg) {
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				out = append(out, Literal{
					Decl:  fn.Name.Name + " -> " + sel.Sel.Name,
					Value: unquoted,
					Pos:   fmt.Sprintf("%s:%d", file, fset.Position(lit.Pos()).Line),
				})
			}
		}
		return true
	})
	return out
}

// localDefines returns the string literals a function body binds with a short
// variable declaration, attributed as "func -> name" so a failure points at the
// builder that inlined the value. Only the right-hand side is walked, and only
// through definedLits, so a call argument, a comparison operand and a `case`
// value are not collected: what separates this shape from those is that `:=`
// declares the value the code goes on to send, rather than recognising one it was
// handed.
func localDefines(fset *token.FileSet, file string, fn *ast.FuncDecl) []Literal {
	if fn.Body == nil {
		return nil
	}
	var out []Literal
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE {
			return true
		}
		for i, rhs := range assign.Rhs {
			name := "_"
			if len(assign.Lhs) == len(assign.Rhs) {
				if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
					name = ident.Name
				}
			} else if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
				name = ident.Name
			}
			for _, lit := range definedLits(rhs) {
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				out = append(out, Literal{
					Decl:  fn.Name.Name + " -> " + name,
					Value: unquoted,
					Pos:   fmt.Sprintf("%s:%d", file, fset.Position(lit.Pos()).Line),
				})
			}
		}
		return true
	})
	return out
}

// definedLits is the right-hand side of a short variable declaration narrowed to
// the shapes that are a declaration of a value rather than a use of one: a bare
// literal, and a slice or map of literals. It hands those to stringLits so the
// recursion rules stay identical to a package-level var, and stops at everything
// else — a call, a comparison, a conversion — because a literal there is an
// argument or an operand. A struct literal is excluded at the top level too,
// since its `Field: "value"` elements are populating a type the package did not
// declare; auditing those is a separate job from guarding a declared vocabulary.
func definedLits(expr ast.Expr) []*ast.BasicLit {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return stringLits(e)
	case *ast.ParenExpr:
		return definedLits(e.X)
	case *ast.CompositeLit:
		switch e.Type.(type) {
		case *ast.ArrayType, *ast.MapType:
			return stringLits(e)
		}
	}
	return nil
}
