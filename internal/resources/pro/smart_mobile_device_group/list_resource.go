// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"
	"fmt"
	"strings"
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

// defaultListTimeout bounds the whole enumeration, which pages the collection
// and then issues one bridge request for the page's platform identifiers. The
// list schema exposes no user-settable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item read issued to hydrate a result,
// kept off the list budget so one slow group cannot exhaust a deadline shared
// across every other.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &SmartMobileDeviceGroupListResource{}
	_ list.ListResourceWithConfigure = &SmartMobileDeviceGroupListResource{}
)

// NewSmartMobileDeviceGroupListResource returns a list resource for smart mobile
// device group queries.
func NewSmartMobileDeviceGroupListResource() list.ListResource {
	return &SmartMobileDeviceGroupListResource{}
}

// SmartMobileDeviceGroupListResource implements Terraform query list support.
//
// The collection read reports a summary of each group and no criteria, so a
// result hydrated from it alone would export a group with an empty criteria
// list. Each group is therefore read individually when Terraform asks for the
// resource body, and the platform identifiers for the whole page come from one
// bridge request rather than one per group.
//
// A group the collection enumerated but the per-item read cannot return is
// dropped, because a result carrying no body generates no configuration and the
// framework reports it as a provider fault besides. The drop is reported: one
// trailing diagnostics-only result names every group left out, so a generated
// configuration is never quietly short of the collection. That same result
// carries any advisory the platform-identifier bridge raised, which has nowhere
// else to go: a list stream has no diagnostics channel of its own, and assigning
// one replaces every result already collected.
type SmartMobileDeviceGroupListResource struct {
	client *pro.Client
	pd     *providerdata.Data
}

// Metadata sets the list resource type name. A list resource shares the type
// name of the managed resource it lists.
func (r *SmartMobileDeviceGroupListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_mobile_device_group"
}

// Configure wires the Jamf Pro client into the list resource and applies the
// family's version refusal.
func (r *SmartMobileDeviceGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smart_mobile_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_smart_mobile_device_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.pd = pd
}

// ListResourceConfigSchema describes the supported list filters.
func (r *SmartMobileDeviceGroupListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro smart mobile device groups, using the same filter clauses as the plural data source." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(SmartMobileDeviceGroupFilterSelectors),
				SmartMobileDeviceGroupFilterSelectors,
			),
		},
	}
}

// List executes the query and streams group identities back to Terraform.
func (r *SmartMobileDeviceGroupListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config SmartMobileDeviceGroupListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(SmartMobileDeviceGroupFilterSelectors))
	tflog.Debug(ctx, "smart mobile device group list filters", map[string]any{"filter": filterExpression})

	groups, err := r.client.ListSmartMobileDeviceGroupsV2(listCtx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro smart mobile device groups", helpers.APIErrorDetail(err)),
		})
		return
	}

	var (
		byPlatformID map[string]string
		trailing     diag.Diagnostics
	)
	if req.IncludeResource {
		resolved, bridgeDiags := progroups.PlatformIDsByJamfProID(listCtx, r.client, r.pd, progroups.DeviceTypeMobile, progroups.GroupKindSmart)
		byPlatformID = resolved
		trailing.Append(bridgeDiags...)
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(groups)) {
		maxResults = int64(len(groups))
	}

	results := make([]list.ListResult, 0, maxResults)
	var skipped []skippedGroup

	for _, g := range groups {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = g.GroupName

		id := types.StringValue(g.GroupID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, smartMobileDeviceGroupIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancelItem := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, getErr := r.client.GetSmartMobileDeviceGroupV2(itemCtx, id.ValueString())
			cancelItem()
			if getErr != nil {
				tflog.Warn(ctx, "Skipping Jamf Pro smart mobile device group from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": getErr.Error(),
				})
				skipped = append(skipped, skippedGroup{id: id.ValueString(), name: g.GroupName, err: getErr})
				continue
			}

			state := SmartMobileDeviceGroupResourceModel{
				ID:         id,
				PlatformID: progroups.PlatformIDValue(byPlatformID, id.ValueString()),
				Timeouts:   helpers.NewResourceTimeoutsNullValue(smartMobileDeviceGroupTimeoutAttributeTypes),
			}
			assignSmartMobileDeviceGroupResourceModel(&state, got)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro smart mobile device groups", map[string]any{
		"filter":   filterExpression,
		"limit":    req.Limit,
		"returned": len(results),
		"skipped":  len(skipped),
	})

	if len(skipped) > 0 {
		trailing.AddWarning(
			"Some Jamf Pro smart mobile device groups were left out of the results",
			skippedGroupsWarningDetail(skipped),
		)
	}

	if len(results) == 0 && len(trailing) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
		if len(trailing) > 0 {
			push(list.ListResult{Diagnostics: trailing})
		}
	}
}

// skippedGroup records a group the collection enumerated but the per-item read
// could not return, so the omission is reported once at the end of the stream
// rather than per item.
type skippedGroup struct {
	id   string
	name string
	err  error
}

// skippedGroupsWarningDetail is the operator-facing detail for the trailing
// warning. It names every group left out with the error that left it out, and
// points at the two causes that report the same way: a permission the
// integration is missing, and a group deleted between the collection read and
// the individual one.
func skippedGroupsWarningDetail(skipped []skippedGroup) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d group(s) appeared in the Jamf Pro collection but could not be read individually, so they carry no configuration and are not in the results:\n", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(&b, "  - %s (id %s): %s\n", s.name, s.id, s.err)
	}
	b.WriteString("\nCheck that the API integration holds the device groups read permission, and that each group still exists. A group deleted between the two reads reports the same way.")
	return b.String()
}
