// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import "fmt"

// DeviceType names which kind of device a construct groups, in the admin UI's
// terms, and selects the wording DeviceGroupAlternative renders.
type DeviceType int

const (
	// DeviceTypeComputer covers the two computer-group constructs.
	DeviceTypeComputer DeviceType = iota
	// DeviceTypeMobile covers the two mobile-device-group constructs.
	DeviceTypeMobile
)

// GroupKind names whether a construct manages a smart group or a static one.
type GroupKind int

const (
	// GroupKindSmart covers the two criteria-driven constructs.
	GroupKindSmart GroupKind = iota
	// GroupKindStatic covers the two membership-driven constructs.
	GroupKindStatic
)

// DeviceGroupAlternative returns the paragraph every construct in this family
// opens its description with: that jamfplatform_device_group is the one to reach
// for, and that Jamf Pro site support is the only reason to prefer this.
//
// It lives here, rendered from one template, so the advice cannot drift between
// sixteen schema descriptions or contradict itself between a resource and its own
// data source. It is deliberately the same sentence across the family rather than
// sixteen paraphrases of it.
func DeviceGroupAlternative(dt DeviceType, kind GroupKind) string {
	return fmt.Sprintf(
		"Use this only when the group must belong to a Jamf Pro site. For everything else prefer `jamfplatform_device_group` with `device_type = %q` and `group_type = %q`, which manages the same %s across Jamf Platform and is the supported path; it has no site attribute, which is the one thing this adds.",
		deviceTypeArgument(dt), groupKindArgument(kind), groupNoun(dt),
	)
}

// Label names one of these groups the way the Jamf Pro admin UI does — "smart
// computer group", "static mobile device group" — so a diagnostic or an impact
// alert reads the way Jamf Pro's own does.
func Label(dt DeviceType, kind GroupKind) string {
	return fmt.Sprintf("%s %s", groupKindArgument(kind), groupNoun(dt))
}

// MemberNoun returns what one of these groups contains, plural, in the admin UI's
// terms. Impact alerts count in these words.
func MemberNoun(dt DeviceType) string {
	if dt == DeviceTypeMobile {
		return "mobile devices"
	}
	return "computers"
}

// deviceTypeArgument is the device_type value jamfplatform_device_group expects
// for this device kind.
func deviceTypeArgument(dt DeviceType) string {
	if dt == DeviceTypeMobile {
		return "mobile"
	}
	return "computer"
}

// groupKindArgument is the group_type value jamfplatform_device_group expects for
// this group kind.
func groupKindArgument(kind GroupKind) string {
	if kind == GroupKindStatic {
		return "static"
	}
	return "smart"
}

// groupNoun names the object itself, singular, in the admin UI's terms.
func groupNoun(dt DeviceType) string {
	if dt == DeviceTypeMobile {
		return "mobile device group"
	}
	return "computer group"
}
