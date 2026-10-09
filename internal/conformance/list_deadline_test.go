// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	jamfprovider "github.com/jamf/terraform-provider-jamfplatform/internal/provider"
)

// listDeadlineExempt names list resources whose List method does not bound itself through
// listtimeouts.Bound, each with the reason. It is empty: a list resource that streams without a
// deadline hangs `terraform query` on a stalled gateway, because the SDK no longer caps a request at
// 60s of response-header wait.
var listDeadlineExempt = map[string]string{}

// Every list resource must expose a `timeouts` attribute and bound its context from it inside its
// List method, through listtimeouts.Bound. A bare context.WithTimeout would pass for a stalled
// gateway but leave the practitioner no way to widen the deadline for a large estate.
//
// The walk finds List methods by their `list.ListRequest` parameter, and the count is compared with
// the provider's own registration so a method the walk cannot see, or a registration it never
// reached, fails the test instead of passing it quietly.
func TestEveryListResourceBoundsItsContext(t *testing.T) {
	ctx := context.Background()
	p, ok := jamfprovider.New("test")().(interface {
		provider.Provider
		provider.ProviderWithListResources
	})
	if !ok {
		t.Fatal("provider no longer implements ProviderWithListResources")
	}
	registered := len(p.ListResources(ctx))

	for _, newList := range p.ListResources(ctx) {
		lr := newList()
		var meta resource.MetadataResponse
		lr.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &meta)
		var resp list.ListResourceSchemaResponse
		lr.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &resp)
		if _, hasTimeouts := resp.Schema.Attributes[listtimeouts.AttributeName]; !hasTimeouts {
			t.Errorf("list resource %s declares no `timeouts` attribute", meta.TypeName)
		}
	}

	root := filepath.Join("..", "resources")
	fset := token.NewFileSet()
	var found int
	var missing []string

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv == nil || fn.Name.Name != "List" || fn.Body == nil {
				continue
			}
			if !takesListRequest(fn) {
				continue
			}
			found++
			name := filepath.ToSlash(path)
			if _, exempt := listDeadlineExempt[name]; exempt {
				continue
			}
			if !callsListtimeoutsBound(fn.Body) {
				missing = append(missing, name)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", root, walkErr)
	}

	if found != registered {
		t.Fatalf("found %d List methods taking list.ListRequest under %s but the provider registers %d list resources; the scan is out of step with the provider", found, root, registered)
	}

	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s: List does not call listtimeouts.Bound, so its deadline is not the practitioner-settable `timeouts.list` and a stalled gateway can hang `terraform query`", name)
	}
}

func takesListRequest(fn *ast.FuncDecl) bool {
	for _, param := range fn.Type.Params.List {
		sel, isSel := param.Type.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "ListRequest" {
			continue
		}
		if pkg, isIdent := sel.X.(*ast.Ident); isIdent && pkg.Name == "list" {
			return true
		}
	}
	return false
}

func callsListtimeoutsBound(body *ast.BlockStmt) bool {
	var found bool
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return !found
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel {
			return !found
		}
		pkg, isIdent := sel.X.(*ast.Ident)
		if isIdent && pkg.Name == "listtimeouts" && sel.Sel.Name == "Bound" {
			found = true
		}
		return !found
	})
	return found
}
