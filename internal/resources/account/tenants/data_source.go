// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package tenants implements the jamfplatform_account_tenants data source, a
// read-only view of the tenants a Jamf Account organization holds.
//
// A tenant is one Jamf service instance, such as a Jamf Pro or Jamf Protect
// instance. The organization's tenants are provisioned by Jamf, so there is
// nothing to create, change or delete, and the API exposes no read of a single
// tenant: this package holds only a plural data source, and no list resource over
// a catalogue nobody manages.
//
// The reason it exists is the other half of platform environment membership. A
// platform environment object carries no list of its tenants, because membership
// is recorded on the tenant as the identifier of its environment. Reading the
// tenants is therefore how a configuration finds the unassigned tenants to hand
// to the jamfplatform_account_platform_environment_assign_tenants action, and how
// it sees what a platform environment holds.
//
// # Attribute names
//
//	Terraform attribute        Payload field
//	-------------------------  --------------
//	region                     metaRegion
//	platform_environment_id    environmentId
//
// The console's tenant table labels the metaRegion column "Region", and calls the
// group a tenant belongs to a platform environment. `product` keeps the payload's
// code, such as `pro` or `protect`, where the console shows a display name such
// as "Jamf Pro": the set of codes is open and Jamf adds one with each product, so
// a translation table here would be wrong the day a product ships.
package tenants

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultReadTimeout caps how long the tenants read will wait.
const defaultReadTimeout = 90 * time.Second

// dataSourceID is the fixed ID this data source reports. The collection takes no
// filter, so every read returns the same set and there is nothing to derive an ID
// from.
const dataSourceID = "tenants"

// TenantsDataSource implements the Terraform data source for the tenants of a
// Jamf Account organization.
type TenantsDataSource struct {
	client *account.Client
}

var (
	_ datasource.DataSource              = &TenantsDataSource{}
	_ datasource.DataSourceWithConfigure = &TenantsDataSource{}
)

// NewTenantsDataSource returns a new instance of TenantsDataSource.
func NewTenantsDataSource() datasource.DataSource {
	return &TenantsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *TenantsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_tenants"
}

// Schema returns the data source schema.
func (d *TenantsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every tenant your Jamf Account organization holds. A tenant is one Jamf " +
			"service instance, such as a Jamf Pro or Jamf Protect instance.\n\n" +
			"A tenant belongs to at most one platform environment, and `platform_environment_id` is null while " +
			"it is unassigned. Use this data source to find unassigned tenants for the " +
			"`jamfplatform_account_platform_environment_assign_tenants` action and to see what a platform " +
			"environment holds. Jamf Account offers no search arguments for tenants, so this data source " +
			"takes none. Filter the result in Terraform." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"tenants": schema.ListNestedAttribute{
				MarkdownDescription: "The tenants the organization holds, in the order Jamf Account returns them.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier Jamf Account assigns to the tenant. The assign tenants " +
								"action takes this value.",
							Computed: true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Display name of the tenant. Null when Jamf Account has no name " +
								"for it.",
							Computed: true,
						},
						"product": schema.StringAttribute{
							MarkdownDescription: "Code for the Jamf product the tenant runs, such as `pro` for " +
								"Jamf Pro, `school` or `protect`. Each new product adds a code, so the " +
								"set is open.",
							Computed: true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Class of the product, such as `management` for products that " +
								"manage devices and `security` for security products. The set is open. " +
								"Null when Jamf Account has no class for the product.",
							Computed: true,
						},
						"url": schema.StringAttribute{
							MarkdownDescription: "Address of the tenant. Null for a product whose tenants have no " +
								"address of their own.",
							Computed: true,
						},
						"region": schema.StringAttribute{
							MarkdownDescription: "**\"Region\"** in the Jamf Account console: where the tenant is " +
								"hosted, such as `US`, `EU` or `APAC`. A tenant joins only a platform " +
								"environment in the same region, or a global one.",
							Computed: true,
						},
						"platform_environment_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the platform environment the tenant belongs to. " +
								"Null while the tenant is unassigned.",
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *TenantsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_tenants")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every tenant and populates Terraform state.
func (d *TenantsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check the provider block.",
		)
		return
	}

	var data TenantsDataSourceModel
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

	tenants, err := d.client.ListTenants(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Account tenants", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(dataSourceID)
	data.Tenants = make([]TenantModel, 0, len(tenants))
	for _, t := range tenants {
		data.Tenants = append(data.Tenants, buildTenantModel(t))
	}

	tflog.Trace(ctx, "read Jamf Account tenants data source", map[string]any{"count": len(data.Tenants)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// buildTenantModel maps one tenant into a data source result element.
func buildTenantModel(t account.Tenant) TenantModel {
	return TenantModel{
		ID:                    types.StringValue(t.ID),
		Name:                  types.StringPointerValue(t.Name),
		Product:               types.StringPointerValue(t.Product),
		Type:                  types.StringPointerValue(t.Type),
		URL:                   types.StringPointerValue(t.URL),
		Region:                types.StringPointerValue(t.MetaRegion),
		PlatformEnvironmentID: types.StringPointerValue(t.EnvironmentID),
	}
}
