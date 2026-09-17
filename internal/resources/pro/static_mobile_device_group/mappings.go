// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

// FilterSelectors is the selector vocabulary the static mobile device group
// search accepts, spelled as the query language expects rather than as the
// Terraform attributes are named. Three fields are filterable; a selector
// outside this set is refused, so it is validated at plan time.
//
// A sited administrator has the site selector applied for them whether they ask
// for it or not, so a filter on it narrows within what they can already see.
var FilterSelectors = []string{
	"groupId",
	"groupName",
	"siteId",
}
