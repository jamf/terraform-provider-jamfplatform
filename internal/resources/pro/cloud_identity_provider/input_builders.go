// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"encoding/base64"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// defaultKeystoreFileName is used when the user supplies a keystore without an
// explicit file_name.
const defaultKeystoreFileName = "keystore.p12"

// --- Google (Cloud LDAP) input builders ---------------------------------

// buildGoogleCreateRequest assembles the Cloud LDAP create body from the plan
// (server connection + mappings) and config (the WriteOnly keystore). The
// keystore is always included on create — Google Secure LDAP requires a client
// certificate; an absent one falls to a clear server 400.
func buildGoogleCreateRequest(plan, cfg CloudIdentityProviderResourceModel) (*pro.LdapConfigurationRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	server := plan.Google.Server

	keystore, d := buildKeystoreFile(server.Keystore, configKeystore(cfg))
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	req := &pro.LdapConfigurationRequest{
		CloudIDPCommon: pro.CloudIDPCommonRequest{
			DisplayName:  plan.DisplayName.ValueString(),
			ProviderName: providerGoogle,
		},
		Server:   buildServerRequest(server, keystore),
		Mappings: buildMappingsRequest(plan.Google.Mappings),
	}
	return req, diags
}

// buildGoogleUpdateRequest assembles the Cloud LDAP update body. The keystore
// is included only when the rotation trigger (wo_version) changed; otherwise
// it is omitted (nil) and Jamf Pro preserves the stored certificate.
func buildGoogleUpdateRequest(plan, state, cfg CloudIdentityProviderResourceModel) (*pro.LdapConfigurationUpdate, diag.Diagnostics) {
	var diags diag.Diagnostics
	server := plan.Google.Server

	var keystorePtr *pro.CloudLdapKeystoreFile
	if keystoreRotated(plan, state) {
		ks, d := buildKeystoreFile(server.Keystore, configKeystore(cfg))
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		keystorePtr = &ks
	}

	req := &pro.LdapConfigurationUpdate{
		CloudIDPCommon: pro.CloudIDPCommon{
			ID:           plan.ID.ValueString(),
			DisplayName:  plan.DisplayName.ValueString(),
			ProviderName: providerGoogle,
		},
		Server: pro.CloudLdapServerUpdate{
			ServerURL:                                server.ServerURL.ValueString(),
			DomainName:                               server.DomainName.ValueString(),
			Port:                                     int(server.Port.ValueInt64()),
			ConnectionType:                           server.ConnectionType.ValueString(),
			ConnectionTimeout:                        int(server.ConnectionTimeout.ValueInt64()),
			SearchTimeout:                            int(server.SearchTimeout.ValueInt64()),
			UseWildcards:                             server.UseWildcards.ValueBool(),
			Enabled:                                  server.Enabled.ValueBool(),
			MembershipCalculationOptimizationEnabled: helpers.OptionalBoolPointer(server.MembershipCalculationOptimizationEnabled),
			Keystore:                                 keystorePtr,
		},
		Mappings: buildMappingsRequest(plan.Google.Mappings),
	}
	return req, diags
}

// buildServerRequest builds the create-shape server config (keystore is a
// required non-pointer field on create).
func buildServerRequest(server *cloudLdapServerModel, keystore pro.CloudLdapKeystoreFile) pro.CloudLdapServerRequest {
	return pro.CloudLdapServerRequest{
		ServerURL:                                server.ServerURL.ValueString(),
		DomainName:                               server.DomainName.ValueString(),
		Port:                                     int(server.Port.ValueInt64()),
		ConnectionType:                           server.ConnectionType.ValueString(),
		ConnectionTimeout:                        int(server.ConnectionTimeout.ValueInt64()),
		SearchTimeout:                            int(server.SearchTimeout.ValueInt64()),
		UseWildcards:                             server.UseWildcards.ValueBool(),
		Enabled:                                  server.Enabled.ValueBool(),
		MembershipCalculationOptimizationEnabled: helpers.OptionalBoolPointer(server.MembershipCalculationOptimizationEnabled),
		Keystore:                                 keystore,
	}
}

// buildKeystoreFile decodes the WriteOnly base64 keystore from config into the
// SDK keystore-file shape. file_name comes from the plan (it is not WriteOnly);
// it falls back to a default when the user did not set it.
func buildKeystoreFile(planKeystore, cfgKeystore *cloudLdapKeystoreModel) (pro.CloudLdapKeystoreFile, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := pro.CloudLdapKeystoreFile{FileName: defaultKeystoreFileName}

	if cfgKeystore != nil && !cfgKeystore.File.IsNull() && !cfgKeystore.File.IsUnknown() {
		raw := strings.TrimSpace(cfgKeystore.File.ValueString())
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			diags.AddError(
				"Invalid keystore encoding",
				"The Google keystore `file` is not valid base64. Supply the PKCS#12 certificate as base64, e.g. file = filebase64(\"google-ldap.p12\"). Decode error: "+helpers.APIErrorDetail(err),
			)
			return out, diags
		}
		out.FileBytes = decoded
	}
	if cfgKeystore != nil && !cfgKeystore.Password.IsNull() && !cfgKeystore.Password.IsUnknown() {
		out.Password = cfgKeystore.Password.ValueString()
	}
	if planKeystore != nil && !planKeystore.FileName.IsNull() && !planKeystore.FileName.IsUnknown() && planKeystore.FileName.ValueString() != "" {
		out.FileName = planKeystore.FileName.ValueString()
	}
	return out, diags
}

// configKeystore returns the config-side keystore model (where the WriteOnly
// file + password survive), or nil when the config has no google/server/keystore.
func configKeystore(cfg CloudIdentityProviderResourceModel) *cloudLdapKeystoreModel {
	if cfg.Google == nil || cfg.Google.Server == nil {
		return nil
	}
	return cfg.Google.Server.Keystore
}

// keystoreRotated reports whether the keystore rotation trigger (wo_version)
// changed between state and plan — the signal to re-upload the certificate.
func keystoreRotated(plan, state CloudIdentityProviderResourceModel) bool {
	planV := keystoreWoVersion(plan)
	stateV := keystoreWoVersion(state)
	return !planV.Equal(stateV)
}

// keystoreWoVersion safely extracts the keystore rotation trigger.
func keystoreWoVersion(m CloudIdentityProviderResourceModel) types.Int64 {
	if m.Google == nil || m.Google.Server == nil || m.Google.Server.Keystore == nil {
		return types.Int64Null()
	}
	return m.Google.Server.Keystore.WoVersion
}

// buildMappingsRequest builds the inline mappings payload, or nil when the
// user omitted the whole block (Jamf Pro then generates Google defaults).
func buildMappingsRequest(m *cloudLdapMappingsModel) *pro.CloudLdapMappingsRequest {
	if m == nil {
		return nil
	}
	return &pro.CloudLdapMappingsRequest{
		UserMappings:       buildUserMappings(m.UserMappings),
		GroupMappings:      buildGroupMappings(m.GroupMappings),
		MembershipMappings: buildMembershipMappings(m.MembershipMappings),
	}
}

func buildUserMappings(m *cloudLdapUserMappingsModel) pro.UserMappings {
	if m == nil {
		return pro.UserMappings{}
	}
	return pro.UserMappings{
		ObjectClassLimitation: m.ObjectClassLimitation.ValueString(),
		ObjectClasses:         m.ObjectClasses.ValueString(),
		SearchBase:            m.SearchBase.ValueString(),
		SearchScope:           m.SearchScope.ValueString(),
		AdditionalSearchBase:  helpers.OptionalStringPointer(m.AdditionalSearchBase),
		UserID:                m.UserID.ValueString(),
		Username:              m.Username.ValueString(),
		RealName:              m.RealName.ValueString(),
		EmailAddress:          m.EmailAddress.ValueString(),
		Department:            m.Department.ValueString(),
		Building:              m.Building.ValueString(),
		Room:                  m.Room.ValueString(),
		Phone:                 m.Phone.ValueString(),
		Position:              m.Position.ValueString(),
		UserUUID:              m.UserUUID.ValueString(),
	}
}

func buildGroupMappings(m *cloudLdapGroupMappingsModel) pro.GroupMappings {
	if m == nil {
		return pro.GroupMappings{}
	}
	return pro.GroupMappings{
		ObjectClassLimitation: m.ObjectClassLimitation.ValueString(),
		ObjectClasses:         m.ObjectClasses.ValueString(),
		SearchBase:            m.SearchBase.ValueString(),
		SearchScope:           m.SearchScope.ValueString(),
		GroupID:               m.GroupID.ValueString(),
		GroupName:             m.GroupName.ValueString(),
		GroupUUID:             m.GroupUUID.ValueString(),
	}
}

func buildMembershipMappings(m *cloudLdapMembershipMappingsModel) pro.MembershipMappings {
	if m == nil {
		return pro.MembershipMappings{}
	}
	return pro.MembershipMappings{
		GroupMembershipMapping: m.GroupMembershipMapping.ValueString(),
	}
}

// --- Azure (Cloud Azure) input builders are in input_builders_azure.go ---
