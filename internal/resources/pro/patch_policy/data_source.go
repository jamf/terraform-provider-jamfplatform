// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PatchPolicyDataSource implements the Terraform data source for Jamf Pro patch
// policies. Lookup is by id only, and the reason is no longer the one it used to
// be: the classic /patchpolicies collection reads that made a name selector an
// N-GET scan are no longer what this construct enumerates on, and the Pro v2
// collection that replaced them accepts a server-side RSQL query on policyName,
// so a name selector would now cost one filtered list call plus one by-id read. It is
// therefore unbuilt rather than rejected — note that RSQL string comparison is
// case-sensitive, so a name selector added here would match exactly, unlike the
// list resource's deliberately case-insensitive substring filter. The data
// source surfaces the general fields, including the server-derived ones; manage
// the policy as a resource for full scope / user_interaction detail.
type PatchPolicyDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource              = &PatchPolicyDataSource{}
	_ datasource.DataSourceWithConfigure = &PatchPolicyDataSource{}
)

// NewPatchPolicyDataSource returns a new instance of PatchPolicyDataSource.
func NewPatchPolicyDataSource() datasource.DataSource {
	return &PatchPolicyDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PatchPolicyDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_policy"
}

// Schema returns the data source schema. id is the sole selector; the remaining
// attributes are populated from the SDK response.
func (d *PatchPolicyDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro patch policy by ID. Surfaces the general settings, including the read-only `release_date`, `incremental_update`, `reboot`, `minimum_os` and `kill_apps` fields returned by Jamf Pro. Scope and user interaction are not surfaced; manage the policy as a resource for that detail." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Patch policy ID.",
				Required:            true,
			},
			"software_title_configuration_id": schema.StringAttribute{
				MarkdownDescription: "ID of the patch software title configuration this policy deploys.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the patch policy.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the patch policy is enabled.",
				Computed:            true,
			},
			"target_version": schema.StringAttribute{
				MarkdownDescription: "The software version this policy deploys.",
				Computed:            true,
			},
			"distribution_method": schema.StringAttribute{
				MarkdownDescription: "How the patch is delivered (`selfservice` or `prompt`).",
				Computed:            true,
			},
			"allow_downgrade": schema.BoolAttribute{
				MarkdownDescription: "Whether installing the target version is allowed when a newer version is present.",
				Computed:            true,
			},
			"patch_unknown": schema.BoolAttribute{
				MarkdownDescription: "Whether computers with an unknown installed version are patched.",
				Computed:            true,
			},
			"release_date": schema.Int64Attribute{
				MarkdownDescription: "Release date of the target version's patch definition (Unix epoch in milliseconds).",
				Computed:            true,
			},
			"incremental_update": schema.BoolAttribute{
				MarkdownDescription: "Whether the target version's patch definition is an incremental update.",
				Computed:            true,
			},
			"reboot": schema.BoolAttribute{
				MarkdownDescription: "Whether installing the target version requires a reboot.",
				Computed:            true,
			},
			"minimum_os": schema.StringAttribute{
				MarkdownDescription: "Minimum macOS version required by the target version's patch definition.",
				Computed:            true,
			},
			"kill_apps": schema.ListNestedAttribute{
				MarkdownDescription: "Applications the patch definition closes before installing the target version.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"kill_app_name": schema.StringAttribute{
							MarkdownDescription: "Display name of the application closed before patching.",
							Computed:            true,
						},
						"kill_app_bundle_id": schema.StringAttribute{
							MarkdownDescription: "Bundle identifier of the application closed before patching.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *PatchPolicyDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_policy")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a patch policy by ID and populates Terraform state.
func (d *PatchPolicyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PatchPolicyDataSourceModel
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

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing patch policy selector", "id must be supplied.")
		return
	}

	got, err := d.client.GetPatchPolicyByID(readCtx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro patch policy", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignPatchPolicyDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro patch policy data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// assignPatchPolicyDataSourceModel populates the flat data source model from the
// SDK response. The data source adopts all general fields verbatim (no
// sticky-read gating — there is no plan to reconcile against).
func assignPatchPolicyDataSourceModel(ctx context.Context, data *PatchPolicyDataSourceModel, p *proclassic.PatchPolicy) diag.Diagnostics {
	var diags diag.Diagnostics
	if p == nil {
		return diags
	}
	if id := extractPatchPolicyID(p); id != "" {
		data.ID = types.StringValue(id)
	}
	data.SoftwareTitleConfigurationID = helpers.StringValueFromIntPtr(p.SoftwareTitleConfigurationID)

	if g := p.General; g != nil {
		data.Name = helpers.StringPointerValueOrNull(g.Name)
		data.Enabled = helpers.BoolPointerValueOrNull(g.Enabled)
		data.TargetVersion = helpers.StringPointerValueOrNull(g.TargetVersion)
		data.DistributionMethod = helpers.StringPointerValueOrNull(g.DistributionMethod)
		data.AllowDowngrade = helpers.BoolPointerValueOrNull(g.AllowDowngrade)
		data.PatchUnknown = helpers.BoolPointerValueOrNull(g.PatchUnknown)
		data.ReleaseDate = int64ValueOrNull(g.ReleaseDate)
		data.IncrementalUpdate = helpers.BoolPointerValueOrNull(g.IncrementalUpdate)
		data.Reboot = helpers.BoolPointerValueOrNull(g.Reboot)
		data.MinimumOS = helpers.StringPointerValueOrNull(g.MinimumOs)
		killApps, d := flattenKillApps(ctx, g.KillApps)
		diags.Append(d...)
		data.KillApps = killApps
	} else {
		data.KillApps = types.ListNull(types.ObjectType{AttrTypes: killAppAttrTypes})
	}

	return diags
}
