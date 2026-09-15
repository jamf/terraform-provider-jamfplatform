// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package patch_internal_source implements the read-only
// jamfplatform_pro_patch_internal_source data source backed by the Jamf
// ProClassic patch internal sources API. Internal sources are Jamf-managed (the
// built-in "Jamf" definition source) and not user-creatable, so this package
// ships a data source only — no resource or list resource.
package patch_internal_source

import (
	"context"
	"time"

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

// defaultReadTimeout is generous: the available-titles catalog for the built-in
// Jamf source runs to ~1500 entries, so the read fetches a large payload.
const defaultReadTimeout = 90 * time.Second

// minJamfProVersion is the minimum Jamf Pro tenant version required by this data
// source. Empty: the classic /patchinternalsources endpoint predates the
// provider's overall floor (11.0.0). The provider-level advisory still fires
// through providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

// PatchInternalSourceDataSource implements the Terraform data source for Jamf Pro
// patch internal sources. Lookup is by ID OR by name — exactly one must be supplied.
type PatchInternalSourceDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &PatchInternalSourceDataSource{}
	_ datasource.DataSourceWithConfigure        = &PatchInternalSourceDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PatchInternalSourceDataSource{}
)

// NewPatchInternalSourceDataSource returns a new instance of PatchInternalSourceDataSource.
func NewPatchInternalSourceDataSource() datasource.DataSource {
	return &PatchInternalSourceDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PatchInternalSourceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_internal_source"
}

// Schema returns the data source schema. id and name are the mutually-exclusive
// selectors (Optional+Computed); the remaining attributes are populated from the
// SDK response.
func (d *PatchInternalSourceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro patch internal source by ID or by exact name, and read the catalog of software titles it publishes. Internal sources are managed by Jamf (the built-in \"Jamf\" definition source) and cannot be created or modified. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Patch internal source ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Patch internal source display name (exact match), e.g. `Jamf`. Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the patch internal source is enabled.",
				Computed:            true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "URL of the definition server backing this source.",
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
func (d *PatchInternalSourceDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *PatchInternalSourceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_internal_source")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a patch internal source by ID or by name, then its published
// available-titles catalog, and populates Terraform state.
func (d *PatchInternalSourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PatchInternalSourceDataSourceModel
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
		got *proclassic.PatchInternalSource
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetPatchInternalSourceByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetPatchInternalSourceByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing patch internal source selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro patch internal source", helpers.APIErrorDetail(err))
		return
	}
	assignPatchInternalSourceDataSourceModel(&data, got)

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

	tflog.Trace(ctx, "read Jamf Pro patch internal source data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString(), "available_titles": len(data.AvailableTitles)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
