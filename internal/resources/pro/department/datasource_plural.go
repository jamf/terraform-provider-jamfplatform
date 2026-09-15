// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// DepartmentFilterSelectors enumerates the RSQL selectors accepted by the departments endpoint.
// RSQL selectors retain API-native spelling per STYLE_GUIDE.md §Schema Guidelines.
var DepartmentFilterSelectors = []string{
	"id",
	"name",
}

// DepartmentsDataSource implements the Terraform data source for Jamf Pro department searches.
type DepartmentsDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &DepartmentsDataSource{}
	_ datasource.DataSourceWithConfigure = &DepartmentsDataSource{}
)

// NewDepartmentsDataSource returns a new instance of DepartmentsDataSource.
func NewDepartmentsDataSource() datasource.DataSource {
	return &DepartmentsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DepartmentsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_departments"
}

// Schema returns the plural data source schema.
func (d *DepartmentsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Search Jamf Pro departments using optional RSQL filters." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(DepartmentFilterSelectors),
				DepartmentFilterSelectors,
			),
			"departments": schema.ListNestedAttribute{
				MarkdownDescription: "Departments matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{MarkdownDescription: "Department ID assigned by Jamf Pro.", Computed: true},
						"name": schema.StringAttribute{MarkdownDescription: "Department display name.", Computed: true},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *DepartmentsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_departments")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches departments matching the supplied filters and populates state.
func (d *DepartmentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DepartmentsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(DepartmentFilterSelectors))
	tflog.Debug(ctx, "departments filter expression", map[string]any{"filter": filterExpression})

	list, err := d.client.ListDepartmentsV1(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro departments", helpers.APIErrorDetail(err))
		return
	}

	results := make([]DepartmentsDataSourceResultModel, 0, len(list))
	for _, dep := range list {
		results = append(results, DepartmentsDataSourceResultModel{
			ID:   helpers.StringPointerValueOrNull(dep.ID),
			Name: types.StringValue(dep.Name),
		})
	}

	data.Departments = results
	data.ID = types.StringValue("departments")

	tflog.Trace(ctx, "listed Jamf Pro departments data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
