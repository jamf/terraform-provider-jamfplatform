// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps the search behind a list query.
const defaultListTimeout = 90 * time.Second

var (
	_ list.ListResource              = &StaticComputerGroupListResource{}
	_ list.ListResourceWithConfigure = &StaticComputerGroupListResource{}
)

// StaticComputerGroupListResource implements query list support.
//
// Hydration for `include_resource` comes from the search result alone, plus one
// request for the whole page's platform identifiers. It deliberately leaves
// `assigned_computer_ids` unset: reading membership costs one request per group
// against a second API surface, and a listing of every static group on a large
// tenant would pay that for each one. Leaving it out is also the right generated
// configuration, because omitting the attribute is how an operator says "leave
// the membership to Jamf Pro".
//
// A group whose identity or resource state cannot be written keeps its place in
// the stream with the reason attached to the result itself. It deliberately does
// not replace the stream: a diagnostics-only stream assigned part-way through a
// loop discards every result already gathered, so a single unwritable group
// would empty the whole query.
type StaticComputerGroupListResource struct {
	client *pro.Client
	pd     *providerdata.Data
}

// NewStaticComputerGroupListResource returns a list resource for static computer
// group queries.
func NewStaticComputerGroupListResource() list.ListResource {
	return &StaticComputerGroupListResource{}
}

// Metadata sets the list resource type name, which matches the managed resource
// type it lists.
func (r *StaticComputerGroupListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_computer_group"
}

// Configure wires the Jamf Pro client and the shared provider data onto the list
// resource.
func (r *StaticComputerGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_computer_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_computer_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.pd = pd
}

// ListResourceConfigSchema describes the supported filters.
func (r *StaticComputerGroupListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Finds Jamf Pro static computer groups using the same filter clauses as the `jamfplatform_pro_static_computer_groups` data source. Generated configurations leave `assigned_computer_ids` out, so membership stays as Jamf Pro holds it until you declare otherwise." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(StaticComputerGroupFilterSelectors),
				StaticComputerGroupFilterSelectors,
			),
		},
	}
}

// List runs the query and streams group identities back to Terraform.
func (r *StaticComputerGroupListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command once `terraform init` completes successfully.",
			),
		})
		return
	}

	var config StaticComputerGroupListResourceModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(StaticComputerGroupFilterSelectors))
	tflog.Debug(ctx, "static computer group list filters", map[string]any{"filter": filterExpression})

	groups, err := r.client.ListStaticComputerGroupsV3(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro static computer groups", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(groups)) {
		maxResults = int64(len(groups))
	}

	var (
		byJamfProID map[string]string
		bridgeDiags diag.Diagnostics
	)
	if req.IncludeResource {
		byJamfProID, bridgeDiags = progroups.PlatformIDsByJamfProID(listCtx, r.client, r.pd, progroups.DeviceTypeComputer, progroups.GroupKindStatic)
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, group := range groups {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = group.Name

		id := types.StringValue(group.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, staticComputerGroupIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			results = append(results, result)
			continue
		}

		if req.IncludeResource {
			state := StaticComputerGroupResourceModel{
				ID:                  id,
				PlatformID:          progroups.PlatformIDValue(byJamfProID, group.ID),
				Name:                types.StringValue(group.Name),
				Description:         helpers.StringPointerValueOrNull(group.Description),
				SiteID:              progroups.SiteIDForState(group.SiteID),
				AssignedComputerIDs: types.SetNull(types.StringType),
				Timeouts:            helpers.NewResourceTimeoutsNullValue(staticComputerGroupTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				results = append(results, result)
				continue
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro static computer groups", map[string]any{
		"filter":   filterExpression,
		"limit":    req.Limit,
		"returned": len(results),
	})

	if len(results) == 0 && len(bridgeDiags) == 0 {
		stream.Results = list.NoListResults
		return
	}

	// The bridge's advisories travel as one trailing diagnostics-only result: a
	// failed platform-identifier lookup nulls platform_id across the whole page
	// and has to reach the operator, and attaching it to the stream would
	// replace every result already accumulated. Built as a literal, never from
	// req.NewListResult, which allocates an identity the pass-through branch
	// then rejects.
	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if len(bridgeDiags) > 0 {
			push(list.ListResult{Diagnostics: bridgeDiags})
		}
	}
}
