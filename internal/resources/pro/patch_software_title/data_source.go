// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// lookupByName resolves a patch software title by exact display name via
// ListPatchSoftwareTitleConfigurationsV3 + client-side name match. The
// configurations list has no name-based lookup endpoint, but it returns whole
// configuration objects rather than stubs, so the match itself is the answer
// and no follow-up get is needed. `name` is a freeform Required field the
// caller supplies at Create, so collisions across titles are possible —
// multiple exact matches surface as *jamfplatform.AmbiguousMatchError rather
// than silently returning the first hit.
//
// An entry carrying no id is not a match. Nothing downstream can read a title by
// an empty id, so counting one would only move the failure to the version read
// and have that blame the versions rather than the malformed entry. There is no
// guard on an empty list: the configurations list returns a plain slice, in
// which nil and empty are indistinguishable, and a tenant with no patch titles
// at all is legitimate.
func lookupByName(ctx context.Context, c *pro.Client, name string) (*pro.PatchSoftwareTitleConfiguration, error) {
	list, err := c.ListPatchSoftwareTitleConfigurationsV3(ctx)
	if err != nil {
		return nil, fmt.Errorf("list patch software titles while resolving %q: %w", name, err)
	}
	var matched *pro.PatchSoftwareTitleConfiguration
	var matches []string
	for i := range list {
		if list[i].DisplayName != name || list[i].ID == "" {
			continue
		}
		matched = &list[i]
		matches = append(matches, list[i].ID)
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no patch software title named %q", name)
	case 1:
		return matched, nil
	default:
		return nil, &jamfplatform.AmbiguousMatchError{Name: name, Matches: matches}
	}
}

// PatchSoftwareTitleDataSource implements the Terraform data source for Jamf Pro
// patch software titles. Lookup is by ID or by exact display name — exactly
// one of the two must be supplied. The full server view is surfaced,
// including every assigned version→package pair.
//
// source_id is resolved, not read: the v3 configuration names a title's patch
// source but never numbers it, and a data source has no prior state to carry the
// number in. The resolution is best-effort — source_id comes back null with a
// warning when the name matches no patch source, matches one in both
// catalogues, or the catalogues cannot be read — because the attribute is
// Computed and informational here, unlike the managed resource where it defines
// the title. It goes through the provider-instance catalogue cache so a
// configuration of many titles reads the catalogues once, not twice per title.
type PatchSoftwareTitleDataSource struct {
	sources   *providerdata.PatchSourceCache
	proClient *pro.Client
}

var (
	_ datasource.DataSource                     = &PatchSoftwareTitleDataSource{}
	_ datasource.DataSourceWithConfigure        = &PatchSoftwareTitleDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PatchSoftwareTitleDataSource{}
)

// NewPatchSoftwareTitleDataSource returns a new instance of PatchSoftwareTitleDataSource.
func NewPatchSoftwareTitleDataSource() datasource.DataSource {
	return &PatchSoftwareTitleDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PatchSoftwareTitleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_software_title"
}

// Schema returns the data source schema. id is the sole selector; the remaining
// attributes are populated from the SDK response.
func (d *PatchSoftwareTitleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro patch software title by ID or by exact display name. Exactly one of `id` or `name` must be supplied." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Patch software title ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the patch software title (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"name_id": schema.StringAttribute{
				MarkdownDescription: "Patch catalog key that defines the title.",
				Computed:            true,
			},
			"source_id": schema.Int64Attribute{
				MarkdownDescription: "Patch source ID this title is sourced from. Jamf Pro reports the source by name here, so the provider matches that name against the tenant's internal and external patch sources: the value is null, with a warning, when the name matches none of them, matches both an internal and an external source, or those sources cannot be read.",
				Computed:            true,
			},
			"category_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro category ID.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site ID.",
				Computed:            true,
			},
			"web_notification": schema.BoolAttribute{
				MarkdownDescription: "Whether a Jamf Pro notification is raised for new versions.",
				Computed:            true,
			},
			"email_notification": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro sends an email notification for new versions.",
				Computed:            true,
			},
			"version_packages": schema.MapAttribute{
				MarkdownDescription: "Every version→package assignment on the title (software_version → package ID).",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"available_versions": schema.ListAttribute{
				MarkdownDescription: "All software_version strings the patch source publishes for this title.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *PatchSoftwareTitleDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Pro client used for the v3 configuration reads, plus the
// shared patch source catalogue cache source_id resolves through.
func (d *PatchSoftwareTitleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_software_title")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.proClient = proClient
	d.sources = providerdata.ConfigurePatchSources(req.ProviderData, fetchPatchSourceCatalogues)
}

// Read fetches a patch software title by ID and populates Terraform state.
func (d *PatchSoftwareTitleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.proClient == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PatchSoftwareTitleDataSourceModel
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
		got *pro.PatchSoftwareTitleConfiguration
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.proClient.GetPatchSoftwareTitleConfigurationV3(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = lookupByName(readCtx, d.proClient, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing patch software title selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		if _, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err); ok {
			resp.Diagnostics.AddError(
				"Multiple Jamf Pro patch software titles match this display name",
				helpers.APIErrorDetail(err)+". Look the title up by id instead.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to find Jamf Pro patch software title", helpers.APIErrorDetail(err))
		return
	}
	defs, err := d.proClient.ListPatchSoftwareTitleDefinitionsV3(readCtx, got.ID, nil, "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro patch software title versions", helpers.APIErrorDetail(err))
		return
	}

	sourceID := types.Int64Null()
	catalogues, sourceErr := d.sources.Catalogues(readCtx)
	if sourceErr == nil {
		sourceID, sourceErr = sourceIDFromCatalogues(catalogues, got.PatchSourceName)
	}
	if sourceErr != nil {
		resp.Diagnostics.AddWarning(
			"Unable to determine the patch software title's source_id",
			unresolvedSourceIDWarningDetail(got.DisplayName, got.PatchSourceName, sourceErr),
		)
	}

	resp.Diagnostics.Append(assignPatchSoftwareTitleDataSourceModel(readCtx, &data, got, definitionVersions(defs), sourceID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro patch software title data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
