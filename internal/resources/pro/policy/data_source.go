// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PolicyDataSource implements the Terraform data source for Jamf Pro classic
// policies. Lookup is by ID or by exact name — exactly one must be supplied.
// The data source surfaces a flat Computed projection of the most-frequently
// looked-up fields (general identity, category, site, basic triggers). For
// full policy detail, manage the policy as a resource or import it.
type PolicyDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &PolicyDataSource{}
	_ datasource.DataSourceWithConfigure        = &PolicyDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PolicyDataSource{}
)

// PolicyDataSourceFlatModel is the flat data source model. Distinct from
// PolicyDataSourceModel (which mirrors the nested resource shape) — we
// surface a small read-only projection so users can resolve IDs by name.
type PolicyDataSourceFlatModel struct {
	ID           types.String   `tfsdk:"id"`
	Name         types.String   `tfsdk:"name"`
	Enabled      types.Bool     `tfsdk:"enabled"`
	Frequency    types.String   `tfsdk:"frequency"`
	Trigger      types.String   `tfsdk:"trigger"`
	CategoryID   types.String   `tfsdk:"category_id"`
	CategoryName types.String   `tfsdk:"category_name"`
	SiteID       types.String   `tfsdk:"site_id"`
	SiteName     types.String   `tfsdk:"site_name"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

// NewPolicyDataSource returns a new instance of PolicyDataSource.
func NewPolicyDataSource() datasource.DataSource {
	return &PolicyDataSource{}
}

// Metadata sets the data source type name.
func (d *PolicyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_policy"
}

// Schema returns the data source schema.
func (d *PolicyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro policy by ID or by exact name. Exactly one of `id` or `name` must be supplied. Surfaces a flat read-only projection of the most-frequently looked-up fields; manage the policy as a resource for full detail." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Policy ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Policy display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"enabled":       schema.BoolAttribute{MarkdownDescription: "Whether the policy is enabled.", Computed: true},
			"frequency":     schema.StringAttribute{MarkdownDescription: "Policy frequency.", Computed: true},
			"trigger":       schema.StringAttribute{MarkdownDescription: "Aggregate trigger label.", Computed: true},
			"category_id":   schema.StringAttribute{MarkdownDescription: "Category ID.", Computed: true},
			"category_name": schema.StringAttribute{MarkdownDescription: "Category display name.", Computed: true},
			"site_id":       schema.StringAttribute{MarkdownDescription: "Site ID. `-1` means no site.", Computed: true},
			"site_name":     schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
			"timeouts":      timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *PolicyDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *PolicyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_policy")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a policy by ID or by name.
func (d *PolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PolicyDataSourceFlatModel
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

	var (
		got *proclassic.Policy
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetPolicyByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetPolicyByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing policy selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro policy", helpers.APIErrorDetail(err))
		return
	}
	assignPolicyFlatDataSource(&data, got)

	tflog.Trace(ctx, "read Jamf Pro policy data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignPolicyFlatDataSource projects a *proclassic.Policy into the flat
// data source model.
func assignPolicyFlatDataSource(state *PolicyDataSourceFlatModel, p *proclassic.Policy) {
	if p == nil {
		return
	}
	if p.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(p.ID)
	}
	if p.General != nil {
		if p.General.Name != nil {
			state.Name = helpers.StringPointerValueOrNull(p.General.Name)
		}
		state.Enabled = helpers.BoolPointerValueOrNull(p.General.Enabled)
		state.Frequency = helpers.StringPointerValueOrNull(p.General.Frequency)
		state.Trigger = helpers.StringPointerValueOrNull(p.General.Trigger)
		if p.General.Category != nil {
			state.CategoryID = helpers.StringValueFromIntPtr(p.General.Category.ID)
			state.CategoryName = helpers.DerivedRefName(p.General.Category.ID, p.General.Category.Name)
		}
		if p.General.Site != nil {
			state.SiteID = helpers.StringValueFromIntPtr(p.General.Site.ID)
			state.SiteName = helpers.DerivedRefName(p.General.Site.ID, p.General.Site.Name)
		}
	}
}
