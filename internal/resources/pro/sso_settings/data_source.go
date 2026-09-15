// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

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

// SsoSettingsDataSource is the read-only mirror of the SSO settings resource.
type SsoSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SsoSettingsDataSource{}

// NewSsoSettingsDataSource constructs a new SsoSettingsDataSource.
func NewSsoSettingsDataSource() datasource.DataSource {
	return &SsoSettingsDataSource{}
}

// Metadata sets the data source type name.
func (d *SsoSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_sso_settings"
}

// Schema returns the data source schema. Every attribute is Computed; the
// resource schema is mirrored field-for-field minus WriteOnly inputs and
// the rotation companions, which carry no read-side signal.
func (d *SsoSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Single Sign-On configuration. One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":                                 schema.StringAttribute{Computed: true, MarkdownDescription: "Fixed singleton identifier. Always `singleton`."},
			"sso_enabled":                        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether SSO is enabled on the tenant."},
			"sso_bypass_allowed":                 schema.BoolAttribute{Computed: true, MarkdownDescription: "Allow administrators to bypass SSO when signing in."},
			"sso_for_enrollment_enabled":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Enable SSO for user-initiated enrollment."},
			"sso_for_macos_self_service_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Enable SSO for the macOS Self Service app."},
			"enrollment_sso_for_account_driven_enrollment_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Enable SSO for Account-Driven Enrollment."},
			"group_enrollment_access_enabled":                      schema.BoolAttribute{Computed: true, MarkdownDescription: "Restrict enrollment SSO to a single LDAP/IdP group."},
			"group_enrollment_access_name":                         schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the LDAP/IdP group allowed to enroll."},
			"configuration_type":                                   schema.StringAttribute{Computed: true, MarkdownDescription: "SSO configuration type."},
			"oidc_settings": schema.SingleNestedAttribute{
				Computed: true, MarkdownDescription: "OIDC configuration.",
				Attributes: map[string]schema.Attribute{
					"user_mapping":                     schema.StringAttribute{Computed: true},
					"jamf_id_authentication_enabled":   schema.BoolAttribute{Computed: true},
					"username_attribute_claim_mapping": schema.StringAttribute{Computed: true},
				},
			},
			"saml_settings": schema.SingleNestedAttribute{
				Computed: true, MarkdownDescription: "SAML configuration.",
				Attributes: map[string]schema.Attribute{
					"idp_provider_type":         schema.StringAttribute{Computed: true},
					"other_provider_type_name":  schema.StringAttribute{Computed: true},
					"entity_id":                 schema.StringAttribute{Computed: true},
					"metadata_source":           schema.StringAttribute{Computed: true},
					"idp_url":                   schema.StringAttribute{Computed: true},
					"federation_metadata_file":  schema.StringAttribute{Computed: true, Sensitive: true},
					"metadata_file_name":        schema.StringAttribute{Computed: true},
					"session_timeout":           schema.Int64Attribute{Computed: true},
					"token_expiration_disabled": schema.BoolAttribute{Computed: true},
					"user_mapping":              schema.StringAttribute{Computed: true},
					"user_attribute_enabled":    schema.BoolAttribute{Computed: true},
					"user_attribute_name":       schema.StringAttribute{Computed: true},
					"group_attribute_name":      schema.StringAttribute{Computed: true},
					"group_rdn_key":             schema.StringAttribute{Computed: true},
				},
			},
			"enrollment_sso_config": schema.SingleNestedAttribute{
				Computed: true, MarkdownDescription: "Account-Driven Enrollment SSO configuration.",
				Attributes: map[string]schema.Attribute{
					"hosts":           schema.SetAttribute{Computed: true, ElementType: types.StringType},
					"management_hint": schema.StringAttribute{Computed: true},
				},
			},
			"signing_certificate": schema.SingleNestedAttribute{
				Computed: true, MarkdownDescription: "Currently-installed SSO signing certificate details.",
				Attributes: map[string]schema.Attribute{
					"setup_type":         schema.StringAttribute{Computed: true},
					"type":               schema.StringAttribute{Computed: true},
					"key":                schema.StringAttribute{Computed: true},
					"keystore_file_name": schema.StringAttribute{Computed: true},
					"serial_number":      schema.StringAttribute{Computed: true},
					"subject":            schema.StringAttribute{Computed: true},
					"issuer":             schema.StringAttribute{Computed: true},
					"expiration":         schema.StringAttribute{Computed: true},
					"keys": schema.ListNestedAttribute{
						Computed: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":    schema.StringAttribute{Computed: true},
								"valid": schema.BoolAttribute{Computed: true},
							},
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *SsoSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_sso_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read populates state from /v3/sso + /v2/sso/cert.
func (d *SsoSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SsoSettingsDataSourceModel
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

	settings, err := d.client.GetSsoSettingsV3(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro SSO settings", helpers.APIErrorDetail(err))
		return
	}

	cert, err := d.client.GetSsoCertificateV2(readCtx)
	if err != nil && !helpers.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Unable to read Jamf Pro SSO signing certificate", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignSsoSettingsDataSourceModel(ctx, &data, settings, cert)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro SSO settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
