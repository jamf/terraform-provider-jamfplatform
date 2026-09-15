// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListDeviceEnrollmentsV1
// Status: current. Last reviewed 2026-05-25.

package automated_device_enrollment

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

// defaultListTimeout caps how long the list operation will wait on the
// Pro /v1/device-enrollments endpoint. The list resource schema does not
// expose a user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 60 * time.Second

var (
	_ list.ListResource              = &AutomatedDeviceEnrollmentListResource{}
	_ list.ListResourceWithConfigure = &AutomatedDeviceEnrollmentListResource{}
)

// AutomatedDeviceEnrollmentListResource implements Terraform query list
// support for Jamf Pro Automated Device Enrollment instances. The Pro list
// endpoint accepts no RSQL filter, so the optional `filter` block is applied
// client-side via filters.ApplyClassicFilter after the full list is fetched.
// The list endpoint returns the full `DeviceEnrollmentInstance` shape per
// row, so when IncludeResource=true the row is populated from the same
// response — no N+1 follow-up GET is required.
type AutomatedDeviceEnrollmentListResource struct {
	client *pro.Client
}

// NewAutomatedDeviceEnrollmentListResource returns a list resource for Jamf
// Pro Automated Device Enrollment queries.
func NewAutomatedDeviceEnrollmentListResource() list.ListResource {
	return &AutomatedDeviceEnrollmentListResource{}
}

// Metadata sets the list resource type name.
func (r *AutomatedDeviceEnrollmentListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_automated_device_enrollment"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *AutomatedDeviceEnrollmentListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_automated_device_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *AutomatedDeviceEnrollmentListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro Automated Device Enrollment instances. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched. The Jamf Pro list response carries every attribute, so `include_resource = true` does not require a follow-up read per item." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams ADE instance identities back to
// Terraform.
func (r *AutomatedDeviceEnrollmentListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config AutomatedDeviceEnrollmentListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListDeviceEnrollmentsV1(listCtx, nil)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro Automated Device Enrollment instances", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, automatedDeviceEnrollmentListItemName)

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
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, automatedDeviceEnrollmentIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := AutomatedDeviceEnrollmentResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(automatedDeviceEnrollmentTimeoutAttributeTypes),
			}
			assignAutomatedDeviceEnrollmentResourceModel(&state, &item)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro Automated Device Enrollment instances", map[string]any{
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

// automatedDeviceEnrollmentListItemName is the name accessor passed to
// filters.ApplyClassicFilter. `DeviceEnrollmentInstance.Name` is a plain
// `string` field (not a pointer), so no nil check is required.
func automatedDeviceEnrollmentListItemName(item pro.DeviceEnrollmentInstance) string {
	return item.Name
}
