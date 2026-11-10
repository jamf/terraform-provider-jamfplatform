// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package permissions

import (
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestSectionWith_AppendsConditionalTable(t *testing.T) {
	h := Helper{When: "a thing is configured", Registry: pro.Privileges, Methods: []string{"SearchLdapGroupsV1"}}
	got := SectionWith(pro.Privileges, []string{"GetCategoryV1"}, h)

	if !strings.Contains(got, "**Conditional permissions**") || !strings.Contains(got, "Also grant these when a thing is configured:") {
		t.Fatalf("conditional heading missing:\n%s", got)
	}
	cond := got[strings.Index(got, "**Conditional permissions**"):]
	if !Renders(cond, "ldap-servers:read") {
		t.Fatalf("conditional table does not tick ldap-servers:read:\n%s", cond)
	}
}

func TestSectionWith_DropsPrivilegesTheConstructAlreadyHolds(t *testing.T) {
	h := Helper{When: "x", Registry: pro.Privileges, Methods: []string{"SearchLdapGroupsV1"}}
	got := SectionWith(pro.Privileges, []string{"SearchLdapGroupsV1"}, h)
	if strings.Contains(got, "Conditional permissions") {
		t.Fatalf("a privilege the construct already declares must not repeat:\n%s", got)
	}
	if got != Section(pro.Privileges, "SearchLdapGroupsV1") {
		t.Fatal("with nothing to add, SectionWith must equal Section")
	}
}
