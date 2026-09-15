// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// siteIDNoneSentinel is the value Jamf Pro reports when a customization is
// not associated with a site.
const siteIDNoneSentinel = "-1"

// Panel type discriminators used on the wire (lowercased exactly as Jamf Pro
// emits them).
const (
	panelTypeText = "text"
	panelTypeLdap = "ldap"
	panelTypeSso  = "sso"
)

// panelIndex groups the list of panels returned by the parent endpoint by
// type. Used to drive the diff between plan and state during Update and to
// hydrate state during Read.
type panelIndex struct {
	Text []pro.GetEnrollmentCustomizationPanel
	Ldap []pro.GetEnrollmentCustomizationPanel
	Sso  []pro.GetEnrollmentCustomizationPanel
}

// buildPanelIndex groups the panels by type. Unknown types are dropped — the
// SDK enumerates the three known kinds and any future addition needs explicit
// schema support before it can round-trip safely.
func buildPanelIndex(panels []pro.GetEnrollmentCustomizationPanel) panelIndex {
	var idx panelIndex
	for _, p := range panels {
		switch p.Type {
		case panelTypeText:
			idx.Text = append(idx.Text, p)
		case panelTypeLdap:
			idx.Ldap = append(idx.Ldap, p)
		case panelTypeSso:
			idx.Sso = append(idx.Sso, p)
		}
	}
	return idx
}
