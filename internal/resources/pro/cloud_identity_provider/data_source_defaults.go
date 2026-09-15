// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	pro.GetCloudAzureDefaultServerConfigurationV1
//	pro.GetCloudLdapDefaultServerConfigurationV2
//	pro.GetCloudLdapDefaultMappingsV2
//
// Status: current. Last reviewed 2026-09-08.
//
// The Entra ID defaults come from one read because the server-configuration
// response carries its mappings inline. There is a second endpoint that returns
// those mappings alone, /v1/cloud-azure/defaults/mappings, and it is deliberately
// not called: the spec marks it deprecated (deprecation-date 2025-05-21) and its
// body is a subset of what the adopted endpoint already returns. Both were
// captured on the EU tenant 2026-09-08 and compared.
//
// Google needs two reads, and its path carries a provider segment with exactly
// one legal value. GOOGLE and google both answer 200; AZURE, ENTRA_ID, OPENLDAP
// and a nonsense value each answer 404 NOT_FOUND "The provider name in the URL is
// invalid or not supported." So the segment is a constant, not an argument.
package cloud_identity_provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// CloudIdentityProviderDefaultsDataSource implements the Terraform data source
// for the attribute mappings and connection settings Jamf Pro pre-fills when an
// administrator adds a cloud identity provider.
//
// It takes no arguments. Both products' defaults exist independently of what the
// tenant has configured, so both blocks are always populated and there is no
// provider selector to supply.
type CloudIdentityProviderDefaultsDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &CloudIdentityProviderDefaultsDataSource{}
	_ datasource.DataSourceWithConfigure = &CloudIdentityProviderDefaultsDataSource{}
)

// NewCloudIdentityProviderDefaultsDataSource returns a new instance of
// CloudIdentityProviderDefaultsDataSource.
func NewCloudIdentityProviderDefaultsDataSource() datasource.DataSource {
	return &CloudIdentityProviderDefaultsDataSource{}
}

// Metadata sets the data source type name.
func (d *CloudIdentityProviderDefaultsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_identity_provider_defaults"
}

// Schema returns the data source schema.
//
// Every mapping attribute name matches the managed resource's, so a whole
// mappings object can be assigned across. The reused model types make that hold
// by construction and TestDefaultsSchema_MappingsMatchTheResource pins it.
func (d *CloudIdentityProviderDefaultsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Attribute mappings and connection settings Jamf Pro pre-fills when an administrator adds a cloud identity provider, for Microsoft Entra ID and for Google Secure LDAP. " +
			"Jamf Pro reports both whether or not the tenant has a connection of either kind, so there is nothing to select. " +
			"Use it to seed a `jamfplatform_pro_cloud_identity_provider` mappings block, which owns every field in it once declared." + defaultsDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"entra_id": schema.SingleNestedAttribute{
				MarkdownDescription: "Microsoft Entra ID defaults.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"type":                                        computedString("Entra connection type (e.g. `PUBLIC`)."),
					"search_timeout":                              computedInt64("Search timeout in seconds."),
					"transitive_membership_enabled":               computedBool("Whether transitive group membership is enabled."),
					"transitive_membership_user_field":            computedString("User field used for transitive membership lookups."),
					"transitive_directory_membership_enabled":     computedBool("Whether transitive directory membership is enabled."),
					"membership_calculation_optimization_enabled": computedBool("Whether membership-calculation optimization is enabled."),
					"mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "Default Entra ID attribute mappings. `building` and `room` come back empty; the other nine name a directory attribute.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"user_id":    computedString("Attribute mapped to user ID."),
							"user_name":  computedString("Attribute mapped to username."),
							"real_name":  computedString("Attribute mapped to real name."),
							"email":      computedString("Attribute mapped to email."),
							"department": computedString("Attribute mapped to department."),
							"building":   computedString("Attribute mapped to building."),
							"room":       computedString("Attribute mapped to room."),
							"phone":      computedString("Attribute mapped to phone."),
							"position":   computedString("Attribute mapped to position."),
							"group_id":   computedString("Attribute mapped to group ID."),
							"group_name": computedString("Attribute mapped to group name."),
						},
					},
				},
			},

			"google": schema.SingleNestedAttribute{
				MarkdownDescription: "Google Secure LDAP defaults.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"server": schema.SingleNestedAttribute{
						MarkdownDescription: "Default Google LDAP connection settings. Jamf Pro pre-fills no domain and no certificate keystore, so neither appears here.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"server_url":         computedString("Google Secure LDAP hostname."),
							"port":               computedInt64("LDAPS port."),
							"connection_type":    computedString("Connection type (e.g. `LDAPS`)."),
							"connection_timeout": computedInt64("Connection timeout in seconds."),
							"search_timeout":     computedInt64("Search timeout in seconds."),
							"use_wildcards":      computedBool("Whether searches use wildcards."),
							"membership_calculation_optimization_enabled": computedBool("Whether membership-calculation optimization is enabled."),
						},
					},
					"mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "Default Google LDAP attribute mappings. `user_mappings.additional_search_base` reads as null: Jamf Pro pre-fills an empty value and then rejects an empty value on a write, so supply a distinguished name of your own.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"user_mappings": schema.SingleNestedAttribute{
								MarkdownDescription: "Default user attribute mappings.",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"object_class_limitation": computedString("Object-class limitation (e.g. `ANY_OBJECT_CLASSES`)."),
									"object_classes":          computedString("Object classes (e.g. `inetOrgPerson`)."),
									"search_base":             computedString("User search base (e.g. `ou=Users`)."),
									"search_scope":            computedString("User search scope (e.g. `ALL_SUBTREES`)."),
									"additional_search_base":  computedString("Additional user search base. Jamf Pro pre-fills an empty value and rejects one on a write, so this reads as null."),
									"user_id":                 computedString("Attribute mapped to user ID."),
									"username":                computedString("Attribute mapped to username."),
									"real_name":               computedString("Attribute mapped to real name."),
									"email_address":           computedString("Attribute mapped to email address."),
									"department":              computedString("Attribute mapped to department."),
									"building":                computedString("Attribute mapped to building."),
									"room":                    computedString("Attribute mapped to room."),
									"phone":                   computedString("Attribute mapped to phone."),
									"position":                computedString("Attribute mapped to position."),
									"user_uuid":               computedString("Attribute mapped to user UUID."),
								},
							},
							"group_mappings": schema.SingleNestedAttribute{
								MarkdownDescription: "Default group attribute mappings.",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"object_class_limitation": computedString("Object-class limitation (e.g. `ANY_OBJECT_CLASSES`)."),
									"object_classes":          computedString("Object classes (e.g. `groupOfNames`)."),
									"search_base":             computedString("Group search base (e.g. `ou=Groups`)."),
									"search_scope":            computedString("Group search scope (e.g. `ALL_SUBTREES`)."),
									"group_id":                computedString("Attribute mapped to group ID."),
									"group_name":              computedString("Attribute mapped to group name."),
									"group_uuid":              computedString("Attribute mapped to group UUID."),
								},
							},
							"membership_mappings": schema.SingleNestedAttribute{
								MarkdownDescription: "Default group-membership attribute mapping.",
								Computed:            true,
								Attributes: map[string]schema.Attribute{
									"group_membership_mapping": computedString("Attribute mapped to group membership."),
								},
							},
						},
					},
				},
			},

			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *CloudIdentityProviderDefaultsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_identity_provider_defaults")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches both products' defaults.
//
// Three reads, and a failure on any of them fails the data source. Reporting a
// partial answer would read as "Jamf Pro has no Google defaults", which is worse
// than an error naming the read that failed.
func (d *CloudIdentityProviderDefaultsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var data CloudIdentityProviderDefaultsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	entra, err := d.client.GetCloudAzureDefaultServerConfigurationV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Microsoft Entra ID cloud identity provider defaults", helpers.APIErrorDetail(err))
		return
	}
	googleServer, err := d.client.GetCloudLdapDefaultServerConfigurationV2(readCtx, providerGoogle)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Google Secure LDAP connection defaults", helpers.APIErrorDetail(err))
		return
	}
	googleMappings, err := d.client.GetCloudLdapDefaultMappingsV2(readCtx, providerGoogle)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Google Secure LDAP attribute mapping defaults", helpers.APIErrorDetail(err))
		return
	}

	data.EntraID = assignEntraIDDefaults(entra)
	data.Google = assignGoogleDefaults(googleServer, googleMappings)

	tflog.Trace(ctx, "read Jamf Pro cloud identity provider defaults")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignEntraIDDefaults folds the Entra ID defaults response into the data
// source model, mappings included: that response carries them inline.
func assignEntraIDDefaults(s *pro.AzureServerConfiguration) *cloudIdentityProviderEntraIDDefaultsModel {
	if s == nil {
		return nil
	}
	return &cloudIdentityProviderEntraIDDefaultsModel{
		Type:                                     types.StringValue(s.Type),
		SearchTimeout:                            types.Int64Value(int64(s.SearchTimeout)),
		TransitiveMembershipEnabled:              types.BoolValue(s.TransitiveMembershipEnabled),
		TransitiveMembershipUserField:            types.StringValue(s.TransitiveMembershipUserField),
		TransitiveDirectoryMembershipEnabled:     types.BoolValue(s.TransitiveDirectoryMembershipEnabled),
		MembershipCalculationOptimizationEnabled: types.BoolValue(s.MembershipCalculationOptimizationEnabled),
		Mappings:                                 assignAzureMappingsDefaults(s.Mappings),
	}
}

// assignAzureMappingsDefaults folds the Entra ID mapping defaults into the
// resource's own mappings model, so the object assigns straight into a managed
// resource's mappings block.
func assignAzureMappingsDefaults(m *pro.AzureMappings) *cloudAzureMappingsModel {
	if m == nil {
		return nil
	}
	return &cloudAzureMappingsModel{
		UserID:     types.StringValue(m.UserID),
		UserName:   types.StringValue(m.UserName),
		RealName:   types.StringValue(m.RealName),
		Email:      types.StringValue(m.Email),
		Department: types.StringValue(m.Department),
		Building:   types.StringValue(m.Building),
		Room:       types.StringValue(m.Room),
		Phone:      types.StringValue(m.Phone),
		Position:   types.StringValue(m.Position),
		GroupID:    types.StringValue(m.GroupID),
		GroupName:  types.StringValue(m.GroupName),
	}
}

// assignGoogleDefaults folds the two Google reads into one block.
func assignGoogleDefaults(s *pro.CloudLdapServerResponse, m *pro.CloudLdapMappingsResponse) *cloudIdentityProviderGoogleDefaultsModel {
	out := &cloudIdentityProviderGoogleDefaultsModel{}
	if s != nil {
		out.Server = &cloudIdentityProviderGoogleServerDefaultsModel{
			ServerURL:                                types.StringValue(s.ServerURL),
			Port:                                     types.Int64Value(int64(s.Port)),
			ConnectionType:                           types.StringValue(s.ConnectionType),
			ConnectionTimeout:                        types.Int64Value(int64(s.ConnectionTimeout)),
			SearchTimeout:                            types.Int64Value(int64(s.SearchTimeout)),
			UseWildcards:                             types.BoolValue(s.UseWildcards),
			MembershipCalculationOptimizationEnabled: types.BoolValue(s.MembershipCalculationOptimizationEnabled),
		}
	}
	if m != nil {
		out.Mappings = &cloudLdapMappingsModel{
			UserMappings:       assignGoogleUserMappingsDefaults(m.UserMappings),
			GroupMappings:      assignGoogleGroupMappingsDefaults(m.GroupMappings),
			MembershipMappings: assignGoogleMembershipMappingsDefaults(m.MembershipMappings),
		}
	}
	return out
}

// assignGoogleUserMappingsDefaults folds the default user mappings into the
// resource's own model.
func assignGoogleUserMappingsDefaults(m *pro.UserMappings) *cloudLdapUserMappingsModel {
	if m == nil {
		return nil
	}
	return &cloudLdapUserMappingsModel{
		ObjectClassLimitation: types.StringValue(m.ObjectClassLimitation),
		ObjectClasses:         types.StringValue(m.ObjectClasses),
		SearchBase:            types.StringValue(m.SearchBase),
		SearchScope:           types.StringValue(m.SearchScope),
		AdditionalSearchBase:  helpers.StringPointerValueOrNull(m.AdditionalSearchBase),
		UserID:                types.StringValue(m.UserID),
		Username:              types.StringValue(m.Username),
		RealName:              types.StringValue(m.RealName),
		EmailAddress:          types.StringValue(m.EmailAddress),
		Department:            types.StringValue(m.Department),
		Building:              types.StringValue(m.Building),
		Room:                  types.StringValue(m.Room),
		Phone:                 types.StringValue(m.Phone),
		Position:              types.StringValue(m.Position),
		UserUUID:              types.StringValue(m.UserUUID),
	}
}

// assignGoogleGroupMappingsDefaults folds the default group mappings into the
// resource's own model.
func assignGoogleGroupMappingsDefaults(m *pro.GroupMappings) *cloudLdapGroupMappingsModel {
	if m == nil {
		return nil
	}
	return &cloudLdapGroupMappingsModel{
		ObjectClassLimitation: types.StringValue(m.ObjectClassLimitation),
		ObjectClasses:         types.StringValue(m.ObjectClasses),
		SearchBase:            types.StringValue(m.SearchBase),
		SearchScope:           types.StringValue(m.SearchScope),
		GroupID:               types.StringValue(m.GroupID),
		GroupName:             types.StringValue(m.GroupName),
		GroupUUID:             types.StringValue(m.GroupUUID),
	}
}

// assignGoogleMembershipMappingsDefaults folds the default membership mapping
// into the resource's own model.
func assignGoogleMembershipMappingsDefaults(m *pro.MembershipMappings) *cloudLdapMembershipMappingsModel {
	if m == nil {
		return nil
	}
	return &cloudLdapMembershipMappingsModel{
		GroupMembershipMapping: types.StringValue(m.GroupMembershipMapping),
	}
}
