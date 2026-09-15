// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignConnectionResourceModel populates a resource model from a connection
// read and its collection entry.
//
// Both are needed, because neither is complete: the read of one connection
// carries the per-provider settings and no products, and the collection entry
// carries the products and no settings. summary may be nil, which leaves the two
// attributes only it can supply empty rather than wrong.
//
// adopt says whether the model is being filled from nothing — an import, or the
// identity-only refresh Terraform performs when it holds an identity and no
// state. It decides two things a read alone cannot. First, where `name` comes
// from: ordinarily not from Jamf at all, because Jamf may store a uniquified form
// of whatever was configured and overwriting the configured value with it would
// give every such connection a difference on every plan; on an import there is no
// configured value to protect, so the configured form is recovered from the
// stored one by configuredNameFromStored rather than taken as Jamf holds it.
// Second, whether a settings block absent from the target is populated:
// STYLE_GUIDE §`SingleNestedAttribute` blocks requires the
// repopulation of a block to be gated on the model being written rather than on
// what Jamf returned, since populating one the plan said was empty breaks the
// framework's consistency contract. An import is the one case where the plan said
// nothing at all.
//
// `pkce` and `auth_method` are read through pkceForConnectionType and
// authMethodForConnectionType rather than adopted, because Jamf returns both for
// every connection while this resource refuses each on the families the Jamf
// Account console does not offer it for. Anything else added here that a
// per-connection-type validator can refuse needs the same treatment —
// TestGeneratedConfigurationPassesItsOwnValidators is what enforces that.
//
// Two collections are deliberately never touched here. `enabled_products` and
// `enabled_environments` are configuration-authoritative: nothing Jamf returns
// echoes their tenants or environments, so adopting anything would mean inventing
// it, and leaving them alone is what keeps a configured value from being
// overwritten with a guess. `enabled_product_names` is the part that does come
// back.
func assignConnectionResourceModel(
	state *ConnectionResourceModel,
	c *account.Connection,
	summary *account.ConnectionSummary,
	adopt bool,
) diag.Diagnostics {
	var diags diag.Diagnostics

	manageOIDC := state.GenericOIDC != nil || adopt
	manageEntra := state.Entra != nil || adopt
	manageOkta := state.Okta != nil || adopt
	manageGoogle := state.GoogleWorkspace != nil || adopt
	manageFilter := state.GroupNameFilter != nil || adopt

	state.ID = types.StringValue(c.ID)
	state.InternalName = types.StringValue(c.Name)
	if adopt {
		state.Name = types.StringValue(configuredNameFromStored(c.Name))
	}
	state.ConnectionType = renameFromWire(c.Type, connectionTypeFromWire)
	state.HostingRegion = stringOrNull(c.Region)
	state.AuthMethod = authMethodForConnectionType(state.ConnectionType, c.TokenEndpointAuthMethod)
	state.ClientID = stringOrNull(c.ClientID)
	state.Scopes = preferEquivalentScopes(state.Scopes, c.Scopes)
	state.PKCE = pkceForConnectionType(state.ConnectionType, c.PkceAuthType)
	state.SendNonce = types.BoolValue(c.SendNonce)
	state.SyncAttributesAtLogin = types.BoolValue(c.SyncUserProfileAttributesAtLogin)
	state.OmitLoginHint = omitLoginHintFromWire(c.AliasLoginHintToIdp)
	state.CustomUsernameClaimName = stringOrNull(c.CustomUsernameClaimName)
	state.UsernameDomain = stringOrNull(c.UsernameDomain)
	state.AttributeMap = preferEquivalentJSON(state.AttributeMap, c.AttributeMap)
	state.ConsentFlow = types.BoolValue(c.ConsentFlow)
	state.EasyConfig = types.BoolValue(c.EasyConfig)

	state.SessionDurationMinutes, state.InactivityTimeoutMinutes = sessionMinutes(c.SessionInfo)

	domains, domainDiags := stringsToSet(c.Domains)
	diags.Append(domainDiags...)
	state.Domains = domains

	if manageFilter {
		filter, filterDiags := parseGroupNameFilter(c.GroupNameFilter)
		diags.Append(filterDiags...)
		state.GroupNameFilter = filter
	}

	if manageOIDC {
		state.GenericOIDC = buildOIDCStateModel(c.OidcOptions)
	}
	if manageEntra {
		state.Entra = buildEntraStateModel(c.AzureOptions)
	}
	if manageOkta {
		state.Okta = buildOktaStateModel(c.OktaOptions)
	}
	if manageGoogle {
		state.GoogleWorkspace = buildGoogleStateModel(c.GoogleOptions)
	}

	names, ticket := collectionDerivedValues(summary, c.GoogleOptions)
	productNames, nameDiags := stringsToSet(names)
	diags.Append(nameDiags...)
	state.EnabledProductNames = productNames
	state.TicketURL = ticket

	return diags
}

// configuredNameFromStored recovers the name that was configured from the name
// Jamf Account stores.
//
// Jamf appends a suffix of its own to whichever name it was given — a create
// sending `tfReviewMin` was stored as
// `tfReviewMin-jqxld7tl4m454ed7s35647nmjssypo` — and eighteen of the twenty-two
// connections read carry one. The split is unambiguous, because a configured name
// may hold only letters and digits (nameAllowedPattern, wire-established): the
// first hyphen in a stored value can only begin Jamf's suffix, so everything
// before it can only be what was configured, however many further hyphens the
// suffix itself carries.
//
// Recovering it matters because `name` is validated against that same pattern and
// sits in connectionComparisons, so it forces a replacement when it differs from
// state. Adopting the stored value would put a name in state that no
// configuration can hold — the hyphen is refused at plan time — and the first
// apply after an import would then destroy a live connection, with no
// configuration value that avoids it. `internal_name` is where the stored value
// belongs, and keeps it whole.
//
// A stored value carrying no hyphen is returned as it is: four of the connections
// read carry no suffix. So is one that begins with a hyphen, which no name Jamf
// accepted could produce, and reporting nothing for it would be worse than
// reporting what Jamf holds.
func configuredNameFromStored(stored string) string {
	if base, _, found := strings.Cut(stored, "-"); found && base != "" {
		return base
	}
	return stored
}

// pkceForConnectionType reports the PKCE method to record, which is nothing at
// all for the two families that cannot be configured with one.
//
// The Jamf Account console offers a PKCE setting only for a generic OpenID
// Connect or an Okta connection, and validators.go refuses a configured value on
// an Entra or a Google Workspace one. Jamf returns a pkceAuthType regardless, so
// recording it made the resource contradict its own rule — and the way that
// surfaces is `terraform plan -generate-config-out`, which writes state back out
// as configuration: an Entra connection came back with `pkce = "disabled"`
// written in, and the generated file could not be planned (issue #379).
//
// Recording nothing is stable rather than merely quieter, which is the part worth
// stating because the attribute is Optional and Computed. A plan that changes
// nothing leaves a null alone, so there is no perpetual difference; a plan that
// changes something marks the attribute unknown, since UseNonNullStateForUnknown
// has no non-null prior value to hold on to — and ModifyPlan reads an unknown
// planned value as unchanged, so it adds no replacement of its own. Whatever
// writes then follow commit through this same function, so the unknown resolves
// to the null it planned for. The empty plan and the exempted unknown were both
// observed by driving the framework's plan pipeline, not deduced from the schema.
//
// The connection type comes from the value already put in state a few lines
// above, not from the wire, so this cannot disagree with the type the resource
// reports.
// authMethodForConnectionType reports the token endpoint authentication method to
// record, which is nothing at all for a Google Workspace connection.
//
// It is pkceForConnectionType's twin and exists for the same reason. Jamf returns
// a tokenEndpointAuthMethod for every connection — "client_secret" on all five
// connections of the probe organization, Google Workspace included — while
// validators.go refuses a configured value there, because the Jamf Account console
// offers no such choice for Google Workspace: it always uses a client secret. So
// recording Jamf's value made the resource contradict its own rule, and the way
// that surfaced is `terraform plan -generate-config-out`, which writes state back
// out as configuration.
//
// The stability argument is pkceForConnectionType's, unchanged: the attribute is
// Optional and Computed, a plan that changes nothing leaves the null alone, and a
// plan that changes something marks it unknown, which ModifyPlan reads as
// unchanged and which resolves back through this same function.
func authMethodForConnectionType(connectionType types.String, wire *string) types.String {
	if connectionType.ValueString() == connectionTypeGoogle {
		return types.StringNull()
	}
	return renameFromWire(wire, authMethodFromWire)
}

func pkceForConnectionType(connectionType types.String, wire *string) types.String {
	switch connectionType.ValueString() {
	case connectionTypeEntra, connectionTypeGoogle:
		return types.StringNull()
	}
	return renameFromWire(wire, pkceFromWire)
}

// assignConnectionDataSourceModel populates the singular data source model.
//
// Everything is adopted from Jamf, with none of the resource's protections: a
// data source owns no configuration to be inconsistent with, so the stored name
// is reported as it is and every settings block is filled from whatever Jamf
// returned. Reading a connection this provider could not manage — one built with
// Microsoft admin consent, say — is exactly what a data source is for.
func assignConnectionDataSourceModel(
	state *ConnectionDataSourceModel,
	c *account.Connection,
	summary *account.ConnectionSummary,
) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(c.ID)
	state.Name = types.StringValue(c.Name)
	state.ConnectionType = renameFromWire(c.Type, connectionTypeFromWire)
	state.HostingRegion = stringOrNull(c.Region)
	state.AuthMethod = renameFromWire(c.TokenEndpointAuthMethod, authMethodFromWire)
	state.ClientID = stringOrNull(c.ClientID)
	state.Scopes = stringOrNull(c.Scopes)
	state.PKCE = renameFromWire(c.PkceAuthType, pkceFromWire)
	state.SendNonce = types.BoolValue(c.SendNonce)
	state.SyncAttributesAtLogin = types.BoolValue(c.SyncUserProfileAttributesAtLogin)
	state.OmitLoginHint = omitLoginHintFromWire(c.AliasLoginHintToIdp)
	state.CustomUsernameClaimName = stringOrNull(c.CustomUsernameClaimName)
	state.UsernameDomain = stringOrNull(c.UsernameDomain)
	state.AttributeMap = stringOrNull(c.AttributeMap)
	state.ConsentFlow = types.BoolValue(c.ConsentFlow)
	state.EasyConfig = types.BoolValue(c.EasyConfig)

	state.SessionDurationMinutes, state.InactivityTimeoutMinutes = sessionMinutes(c.SessionInfo)

	domains, domainDiags := stringsToSet(c.Domains)
	diags.Append(domainDiags...)
	state.Domains = domains

	filter, filterDiags := parseGroupNameFilter(c.GroupNameFilter)
	diags.Append(filterDiags...)
	state.GroupNameFilter = filter

	state.GenericOIDC = buildOIDCStateModel(c.OidcOptions)
	state.Entra = buildEntraStateModel(c.AzureOptions)
	state.Okta = buildOktaStateModel(c.OktaOptions)
	state.GoogleWorkspace = buildGoogleStateModel(c.GoogleOptions)

	names, ticket := collectionDerivedValues(summary, c.GoogleOptions)
	productNames, nameDiags := stringsToSet(names)
	diags.Append(nameDiags...)
	state.EnabledProductNames = productNames
	state.TicketURL = ticket

	return diags
}

// buildConnectionsResultModel maps one collection entry into a plural data
// source result.
func buildConnectionsResultModel(summary account.ConnectionSummary) (ConnectionsDataSourceResultModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	domains, domainDiags := stringsToSet(summary.Domains)
	diags.Append(domainDiags...)

	names, nameDiags := stringsToSet(summary.EnabledApplications)
	diags.Append(nameDiags...)

	return ConnectionsDataSourceResultModel{
		ID:                    types.StringValue(summary.ID),
		Name:                  types.StringValue(summary.Name),
		ConnectionType:        renameFromWire(summary.Type, connectionTypeFromWire),
		HostingRegion:         stringOrNull(summary.Region),
		AuthMethod:            renameFromWire(summary.TokenEndpointAuthMethod, authMethodFromWire),
		SyncAttributesAtLogin: types.BoolValue(summary.SyncUserProfileAttributesAtLogin),
		Domains:               domains,
		EnabledProductNames:   names,
		TicketURL:             stringOrNull(summary.TicketURL),
		EasyConfig:            types.BoolValue(summary.EasyConfig),
	}, diags
}

// collectionDerivedValues returns the two values only the collection entry
// carries.
//
// The consent ticket has a second source. It appears on the collection entry for
// every family, and again inside the Google Workspace settings of a single read
// — so a Google connection read on its own identifier can supply it even when
// the collection entry is unavailable. The collection entry wins where both are
// present, being the one that applies to every family.
func collectionDerivedValues(summary *account.ConnectionSummary, google *account.GoogleOptions) ([]string, types.String) {
	names := []string{}
	ticket := types.StringNull()

	if summary != nil {
		names = summary.EnabledApplications
		if names == nil {
			names = []string{}
		}
		ticket = stringOrNull(summary.TicketURL)
	}
	if ticket.IsNull() && google != nil {
		ticket = stringOrNull(google.TicketURL)
	}
	return names, ticket
}

// sessionMinutes renders the two session limits, leaving both empty where Jamf
// reports no object at all.
func sessionMinutes(info *account.SessionInfo) (types.Int64, types.Int64) {
	if info == nil {
		return types.Int64Null(), types.Int64Null()
	}
	return int64OrNull(info.MaxSessionTimeInMinutes), int64OrNull(info.MaxInactivityTimeInMinutes)
}

// renameFromWire projects one of Jamf's vocabulary values into Terraform's.
//
// A value the rename table does not cover is carried through as Jamf spelled it
// rather than dropped, so a vocabulary Jamf extends is reported honestly instead
// of read back as nothing. mappings_test.go is what fails in that case, which is
// where the gap belongs.
func renameFromWire(value *string, fromWire map[string]string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	if renamed, ok := fromWire[*value]; ok {
		return types.StringValue(renamed)
	}
	return types.StringValue(*value)
}

// parseGroupNameFilter turns Jamf's stored filter document back into the block.
//
// An unparseable document is reported rather than swallowed: it means Jamf holds
// a filter in a shape this provider does not understand, and silently reading it
// back as no filter would let the next apply clear a live filter.
func parseGroupNameFilter(raw *string) (*GroupNameFilterModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, diags
	}

	decoded, err := decodeJSONObject(*raw)
	if err != nil {
		diags.AddError(
			"Unable to read the group filter Jamf Account holds",
			"Jamf Account holds a group filter for this connection that this provider cannot read: "+helpers.APIErrorDetail(err)+
				". Reading it back as no filter would let the next apply clear it, so the refresh has stopped "+
				"instead. Please report this issue to the provider developers, quoting the value: "+*raw,
		)
		return nil, diags
	}

	op, _ := decoded[filterOpKey].(string)
	operator, known := filterOperatorFromWire[op]
	if !known {
		diags.AddError(
			"Unable to read the group filter Jamf Account holds",
			"The group filter Jamf Account holds for this connection joins its groups with \""+op+"\", which "+
				"this provider does not recognise — it understands "+
				markdownValueList(filterOperatorValues())+". Reading it back as no filter would let the next "+
				"apply clear it, so the refresh has stopped instead. Please report this issue to the provider "+
				"developers.",
		)
		return nil, diags
	}

	csv, _ := decoded[filterGroupsKey].(string)
	groups, groupDiags := stringsToSet(splitFilterGroups(csv))
	diags.Append(groupDiags...)
	if diags.HasError() {
		return nil, diags
	}

	return &GroupNameFilterModel{
		Operator: types.StringValue(operator),
		Groups:   groups,
	}, diags
}

// preferEquivalentScopes keeps the scope string Terraform planned when Jamf's
// copy names the same scopes in a different order.
//
// The order of OAuth scopes carries no meaning, and Jamf's copy of them was
// never seen written back — the write path is refused for every request — so
// whether it re-orders them is unknown. Treating a re-ordering as no change costs
// nothing if it never happens and avoids a permanent difference if it does. A
// copy naming *different* scopes is adopted, which is a real difference and shows
// up as one.
func preferEquivalentScopes(planned types.String, fromWire *string) types.String {
	incoming := stringOrNull(fromWire)
	if planned.IsNull() || planned.IsUnknown() || incoming.IsNull() {
		return incoming
	}
	if sameScopeSet(planned.ValueString(), incoming.ValueString()) {
		return planned
	}
	return incoming
}

// sameScopeSet reports whether two space-separated scope strings name the same
// scopes.
func sameScopeSet(left, right string) bool {
	leftFields := strings.Fields(left)
	rightFields := strings.Fields(right)
	if len(leftFields) != len(rightFields) {
		return false
	}
	counts := make(map[string]int, len(leftFields))
	for _, f := range leftFields {
		counts[f]++
	}
	for _, f := range rightFields {
		counts[f]--
		if counts[f] < 0 {
			return false
		}
	}
	return true
}

// buildOIDCStateModel fills the generic OpenID Connect block from a read.
func buildOIDCStateModel(options *account.OidcOptions) *GenericOIDCSettingsModel {
	if options == nil {
		return nil
	}
	return &GenericOIDCSettingsModel{
		IssuerURL:             stringOrNull(options.IssuerURL),
		AuthorizationEndpoint: stringOrNull(options.AuthorizationEndpoint),
		TokenEndpoint:         stringOrNull(options.TokenEndpoint),
		JWKSURI:               stringOrNull(options.JwksUri),
		UserInfoEndpoint:      stringOrNull(options.UserInfoEndpoint),
	}
}

// buildEntraStateModel fills the Entra block from a read.
//
// Three of the block's options sit one level deeper in Jamf's copy than they do
// in the settings it accepts, which is the read and write shapes disagreeing
// rather than anything meaningful — an absent inner object leaves the three
// empty.
func buildEntraStateModel(options *account.EntraOptions) *EntraSettingsModel {
	if options == nil {
		return nil
	}
	out := &EntraSettingsModel{
		Domain:            stringOrNull(options.Domain),
		TenantDomain:      stringOrNull(options.TenantDomain),
		UseCommonEndpoint: boolOrNull(options.UseCommonEndpoint),
		IdentityAPI:       stringOrNull(options.IdentityApi),
		MaxGroups:         int64OrNull(options.MaxGroups),
		SetEmailsVerified: boolOrNull(options.SetEmailsVerified),
		EnableUsersAPI:    boolOrNull(options.EnableUsersApi),
		UseWSFed:          boolOrNull(options.UseWsfed),
		BasicProfile:      boolOrNull(options.BasicProfile),
	}
	if options.ExtOptions != nil {
		out.ExtendedProfile = boolOrNull(options.ExtOptions.ExtendedProfile)
		out.GetUserGroups = boolOrNull(options.ExtOptions.Groups)
	}

	// The two group options are gated on get_user_groups, the way pkce and
	// auth_method are gated on connection_type: rule 14
	// (validateEntraGroupOptions) refuses both when groups are off, so adopting
	// what Jamf returned would commit a value this resource's own validator
	// rejects — and `-generate-config-out` would then write a configuration that
	// cannot plan. Same defect class as the seven in issue #379.
	//
	// The two are gated differently because their schemas differ.
	// `groups_scope` is Optional only, so a null is safe: a configuration that
	// sets it under groups-off fails at plan and this Read never has to
	// reconcile with it. `include_nested_groups` is Optional+Computed with
	// UseNonNullStateForUnknown, where a null would fight the plan modifier, so
	// it settles to false — which the validator accepts, and which is what Jamf
	// actually does with nested groups when group membership is off.
	switch groupMembership(options.ExtOptions) {
	case groupsOn:
		out.GroupsScope = stringOrNull(options.GroupsScope)
		out.IncludeNestedGroups = boolOrNull(options.ExtOptions.NestedGroups)
	case groupsOff:
		out.GroupsScope = types.StringNull()
		out.IncludeNestedGroups = types.BoolValue(false)
	default: // groupsUnreported
		// Jamf sent no group settings at all. include_nested_groups stays empty
		// rather than guessing at false — TestBuildEntraStateModel_WithoutTheNestedOptions
		// pins that — while groups_scope is still dropped, because the validator
		// reads an absent get_user_groups as off and would refuse it.
		out.GroupsScope = types.StringNull()
	}
	return out
}

// groupMembershipState is whether an Entra connection reads group membership,
// which is what makes groups_scope and include_nested_groups meaningful.
type groupMembershipState int

const (
	// groupsUnreported means Jamf returned no group settings for the connection,
	// which is distinct from returning them switched off.
	groupsUnreported groupMembershipState = iota
	groupsOff
	groupsOn
)

// groupMembership classifies the group switch on an Entra read.
func groupMembership(ext *account.EntraExtendedOptions) groupMembershipState {
	if ext == nil || ext.Groups == nil {
		return groupsUnreported
	}
	if *ext.Groups {
		return groupsOn
	}
	return groupsOff
}

// buildOktaStateModel fills the Okta block from a read.
func buildOktaStateModel(options *account.OktaOptions) *OktaSettingsModel {
	if options == nil {
		return nil
	}
	return &OktaSettingsModel{
		Domain:                stringOrNull(options.Domain),
		IssuerURL:             stringOrNull(options.IssuerURL),
		AuthorizationEndpoint: stringOrNull(options.AuthorizationEndpoint),
		TokenEndpoint:         stringOrNull(options.TokenEndpoint),
		JWKSURI:               stringOrNull(options.JwksUri),
	}
}

// buildGoogleStateModel fills the Google Workspace block from a read.
//
// Jamf's names for two of these differ from the ones its settings accept, and one
// of them is a trap: `extGroups` is the group-membership switch and
// `extGroupsExtended` is the directory read, so the shorter name is not the
// simpler option. The mapping is in the package doc.
func buildGoogleStateModel(options *account.GoogleOptions) *GoogleWorkspaceSettingsModel {
	if options == nil {
		return nil
	}
	return &GoogleWorkspaceSettingsModel{
		Domain:         stringOrNull(options.Domain),
		GetUserGroups:  boolOrNull(options.ExtGroups),
		ExtendedGroups: boolOrNull(options.ExtGroupsExtended),
		EnableUsersAPI: boolOrNull(options.EnableUsersApi),
	}
}
