// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_predefined_apps

import (
	"os"
	"regexp"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// clientCallRe matches SDK client method calls of the form d.client.<Method>(.
var clientCallRe = regexp.MustCompile(`\bclient\.([A-Za-z0-9]+)\(`)

func TestDataSourceSDKMethods_KnownToSDK(t *testing.T) {
	if missing := permissions.Missing(securitycloud.Privileges, dataSourceSDKMethods...); len(missing) > 0 {
		t.Fatalf("dataSourceSDKMethods not present in securitycloud.Privileges (SDK drift): %v", missing)
	}
}

// TestDataSourceSDKMethods_MatchCalls keeps the privileges table honest as the
// read path changes.
func TestDataSourceSDKMethods_MatchCalls(t *testing.T) {
	src, err := os.ReadFile("data_source.go")
	if err != nil {
		t.Fatalf("reading data_source.go: %v", err)
	}
	called := map[string]bool{}
	for _, m := range clientCallRe.FindAllStringSubmatch(string(src), -1) {
		if _, ok := securitycloud.Privileges[m[1]]; ok {
			called[m[1]] = true
		}
	}
	for _, declared := range dataSourceSDKMethods {
		if !called[declared] {
			t.Errorf("declared list has method data_source.go does not call: %s", declared)
		}
		delete(called, declared)
	}
	for m := range called {
		t.Errorf("data_source.go calls an SDK method missing from the declared list: %s", m)
	}
}

func TestDataSourcePrivileges_Rendered(t *testing.T) {
	if !permissions.Renders(dataSourcePrivileges, "ztna:read") {
		t.Fatalf("dataSourcePrivileges did not render the predefined app privileges:\n%s", dataSourcePrivileges)
	}
}
