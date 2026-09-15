// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_settings

import (
	"context"
	"strconv"
	"strings"

	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// SsoDependenciesDataSource exposes /v3/sso/dependencies as a data source.
type SsoDependenciesDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SsoDependenciesDataSource{}

// NewSsoDependenciesDataSource constructs a new SsoDependenciesDataSource.
func NewSsoDependenciesDataSource() datasource.DataSource {
	return &SsoDependenciesDataSource{}
}

// SsoDependenciesDataSourceModel is the Terraform model for the dependencies DS.
type SsoDependenciesDataSourceModel struct {
	ID           types.String             `tfsdk:"id"`
	Dependencies types.List               `tfsdk:"dependencies"`
	Timeouts     datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// ssoDependencyAttrTypes is the attribute type map for each dependency object.
var ssoDependencyAttrTypes = map[string]attr.Type{
	"id":                  types.StringType,
	"name":                types.StringType,
	"human_readable_name": types.StringType,
	"hyperlink":           types.StringType,
}

// Metadata sets the data source type name.
func (d *SsoDependenciesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_sso_dependencies"
}

// Schema returns the data source schema.
func (d *SsoDependenciesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List Jamf Pro objects (typically Enrollment Customizations) currently consuming the tenant's SSO configuration." + dependenciesDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"dependencies": schema.ListNestedAttribute{
				MarkdownDescription: "SSO consumers.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Numeric identifier extracted from the consumer's UI hyperlink."},
						"name":                schema.StringAttribute{Computed: true, MarkdownDescription: "Display name configured on the consumer."},
						"human_readable_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Consumer object category (e.g. `Enrollment Customization`)."},
						"hyperlink":           schema.StringAttribute{Computed: true, MarkdownDescription: "Path to the consumer in the Jamf Pro admin UI."},
					},
				},
			},
			"timeouts": datasourceTimeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *SsoDependenciesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_sso_dependencies")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches /v3/sso/dependencies.
func (d *SsoDependenciesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SsoDependenciesDataSourceModel
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

	got, err := d.client.GetSsoDependenciesV3(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro SSO dependencies", helpers.APIErrorDetail(err))
		return
	}

	list, listDiags := buildSsoDependenciesList(ctx, got)
	resp.Diagnostics.Append(listDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Dependencies = list
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro SSO dependencies")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// buildSsoDependenciesList converts the SDK response into a list-of-object
// Terraform value. The numeric ID is extracted from the trailing path
// segment of `hyperlink` (e.g. `/.../enrollment-customization/108` → "108").
func buildSsoDependenciesList(ctx context.Context, got *pro.EnrollmentCustomizationDependencies) (types.List, diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: ssoDependencyAttrTypes}
	if got == nil || len(got.Dependencies) == 0 {
		return types.ListValueMust(elemType, []attr.Value{}), nil
	}
	values := make([]attr.Value, 0, len(got.Dependencies))
	for _, dep := range got.Dependencies {
		obj, d := types.ObjectValue(ssoDependencyAttrTypes, map[string]attr.Value{
			"id":                  types.StringValue(extractIDFromHyperlink(dep.Hyperlink)),
			"name":                types.StringValue(dep.Name),
			"human_readable_name": types.StringValue(dep.HumanReadableName),
			"hyperlink":           types.StringValue(dep.Hyperlink),
		})
		if d.HasError() {
			return types.ListNull(elemType), d
		}
		values = append(values, obj)
	}
	return types.ListValueFrom(ctx, elemType, values)
}

// extractIDFromHyperlink returns the trailing path segment of a UI
// hyperlink, or "" when the value is empty.
func extractIDFromHyperlink(link string) string {
	if link == "" {
		return ""
	}
	trimmed := strings.TrimRight(link, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		candidate := trimmed[idx+1:]
		if _, err := strconv.Atoi(candidate); err == nil {
			return candidate
		}
		return candidate
	}
	return trimmed
}
