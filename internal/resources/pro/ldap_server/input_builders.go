// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildLdapServerInput converts the Terraform plan model into the SDK
// LdapServerPost payload used for both Create and Update. `password` is the
// resolved plaintext bind password: non-nil only when it should be written
// (always on Create when supplied, on Update only when password_wo_version
// changed). When nil, the `<password>` element is omitted so Classic's
// partial-merge PUT retains the stored value.
//
// `ID` is omitted on the wire — Create uses path id="0"; Update derives the
// ID from the path. Server-managed echoes (is_enabled, migrated_to_id,
// certificates_used, group_membership_enabled_when_user_membership_selected)
// are never sent.
func buildLdapServerInput(plan LdapServerResourceModel, password *string) *proclassic.LdapServerPost {
	in := &proclassic.LdapServerPost{}
	if plan.Connection != nil {
		in.Connection = buildConnectionInput(plan.Connection, password)
	}
	if plan.MappingsForUsers != nil {
		in.MappingsForUsers = buildMappingsInput(plan.MappingsForUsers)
	}
	return in
}

// buildConnectionInput converts the connection block into the SDK shape.
func buildConnectionInput(c *ldapConnectionModel, password *string) *proclassic.LdapServerPostConnection {
	conn := &proclassic.LdapServerPostConnection{
		Name:               helpers.OptionalStringPointer(c.DisplayName),
		ServerType:         helpers.OptionalStringPointer(c.DirectoryService),
		Hostname:           helpers.OptionalStringPointer(c.Hostname),
		Port:               helpers.OptionalInt64Pointer(c.Port),
		UseSsl:             helpers.OptionalBoolPointer(c.UseSSL),
		AuthenticationType: helpers.OptionalStringPointer(c.AuthenticationType),
		OpenCloseTimeout:   helpers.OptionalInt64Pointer(c.ConnectionTimeout),
		SearchTimeout:      helpers.OptionalInt64Pointer(c.SearchTimeout),
		ReferralResponse:   helpers.OptionalStringPointer(c.ReferralResponse),
		UseWildcards:       helpers.OptionalBoolPointer(c.UseWildcards),
	}
	if c.Account != nil {
		conn.Account = &proclassic.LdapServerConnectionAccount{
			DistinguishedUsername: helpers.OptionalStringPointer(c.Account.DistinguishedUsername),
			Password:              password,
		}
	}
	return conn
}

// buildMappingsInput converts the mappings_for_users block into the SDK shape.
func buildMappingsInput(m *ldapMappingsModel) *proclassic.LdapServerPostMappingsForUsers {
	out := &proclassic.LdapServerPostMappingsForUsers{}
	if m.UserMappings != nil {
		u := m.UserMappings
		out.UserMappings = &proclassic.LdapServerMappingsForUsersUserMappings{
			MapObjectClassToAnyOrAll: helpers.OptionalStringPointer(u.ObjectClassLimitation),
			ObjectClasses:            helpers.OptionalStringPointer(u.ObjectClasses),
			SearchBase:               helpers.OptionalStringPointer(u.SearchBase),
			SearchScope:              helpers.OptionalStringPointer(u.SearchScope),
			MapUserID:                helpers.OptionalStringPointer(u.UserID),
			MapUsername:              helpers.OptionalStringPointer(u.Username),
			MapRealname:              helpers.OptionalStringPointer(u.RealName),
			MapEmailAddress:          helpers.OptionalStringPointer(u.EmailAddress),
			AppendToEmailResults:     helpers.OptionalStringPointer(u.AppendToEmailResults),
			MapDepartment:            helpers.OptionalStringPointer(u.Department),
			MapBuilding:              helpers.OptionalStringPointer(u.Building),
			MapRoom:                  helpers.OptionalStringPointer(u.Room),
			MapPhone:                 helpers.OptionalStringPointer(u.Phone),
			MapPosition:              helpers.OptionalStringPointer(u.Position),
			MapUserUUID:              helpers.OptionalStringPointer(u.UserUUID),
		}
	}
	if m.UserGroupMappings != nil {
		g := m.UserGroupMappings
		out.UserGroupMappings = &proclassic.LdapServerMappingsForUsersUserGroupMappings{
			MapObjectClassToAnyOrAll: helpers.OptionalStringPointer(g.ObjectClassLimitation),
			ObjectClasses:            helpers.OptionalStringPointer(g.ObjectClasses),
			SearchBase:               helpers.OptionalStringPointer(g.SearchBase),
			SearchScope:              helpers.OptionalStringPointer(g.SearchScope),
			MapGroupID:               helpers.OptionalStringPointer(g.GroupID),
			MapGroupName:             helpers.OptionalStringPointer(g.GroupName),
			MapGroupUUID:             helpers.OptionalStringPointer(g.GroupUUID),
		}
	}
	if m.UserGroupMembershipMappings != nil {
		b := m.UserGroupMembershipMappings
		out.UserGroupMembershipMappings = &proclassic.LdapServerMappingsForUsersUserGroupMembershipMappings{
			UserGroupMembershipStoredIn:                      helpers.OptionalStringPointer(b.MembershipLocation),
			MapUserMembershipToGroupField:                    helpers.OptionalStringPointer(b.MemberUserMapping),
			MapGroupMembershipToUserField:                    helpers.OptionalStringPointer(b.GroupMembershipMapping),
			AppendToUsername:                                 helpers.OptionalStringPointer(b.AppendToUsername),
			UseDn:                                            helpers.OptionalBoolPointer(b.UseDN),
			UserGroupMembershipUseLdapCompare:                helpers.OptionalBoolPointer(b.UseLDAPCompare),
			RecursiveLookups:                                 helpers.OptionalBoolPointer(b.RecursiveLookups),
			MapUserMembershipUseDn:                           helpers.OptionalBoolPointer(b.MapUserMembershipUseDN),
			MembershipScopingOptimization:                    helpers.OptionalBoolPointer(b.MembershipCalculationOptimization),
			GroupMembershipEnabledWhenUserMembershipSelected: helpers.OptionalBoolPointer(b.UseMemberFieldForSelectQueries),
			MapObjectClassToAnyOrAll:                         helpers.OptionalStringPointer(b.ObjectClassLimitation),
			ObjectClasses:                                    helpers.OptionalStringPointer(b.ObjectClasses),
			SearchBase:                                       helpers.OptionalStringPointer(b.SearchBase),
			SearchScope:                                      helpers.OptionalStringPointer(b.SearchScope),
			Username:                                         helpers.OptionalStringPointer(b.UsernameMapping),
			GroupID:                                          helpers.OptionalStringPointer(b.GroupIDMapping),
		}
	}
	return out
}
