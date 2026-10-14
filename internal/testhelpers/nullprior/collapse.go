// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package nullprior

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
)

// unknownOnlyModifiers are the plan modifiers that change a plan value only
// while it is Unknown. TestUnknownOnlyModifiersLeaveAKnownPlanAlone proves that
// for each one, so adding a modifier here without it passing that test fails.
var unknownOnlyModifiers = []any{
	stringplanmodifier.UseStateForUnknown(),
	stringplanmodifier.UseNonNullStateForUnknown(),
	boolplanmodifier.UseStateForUnknown(),
	boolplanmodifier.UseNonNullStateForUnknown(),
	int64planmodifier.UseStateForUnknown(),
	int64planmodifier.UseNonNullStateForUnknown(),
	planmodifiers.UseStateForUnknownInExistingBlockString(),
	planmodifiers.UseStateForUnknownInExistingBlockBool(),
	planmodifiers.UseStateForUnknownInExistingBlockInt64(),
	planmodifiers.UseKeyedStateForUnknownString("id"),
	planmodifiers.UseKeyedStateForUnknownBool("id"),
}

// AssertNoPlanCollapse checks the condition under which an Optional+Computed
// leaf of an Optional single-nested block may use UseNonNullStateForUnknown
// (STYLE_GUIDE rule 1a): nothing in the resource can collapse a configured
// change back to prior state.
//
// UseNonNullStateForUnknown leaves a leaf with a null prior Unknown in any plan
// whose proposed state differs from prior state, because the framework marks
// every Computed attribute with a null configuration Unknown before any
// modifier runs. That is harmless while the plan really changes something. It
// is a permanent diff when something restores the change that triggered the
// marking, because then the Unknowns are all that is left and `terraform plan
// -refresh=false` is never empty. The framework marks only attributes whose
// configuration is null, and Terraform core accepts a planned value that
// differs from a non-null configuration only when it equals the prior value, so
// there are exactly two ways to restore one:
//
//   - A plan modifier on a configurable attribute that plans the prior value in
//     place of the configured one, as the CRLF/LF equality on
//     jamfplatform_pro_mobile_device_app app_configuration.preferences does.
//     Every plan modifier on an Optional or Required attribute of sch must be one
//     of unknownOnlyModifiers. A modifier on a Computed-only attribute is not
//     checked: its configuration is always null, so it cannot restore a
//     configured value.
//   - A resource ModifyPlan that writes the plan. No non-test file in pkgDir, or
//     in any package of this module that it imports directly or indirectly, may
//     call Set or SetAttribute on a Plan field or assign to one. The check is
//     syntactic; a write through an alias of the plan is not seen.
//
// Semantic equality needs no check, because the framework runs it on Read and
// apply responses and never at plan time. A Default needs none either: it
// applies only to a null configuration.
//
// Run it from the resource's schema test with pkgDir ".". A failure means the
// resource has, or may have, a collapse mechanism, so its singleton-block leaves
// need planmodifiers.UseStateForUnknownInExistingBlock* and its Computed-only
// leaves need a modifier that carries a null prior, or the new modifier needs
// adding to unknownOnlyModifiers with proof.
func AssertNoPlanCollapse(t *testing.T, sch schema.Schema, pkgDir string) {
	t.Helper()
	for _, problem := range collapseProblems(sch, pkgDir) {
		t.Error(problem)
	}
}

func collapseProblems(sch schema.Schema, pkgDir string) []string {
	allowed := map[reflect.Type]bool{}
	for _, m := range unknownOnlyModifiers {
		allowed[reflect.TypeOf(m)] = true
	}
	var problems []string
	if len(sch.Blocks) > 0 {
		problems = append(problems, "schema has blocks, which AssertNoPlanCollapse does not walk; extend it before relying on it")
	}
	walkModifiers("", sch.Attributes, allowed, &problems)
	problems = append(problems, planWrites(pkgDir)...)
	sort.Strings(problems)
	return problems
}

func walkModifiers(prefix string, attrs map[string]schema.Attribute, allowed map[reflect.Type]bool, problems *[]string) {
	for name, a := range attrs {
		p := prefix + name
		if a.IsOptional() || a.IsRequired() {
			checkModifiers(p, reflect.ValueOf(a).FieldByName("PlanModifiers"), allowed, problems)
		}
		switch n := a.(type) {
		case schema.SingleNestedAttribute:
			walkModifiers(p+".", n.Attributes, allowed, problems)
		case schema.ListNestedAttribute:
			checkModifiers(p+"[*]", reflect.ValueOf(n.NestedObject.PlanModifiers), allowed, problems)
			walkModifiers(p+"[*].", n.NestedObject.Attributes, allowed, problems)
		case schema.SetNestedAttribute:
			checkModifiers(p+"[*]", reflect.ValueOf(n.NestedObject.PlanModifiers), allowed, problems)
			walkModifiers(p+"[*].", n.NestedObject.Attributes, allowed, problems)
		case schema.MapNestedAttribute:
			checkModifiers(p+"[*]", reflect.ValueOf(n.NestedObject.PlanModifiers), allowed, problems)
			walkModifiers(p+"[*].", n.NestedObject.Attributes, allowed, problems)
		}
	}
}

func checkModifiers(p string, mods reflect.Value, allowed map[reflect.Type]bool, problems *[]string) {
	if !mods.IsValid() || mods.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < mods.Len(); i++ {
		m := mods.Index(i).Elem()
		if !m.IsValid() || allowed[m.Type()] {
			continue
		}
		*problems = append(*problems, fmt.Sprintf("%s: plan modifier %s is not known to leave a configured value alone. If it can plan the prior value in place of the configuration, it collapses changes and leaves every UseNonNullStateForUnknown leaf with a null prior Unknown in a plan that is otherwise empty (STYLE_GUIDE rule 1a).", p, m.Type()))
	}
}

// planWrites returns one problem for each write to a Plan field in pkgDir and
// in the module's own packages that pkgDir imports.
func planWrites(pkgDir string) []string {
	root, module, err := findModule(pkgDir)
	if err != nil {
		return []string{fmt.Sprintf("cannot locate go.mod above %s: %v", pkgDir, err)}
	}
	var problems []string
	seen := map[string]bool{}
	queue := []string{filepath.Clean(pkgDir)}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		abs, err := filepath.Abs(dir)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		files, err := filepath.Glob(filepath.Join(abs, "*.go"))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", abs, err))
			continue
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, 0)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", file, err))
				continue
			}
			for _, imp := range f.Imports {
				ip, _ := strconv.Unquote(imp.Path.Value)
				if rel, ok := strings.CutPrefix(ip, module+"/"); ok {
					queue = append(queue, filepath.Join(root, filepath.FromSlash(rel)))
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range x.Lhs {
						if isPlanField(lhs) || isPlanRaw(lhs) {
							problems = append(problems, fmt.Sprintf("%s: assigns to a plan; a ModifyPlan that writes the plan can collapse a configured change (STYLE_GUIDE rule 1a).", fset.Position(x.Pos())))
						}
					}
				case *ast.CallExpr:
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Set" || sel.Sel.Name == "SetAttribute") && isPlanField(sel.X) {
						problems = append(problems, fmt.Sprintf("%s: calls Plan.%s; a ModifyPlan that writes the plan can collapse a configured change (STYLE_GUIDE rule 1a).", fset.Position(x.Pos()), sel.Sel.Name))
					}
				}
				return true
			})
		}
	}
	return problems
}

func isPlanField(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Plan"
}

func isPlanRaw(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Raw" && isPlanField(sel.X)
}

func findModule(dir string) (root, module string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		body, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			for line := range strings.Lines(string(body)) {
				if m, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					return d, strings.TrimSpace(m), nil
				}
			}
			return "", "", fmt.Errorf("%s/go.mod has no module line", d)
		}
		if filepath.Dir(d) == d {
			return "", "", os.ErrNotExist
		}
	}
}
