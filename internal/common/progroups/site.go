// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// NoSiteID is the sentinel Jamf Pro stores and returns for a group that belongs
// to no site. It is the schema default for site_id, so an operator who says
// nothing about sites gets the same group they would from
// jamfplatform_device_group.
const NoSiteID = "-1"

// SiteIDForWrite returns the site id to put on a write, always non-nil.
//
// Every write in this family emits siteId unconditionally. Two independent
// reasons, and either alone would be enough: a computer write that omits it
// CLEARS the group's site, because the endpoints full-replace their scalars; and
// a mobile write that omits it is refused outright with 403 INVALID_PRIVILEGE.
// So there is no "leave the site alone" body to send, and the plan value —
// defaulted to NoSiteID — is the whole truth about the group's site.
//
// An Unknown or Null plan value collapses to NoSiteID rather than to the empty
// string, which Jamf Pro does not accept as a site reference.
func SiteIDForWrite(planned types.String) *string {
	out := NoSiteID
	if helpers.IsConfiguredValue(planned) && planned.ValueString() != "" {
		out = planned.ValueString()
	}
	return &out
}

// SiteIDForState normalises a site id read back from Jamf Pro into the single
// user-facing sentinel.
//
// The endpoints are not consistent about how they say "no site": the computer
// reads echo the string "-1", a PUT response echoes JSON null for the same group,
// and the classic read reports id -1 with name "NONE". Collapsing every one of
// them to NoSiteID keeps state stable across refreshes and keeps a group with no
// site from flipping between "-1" and null, which would surface as an
// ImportStateVerify difference or a post-apply inconsistency on the no-site path
// only — the shape STYLE_GUIDE §Server-derived computed fields warns reads as a
// flake.
func SiteIDForState(wire *string) types.String {
	if wire == nil || *wire == "" {
		return types.StringValue(NoSiteID)
	}
	return types.StringValue(*wire)
}
