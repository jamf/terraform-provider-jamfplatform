// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait on the v3
// configurations list plus the two patch-source catalogue reads. The list
// resource schema does not expose a user-overridable timeout, so this is a
// fixed safety bound.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &PatchSoftwareTitleListResource{}
var _ list.ListResourceWithConfigure = &PatchSoftwareTitleListResource{}

// NewPatchSoftwareTitleListResource returns a list resource for Jamf Pro patch
// software title queries.
func NewPatchSoftwareTitleListResource() list.ListResource {
	return &PatchSoftwareTitleListResource{}
}

// PatchSoftwareTitleListResource implements Terraform query list support. The v3
// configurations list accepts no query parameters, so the optional `filter`
// block is applied client-side via filters.ApplyClassicFilter after the full
// list is fetched.
//
// source_id is resolved, not read: the v3 configuration names a title's patch
// source but never numbers it. The resolution goes through the shared
// provider-instance catalogue cache, so a list pays the two catalogue reads once
// however many titles it returns, and it is best-effort — see List.
type PatchSoftwareTitleListResource struct {
	sources   *providerdata.PatchSourceCache
	proClient *pro.Client
}

// Metadata sets the list resource type name.
func (r *PatchSoftwareTitleListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_software_title"
}

// Configure wires the Pro client used for the v3 configurations list, plus the
// shared patch source catalogue cache source_id resolves through.
func (r *PatchSoftwareTitleListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_software_title")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.proClient = proClient
	r.sources = providerdata.ConfigurePatchSources(req.ProviderData, fetchPatchSourceCatalogues)
}

// ListResourceConfigSchema describes the supported list filters.
func (r *PatchSoftwareTitleListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro patch software titles. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched, matching each title's display name." +
			listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams patch software title identities back to
// Terraform.
//
// The v3 configurations list returns whole configuration objects rather than
// stubs, so include_resource hydrates everything that payload carries. Two
// attributes stay null because filling them would cost a call per item:
// available_versions, which lives on the /definitions sub-resource, and
// extension_attributes, whose display names come from the /extension-attributes
// sub-resource (the configuration body carries only ids and accept flags). A
// list preview does not require full hydration — consumers needing those should
// use the singular data source or the managed resource.
//
// source_id is not on the payload either. The provider tries to resolve it from
// the patch source name, out of two small tenant-wide catalogues read once for
// the whole list, but the attempt can fail without the list failing: a name
// present in both catalogues cannot be resolved at all, a source renamed or
// removed since the title was created matches neither, and the catalogues need
// privileges of their own. Every one of those leaves that title's source_id null
// and attaches a warning naming the title — a preview reports what it could not
// determine rather than dropping it, and rather than failing the whole listing
// over an informational attribute.
//
// version_packages goes through assignedVersionPackagesValue rather than a
// direct fold, because a title with no assignments must land as an unset
// attribute: an empty map is not a legal configuration value, so emitting one
// would hand `terraform plan -generate-config-out` configuration this provider
// then refuses.
func (r *PatchSoftwareTitleListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.proClient == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config PatchSoftwareTitleListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.proClient.ListPatchSoftwareTitleConfigurationsV3(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro patch software titles", helpers.APIErrorDetail(err)),
		})
		return
	}

	var (
		catalogues   providerdata.PatchSourceCatalogues
		catalogueErr error
	)
	if req.IncludeResource {
		catalogues, catalogueErr = r.sources.Catalogues(listCtx)
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, patchSoftwareTitleDisplayName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)
	catalogueWarned := false

	for _, s := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = s.DisplayName

		id := types.StringValue(s.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, patchSoftwareTitleIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			assignments, assignDiags := assignedVersionPackagesValue(ctx, s.Packages)
			if assignDiags.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(assignDiags)
				return
			}

			sourceID := types.Int64Null()
			if catalogueErr != nil {
				if !catalogueWarned {
					catalogueWarned = true
					result.Diagnostics.AddWarning(
						"Unable to read this tenant's patch sources",
						unreadableCataloguesWarningDetail(catalogueErr),
					)
				}
			} else if resolved, resolveErr := sourceIDFromCatalogues(catalogues, s.PatchSourceName); resolveErr != nil {
				result.Diagnostics.AddWarning(
					"Unable to determine source_id for a patch software title",
					unresolvedSourceIDWarningDetail(s.DisplayName, s.PatchSourceName, resolveErr),
				)
			} else {
				sourceID = resolved
			}

			// available_versions and extension_attributes stay null: both cost a
			// call per item (see method doc).
			state := PatchSoftwareTitleResourceModel{
				ID:                        id,
				Name:                      types.StringValue(s.DisplayName),
				NameID:                    types.StringValue(s.SoftwareTitleNameID),
				SourceID:                  sourceID,
				CategoryID:                refIDValue(s.CategoryID),
				SiteID:                    refIDValue(s.SiteID),
				WebNotification:           types.BoolValue(s.UiNotifications),
				EmailNotification:         types.BoolValue(s.EmailNotifications),
				VersionPackages:           assignments,
				AvailableVersions:         types.ListNull(types.StringType),
				AcceptExtensionAttributes: types.BoolNull(),
				// Typed null (not the zero-value types.List, which is an
				// untyped/DynamicPseudoType list and fails the schema type check).
				ExtensionAttributes: types.ListNull(eaElementType),
				Timeouts:            helpers.NewResourceTimeoutsNullValue(patchSoftwareTitleTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro patch software titles", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
	})

	if len(results) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}

// patchSoftwareTitleDisplayName is the name accessor passed to
// filters.ApplyClassicFilter, matching each title's display name.
func patchSoftwareTitleDisplayName(s pro.PatchSoftwareTitleConfiguration) string {
	return s.DisplayName
}
