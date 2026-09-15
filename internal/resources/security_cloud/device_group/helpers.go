// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// appendWriteDiagnostics turns a create/update failure into the most specific
// diagnostic the error body supports, and reports whether it recognised one.
//
// Only codes whose remedy is not obvious from the server's own wording are
// translated. GROUP_ALREADY_EXISTS earns one because the message says a name is
// taken without saying that the comparison is exact, so an operator who has
// already checked the UI for "Example" will not think to look for "example".
// RESERVED_GROUP_NAME earns one because the built-in group is not manageable at
// all, which the message does not say. INVALID_FIELD earns one because the
// server's "must not be blank" is reached by a whitespace-only name too.
//
// BAD_PERMISSIONS is deliberately absent — see mappings.go for why translating it
// would be a guess.
func appendWriteDiagnostics(diags *diag.Diagnostics, err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil {
		return false
	}
	matched := false
	for _, detail := range apiErr.Details() {
		switch detail.Code {
		case codeGroupAlreadyExists:
			diags.AddAttributeError(
				path.Root("name"),
				"Device group name already in use",
				"Jamf Security Cloud requires device group names to be unique on the tenant. The comparison is "+
					"exact, so a group differing only in capitalisation still counts as a different name and is not "+
					"the one clashing here. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeReservedGroupName:
			diags.AddAttributeError(
				path.Root("name"),
				"Device group name is reserved",
				"Jamf Security Cloud reserves this name for the built-in group every tenant carries, and matches it "+
					"regardless of capitalisation and surrounding whitespace. That group cannot be managed by "+
					"Terraform — it has no identifier — so pick a different name. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		case codeInvalidField:
			diags.AddAttributeError(
				path.Root("name"),
				"Device group name not accepted",
				"Jamf Security Cloud rejected this group name. A name that is empty, or made up only of whitespace, "+
					"counts as blank. Reported by Jamf Security Cloud: "+detail.Description,
			)
		case codeNotEntitled:
			diags.AddError(
				"Tenant not entitled to Jamf Security Cloud device groups",
				"The credentials authenticated successfully but this tenant does not have the device groups surface "+
					"enabled. Contact Jamf to have it provisioned. Reported by Jamf Security Cloud: "+
					detail.Description,
			)
		default:
			continue
		}
		matched = true
	}
	return matched
}

// groupsNamedExactly returns every group list entry whose name matches name
// exactly.
//
// The comparison is exact because Jamf Security Cloud's own uniqueness check is:
// "Example" and "example" are two different groups, so folding case here could
// hand back the wrong one.
//
// This exists instead of the SDK's ResolveDeviceGroupV2ByName, which cannot be
// used for the singular data source's name lookup. Where the match is the
// implicit "Default Group" — the one entry the list returns with no id key — the
// SDK resolver fails with "matched element has no id field" and discards the
// matched element, so a caller can never see the id-less entry in order to refuse
// it by name. Matching locally over the same list keeps that case reachable.
//
// The caller distinguishes the outcomes: no elements is not-found, one element is
// the answer, and more than one is a name the server should have refused to store
// twice.
func groupsNamedExactly(items []securitycloud.GroupListItem, name string) []securitycloud.GroupListItem {
	matches := make([]securitycloud.GroupListItem, 0, 1)
	for _, item := range items {
		if item.Name == name {
			matches = append(matches, item)
		}
	}
	return matches
}

// sortGroupsByName returns items ordered by name, so the plural data source and
// the list resource can promise an order instead of describing one they observed.
//
// The group list endpoint accepts no parameters at all — not even a sort, which is
// what dns_zone passes to pin its own order — so the server's ordering is
// undocumented and unowned. It has been observed to come back name-ascending, but
// an observation is not a contract, and `device_groups[0].id` is a reasonable thing
// to write against a documented order: were the server to switch to insertion
// order, creating a group in the admin UI would silently retarget that reference
// with no diff to warn on. Sorting here makes the documented behaviour the
// provider's own guarantee rather than the server's habit.
//
// The comparison is bytewise for the same reason the name lookup is: uniqueness on
// this API is case-sensitive, so folding case would order two genuinely different
// groups arbitrarily against each other. The sort is stable, so the built-in group
// keeps its position among any same-named entries rather than moving between runs.
func sortGroupsByName(items []securitycloud.GroupListItem) []securitycloud.GroupListItem {
	sorted := slices.Clone(items)
	slices.SortStableFunc(sorted, func(a, b securitycloud.GroupListItem) int {
		return strings.Compare(a.Name, b.Name)
	})
	return sorted
}
