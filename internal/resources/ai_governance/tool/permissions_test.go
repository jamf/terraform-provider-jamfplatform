// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tool

import (
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form d.client.<Method>(.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

// assertMatch fails if the SDK methods a file calls and the declared list differ.
func assertMatch(t *testing.T, filename string, declared []string) {
	t.Helper()
	src, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	called := map[string]bool{}
	for _, match := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := aigovernance.Privileges[match[1]]; ok {
			called[match[1]] = true
		}
	}
	want := map[string]bool{}
	for _, method := range declared {
		want[method] = true
	}

	var undeclared, uncalled []string
	for method := range called {
		if !want[method] {
			undeclared = append(undeclared, method)
		}
	}
	for method := range want {
		if !called[method] {
			uncalled = append(uncalled, method)
		}
	}
	sort.Strings(undeclared)
	sort.Strings(uncalled)

	if len(undeclared) > 0 {
		t.Errorf("%s calls SDK methods missing from the declared list: %v", filename, undeclared)
	}
	if len(uncalled) > 0 {
		t.Errorf("declared list has methods %s does not call: %v", filename, uncalled)
	}
}

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(aigovernance.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in aigovernance.Privileges (SDK drift): %v", missing)
	}
}

func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "data_source.go", dataSourceSDKMethods)
}

func TestPluralDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(aigovernance.Privileges, pluralDataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("pluralDataSourceSDKMethods not present in aigovernance.Privileges (SDK drift): %v", missing)
	}
}

func TestPluralDataSourceSDKMethods_MatchCalls(t *testing.T) {
	assertMatch(t, "datasource_plural.go", pluralDataSourceSDKMethods)
}

func TestPrivileges_Rendered(t *testing.T) {
	for name, rendered := range map[string]string{
		"dataSourcePrivileges":       dataSourcePrivileges,
		"pluralDataSourcePrivileges": pluralDataSourcePrivileges,
	} {
		if !permissions.Renders(rendered, "ai-policies:read") {
			t.Errorf("%s did not render the read privilege:\n%s", name, rendered)
		}
	}
}
