// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import "github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"

// groupLabel names this object the way the Jamf Pro admin UI does, so a
// diagnostic and an impact alert read the way Jamf Pro's own do. It comes from
// the shared renderer rather than a literal, so the four constructs in this
// family cannot describe themselves differently.
var groupLabel = progroups.Label(progroups.DeviceTypeComputer, progroups.GroupKindStatic)

// memberNoun is what this group contains, plural, in the admin UI's terms.
// Impact alerts count in these words.
var memberNoun = progroups.MemberNoun(progroups.DeviceTypeComputer)

// StaticComputerGroupFilterSelectors enumerates the fields the static computer
// group search accepts in a filter clause.
//
// These are the selectors the endpoint itself documents and they pass through
// verbatim, so they keep their API spelling rather than the snake_case every
// Terraform attribute name uses (STYLE_GUIDE §Schema Guidelines exempts RSQL
// selectors). A sited API integration has siteId applied for it whether or not
// the clause is present.
var StaticComputerGroupFilterSelectors = []string{
	"id",
	"name",
	"siteId",
}
