// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldapgroups

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// Permissions is the privilege a construct needs when its configuration names a
// directory group. Resolve, ResolveByName and Validate return the search error
// unchanged, so a missing privilege fails the plan or apply. Every construct
// importing this package renders it through permissions.SectionWith;
// internal/conformance enforces that.
var Permissions = permissions.Helper{
	When:     "the configuration names a directory (LDAP or cloud identity provider) group",
	Registry: pro.Privileges,
	Methods:  []string{"SearchLdapGroupsV1"},
}
