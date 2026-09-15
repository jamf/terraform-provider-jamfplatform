// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DepartmentDataSource implements the Terraform data source for Jamf Pro departments.
type DepartmentDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &DepartmentDataSource{}
	_ datasource.DataSourceWithConfigure = &DepartmentDataSource{}
)

// NewDepartmentDataSource returns a new instance of DepartmentDataSource.
func NewDepartmentDataSource() datasource.DataSource {
	return &DepartmentDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DepartmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_department"
}

// Schema returns the data source schema.
func (d *DepartmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro department by ID." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Department ID to look up.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Department display name.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *DepartmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_department")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a department by ID and populates Terraform state.
func (d *DepartmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DepartmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "The id attribute must be provided to read a Jamf Pro department.")
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := d.client.GetDepartmentV1(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro department", helpers.APIErrorDetail(err))
		return
	}
	assignDepartmentDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro department data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
