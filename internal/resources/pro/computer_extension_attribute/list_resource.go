// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListComputerExtensionAttributesV1
// Status: current. Last reviewed 2026-06-02.

package computer_extension_attribute

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
// /v1/computer-extension-attributes endpoint.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &ComputerExtensionAttributeListResource{}
var _ list.ListResourceWithConfigure = &ComputerExtensionAttributeListResource{}

// NewComputerExtensionAttributeListResource returns a list resource for computer
// extension attribute queries.
func NewComputerExtensionAttributeListResource() list.ListResource {
	return &ComputerExtensionAttributeListResource{}
}

// ComputerExtensionAttributeListResource implements Terraform query list support.
// The Pro list endpoint returns full ComputerExtensionAttributes objects, so
// `include_resource` hydrates every attribute directly from the list element —
// no per-row follow-up GET. The optional `filter` block applies a
// case-insensitive name substring match client-side after the full list is
// fetched.
type ComputerExtensionAttributeListResource struct {
	client *pro.Client
}

// ComputerExtensionAttributeListResourceModel is the config model for list
// queries.
type ComputerExtensionAttributeListResourceModel struct {
	Filter *filters.ClassicFilterModel `tfsdk:"filter"`
}

// Metadata sets the list resource type name.
func (r *ComputerExtensionAttributeListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_extension_attribute"
}

// Configure wires the Jamf Pro client into the list resource.
func (r *ComputerExtensionAttributeListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *ComputerExtensionAttributeListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description:         "Lists Jamf Pro computer extension attributes. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched.",
		MarkdownDescription: "Lists Jamf Pro computer extension attributes. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams computer extension attribute identities
// back to Terraform.
func (r *ComputerExtensionAttributeListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config ComputerExtensionAttributeListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	items, err := r.client.ListComputerExtensionAttributesV1(listCtx, nil, "")
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro computer extension attributes", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, computerExtensionAttributeListItemName)

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
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, computerExtensionAttributeIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// The list response carries the full object; hydrate every attribute
			// directly — no follow-up GET.
			state := ComputerExtensionAttributeResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(computerExtensionAttributeTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignComputerExtensionAttributeResourceModel(listCtx, &state, &item)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro computer extension attributes", map[string]any{
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

// computerExtensionAttributeListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func computerExtensionAttributeListItemName(s pro.ComputerExtensionAttributes) string {
	return s.Name
}
