// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldapgroups

import (
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestPermissionsMatchCalls fails if the package calls an SDK method Permissions
// does not declare, or declares one it does not call, across every non-test file.
func TestPermissionsMatchCalls(t *testing.T) {
	if missing := permissions.Missing(Permissions.Registry, Permissions.Methods...); len(missing) > 0 {
		t.Fatalf("Permissions.Methods not in the SDK registry: %v", missing)
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	called := map[string]bool{}
	for _, f := range files {
		if len(f) > 8 && f[len(f)-8:] == "_test.go" || f == "permissions.go" {
			continue
		}
		m, err := permissions.SDKCallsInFile(f, Permissions.Registry)
		if err != nil {
			t.Fatal(err)
		}
		for k := range m {
			called[k] = true
		}
	}
	var got []string
	for k := range called {
		got = append(got, k)
	}
	want := slices.Clone(Permissions.Methods)
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("package calls %v, Permissions declares %v", got, want)
	}
}
