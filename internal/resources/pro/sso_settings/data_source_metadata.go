// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"

	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// SsoSpMetadataDataSource exposes the Jamf Pro Service Provider SAML
// metadata document (/v3/sso/metadata/download) as a data source.
type SsoSpMetadataDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SsoSpMetadataDataSource{}

// NewSsoSpMetadataDataSource constructs a new SsoSpMetadataDataSource.
func NewSsoSpMetadataDataSource() datasource.DataSource {
	return &SsoSpMetadataDataSource{}
}

// SsoSpMetadataDataSourceModel is the Terraform model for the metadata DS.
type SsoSpMetadataDataSourceModel struct {
	ID       types.String             `tfsdk:"id"`
	XML      types.String             `tfsdk:"xml"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// Metadata sets the data source type name.
func (d *SsoSpMetadataDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_sso_sp_metadata"
}

// Schema returns the data source schema.
func (d *SsoSpMetadataDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Download the Jamf Pro **Service Provider** SAML metadata XML for the current tenant. Typically consumed by the IdP administrator to register Jamf Pro as a relying party. " +
			"Returns an empty `xml` and a warning when the tenant is configured for pure OIDC (no SAML metadata to publish)." + metadataDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"xml": schema.StringAttribute{
				MarkdownDescription: "Raw SAML SP metadata XML.",
				Computed:            true,
			},
			"timeouts": datasourceTimeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *SsoSpMetadataDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_sso_sp_metadata")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read downloads the SP metadata. A 404 (no SAML configured) is surfaced as
// a warning + empty xml so OIDC-only tenants can still reference the DS
// without an apply-blocking error.
func (d *SsoSpMetadataDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SsoSpMetadataDataSourceModel
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

	// Probe configuration_type first. The /v3/sso/metadata/download
	// endpoint returns 404 in pure OIDC mode, and the SDK retries 404
	// responses as "eventual consistency" — which on a stable OIDC
	// tenant burns ~3 minutes per Read for no useful signal. Skip the
	// download entirely when SAML is not part of the configuration.
	settings, err := d.client.GetSsoSettingsV3(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro SSO settings before metadata download", helpers.APIErrorDetail(err))
		return
	}
	if settings != nil && settings.ConfigurationType == configurationTypeOIDC {
		resp.Diagnostics.AddWarning(
			"Jamf Pro SP metadata not available",
			"The tenant is in pure OIDC mode; there is no SAML Service Provider metadata to download. Set `configuration_type` to `SAML` or `OIDC_WITH_SAML` on `jamfplatform_pro_sso_settings` to enable metadata download.",
		)
		data.XML = types.StringValue("")
		data.ID = types.StringValue(helpers.SingletonID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	body, err := d.client.DownloadSsoMetadataV3(readCtx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddWarning(
				"Jamf Pro SP metadata not available",
				"Jamf Pro returned 404 for the metadata download endpoint even though `configuration_type` includes SAML. The tenant may not have a fully-configured SAML IdP yet.",
			)
			data.XML = types.StringValue("")
			data.ID = types.StringValue(helpers.SingletonID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			return
		}
		resp.Diagnostics.AddError("Unable to download Jamf Pro SSO SP metadata", helpers.APIErrorDetail(err))
		return
	}

	data.XML = types.StringValue(string(body))
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "downloaded Jamf Pro SSO SP metadata")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
