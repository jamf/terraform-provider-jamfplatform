// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_external_source

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/availabletitles"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PatchExternalSourceDataSource implements the Terraform data source for Jamf Pro
// patch external sources. The singular data source supports lookup by ID OR by
// name — exactly one of the two must be supplied.
type PatchExternalSourceDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &PatchExternalSourceDataSource{}
	_ datasource.DataSourceWithConfigure        = &PatchExternalSourceDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PatchExternalSourceDataSource{}
)

// NewPatchExternalSourceDataSource returns a new instance of PatchExternalSourceDataSource.
func NewPatchExternalSourceDataSource() datasource.DataSource {
	return &PatchExternalSourceDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PatchExternalSourceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_external_source"
}

// Schema returns the data source schema. id and name are the mutually-exclusive
// selectors (Optional+Computed); the remaining attributes are populated from the
// SDK response.
func (d *PatchExternalSourceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro patch external source by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Patch external source ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Patch external source display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the patch external source is enabled.",
				Computed:            true,
			},
			"host_name": schema.StringAttribute{
				MarkdownDescription: "Server host name of the external patch source.",
				Computed:            true,
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "TCP port of the external patch source. Null when unset.",
				Computed:            true,
			},
			"ssl_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the source is contacted over SSL.",
				Computed:            true,
			},
			"certificate_validation_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether software title definitions must be signed by a publicly trusted certificate before being downloaded from the source; unsigned definitions are not downloaded.",
				Computed:            true,
			},
			"available_titles": schema.ListNestedAttribute{
				MarkdownDescription: "Software titles this source publishes, used to discover the `name_id` for `jamfplatform_pro_patch_software_title`. The full catalog is fetched on every read.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: availabletitles.DataSourceAttributes(),
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *PatchExternalSourceDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *PatchExternalSourceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_external_source")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a patch external source by ID or by name and populates Terraform state.
func (d *PatchExternalSourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PatchExternalSourceDataSourceModel
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
		got *proclassic.PatchExternalSource
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetPatchExternalSourceByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetPatchExternalSourceByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing patch external source selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro patch external source", helpers.APIErrorDetail(err))
		return
	}
	assignPatchExternalSourceDataSourceModel(&data, got)

	// Available titles are a second, source-id-keyed fetch. A failure here is
	// non-fatal (Warning + empty list): the source metadata resolved fine and a
	// consumer may only need id/enabled, so a flaky catalog must not break the plan.
	data.AvailableTitles = []availabletitles.Model{}
	if !data.ID.IsNull() && data.ID.ValueString() != "" {
		titles, titlesErr := d.client.ListPatchAvailableTitlesBySourceID(readCtx, data.ID.ValueString())
		if titlesErr != nil {
			resp.Diagnostics.AddWarning("Unable to list available patch titles", titlesErr.Error())
		} else {
			data.AvailableTitles = availabletitles.MapTitles(titles)
		}
	}

	tflog.Trace(ctx, "read Jamf Pro patch external source data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString(), "available_titles": len(data.AvailableTitles)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
