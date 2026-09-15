// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetDeviceEnrollmentPublicKeyV1
// Status: current. Last reviewed 2026-05-25.

// Package automated_device_enrollment_public_key implements the
// jamfplatform_pro_automated_device_enrollment_public_key data source backed
// by the Jamf Pro `/api/v1/device-enrollments/public-key` endpoint.
package automated_device_enrollment_public_key

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// data source. Empty: defer to the provider-wide floor via
// providerdata.ConfigurePro — the device-enrollments public-key endpoint
// predates the provider's overall minimum.
const minJamfProVersion = ""

const defaultReadTimeout = 60 * time.Second

// singletonID is the literal string used to populate the data source `id`
// attribute. There is exactly one ADE public key per Jamf Pro tenant, so the
// data source takes no input selector and a stable synthetic identifier
// keeps Terraform's required `id` semantics satisfied.
const singletonID = "singleton"

// AutomatedDeviceEnrollmentPublicKeyDataSource implements the singleton
// Terraform data source for the tenant's Jamf Pro ADE public key.
type AutomatedDeviceEnrollmentPublicKeyDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &AutomatedDeviceEnrollmentPublicKeyDataSource{}
	_ datasource.DataSourceWithConfigure = &AutomatedDeviceEnrollmentPublicKeyDataSource{}
)

// NewAutomatedDeviceEnrollmentPublicKeyDataSource returns a new instance of
// the data source.
func NewAutomatedDeviceEnrollmentPublicKeyDataSource() datasource.DataSource {
	return &AutomatedDeviceEnrollmentPublicKeyDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AutomatedDeviceEnrollmentPublicKeyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_automated_device_enrollment_public_key"
}

// Schema returns the data source schema. There is no input selector — the
// endpoint returns a single public key per tenant.
func (d *AutomatedDeviceEnrollmentPublicKeyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the Jamf Pro Automated Device Enrollment (ADE) public key for the current tenant. Jamf Pro returns a binary blob, and the data source base64-encodes the bytes into the `public_key` attribute so it can be safely embedded in Terraform configuration, for example when registering an MDM server in Apple Business Manager or Apple School Manager. Your tenant has one key, so no input selector is required." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Stable synthetic identifier for the singleton public key (`\"singleton\"`).",
				Computed:            true,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded body of the Jamf Pro ADE public key.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source. No version gate
// is applied — the endpoint predates the provider's minimum Pro version.
func (d *AutomatedDeviceEnrollmentPublicKeyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_automated_device_enrollment_public_key")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the tenant ADE public key and populates Terraform state.
func (d *AutomatedDeviceEnrollmentPublicKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AutomatedDeviceEnrollmentPublicKeyDataSourceModel
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

	bytes, err := d.client.GetDeviceEnrollmentPublicKeyV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to fetch Jamf Pro Automated Device Enrollment public key", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(singletonID)
	data.PublicKey = types.StringValue(base64.StdEncoding.EncodeToString(bytes))

	tflog.Trace(ctx, "read Jamf Pro Automated Device Enrollment public key data source", map[string]any{"bytes": len(bytes)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
