// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// defaultMaxGroups is the group ceiling the Jamf Account console offers for a
// new Entra connection, and the value every Entra connection read carried. Jamf
// requires the field and documents no default, so one has to be supplied when an
// operator leaves it out.
const defaultMaxGroups = 250

// groupNameFilterDocument is the shape Jamf stores the group filter in: a
// joining operator and the group names as one comma-separated string.
//
// Declared as a struct rather than assembled from a map so the two properties
// are always written in the order Jamf's own copies carry them. Nothing is known
// to depend on that order — the value is stored as an opaque string — but a
// deterministic body is worth having for free.
type groupNameFilterDocument struct {
	Op     string `json:"op"`
	Groups string `json:"groups"`
}

// commonSettings holds the values every provider family's settings carry.
//
// Jamf declares four flat settings shapes with no shared Go type between them,
// so the shared values are gathered once here and then copied into whichever
// shape the connection type selects. Copying beats a reflective merge: the
// compiler checks every assignment, and a field Jamf adds to one shape and not
// another shows up as a missing assignment rather than as a silently dropped
// value.
type commonSettings struct {
	aliasLoginHintToIdp     bool
	attributeMap            *string
	clientID                *string
	clientSecret            *string
	customUsernameClaimName *string
	groupNameFilter         *string
	name                    string
	pkceAuthType            *string
	region                  string
	sendNonce               *bool
	sessionInfo             *account.SessionInfo
	syncAttributes          *bool
	tokenEndpointAuthMethod *string
	usernameDomain          *string
}

// buildConnectionRequest converts the Terraform plan into the create or update
// payload.
//
// The same builder serves both, and it always emits the complete settings. That
// is spec-derived, not wire-verified: Jamf's documentation says an update
// replaces the connection rather than patching it, so omitting a field clears it
// — and the write path is refused for every request, so no probe could confirm
// it. If it turns out to merge instead, sending everything is still correct,
// which is why this is the safe assumption to build on.
//
// The client secret is the one documented exception to that replacement:
// leaving it out keeps the stored secret, and supplying it rotates it. It
// therefore comes from the configuration rather than the plan, because a
// write-only value is not carried in a plan, and it is left out of the payload
// whenever the configuration does not set it. Also spec-derived, not
// wire-verified.
func buildConnectionRequest(ctx context.Context, plan ConnectionResourceModel, secret types.String) (*account.ConnectionRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	connectionType, known := connectionTypeToWire[plan.ConnectionType.ValueString()]
	if !known {
		diags.AddError(
			"Unsupported connection type",
			"The connection type \""+plan.ConnectionType.ValueString()+"\" has no Jamf Account equivalent in this "+
				"provider release. Please report this issue to the provider developers.",
		)
		return nil, diags
	}

	domains, domainDiags := setToStrings(ctx, plan.Domains)
	diags.Append(domainDiags...)

	products, productDiags := buildEnabledProducts(ctx, plan.EnabledProducts)
	diags.Append(productDiags...)

	environments, environmentDiags := buildEnabledEnvironments(ctx, plan.EnabledEnvironments)
	diags.Append(environmentDiags...)

	common, commonDiags := buildCommonSettings(ctx, plan, secret)
	diags.Append(commonDiags...)

	if diags.HasError() {
		return nil, diags
	}

	settings, settingsDiags := buildSettingsUnion(plan, common)
	diags.Append(settingsDiags...)
	if diags.HasError() {
		return nil, diags
	}

	return &account.ConnectionRequest{
		Connection:          settings,
		ConnectionType:      connectionType,
		Domains:             domains,
		EnabledProducts:     products,
		EnabledEnvironments: environments,
	}, diags
}

// buildCommonSettings gathers the values every provider family shares.
func buildCommonSettings(ctx context.Context, plan ConnectionResourceModel, secret types.String) (commonSettings, diag.Diagnostics) {
	var diags diag.Diagnostics

	filter, filterDiags := buildGroupNameFilter(ctx, plan.GroupNameFilter)
	diags.Append(filterDiags...)

	out := commonSettings{
		aliasLoginHintToIdp:     omitLoginHintToWire(plan.OmitLoginHint),
		attributeMap:            helpers.OptionalStringPointer(plan.AttributeMap),
		clientID:                helpers.OptionalStringPointer(plan.ClientID),
		clientSecret:            helpers.OptionalStringPointer(secret),
		customUsernameClaimName: helpers.OptionalStringPointer(plan.CustomUsernameClaimName),
		groupNameFilter:         filter,
		name:                    plan.Name.ValueString(),
		region:                  plan.HostingRegion.ValueString(),
		sendNonce:               helpers.OptionalBoolPointer(plan.SendNonce),
		sessionInfo:             buildSessionInfo(plan),
		syncAttributes:          helpers.OptionalBoolPointer(plan.SyncAttributesAtLogin),
		usernameDomain:          helpers.OptionalStringPointer(plan.UsernameDomain),
	}

	if helpers.IsConfiguredValue(plan.AuthMethod) {
		if wire, known := authMethodToWire[plan.AuthMethod.ValueString()]; known {
			out.tokenEndpointAuthMethod = &wire
		}
	}
	if helpers.IsConfiguredValue(plan.PKCE) {
		if wire, known := pkceToWire[plan.PKCE.ValueString()]; known {
			out.pkceAuthType = &wire
		}
	}

	return out, diags
}

// buildSessionInfo renders the two session limits.
//
// The object is always sent, with an explicit absence for a limit the operator
// left out, because that is exactly how Jamf reports one: every connection read
// carries the object with both properties present and empty where the Jamf
// default applies. Sending the object with nothing in it therefore says "use the
// defaults" in the same words Jamf uses.
func buildSessionInfo(plan ConnectionResourceModel) *account.SessionInfo {
	info := &account.SessionInfo{}
	if helpers.IsConfiguredValue(plan.SessionDurationMinutes) {
		v := int(plan.SessionDurationMinutes.ValueInt64())
		info.MaxSessionTimeInMinutes = &v
	}
	if helpers.IsConfiguredValue(plan.InactivityTimeoutMinutes) {
		v := int(plan.InactivityTimeoutMinutes.ValueInt64())
		info.MaxInactivityTimeInMinutes = &v
	}
	return info
}

// buildGroupNameFilter renders the group filter, or nothing when the block is
// absent.
//
// The distinction the block preserves is the point: an absent block sends no
// filter at all, while a present block with no groups sends an operator and an
// empty list. Those are different values in Jamf's copies, and the second is the
// shape most connections carry, so collapsing one into the other would rewrite
// live configuration.
func buildGroupNameFilter(ctx context.Context, filter *GroupNameFilterModel) (*string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if filter == nil {
		return nil, diags
	}

	groups, groupDiags := setToStrings(ctx, filter.Groups)
	diags.Append(groupDiags...)
	if diags.HasError() {
		return nil, diags
	}

	op, known := filterOperatorToWire[filter.Operator.ValueString()]
	if !known {
		diags.AddError(
			"Unsupported group filter operator",
			"The group filter operator \""+filter.Operator.ValueString()+"\" has no Jamf Account equivalent in "+
				"this provider release. Please report this issue to the provider developers.",
		)
		return nil, diags
	}

	encoded, err := json.Marshal(groupNameFilterDocument{Op: op, Groups: joinFilterGroups(groups)})
	if err != nil {
		diags.AddError("Unable to build the group filter", helpers.APIErrorDetail(err))
		return nil, diags
	}
	rendered := string(encoded)
	return &rendered, diags
}

// buildEnabledProducts renders the product allow-list.
//
// The collection is always emitted, empty where nothing is configured, because
// Jamf requires the field and names it in the refusal when it is missing while
// accepting an empty one — which is its documented way of saying the connection
// is enabled for no tenant-scoped product.
//
// What an empty collection does to an *existing* connection is spec-derived, not
// wire-verified: only the create side was probed, and the write path is refused
// for every request. Under the specification's replacement semantics an empty
// collection clears the assignment, which is what emitting it says. If it turns
// out to merge instead, an emptied collection would silently leave the previous
// assignment in place — worth re-probing the moment the fault clears, because
// nothing in a plan would reveal it.
func buildEnabledProducts(ctx context.Context, products []EnabledProductModel) ([]account.EnabledProduct, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]account.EnabledProduct, 0, len(products))
	for _, product := range products {
		tenants, tenantDiags := setToStrings(ctx, product.Tenants)
		diags.Append(tenantDiags...)
		out = append(out, account.EnabledProduct{
			Product:          product.Product.ValueString(),
			EnabledTenants:   tenants,
			ManagedAccountID: helpers.OptionalStringPointer(product.ManagedAccountID),
		})
	}
	return out, diags
}

// buildEnabledEnvironments renders the environment allow-list, or nothing when
// none is configured — unlike the product allow-list, Jamf treats this one as
// genuinely optional and accepts its absence.
func buildEnabledEnvironments(ctx context.Context, environments []EnabledEnvironmentModel) (*[]account.EnabledEnvironment, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(environments) == 0 {
		return nil, diags
	}
	out := make([]account.EnabledEnvironment, 0, len(environments))
	for _, environment := range environments {
		names, nameDiags := setToStrings(ctx, environment.Environments)
		diags.Append(nameDiags...)
		out = append(out, account.EnabledEnvironment{
			Product:             environment.Product.ValueString(),
			EnabledEnvironments: names,
			ManagedAccountID:    helpers.OptionalStringPointer(environment.ManagedAccountID),
		})
	}
	return &out, diags
}

// buildSettingsUnion selects and fills the settings shape the connection type
// names.
//
// Exactly one variant is set, which is what the union requires: it refuses to
// serialise two, and Jamf refuses a payload that disagrees with the declared
// type with an internal failure naming nothing. The cross-field validator has
// already established that the right block is present, so a missing one here is a
// provider defect rather than a configuration mistake and is reported as such.
func buildSettingsUnion(plan ConnectionResourceModel, common commonSettings) (account.ConnectionRequestConnection, diag.Diagnostics) {
	var diags diag.Diagnostics

	switch plan.ConnectionType.ValueString() {
	case connectionTypeGenericOIDC:
		if plan.GenericOIDC == nil {
			return missingSettingsBlock(&diags, "generic_oidc")
		}
		return account.ConnectionRequestConnection{
			OidcConnectionSettings: buildOIDCSettings(plan, common),
		}, diags
	case connectionTypeEntra:
		if plan.Entra == nil {
			return missingSettingsBlock(&diags, "entra")
		}
		return account.ConnectionRequestConnection{
			EntraConnectionSettings: buildEntraSettings(plan, common),
		}, diags
	case connectionTypeOkta:
		if plan.Okta == nil {
			return missingSettingsBlock(&diags, "okta")
		}
		return account.ConnectionRequestConnection{
			OktaConnectionSettings: buildOktaSettings(plan, common),
		}, diags
	case connectionTypeGoogle:
		if plan.GoogleWorkspace == nil {
			return missingSettingsBlock(&diags, "google_workspace")
		}
		return account.ConnectionRequestConnection{
			GoogleConnectionSettings: buildGoogleSettings(plan, common),
		}, diags
	}

	diags.AddError(
		"Unsupported connection type",
		"The connection type \""+plan.ConnectionType.ValueString()+"\" has no settings shape in this provider "+
			"release. Please report this issue to the provider developers.",
	)
	return account.ConnectionRequestConnection{}, diags
}

// missingSettingsBlock reports a settings block the validator should have caught.
func missingSettingsBlock(diags *diag.Diagnostics, block string) (account.ConnectionRequestConnection, diag.Diagnostics) {
	diags.AddError(
		"Missing connection settings",
		"The `"+block+"` block is required for this connection type but did not reach the request builder. "+
			"Please report this issue to the provider developers.",
	)
	return account.ConnectionRequestConnection{}, *diags
}

// buildOIDCSettings fills the generic OpenID Connect settings.
func buildOIDCSettings(plan ConnectionResourceModel, common commonSettings) *account.OidcConnectionSettings {
	block := plan.GenericOIDC
	return &account.OidcConnectionSettings{
		AliasLoginHintToIdp:              common.aliasLoginHintToIdp,
		AttributeMap:                     common.attributeMap,
		AuthorizationEndpoint:            block.AuthorizationEndpoint.ValueString(),
		ClientID:                         common.clientID,
		ClientSecret:                     common.clientSecret,
		CustomUsernameClaimName:          common.customUsernameClaimName,
		GroupNameFilter:                  common.groupNameFilter,
		IssuerURL:                        block.IssuerURL.ValueString(),
		JwksUri:                          block.JWKSURI.ValueString(),
		Name:                             common.name,
		PkceAuthType:                     common.pkceAuthType,
		Region:                           common.region,
		Scopes:                           plan.Scopes.ValueString(),
		SendNonce:                        common.sendNonce,
		SessionInfo:                      common.sessionInfo,
		SyncUserProfileAttributesAtLogin: common.syncAttributes,
		TokenEndpoint:                    block.TokenEndpoint.ValueString(),
		TokenEndpointAuthMethod:          common.tokenEndpointAuthMethod,
		UserInfoEndpoint:                 helpers.OptionalStringPointer(block.UserInfoEndpoint),
		UsernameDomain:                   common.usernameDomain,
	}
}

// buildEntraSettings fills the Microsoft Entra settings.
//
// Two of Jamf's required fields have no value when an operator leaves them out,
// so a default is supplied here rather than in the schema: putting it in the
// input builder keeps it visible in one place, unit-testable, and out of the way
// of the plan, where a schema default would compete with the value carried
// forward from prior state.
//
// The basic profile is sent as true unconditionally. It is required, and the
// console renders it ticked and greyed out, so it is not a choice — which is why
// the attribute is reported rather than offered.
func buildEntraSettings(plan ConnectionResourceModel, common commonSettings) *account.EntraConnectionSettings {
	block := plan.Entra

	identityAPI := account.EntraIdentityApiMicrosoftIdentityPlatformV2
	if helpers.IsConfiguredValue(block.IdentityAPI) {
		identityAPI = block.IdentityAPI.ValueString()
	}
	maxGroups := defaultMaxGroups
	if helpers.IsConfiguredValue(block.MaxGroups) {
		maxGroups = int(block.MaxGroups.ValueInt64())
	}
	setEmailsVerified := true
	if helpers.IsConfiguredValue(block.SetEmailsVerified) {
		setEmailsVerified = block.SetEmailsVerified.ValueBool()
	}

	return &account.EntraConnectionSettings{
		AliasLoginHintToIdp:              common.aliasLoginHintToIdp,
		AttributeMap:                     common.attributeMap,
		BasicProfile:                     true,
		ClientID:                         common.clientID,
		ClientSecret:                     common.clientSecret,
		CustomUsernameClaimName:          common.customUsernameClaimName,
		Domain:                           block.Domain.ValueString(),
		EnableUsersApi:                   block.EnableUsersAPI.ValueBool(),
		ExtendedProfile:                  block.ExtendedProfile.ValueBool(),
		GroupNameFilter:                  common.groupNameFilter,
		Groups:                           block.GetUserGroups.ValueBool(),
		GroupsScope:                      helpers.OptionalStringPointer(block.GroupsScope),
		IdentityApi:                      identityAPI,
		MaxGroups:                        maxGroups,
		Name:                             common.name,
		NestedGroups:                     block.IncludeNestedGroups.ValueBool(),
		PkceAuthType:                     common.pkceAuthType,
		Region:                           common.region,
		SendNonce:                        common.sendNonce,
		SessionInfo:                      common.sessionInfo,
		SetEmailsVerified:                setEmailsVerified,
		SyncUserProfileAttributesAtLogin: common.syncAttributes,
		TenantDomain:                     block.TenantDomain.ValueString(),
		TokenEndpointAuthMethod:          common.tokenEndpointAuthMethod,
		UseCommonEndpoint:                block.UseCommonEndpoint.ValueBool(),
		UseWsfed:                         block.UseWSFed.ValueBool(),
		UsernameDomain:                   common.usernameDomain,
	}
}

// buildOktaSettings fills the Okta settings. Only the org domain is sent — Jamf
// works the four addresses out from it, which is why they are reported rather
// than declared.
func buildOktaSettings(plan ConnectionResourceModel, common commonSettings) *account.OktaConnectionSettings {
	return &account.OktaConnectionSettings{
		AliasLoginHintToIdp:              common.aliasLoginHintToIdp,
		AttributeMap:                     common.attributeMap,
		ClientID:                         common.clientID,
		ClientSecret:                     common.clientSecret,
		CustomUsernameClaimName:          common.customUsernameClaimName,
		Domain:                           plan.Okta.Domain.ValueString(),
		GroupNameFilter:                  common.groupNameFilter,
		Name:                             common.name,
		PkceAuthType:                     common.pkceAuthType,
		Region:                           common.region,
		Scopes:                           plan.Scopes.ValueString(),
		SendNonce:                        common.sendNonce,
		SessionInfo:                      common.sessionInfo,
		SyncUserProfileAttributesAtLogin: common.syncAttributes,
		TokenEndpointAuthMethod:          common.tokenEndpointAuthMethod,
		UsernameDomain:                   common.usernameDomain,
	}
}

// buildGoogleSettings fills the Google Workspace settings.
//
// Provisional in the same sense the block is: no live Google Workspace
// connection existed anywhere while this was written, so every field comes from
// Jamf's published shape rather than from something seen round-tripped.
func buildGoogleSettings(plan ConnectionResourceModel, common commonSettings) *account.GoogleConnectionSettings {
	block := plan.GoogleWorkspace
	return &account.GoogleConnectionSettings{
		AliasLoginHintToIdp:              common.aliasLoginHintToIdp,
		ApiEnableUsers:                   block.EnableUsersAPI.ValueBool(),
		AttributeMap:                     common.attributeMap,
		ClientID:                         common.clientID,
		ClientSecret:                     common.clientSecret,
		CustomUsernameClaimName:          common.customUsernameClaimName,
		Domain:                           block.Domain.ValueString(),
		ExtendedGroups:                   helpers.OptionalBoolPointer(block.ExtendedGroups),
		GroupNameFilter:                  common.groupNameFilter,
		Groups:                           block.GetUserGroups.ValueBool(),
		Name:                             common.name,
		PkceAuthType:                     common.pkceAuthType,
		Region:                           common.region,
		Scopes:                           helpers.OptionalStringPointer(plan.Scopes),
		SendNonce:                        common.sendNonce,
		SessionInfo:                      common.sessionInfo,
		SyncUserProfileAttributesAtLogin: common.syncAttributes,
		TokenEndpointAuthMethod:          common.tokenEndpointAuthMethod,
		UsernameDomain:                   common.usernameDomain,
	}
}
