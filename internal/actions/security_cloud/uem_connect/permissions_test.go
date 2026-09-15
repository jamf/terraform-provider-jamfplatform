// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// TestSynchronizeSDKMethods_KnownToSDK fails if a declared method has been
// renamed or removed in the SDK privilege registry.
func TestSynchronizeSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, synchronizeSDKMethods...); len(missing) > 0 {
		t.Fatalf("synchronizeSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestSynchronizeSDKMethods_MatchReadCalls fails if synchronize.go calls an SDK
// method not declared in synchronizeSDKMethods, or declares one it does not
// call — keeping the privileges table honest as the Read path changes.
func TestSynchronizeSDKMethods_MatchReadCalls(t *testing.T) {
	// helpers.go holds the list read the ID-less form falls back to, so both files
	// count toward the declared set.
	var src []byte
	for _, name := range []string{"synchronize.go", "helpers.go"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		src = append(src, b...)
	}
	// The action reaches the client through an embedded struct, so the calls read
	// as a.client.<Method> rather than client.<Method>.
	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		// Only count calls that resolve to a known SDK method so that
		// unrelated client.<helper> calls do not pollute the comparison.
		if _, ok := securitycloud.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	declared := map[string]bool{}
	for _, m := range synchronizeSDKMethods {
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
		t.Errorf("synchronize.go calls SDK methods missing from synchronizeSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("synchronizeSDKMethods declares methods synchronize.go does not call: %v", uncalled)
	}
}

// TestSynchronizePrivileges_Rendered is a guard that the table actually rendered
// into the data source description (catches an empty/parse-skipped registry).
func TestSynchronizePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(synchronizePrivileges, "uem-connect:update") {
		t.Fatalf("synchronizePrivileges did not render the devices privileges:\n%s", synchronizePrivileges)
	}
}

// TestDeployActivationProfileSDKMethods_KnownToSDK fails if the declared method has
// been renamed or removed in the SDK privilege registry.
func TestDeployActivationProfileSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, deployActivationProfileSDKMethods...); len(missing) > 0 {
		t.Fatalf("deployActivationProfileSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestDeployActivationProfileSDKMethods_MatchInvokeCalls fails if
// deploy_activation_profile.go calls an SDK method not declared, or declares one it
// does not call.
//
// Only that one file counts, unlike synchronize: this action needs no connector
// lookup, so it reaches nothing in helpers.go. If that changes, the declared list
// has to grow — a caller granted only the update privilege would otherwise fail on
// the read.
func TestDeployActivationProfileSDKMethods_MatchInvokeCalls(t *testing.T) {
	src, err := os.ReadFile("deploy_activation_profile.go")
	if err != nil {
		t.Fatalf("reading deploy_activation_profile.go: %v", err)
	}

	re := regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)
	called := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if _, ok := securitycloud.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	declared := map[string]bool{}
	for _, m := range deployActivationProfileSDKMethods {
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
		t.Errorf("deploy_activation_profile.go calls SDK methods missing from deployActivationProfileSDKMethods: %v", undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("deployActivationProfileSDKMethods declares methods deploy_activation_profile.go does not call: %v", uncalled)
	}
}

// TestDeployActivationProfilePrivileges_Rendered guards that the table actually
// rendered into the action description.
func TestDeployActivationProfilePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(deployActivationProfilePrivileges, "uem-connect:update") {
		t.Fatalf("deployActivationProfilePrivileges did not render the UEM Connect privileges:\n%s", deployActivationProfilePrivileges)
	}
}
