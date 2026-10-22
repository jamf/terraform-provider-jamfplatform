// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platformenvironmentaction

import (
	"fmt"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accounttenant"
)

// assignmentPlan is what the pre-check concluded about a requested assignment.
type assignmentPlan struct {
	toAssign  []account.Tenant
	alreadyIn []account.Tenant
	problems  []string
}

// planAssignment sorts the requested tenant identifiers into those to send,
// those already in the platform environment, and the reasons any cannot join.
//
// Jamf Account refuses a bad assignment without saying which tenant was at fault
// (400 UNKNOWN_TENANT names an id, the other refusals name nothing), so every
// rule it enforces is checked here against the tenant list and reported per
// tenant. A tenant already in the target is skipped rather than refused, because
// the server accepts only unassigned tenants and a repeat invocation must not
// fail. One in a different platform environment is refused. The region rule
// compares only when both sides report one: a global platform environment
// reports none and accepts any region, and a tenant reporting none cannot be
// compared, so the server is left to decide that case.
func planAssignment(environment *account.Environment, tenants []account.Tenant, requested []string) assignmentPlan {
	byID := make(map[string]account.Tenant, len(tenants))
	for _, t := range tenants {
		byID[t.ID] = t
	}

	var plan assignmentPlan
	for _, id := range requested {
		tenant, ok := byID[id]
		if !ok {
			plan.problems = append(plan.problems, fmt.Sprintf("%s: not a tenant of this organization", id))
			continue
		}
		if tenant.EnvironmentID != nil && *tenant.EnvironmentID != "" {
			if *tenant.EnvironmentID == environment.ID {
				plan.alreadyIn = append(plan.alreadyIn, tenant)
				continue
			}
			plan.problems = append(plan.problems, fmt.Sprintf("%s: already in another platform environment (%s)",
				accounttenant.Describe(tenant), *tenant.EnvironmentID))
			continue
		}
		if environment.MetaRegion != nil && tenant.MetaRegion != nil && *environment.MetaRegion != *tenant.MetaRegion {
			plan.problems = append(plan.problems, fmt.Sprintf("%s: hosted in %s, but the platform environment is pinned to %s",
				accounttenant.Describe(tenant), *tenant.MetaRegion, *environment.MetaRegion))
			continue
		}
		plan.toAssign = append(plan.toAssign, tenant)
	}
	return plan
}

// notInEnvironment returns the identifiers among ids whose tenant does not
// report environmentID in the given tenant list.
func notInEnvironment(tenants []account.Tenant, environmentID string, ids []string) []string {
	in := make(map[string]bool)
	for _, t := range accounttenant.InEnvironment(tenants, environmentID) {
		in[t.ID] = true
	}
	var missing []string
	for _, id := range ids {
		if !in[id] {
			missing = append(missing, id)
		}
	}
	return missing
}
