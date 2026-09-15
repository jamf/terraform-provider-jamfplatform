// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestVerifySSODomainSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestVerifySSODomainSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(account.Privileges, verifySSODomainSDKMethods...); len(missing) > 0 {
		t.Fatalf("verifySSODomainSDKMethods not present in account.Privileges (SDK drift): %v", missing)
	}
}

// TestVerifySSODomainSDKMethods_MatchInvokeCalls fails if the action calls an SDK
// method not declared in verifySSODomainSDKMethods, or declares one it does not
// call — keeping the privileges table honest as the Invoke path changes.
func TestVerifySSODomainSDKMethods_MatchInvokeCalls(t *testing.T) {
	var src []byte
	for _, name := range []string{"verify.go", "helpers.go"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		src = append(src, b...)
	}

	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if _, ok := account.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	declared := map[string]bool{}
	for _, m := range verifySSODomainSDKMethods {
		declared[m] = true
	}

	var undeclared, uncalled []string
	for m := range called {
		if !declared[m] {
			undeclared = append(undeclared, m)
		}
	}
	for m := range declared {
		if !called[m] {
			uncalled = append(uncalled, m)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("the action calls SDK methods missing from verifySSODomainSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("verifySSODomainSDKMethods declares methods the action does not call: %v", uncalled)
	}
}

// TestVerifySSODomainPrivileges_Rendered guards that the table actually rendered
// into the action description, and that it names both permissions.
//
// The read matters as much as the update: it is the lookup a domain named by name
// goes through, which is the form the documentation leads with, so a table omitting
// it would send an operator to create an integration that fails on the common case.
func TestVerifySSODomainPrivileges_Rendered(t *testing.T) {
	for _, scoped := range []string{"sso-domains:update", "sso-domains:read"} {
		if !permissions.Renders(verifySSODomainPrivileges, scoped) {
			t.Errorf("verifySSODomainPrivileges did not render %s:\n%s", scoped, verifySSODomainPrivileges)
		}
	}
}
