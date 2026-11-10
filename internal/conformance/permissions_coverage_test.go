// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	jamfplatform "github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// TestPermissionsCoverEveryCallInThePackage fails when a construct package calls
// an SDK method whose privileges its permissions.go does not already declare.
//
// The per-construct drift guards each scan a hand-picked file list (crud.go and
// a few siblings), so a call that lands in helpers.go or validators.go is
// invisible to them: DeployBlueprint and UndeployBlueprint went undocumented
// for exactly that reason. This walks every non-test file in every package with
// a permissions.go instead.
//
// The comparison is on privileges, not method names, because a call whose
// privilege a sibling method already declares adds nothing to the table
// (RefreshCloudDistributionPointInventoryV1 shares cloud-distribution-point:read
// with GetCloudDistributionPointV1), and a call whose spec declares none has no
// row to add (ListAppStoreCountryCodesV1). Calls made through a shared helper
// package are covered separately, by TestHelperPermissions_EveryImporterDeclaresThem.
func TestPermissionsCoverEveryCallInThePackage(t *testing.T) {
	reg := map[string]jamfplatform.MethodPrivileges{}
	for _, r := range []map[string]jamfplatform.MethodPrivileges{
		account.Privileges, aigovernance.Privileges, blueprints.Privileges,
		compliancebenchmarks.Privileges, deviceactions.Privileges, devicegroups.Privileges,
		devices.Privileges, pro.Privileges, proclassic.Privileges, securitycloud.Privileges,
	} {
		for k, v := range r {
			if _, dup := reg[k]; !dup {
				reg[k] = v
			}
		}
	}

	root := ".."
	var failures []string
	packages := 0
	err := filepath.WalkDir(root, func(dir string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		hasPerm := false
		for _, f := range files {
			if filepath.Base(f) == "permissions.go" {
				hasPerm = true
			}
		}
		if !hasPerm || strings.Contains(filepath.ToSlash(dir), "/common/") {
			return nil
		}
		packages++

		declared := map[string]bool{}
		called := map[string]string{}
		for _, f := range files {
			base := filepath.Base(f)
			if strings.HasSuffix(base, "_test.go") {
				continue
			}
			af, parseErr := parser.ParseFile(token.NewFileSet(), f, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				continue
			}
			ast.Inspect(af, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.BasicLit:
					if base == "permissions.go" && x.Kind == token.STRING {
						s, _ := strconv.Unquote(x.Value)
						if mp, ok := reg[s]; ok {
							for _, p := range mp.Scoped {
								declared[p] = true
							}
						}
					}
				case *ast.CallExpr:
					if base == "permissions.go" {
						return true
					}
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
						if _, ok := reg[sel.Sel.Name]; ok {
							called[sel.Sel.Name] = base
						}
					}
				}
				return true
			})
		}
		for m, file := range called {
			for _, p := range reg[m].Scoped {
				if !declared[p] {
					failures = append(failures, filepath.ToSlash(dir)+": "+file+" calls "+m+" needing "+p+", which permissions.go does not declare")
					break
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if packages < 100 {
		t.Fatalf("walked only %d permissions.go packages; the walk is not reaching the constructs", packages)
	}
	sort.Strings(failures)
	for _, f := range failures {
		t.Error(f)
	}
}
