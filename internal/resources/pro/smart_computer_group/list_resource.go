// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

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

// defaultListTimeout caps the search the list resource pages.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout caps each per-item read issued to hydrate a result,
// kept separate from the list budget so one slow group cannot stall the whole
// query.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &SmartComputerGroupListResource{}
	_ list.ListResourceWithConfigure = &SmartComputerGroupListResource{}
)

// NewSmartComputerGroupListResource returns a list resource for smart computer
// group queries.
func NewSmartComputerGroupListResource() list.ListResource {
	return &SmartComputerGroupListResource{}
}

// SmartComputerGroupListResource implements Terraform query list support for
// Jamf Pro smart computer groups.
//
// A search omits each group's criteria, so on include_resource every result is
// re-read by identifier to fill them in. Without that a generated configuration
// would carry a smart group with no criteria, which is not the group that exists.
// Hydration mirrors a first-time import: there is no plan to stay consistent
// with, so every attribute comes from Jamf Pro.
//
// A per-item read that fails leaves that result unhydrated and attaches the
// reason to the result itself. It deliberately does not replace the stream: a
// diagnostics-only stream assigned part-way through a loop discards every result
// already gathered, so a single unreadable group would empty the whole query.
type SmartComputerGroupListResource struct {
	client *pro.Client
	pd     *providerdata.Data
}

// Metadata sets the list resource type name, which has to match the managed
// resource this lists.
func (r *SmartComputerGroupListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_computer_group"
}

// Configure wires the Jamf Pro client into the list resource, then applies the
// family's version refusal.
func (r *SmartComputerGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, terraformName)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, terraformName)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.pd = pd
}

// ListResourceConfigSchema describes the supported filter clauses.
func (r *SmartComputerGroupListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Queries the smart computer groups in Jamf Pro, taking the same filter clauses as the plural data source. Set `include_resource` to have each result carry the group's full configuration, which costs an extra lookup per group." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(SmartComputerGroupFilterSelectors),
				SmartComputerGroupFilterSelectors,
			),
		},
	}
}

// List runs the query and streams group identities back to Terraform.
//
// The platform identifiers for the whole page come from one bridging request,
// and any advisory it raises rides on the first result. There is nowhere else to
// put it: assigning the stream a diagnostics-only value is the only stream-level
// channel and it throws away every result gathered so far. The bridge runs only
// under include_resource, because platform_id is carried by a result's resource
// state and nothing else: a query that asks for identities alone would pay for a
// whole-tenant sweep it discards, and an advisory naming platform_id on a result
// set that has no platform_id sends an operator after a privilege that changes
// nothing.
func (r *SmartComputerGroupListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config SmartComputerGroupListResourceModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(SmartComputerGroupFilterSelectors))
	tflog.Debug(ctx, "smart computer group list filters", map[string]any{"filter": filterExpression})

	groups, err := r.client.ListSmartComputerGroupsV3(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to search Jamf Pro smart computer groups", helpers.APIErrorDetail(err)),
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
		byJamfProID, bridgeDiags = progroups.PlatformIDsByJamfProID(listCtx, r.client, r.pd, deviceType, groupKind)
	}

	results := make([]list.ListResult, 0, maxResults)
	for _, group := range groups {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = group.Name
		if len(results) == 0 && req.IncludeResource {
			result.Diagnostics.Append(bridgeDiags...)
		}

		id := types.StringValue(group.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, smartComputerGroupIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			results = append(results, result)
			continue
		}

		if req.IncludeResource {
			r.hydrate(ctx, &result, group, byJamfProID)
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "listed Jamf Pro smart computer groups", map[string]any{
		"filter":   filterExpression,
		"limit":    req.Limit,
		"returned": len(results),
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

// hydrate fills one result's resource state by re-reading the group, which a
// search cannot supply because it carries no criteria. A failure attaches a
// warning to that result and leaves its state unset, so the rest of the query
// still returns.
func (r *SmartComputerGroupListResource) hydrate(ctx context.Context, result *list.ListResult, group pro.SmartComputerGroupSearch, byJamfProID map[string]string) {
	itemCtx, cancelItem := context.WithTimeout(ctx, defaultItemReadTimeout)
	defer cancelItem()

	got, err := r.client.GetSmartComputerGroupV3(itemCtx, group.ID)
	if err != nil {
		result.Diagnostics.AddWarning(
			"Could not read the full configuration of "+groupLabel+" "+group.Name,
			"The group is listed but reading it for its criteria failed, so this result carries no configuration."+
				"\n\nJamf Pro's response: "+helpers.APIErrorDetail(err),
		)
		return
	}

	state := SmartComputerGroupResourceModel{
		ID:         types.StringValue(group.ID),
		PlatformID: progroups.PlatformIDValue(byJamfProID, group.ID),
		Timeouts:   helpers.NewResourceTimeoutsNullValue(smartComputerGroupTimeoutAttributeTypes),
	}
	assignSmartComputerGroupResourceModel(&state, got)
	result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
}
