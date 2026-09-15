// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignGoogleState folds a Cloud LDAP GET response into the resource model.
//
// The keystore `wo_version` rotation trigger and the WriteOnly `file` /
// `password` are not returned by the server. `wo_version` is preserved from
// the prior model (`priorWoVersion`) so a refresh does not null it out;
// `file` / `password` are WriteOnly and excluded from state by the framework.
//
// `mappings` is Optional and not Computed, so a block the user did not author
// must stay null in state or the apply trips a "planned null, got object"
// consistency error — hence the prior shape is captured before `google` is
// rebuilt. hydrating releases that gate on a first hydration, where the prior is
// always nil; see assignMappingsState.
func assignGoogleState(state *CloudIdentityProviderResourceModel, resp *pro.LdapConfigurationResponse, priorWoVersion types.Int64, hydrating bool) {
	if resp == nil {
		return
	}

	var priorMappings *cloudLdapMappingsModel
	if state.Google != nil {
		priorMappings = state.Google.Mappings
	}

	if resp.CloudIDPCommon != nil {
		if resp.CloudIDPCommon.ID != "" {
			state.ID = types.StringValue(resp.CloudIDPCommon.ID)
		}
		state.DisplayName = types.StringValue(resp.CloudIDPCommon.DisplayName)
		state.ProviderName = types.StringValue(providerNameFromWire(resp.CloudIDPCommon.ProviderName))
	}

	google := &cloudIdentityProviderGoogleModel{}

	if resp.Server != nil {
		s := resp.Server
		server := &cloudLdapServerModel{
			ServerURL:                                types.StringValue(s.ServerURL),
			DomainName:                               types.StringValue(s.DomainName),
			Port:                                     types.Int64Value(int64(s.Port)),
			ConnectionType:                           types.StringValue(s.ConnectionType),
			ConnectionTimeout:                        types.Int64Value(int64(s.ConnectionTimeout)),
			SearchTimeout:                            types.Int64Value(int64(s.SearchTimeout)),
			UseWildcards:                             types.BoolValue(s.UseWildcards),
			Enabled:                                  types.BoolValue(s.Enabled),
			MembershipCalculationOptimizationEnabled: types.BoolValue(s.MembershipCalculationOptimizationEnabled),
			Keystore:                                 assignKeystoreState(s.Keystore, priorWoVersion),
		}
		google.Server = server
	}

	google.Mappings = assignMappingsState(resp.Mappings, priorMappings, hydrating)

	state.Google = google
}

// assignKeystoreState builds the keystore echo model. file / password are
// WriteOnly (never in state); wo_version is carried from the prior model.
func assignKeystoreState(k *pro.CloudLdapKeystore, priorWoVersion types.Int64) *cloudLdapKeystoreModel {
	out := &cloudLdapKeystoreModel{
		File:      types.StringNull(),
		Password:  types.StringNull(),
		WoVersion: priorWoVersion,
	}
	if k == nil {
		out.FileName = types.StringNull()
		out.Type = types.StringNull()
		out.Subject = types.StringNull()
		out.ExpirationDate = types.StringNull()
		return out
	}
	out.FileName = types.StringValue(k.FileName)
	out.Type = types.StringValue(k.Type)
	out.Subject = types.StringValue(k.Subject)
	out.ExpirationDate = stringPtrValueOrNull(k.ExpirationDate)
	return out
}

// assignMappingsState builds the mappings model from a GET response, scoped to
// the sub-blocks the user actually authored (`prior`). `mappings` is Optional
// and not Computed, so surfacing a block the user did not configure would trip a
// "planned null, got object" consistency error.
//
// hydrating releases that gate on first hydration — import and config
// generation — where there is no plan to stay consistent with and prior is
// always nil, so before this the block stayed null forever and a declared
// mappings block planned as an addition against imported state (issue #391).
// Returns nil when the server carried no mappings, or only empty ones.
func assignMappingsState(m *pro.CloudLdapMappingsResponse, prior *cloudLdapMappingsModel, hydrating bool) *cloudLdapMappingsModel {
	if m == nil || (prior == nil && !hydrating) {
		return nil
	}
	adopt := func(authored bool, wireEmpty bool) bool {
		if authored {
			return true
		}
		return hydrating && !wireEmpty
	}
	out := &cloudLdapMappingsModel{}
	if adopt(prior != nil && prior.UserMappings != nil, m.UserMappings != nil && userMappingsAreWireEmpty(m.UserMappings)) && m.UserMappings != nil {
		u := m.UserMappings
		out.UserMappings = &cloudLdapUserMappingsModel{
			ObjectClassLimitation: types.StringValue(u.ObjectClassLimitation),
			ObjectClasses:         types.StringValue(u.ObjectClasses),
			SearchBase:            types.StringValue(u.SearchBase),
			SearchScope:           types.StringValue(u.SearchScope),
			AdditionalSearchBase:  stringPtrValueOrNull(u.AdditionalSearchBase),
			UserID:                types.StringValue(u.UserID),
			Username:              types.StringValue(u.Username),
			RealName:              types.StringValue(u.RealName),
			EmailAddress:          types.StringValue(u.EmailAddress),
			Department:            types.StringValue(u.Department),
			Building:              types.StringValue(u.Building),
			Room:                  types.StringValue(u.Room),
			Phone:                 types.StringValue(u.Phone),
			Position:              types.StringValue(u.Position),
			UserUUID:              types.StringValue(u.UserUUID),
		}
	}
	if adopt(prior != nil && prior.GroupMappings != nil, m.GroupMappings != nil && groupMappingsAreWireEmpty(m.GroupMappings)) && m.GroupMappings != nil {
		g := m.GroupMappings
		out.GroupMappings = &cloudLdapGroupMappingsModel{
			ObjectClassLimitation: types.StringValue(g.ObjectClassLimitation),
			ObjectClasses:         types.StringValue(g.ObjectClasses),
			SearchBase:            types.StringValue(g.SearchBase),
			SearchScope:           types.StringValue(g.SearchScope),
			GroupID:               types.StringValue(g.GroupID),
			GroupName:             types.StringValue(g.GroupName),
			GroupUUID:             types.StringValue(g.GroupUUID),
		}
	}
	if adopt(prior != nil && prior.MembershipMappings != nil, m.MembershipMappings != nil && m.MembershipMappings.GroupMembershipMapping == "") && m.MembershipMappings != nil {
		out.MembershipMappings = &cloudLdapMembershipMappingsModel{
			GroupMembershipMapping: types.StringValue(m.MembershipMappings.GroupMembershipMapping),
		}
	}
	if prior == nil && out.UserMappings == nil && out.GroupMappings == nil && out.MembershipMappings == nil {
		return nil
	}
	return out
}

// userMappingsAreWireEmpty and its two siblings report whether Jamf Pro returned
// a mappings sub-block with nothing in it. Hydration skips one: storing a block
// of empty strings the practitioner never wrote is the same fabrication the
// collection rule forbids, and the field-by-field emptiness test is the shape
// disk_encryption_configuration already uses for its recovery key.
//
// Google's write path is not wire-probed — a connection needs a real Secure LDAP
// keystore, so no probe could create one — and it differs from Entra in a way
// that could have mattered: `mappings` is a pointer on its create request and is
// omitted entirely when the block is undeclared, leaving room for the server to
// substitute defaults.
//
// Two probes on the EU test tenant, 2026-09-07, make that unlikely. Jamf Pro does
// serve defaults, from GetCloudLdapDefaultMappingsV2 (and the Entra equivalent),
// but they are a static per-provider template rather than per-connection state:
// `id` comes back "0" with an empty domain, which is what the admin UI pre-fills
// a new connection form with. That is almost certainly where this package's
// former claim that "the server always returns generated mappings" came from. And
// on the Entra side, where a create can be made, the server demonstrably did not
// substitute them: all-empty in, all-empty out.
//
// Either way the guard is safe. Google's template populates most fields
// (`objectClasses`, `searchBase`, `userID` and the rest), so were the server to
// apply it, no sub-block would read as empty and hydration would adopt it —
// which is the wanted behaviour.
func userMappingsAreWireEmpty(u *pro.UserMappings) bool {
	return u.ObjectClassLimitation == "" &&
		u.ObjectClasses == "" &&
		u.SearchBase == "" &&
		u.SearchScope == "" &&
		(u.AdditionalSearchBase == nil || *u.AdditionalSearchBase == "") &&
		u.UserID == "" &&
		u.Username == "" &&
		u.RealName == "" &&
		u.EmailAddress == "" &&
		u.Department == "" &&
		u.Building == "" &&
		u.Room == "" &&
		u.Phone == "" &&
		u.Position == "" &&
		u.UserUUID == ""
}

func groupMappingsAreWireEmpty(g *pro.GroupMappings) bool {
	return g.ObjectClassLimitation == "" &&
		g.ObjectClasses == "" &&
		g.SearchBase == "" &&
		g.SearchScope == "" &&
		g.GroupID == "" &&
		g.GroupName == "" &&
		g.GroupUUID == ""
}

// stringPtrValueOrNull converts a *string into a TF String, mapping nil to
// null. Deliberately NOT helpers.StringPointerValueOrNull: that shared helper
// also collapses a non-nil empty string ("") to null, which would break an
// Optional+Computed field the user explicitly set to "" (plan "" vs state null
// → inconsistent result). Here a non-nil "" is preserved as StringValue("").
func stringPtrValueOrNull(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}
