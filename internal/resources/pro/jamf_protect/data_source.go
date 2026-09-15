// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_protect

import (
	"context"

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

// JamfProtectPlansDataSource implements the read-only plural catalog of
// synced Jamf Protect plans (jamfplatform_pro_jamf_protect_plans).
type JamfProtectPlansDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &JamfProtectPlansDataSource{}
	_ datasource.DataSourceWithConfigure = &JamfProtectPlansDataSource{}
)

// NewJamfProtectPlansDataSource returns a new instance of JamfProtectPlansDataSource.
func NewJamfProtectPlansDataSource() datasource.DataSource {
	return &JamfProtectPlansDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *JamfProtectPlansDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_protect_plans"
}

// Schema returns the plural data source schema.
func (d *JamfProtectPlansDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns the Jamf Protect plans previously synced into Jamf Pro (Settings → Jamf apps → Jamf Protect → Plans), with each plan's associated configuration profile. " +
			"The catalog reflects the most recent plans sync rather than the live Protect instance. Refresh it by managing the registration with `jamfplatform_pro_jamf_protect` (which syncs on every apply), or by triggering Sync Plans in the Jamf Pro UI. " +
			"The catalog persists after unregistering, so this data source also works on an unregistered tenant (the rows may then be stale). An empty result is not an error." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"filter": filters.FilterAttribute(
				"RSQL selector (field name) passed through to the Jamf Pro API, e.g. `name` or `id`. The server validates the selector.",
				nil,
			),
			"sort": schema.ListAttribute{
				MarkdownDescription: "Sorting criteria passed through to the Jamf Pro API, each in `property:asc` / `property:desc` format, e.g. `[\"name:asc\"]`.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"plans": schema.ListNestedAttribute{
				MarkdownDescription: "Synced Jamf Protect plans matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                schema.StringAttribute{MarkdownDescription: "Plan ID assigned by Jamf Pro.", Computed: true},
						"uuid":              schema.StringAttribute{MarkdownDescription: "Plan UUID from Jamf Protect.", Computed: true},
						"name":              schema.StringAttribute{MarkdownDescription: "Plan name.", Computed: true},
						"description":       schema.StringAttribute{MarkdownDescription: "Plan description.", Computed: true},
						"profile_id":        schema.Int64Attribute{MarkdownDescription: "ID of the configuration profile created from the plan.", Computed: true},
						"profile_name":      schema.StringAttribute{MarkdownDescription: "Name of the configuration profile created from the plan.", Computed: true},
						"profile_version":   schema.Int64Attribute{MarkdownDescription: "Version of the configuration profile.", Computed: true},
						"scope_description": schema.StringAttribute{MarkdownDescription: "Human-readable description of the profile's scope.", Computed: true},
						"site_id":           schema.StringAttribute{MarkdownDescription: "ID of the site the profile belongs to. `-1` means none.", Computed: true},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *JamfProtectPlansDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_protect_plans")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the synced plans catalog (optionally filtered and sorted
// server-side) and populates state.
func (d *JamfProtectPlansDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data JamfProtectPlansDataSourceModel
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

	var sort []string
	if helpers.IsConfiguredValue(data.Sort) {
		resp.Diagnostics.Append(data.Sort.ElementsAs(ctx, &sort, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// The /v1/jamf-protect/plans endpoint documents no selector allow-list,
	// so the expression is passed through and the server validates selectors.
	filterExpression := filters.BuildRSQLExpression(data.Filters, nil)
	tflog.Debug(ctx, "Jamf Protect plans filter expression", map[string]any{"filter": filterExpression})

	plans, err := d.client.ListJamfProtectPlansV1(readCtx, sort, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Protect plans", helpers.APIErrorDetail(err))
		return
	}

	data.Plans = mapJamfProtectPlans(plans)
	data.ID = types.StringValue("jamf_protect_plans")

	tflog.Trace(ctx, "read Jamf Protect plans data source", map[string]any{
		"filter": filterExpression,
		"count":  len(data.Plans),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
