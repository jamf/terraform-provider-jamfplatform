// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListMobileDeviceExtensionAttributesV1
// Status: current. Last reviewed 2026-06-02.

package mobile_device_extension_attribute

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation waits on the Pro
// /v1/mobile-device-extension-attributes endpoint.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &MobileDeviceExtensionAttributeListResource{}
var _ list.ListResourceWithConfigure = &MobileDeviceExtensionAttributeListResource{}

// NewMobileDeviceExtensionAttributeListResource returns a list resource for
// mobile device extension attribute queries.
func NewMobileDeviceExtensionAttributeListResource() list.ListResource {
	return &MobileDeviceExtensionAttributeListResource{}
}

// MobileDeviceExtensionAttributeListResource implements Terraform query list
// support. The Pro list endpoint returns full objects, so `include_resource`
// hydrates every attribute directly — no per-row follow-up GET.
type MobileDeviceExtensionAttributeListResource struct {
	client *pro.Client
}

// MobileDeviceExtensionAttributeListResourceModel is the config model for list
// queries.
type MobileDeviceExtensionAttributeListResourceModel struct {
	Filter *filters.ClassicFilterModel `tfsdk:"filter"`
}

// Metadata sets the list resource type name.
func (r *MobileDeviceExtensionAttributeListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_extension_attribute"
}

// Configure wires the Jamf Pro client into the list resource.
func (r *MobileDeviceExtensionAttributeListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *MobileDeviceExtensionAttributeListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro mobile device extension attributes. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams mobile device extension attribute
// identities back to Terraform.
func (r *MobileDeviceExtensionAttributeListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config MobileDeviceExtensionAttributeListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListMobileDeviceExtensionAttributesV1(listCtx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro mobile device extension attributes", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, mobileDeviceExtensionAttributeListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for i := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		item := items[i]

		result := req.NewListResult(ctx)
		result.DisplayName = item.Name

		id := helpers.StringPointerValueOrNull(item.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, mobileDeviceExtensionAttributeIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := MobileDeviceExtensionAttributeResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(mobileDeviceExtensionAttributeTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignMobileDeviceExtensionAttributeResourceModel(listCtx, &state, &item)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro mobile device extension attributes", map[string]any{
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

// mobileDeviceExtensionAttributeListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func mobileDeviceExtensionAttributeListItemName(s pro.MobileDeviceExtensionAttributes) string {
	return s.Name
}
