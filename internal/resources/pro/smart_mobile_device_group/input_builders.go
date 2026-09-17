// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// buildSmartMobileDeviceGroupInput converts a plan into the body both the create
// and the update take. One builder serves both, because the endpoint
// full-replaces every scalar it carries and so there is no partial body to
// build: an omitted description clears it, and an omitted site clears the site
// on the computer endpoints and is refused outright here.
//
// Three fields are therefore always emitted. description comes straight from the
// plan rather than through an optional-pointer helper, because a null plan value
// and an empty string mean the same thing to this endpoint and the schema
// defaults the attribute from the server. siteId comes from
// progroups.SiteIDForWrite, which is never nil. criteria comes from the shared
// builder, which returns an empty slice rather than nil for empty input, so
// removing every criterion sends the clear rather than the retain.
//
// groupId is left unset. The create allocates it and the update takes it in the
// path, so a body carrying it would either be ignored or invite a mismatch.
func buildSmartMobileDeviceGroupInput(plan SmartMobileDeviceGroupResourceModel) *pro.SmartGroupAssignmentV2 {
	description := descriptionForWrite(plan.Description)
	groupCriteria := criteria.BuildMobileDeviceSmartGroupCriteria(plan.Criteria)

	return &pro.SmartGroupAssignmentV2{
		GroupName:        plan.Name.ValueString(),
		GroupDescription: &description,
		SiteID:           progroups.SiteIDForWrite(plan.SiteID),
		Criteria:         &groupCriteria,
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

// buildClassicSmartMobileDeviceGroupInput maps the plan onto the body Jamf Pro's
// older group interface takes, which is what a write the group endpoint refused
// is repeated through.
//
// Name, site and criteria all travel in this one body, and splitting the
// criteria across two writes is the mistake this exists to prevent: Jamf Pro
// recalculates membership synchronously with the write, so a half-written group
// is immediately live and everything scoped to it acts on that membership. Under
// the load that makes the workaround worth having, the window is wider rather
// than narrower.
//
// The interface carries no description field of any kind, and none is wanted
// here: a write through it preserves a description it cannot express, so the
// caller sets one through the separate merge patch instead.
//
// MobileDevices stays nil. A smart group's members are Jamf Pro's to compute
// from the criteria, so a membership collection on this body would be an
// instruction about something the group does not have.
//
// One type serves both the create and the update, unlike the computer interface,
// which has a separate create shape. Its fields are all pointers, so every one
// of them is emitted by being set rather than by being non-zero.
func buildClassicSmartMobileDeviceGroupInput(plan SmartMobileDeviceGroupResourceModel) *proclassic.MobileDeviceGroup {
	name := plan.Name.ValueString()
	isSmart := true
	built := criteria.BuildCriterionSlice(plan.Criteria)

	return &proclassic.MobileDeviceGroup{
		Name:     &name,
		IsSmart:  &isSmart,
		Site:     classicSiteObject(plan.SiteID),
		Criteria: &proclassic.MobileDeviceGroupCriteria{Criterion: &built},
	}
}

// classicSiteObject renders the site for the older interface, always non-nil.
//
// The site travels as a number there rather than as a string, and the no-site
// sentinel is -1 on both interfaces, so progroups.SiteIDForWrite converts
// straight across and the always-emit rule holds on both paths. A value that is
// not a number collapses to the no-site sentinel, which no configuration Jamf
// Pro would have accepted can reach: site_id defaults to the sentinel, and a
// site the integration cannot use is refused on the group endpoint as a site
// refusal rather than as the criterion refusal the fallback triggers on.
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
// It carries the description and NOTHING else. The same request accepts criteria,
// and a body including them would put the refused criteria back through the
// endpoint that refuses them, undoing the write it is there to complete. It
// takes the description rather than the whole plan so that there is nothing else
// in scope to send.
func buildDescriptionPatch(planned types.String) *pro.GroupUpdateDtoV2 {
	description := descriptionForWrite(planned)
	return &pro.GroupUpdateDtoV2{GroupDescription: &description}
}

// descriptionNeedsWrite reports whether a fallback write has to issue the
// separate description write at all.
//
// The older interface preserves a description it cannot express, so an update
// that does not change the description needs no second request, and an ordinary
// criteria-only edit stays a single write. Both sides collapse through
// descriptionForWrite, so an attribute dropped from a configuration that had
// stored the empty string is not a change. A create compares against a null
// prior, so a configured description is always written and an absent one never
// is.
func descriptionNeedsWrite(planned, prior types.String) bool {
	return descriptionForWrite(planned) != descriptionForWrite(prior)
}
