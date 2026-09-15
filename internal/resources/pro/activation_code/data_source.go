// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ActivationCodeDataSource implements the Terraform data source for the Jamf Pro
// activation code.
type ActivationCodeDataSource struct {
	client *proclassic.Client
}

var _ datasource.DataSource = &ActivationCodeDataSource{}

// NewActivationCodeDataSource returns a new instance of ActivationCodeDataSource.
func NewActivationCodeDataSource() datasource.DataSource {
	return &ActivationCodeDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ActivationCodeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_activation_code"
}

// Schema returns the data source schema.
func (d *ActivationCodeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage: dataSourceDeprecation,
		MarkdownDescription: dataSourceDeprecationCallout +
			"Read the current Jamf Pro activation code and organization name. One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"organization_name": schema.StringAttribute{
				MarkdownDescription: "The organization name registered against the activation code.",
				Computed:            true,
			},
			"code": schema.StringAttribute{
				MarkdownDescription: "The Jamf Pro activation code (license key). Treated as sensitive.",
				Computed:            true,
				Sensitive:           true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *ActivationCodeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_activation_code")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current activation code and populates Terraform state.
func (d *ActivationCodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ActivationCodeDataSourceModel
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

	//nolint:staticcheck // SA1019: the Classic /activationcode endpoint is deprecated with no replacement read — see deprecation.go.
	got, err := d.client.GetActivationCode(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro activation code", helpers.APIErrorDetail(err))
		return
	}
	assignActivationCodeDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro activation code data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
