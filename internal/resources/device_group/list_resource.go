// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/device_groups"
)

var _ list.ListResource = &DeviceGroupListResource{}
var _ list.ListResourceWithConfigure = &DeviceGroupListResource{}

// NewDeviceGroupListResource returns a list resource for device group queries.
func NewDeviceGroupListResource() list.ListResource {
	return &DeviceGroupListResource{}
}

// DeviceGroupListResource implements Terraform query list support for device groups.
type DeviceGroupListResource struct {
	client    *devicegroups.Client
	proClient *pro.Client
	pd        *providerdata.Data
	groupRef  criteria.GroupResolver
}

// Metadata sets the list resource type name.
func (r *DeviceGroupListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_group"
}

// Configure wires the provider client into the list resource.
func (r *DeviceGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected List Configure Type",
			"Expected *providerdata.Data. Please report this issue to the provider developers.",
		)
		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_device_group", providerdata.DeviceGroupsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client = devicegroups.New(pd.Client)
	r.proClient = pro.New(pd.Client)
	r.groupRef = criteria.NewProGroupResolver(proclassic.New(pd.Client))
	r.pd = pd
}

// ListResourceConfigSchema describes the supported list filters.
func (r *DeviceGroupListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf device groups using the same filter clauses as the device_groups data source." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ListFilterAttribute(
				filters.SelectorDescription(device_groups.DeviceGroupFilterSelectors),
				device_groups.DeviceGroupFilterSelectors,
			),
		},
	}
}

// List executes the query and streams device group identities back to Terraform.
func (r *DeviceGroupListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config DeviceGroupListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	filterExpression := filters.BuildRSQLExpression(config.Filters, filters.AllowList(device_groups.DeviceGroupFilterSelectors))

	tflog.Debug(ctx, "device group list filters", map[string]any{
		"filter": filterExpression,
	})

	groups, err := r.client.ListDeviceGroups(ctx, nil, filterExpression)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unable to list device groups",
				helpers.APIErrorDetail(err),
			),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(groups)) {
		maxResults = int64(len(groups))
	}

	results := make([]list.ListResult, 0, int(maxResults))
	var emitted int64

	for _, grp := range groups {
		if maxResults > 0 && emitted >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = grp.Name

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, deviceGroupIdentityModel{ID: types.StringValue(grp.ID)})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			detail, err := r.client.GetDeviceGroup(ctx, grp.ID)
			if err != nil {
				stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
					diag.NewErrorDiagnostic(
						"Unable to read device group",
						helpers.APIErrorDetail(err),
					),
				})
				return
			}

			manageMembers := strings.EqualFold(detail.GroupType, devicegroups.GroupTypeV1Static)
			var members []string
			if manageMembers {
				members, err = r.client.ListDeviceGroupMembers(ctx, detail.ID)
				if err != nil {
					stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
						diag.NewErrorDiagnostic(
							"Unable to read device group members",
							helpers.APIErrorDetail(err),
						),
					})
					return
				}
			}

			state := DeviceGroupResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(deviceGroupTimeoutAttributeTypes),
			}

			result.Diagnostics.Append(assignDeviceGroupModel(ctx, &state, detail, members, manageMembers, true)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			// No prior state in a list/query result → reverse-resolve any Jamf-group
			// "member of" criterion id back to the group name (11.29 read regression)
			// so `terraform query -generate-config-out` emits names, not ids.
			state.Criteria = readbackGroupRefCriteria(ctx, r.groupRef, dsObjectType(state.DeviceType.ValueString()), state.Criteria, nil)

			jamfProID, jamfProDiags := resolveJamfProID(ctx, r.proClient, r.pd, detail.ID)
			result.Diagnostics.Append(jamfProDiags...)
			state.JamfProID = jamfProID

			state.Timeouts = helpers.EnsureResourceTimeouts(state.Timeouts, deviceGroupTimeoutAttributeTypes)

			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
		emitted++
	}

	tflog.Debug(ctx, "Listed device groups", map[string]any{
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
