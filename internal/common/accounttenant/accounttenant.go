// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package accounttenant holds the helpers the Jamf Account platform environment
// constructs share for reading a tenant list.
//
// A platform environment object carries no list of its tenants: membership is
// recorded on the tenant, as the identifier of its environment. Both the
// platform environment resource, which must refuse to delete one that still
// holds tenants, and the assign tenants action, which must know what is already
// in one, therefore answer the same question from the same tenant list.
package accounttenant

import (
	"fmt"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// InEnvironment returns the tenants whose platform environment is id, in the
// order given.
func InEnvironment(tenants []account.Tenant, id string) []account.Tenant {
	var held []account.Tenant
	for _, t := range tenants {
		if t.EnvironmentID != nil && *t.EnvironmentID == id {
			held = append(held, t)
		}
	}
	return held
}

// Describe renders a tenant for a diagnostic: its name where Jamf Account holds
// one, then its identifier and product.
func Describe(t account.Tenant) string {
	name := "(unnamed)"
	if t.Name != nil && *t.Name != "" {
		name = *t.Name
	}
	product := ""
	if t.Product != nil && *t.Product != "" {
		product = ", " + *t.Product
	}
	return fmt.Sprintf("%s (%s%s)", name, t.ID, product)
}
