// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accounttenant"
)

// maxListedTenants bounds how many tenants a diagnostic names, so an environment
// holding a long tail does not produce a wall of text.
const maxListedTenants = 10

// findEnvironmentByID returns the environment with the given identifier, or nil.
// Identifiers are opaque strings, so the comparison is exact.
func findEnvironmentByID(environments []account.Environment, id string) *account.Environment {
	for i := range environments {
		if environments[i].ID == id {
			return &environments[i]
		}
	}
	return nil
}

// findEnvironmentsByName returns every environment carrying the given name.
// Names are not unique, which is why this returns a slice, and the match is
// exact because a name is free text.
func findEnvironmentsByName(environments []account.Environment, name string) []account.Environment {
	var matches []account.Environment
	for _, e := range environments {
		if e.Name == name {
			matches = append(matches, e)
		}
	}
	return matches
}

// appendHeldTenantsDiagnostics refuses to delete an environment that still holds
// tenants, and reports whether it did.
//
// Jamf Account does not delete such an environment, and no call takes a tenant
// out of one. The refusal is made from the tenant list before any request, so it
// needs no knowledge of the code Jamf Account answers a blocked delete with,
// which was never probed because producing it would have meant assigning a real
// tenant, and an assignment cannot be undone.
func appendHeldTenantsDiagnostics(diags *diag.Diagnostics, environment string, held []account.Tenant) bool {
	if len(held) == 0 {
		return false
	}
	listed := make([]string, 0, maxListedTenants)
	for i, t := range held {
		if i == maxListedTenants {
			listed = append(listed, fmt.Sprintf("and %d more", len(held)-maxListedTenants))
			break
		}
		listed = append(listed, accounttenant.Describe(t))
	}
	diags.AddError(
		"Platform environment still holds tenants",
		fmt.Sprintf(
			"The platform environment \"%s\" holds %d tenant(s): %s. Jamf Account does not delete a platform "+
				"environment that holds tenants, and the provider cannot take tenants out of one. Run "+
				"`terraform state rm` on this address to stop managing the environment, or leave it in place.",
			environment, len(held), strings.Join(listed, "; "),
		),
	)
	return true
}

// appendDuplicateNameDiagnostics refuses a name lookup that matches more than one
// platform environment, naming every identifier so the practitioner can look the
// right one up by `id` instead.
func appendDuplicateNameDiagnostics(diags *diag.Diagnostics, name string, matches []account.Environment) {
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.ID
	}
	diags.AddAttributeError(
		path.Root("name"),
		"More than one platform environment has this name",
		fmt.Sprintf(
			"%d platform environments are named \"%s\" (ids: %s). Jamf Account does not require names to be "+
				"unique. Look the one you want up by `id` instead.",
			len(matches), name, strings.Join(ids, ", "),
		),
	)
}
