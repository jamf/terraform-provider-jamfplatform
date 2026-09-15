// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var (
	_ list.ListResource              = &DeviceGroupListResource{}
	_ list.ListResourceWithConfigure = &DeviceGroupListResource{}
)

// NewDeviceGroupListResource returns a list resource for Jamf Security Cloud
// device group queries.
func NewDeviceGroupListResource() list.ListResource {
	return &DeviceGroupListResource{}
}

// DeviceGroupListResource implements Terraform query list support for Jamf
// Security Cloud device groups. The group list endpoint accepts no query
// parameters at all — not even a sort expression — so there is no filter block
// and the order is imposed by the provider rather than requested from the server:
// the resource returns every manageable group on the tenant sorted by name. See
// sortGroupsByName for why an observed server order is not relied on.
type DeviceGroupListResource struct {
	client *securitycloud.Client
}

// Metadata sets the list resource type name.
func (r *DeviceGroupListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_device_group"
}

// Configure wires the Jamf Security Cloud client into the list resource via the
// shared providerdata.ConfigureSecurityCloud helper.
func (r *DeviceGroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *DeviceGroupListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists every Jamf Security Cloud device group on the tenant that Terraform can manage. Jamf " +
			"Security Cloud exposes no filter parameters for groups, so this list resource takes no filter " +
			"configuration. Results are sorted by name by the provider. The built-in group is not listed: " +
			"it has no identifier, so it cannot be imported or managed. Use the " +
			"jamfplatform_security_cloud_device_groups data source to see it." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams device group identities back to Terraform.
//
// The implicit "Default Group" is skipped. A list result must carry an identity
// and that group has none, so including it would emit a result Terraform could
// neither import nor refresh. The plural data source reports it instead.
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

	groups, err := r.client.ListDeviceGroupsV2(ctx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Security Cloud device groups", helpers.APIErrorDetail(err)),
		})
		return
	}

	manageable := manageableGroups(sortGroupsByName(groups.Groups))

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(manageable)) {
		maxResults = int64(len(manageable))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, g := range manageable {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = g.Name

		id := types.StringValue(g.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, deviceGroupIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := DeviceGroupResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(deviceGroupTimeoutAttributeTypes),
			}
			assignDeviceGroupResourceModel(&state, &securitycloud.Group{ID: g.ID, Name: g.Name})
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Security Cloud device groups", map[string]any{
		"limit":    req.Limit,
		"returned": len(results),
		"skipped":  len(groups.Groups) - len(manageable),
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
