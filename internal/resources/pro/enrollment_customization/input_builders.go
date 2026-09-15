// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildParentInput converts the Terraform model into the SDK
// EnrollmentCustomizationV2 payload sent on Create / Update. The branding
// settings are required server-side (every Create POST with an empty
// branding object returns 400 with FIELD_REQUIRED on all five sub-fields),
// so a populated branding model is expected; an empty palette is a builder
// bug, not a user error.
//
// `iconURLOverride` lets the Create flow pass through the URL returned by an
// image upload step. When supplied (non-empty) it wins over the value the
// user typed in `branding_settings.icon_url`; otherwise the user value is
// used. Empty `iconUrl` is acceptable on the wire — the API only rejects
// missing keys, not empty strings.
//
// `site_id` defaults to the "-1" sentinel when the user omitted it; the API
// will normalise an empty string to the same value but the explicit sentinel
// makes the intent obvious.
func buildParentInput(plan EnrollmentCustomizationResourceModel, iconURLOverride string) *pro.EnrollmentCustomizationV2 {
	branding := plan.BrandingSettings
	iconURL := ""
	if iconURLOverride != "" {
		iconURL = iconURLOverride
	} else if branding != nil && !branding.IconURL.IsNull() && !branding.IconURL.IsUnknown() {
		iconURL = branding.IconURL.ValueString()
	}

	bs := pro.EnrollmentCustomizationBrandingSettings{
		IconURL: iconURL,
	}
	if branding != nil {
		bs.TextColor = branding.BodyTextColor.ValueString()
		bs.ButtonColor = branding.ButtonColor.ValueString()
		bs.ButtonTextColor = branding.ButtonTextColor.ValueString()
		bs.BackgroundColor = branding.BackgroundColor.ValueString()
	}

	siteID := siteIDNoneSentinel
	if !plan.SiteID.IsNull() && !plan.SiteID.IsUnknown() && plan.SiteID.ValueString() != "" {
		siteID = plan.SiteID.ValueString()
	}

	return &pro.EnrollmentCustomizationV2{
		DisplayName:                             plan.DisplayName.ValueString(),
		Description:                             plan.Description.ValueString(),
		SiteID:                                  siteID,
		EnrollmentCustomizationBrandingSettings: bs,
	}
}

// buildTextPanelInput converts a single textPaneModel into the SDK request
// type. Subtext is the only optional scalar; the request body uses *string
// with omitempty so omission round-trips cleanly.
func buildTextPanelInput(p textPaneModel) *pro.EnrollmentCustomizationPanelText {
	out := &pro.EnrollmentCustomizationPanelText{
		DisplayName:        p.DisplayName.ValueString(),
		Rank:               int(p.Rank.ValueInt64()),
		Title:              p.Title.ValueString(),
		Body:               p.Body.ValueString(),
		BackButtonText:     p.PreviousButtonText.ValueString(),
		ContinueButtonText: p.NextButtonText.ValueString(),
	}
	if !p.Subtext.IsNull() && !p.Subtext.IsUnknown() {
		v := p.Subtext.ValueString()
		out.Subtext = &v
	}
	return out
}

// buildLdapPanelInput converts a single ldapPaneModel into the SDK request
// type. The directory_service_groups list maps to ldapGroupAccess; an empty
// or absent list serialises as a nil slice so the wire omits the key
// (consistent with the wire-probe note that an empty array is also accepted).
func buildLdapPanelInput(p ldapPaneModel) *pro.EnrollmentCustomizationPanelLdapAuth {
	out := &pro.EnrollmentCustomizationPanelLdapAuth{
		DisplayName:        p.DisplayName.ValueString(),
		Rank:               int(p.Rank.ValueInt64()),
		Title:              p.Title.ValueString(),
		UsernameLabel:      p.UsernameText.ValueString(),
		PasswordLabel:      p.PasswordText.ValueString(),
		BackButtonText:     p.PreviousButtonText.ValueString(),
		ContinueButtonText: p.LoginButtonText.ValueString(),
	}
	if len(p.DirectoryServiceGroups) > 0 {
		groups := make([]pro.EnrollmentCustomizationLdapGroupAccess, 0, len(p.DirectoryServiceGroups))
		for _, g := range p.DirectoryServiceGroups {
			name := g.GroupName.ValueString()
			id := int(g.DirectoryServiceServerID.ValueInt64())
			groups = append(groups, pro.EnrollmentCustomizationLdapGroupAccess{
				GroupName:    &name,
				LdapServerID: &id,
			})
		}
		out.LdapGroupAccess = &groups
	}
	return out
}

// buildSsoPanelInput converts a single ssoPaneModel into the SDK request type.
// The `enrollment_access` enum maps to the boolean
// `isGroupEnrollmentAccessEnabled`: "specific_group" → true, "any_idp_user"
// → false. `access_group_name` is only meaningful when the flag is true; an
// empty string is sent otherwise so the API field is always present.
func buildSsoPanelInput(p ssoPaneModel) *pro.EnrollmentCustomizationPanelSsoAuth {
	specific := p.EnrollmentAccess.ValueString() == enrollmentAccessSpecificGroup
	groupName := ""
	if specific && !p.AccessGroupName.IsNull() && !p.AccessGroupName.IsUnknown() {
		groupName = p.AccessGroupName.ValueString()
	}
	return &pro.EnrollmentCustomizationPanelSsoAuth{
		DisplayName:                    p.DisplayName.ValueString(),
		Rank:                           int(p.Rank.ValueInt64()),
		IsGroupEnrollmentAccessEnabled: specific,
		GroupEnrollmentAccessName:      groupName,
		IsUseJamfConnect:               p.PassUserInfoToJamfConnect.ValueBool(),
		ShortNameAttribute:             p.AccountNameAttribute.ValueString(),
		LongNameAttribute:              p.AccountFullNameAttribute.ValueString(),
	}
}
