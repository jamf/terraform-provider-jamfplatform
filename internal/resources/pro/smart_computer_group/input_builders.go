// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// buildSmartComputerGroupInput maps the plan onto the smart computer group write
// body, for both the create and the update.
//
// Every scalar is emitted on every write. The endpoint full-replaces, so a body
// omitting description clears it to the empty string and one omitting siteId
// clears the site, which means there is no "leave this alone" body to send and
// the plan is the whole truth about both fields.
//
// Criteria come from the shared builder, which is always non-nil: an empty slice
// is how the resource removes every criterion, and the endpoint treats it the
// same way as omitting the key.
func buildSmartComputerGroupInput(plan SmartComputerGroupResourceModel) *pro.SmartComputerGroupV3 {
	description := descriptionForWrite(plan.Description)
	built := criteria.BuildComputerSmartGroupCriteria(plan.Criteria)
	return &pro.SmartComputerGroupV3{
		Name:        plan.Name.ValueString(),
		Description: &description,
		SiteID:      progroups.SiteIDForWrite(plan.SiteID),
		Criteria:    &built,
	}
}

// descriptionForWrite collapses an unset description to the empty string, which
// is what Jamf Pro stores for a group without one. An Unknown value reaches here
// only on the first create of a configuration that says nothing about the
// description, which is the same intent.
func descriptionForWrite(planned types.String) string {
	if helpers.IsConfiguredValue(planned) {
		return planned.ValueString()
	}
	return ""
}

// buildClassicSmartComputerGroupInput maps the plan onto the body Jamf Pro's
// older group interface takes, which is what a write refused by the group
// endpoints is repeated through.
//
// Name, site and criteria are all emitted, and all three travel in this one
// body. Splitting the criteria across two writes is the mistake to avoid:
// membership is recalculated synchronously with the write, so an intermediate
// group is immediately live and everything scoped to it acts on that half-built
// membership — and under the load that makes this workaround worth having the
// window is wider, not narrower.
//
// There is no description field on this interface at all, and none is needed: a
// write here preserves a description it cannot express, so the caller sets one
// through the separate merge patch instead.
//
// Computers stays nil. A smart group has no membership payload and sending one
// answers 409 "Unable to match computer".
func buildClassicSmartComputerGroupInput(plan SmartComputerGroupResourceModel) *proclassic.ComputerGroupPost {
	name := plan.Name.ValueString()
	isSmart := true
	built := criteria.BuildCriterionSlice(plan.Criteria)
	return &proclassic.ComputerGroupPost{
		Name:     &name,
		IsSmart:  &isSmart,
		Site:     classicSiteObject(plan.SiteID),
		Criteria: &proclassic.ComputerGroupPostCriteria{Criterion: &built},
	}
}

// classicSiteObject renders the site for the older interface, always non-nil.
//
// The site travels as a number there rather than a string, and the no-site
// sentinel is -1 on both interfaces, so progroups.SiteIDForWrite converts
// straight across and the always-emit rule holds on both paths. A value that is
// not a number collapses to the no-site sentinel, which no configuration Jamf
// Pro would have accepted can reach: site_id defaults to the sentinel, and a
// non-numeric id is refused on the group endpoints before the criteria the
// fallback exists for are ever reached.
func classicSiteObject(planned types.String) *proclassic.SiteObject {
	id, err := strconv.Atoi(*progroups.SiteIDForWrite(planned))
	if err != nil {
		id, _ = strconv.Atoi(progroups.NoSiteID)
	}
	return &proclassic.SiteObject{ID: &id}
}

// buildDescriptionPatch is the body of the write that sets a group's description
// on its own, after a fallback write that could not carry it.
//
// It carries the description and NOTHING else. The same request accepts criteria
// and a body including them would re-run the very validation the fallback exists
// to get around, undoing the write it is meant to complete.
func buildDescriptionPatch(planned types.String) *pro.GroupUpdateDtoV2 {
	description := descriptionForWrite(planned)
	return &pro.GroupUpdateDtoV2{GroupDescription: &description}
}

// descriptionNeedsWrite reports whether a fallback update has to issue the
// separate description write at all.
//
// The older interface preserves a description it cannot express, so an update
// that does not change it needs no second request and an ordinary criteria-only
// edit stays a single write. Both sides collapse through descriptionForWrite, so
// an attribute dropped from a configuration that had stored the empty string is
// not a change.
func descriptionNeedsWrite(planned, prior types.String) bool {
	return descriptionForWrite(planned) != descriptionForWrite(prior)
}
