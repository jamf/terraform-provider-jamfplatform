// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignParentToResource copies the parent record fields from the API into
// the supplied resource model. `id` and `branding_settings` are always
// populated from the server; `icon_source` and `icon_source_hash` are
// preserved from prior state by the framework's `UseStateForUnknown` plan
// modifier (the API does not echo back the upload source path or its hash).
func assignParentToResource(state *EnrollmentCustomizationResourceModel, p *pro.EnrollmentCustomizationV2) {
	if p == nil {
		return
	}
	if p.ID != nil {
		state.ID = types.StringValue(*p.ID)
	}
	state.DisplayName = types.StringValue(p.DisplayName)
	state.Description = types.StringValue(p.Description)
	state.SiteID = types.StringValue(p.SiteID)
	state.BrandingSettings = &brandingSettingsModel{
		BodyTextColor:   types.StringValue(p.EnrollmentCustomizationBrandingSettings.TextColor),
		ButtonColor:     types.StringValue(p.EnrollmentCustomizationBrandingSettings.ButtonColor),
		ButtonTextColor: types.StringValue(p.EnrollmentCustomizationBrandingSettings.ButtonTextColor),
		BackgroundColor: types.StringValue(p.EnrollmentCustomizationBrandingSettings.BackgroundColor),
		IconURL:         types.StringValue(p.EnrollmentCustomizationBrandingSettings.IconURL),
	}
}

// assignParentToDataSource copies the parent record into the data source
// model. Identical field set to the resource model minus the panes and the
// upload-side attrs.
func assignParentToDataSource(state *EnrollmentCustomizationDataSourceModel, p *pro.EnrollmentCustomizationV2) {
	if p == nil {
		return
	}
	if p.ID != nil {
		state.ID = types.StringValue(*p.ID)
	}
	state.DisplayName = types.StringValue(p.DisplayName)
	state.Description = types.StringValue(p.Description)
	state.SiteID = types.StringValue(p.SiteID)
	state.BrandingSettings = &brandingSettingsModel{
		BodyTextColor:   types.StringValue(p.EnrollmentCustomizationBrandingSettings.TextColor),
		ButtonColor:     types.StringValue(p.EnrollmentCustomizationBrandingSettings.ButtonColor),
		ButtonTextColor: types.StringValue(p.EnrollmentCustomizationBrandingSettings.ButtonTextColor),
		BackgroundColor: types.StringValue(p.EnrollmentCustomizationBrandingSettings.BackgroundColor),
		IconURL:         types.StringValue(p.EnrollmentCustomizationBrandingSettings.IconURL),
	}
}

// assignTextPanel maps a single SDK GetText panel response into the TF text
// pane element model.
func assignTextPanel(p *pro.GetEnrollmentCustomizationPanelText) textPaneModel {
	return textPaneModel{
		ID:                 types.StringValue(strconv.Itoa(p.ID)),
		DisplayName:        types.StringValue(p.DisplayName),
		Rank:               types.Int64Value(int64(p.Rank)),
		Title:              types.StringValue(p.Title),
		Body:               types.StringValue(p.Body),
		Subtext:            types.StringValue(p.Subtext),
		PreviousButtonText: types.StringValue(p.BackButtonText),
		NextButtonText:     types.StringValue(p.ContinueButtonText),
	}
}

// assignLdapPanel maps a single SDK GetLdap panel response into the TF ldap
// pane element model. The `ldap_group_access` array is always materialised
// when non-empty; nil slices collapse to a nil TF slice (the schema treats
// the inner list as Optional, so omission is fine).
func assignLdapPanel(p *pro.GetEnrollmentCustomizationPanelLdapAuth) ldapPaneModel {
	out := ldapPaneModel{
		ID:                 types.StringValue(strconv.Itoa(p.ID)),
		DisplayName:        types.StringValue(p.DisplayName),
		Rank:               types.Int64Value(int64(p.Rank)),
		Title:              types.StringValue(p.Title),
		UsernameText:       types.StringValue(p.UsernameLabel),
		PasswordText:       types.StringValue(p.PasswordLabel),
		PreviousButtonText: types.StringValue(p.BackButtonText),
		LoginButtonText:    types.StringValue(p.ContinueButtonText),
	}
	if len(p.LdapGroupAccess) > 0 {
		out.DirectoryServiceGroups = make([]directoryServiceGroupModel, 0, len(p.LdapGroupAccess))
		for _, g := range p.LdapGroupAccess {
			entry := directoryServiceGroupModel{}
			if g.GroupName != nil {
				entry.GroupName = types.StringValue(*g.GroupName)
			} else {
				entry.GroupName = types.StringValue("")
			}
			if g.LdapServerID != nil {
				entry.DirectoryServiceServerID = types.Int64Value(int64(*g.LdapServerID))
			} else {
				entry.DirectoryServiceServerID = types.Int64Value(0)
			}
			out.DirectoryServiceGroups = append(out.DirectoryServiceGroups, entry)
		}
	}
	return out
}

// assignSsoPanel maps a single SDK GetSso panel response into the TF sso
// pane element model. The `isGroupEnrollmentAccessEnabled` bool inverts to
// the `enrollment_access` enum string; "true" → "specific_group", "false" →
// "any_idp_user".
func assignSsoPanel(p *pro.GetEnrollmentCustomizationPanelSsoAuth) ssoPaneModel {
	access := enrollmentAccessAnyIdpUser
	if p.IsGroupEnrollmentAccessEnabled {
		access = enrollmentAccessSpecificGroup
	}
	groupName := types.StringNull()
	if p.GroupEnrollmentAccessName != "" {
		groupName = types.StringValue(p.GroupEnrollmentAccessName)
	}
	return ssoPaneModel{
		ID:                        types.StringValue(strconv.Itoa(p.ID)),
		DisplayName:               types.StringValue(p.DisplayName),
		Rank:                      types.Int64Value(int64(p.Rank)),
		EnrollmentAccess:          types.StringValue(access),
		AccessGroupName:           groupName,
		PassUserInfoToJamfConnect: types.BoolValue(p.IsUseJamfConnect),
		AccountNameAttribute:      types.StringValue(p.ShortNameAttribute),
		AccountFullNameAttribute:  types.StringValue(p.LongNameAttribute),
	}
}

// sortByRank stable-sorts a slice by rank ascending. Tie-break on id keeps the
// ordering deterministic so the framework's positional reconcile does not
// churn on identical-rank elements.
func sortTextByRank(panes []textPaneModel) {
	sort.SliceStable(panes, func(i, j int) bool {
		ri, rj := panes[i].Rank.ValueInt64(), panes[j].Rank.ValueInt64()
		if ri != rj {
			return ri < rj
		}
		return panes[i].ID.ValueString() < panes[j].ID.ValueString()
	})
}

func sortLdapByRank(panes []ldapPaneModel) {
	sort.SliceStable(panes, func(i, j int) bool {
		ri, rj := panes[i].Rank.ValueInt64(), panes[j].Rank.ValueInt64()
		if ri != rj {
			return ri < rj
		}
		return panes[i].ID.ValueString() < panes[j].ID.ValueString()
	})
}

func sortSsoByRank(panes []ssoPaneModel) {
	sort.SliceStable(panes, func(i, j int) bool {
		ri, rj := panes[i].Rank.ValueInt64(), panes[j].Rank.ValueInt64()
		if ri != rj {
			return ri < rj
		}
		return panes[i].ID.ValueString() < panes[j].ID.ValueString()
	})
}
