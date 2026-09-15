// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

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

// defaultListTimeout caps how long the list operation will wait on the
// /app-request/form-input-fields endpoint. The list resource schema does not expose a
// user-overridable timeout, so this is a fixed safety bound.
const defaultListTimeout = 90 * time.Second

var _ list.ListResource = &AppRequestFormFieldListResource{}
var _ list.ListResourceWithConfigure = &AppRequestFormFieldListResource{}

// NewAppRequestFormFieldListResource returns a list resource for Jamf Pro App Request form
// field queries.
func NewAppRequestFormFieldListResource() list.ListResource {
	return &AppRequestFormFieldListResource{}
}

// AppRequestFormFieldListResource implements Terraform query list support for Jamf Pro App
// Request form fields. The /app-request/form-input-fields endpoint accepts no query
// parameters, so the optional `filter` block is applied client-side via
// filters.ApplyClassicFilter after the full list is fetched.
type AppRequestFormFieldListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *AppRequestFormFieldListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_request_form_field"
}

// Configure wires the Jamf Pro client into the list resource via the shared
// providerdata.ConfigurePro helper.
func (r *AppRequestFormFieldListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_request_form_field")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *AppRequestFormFieldListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro App Request form fields. Supply an optional case-insensitive `name_substring` filter (matched against the field title); filtering is applied client-side after the full list is fetched." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams App Request form field identities back to Terraform.
func (r *AppRequestFormFieldListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config AppRequestFormFieldListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListAppRequestFormInputFieldsV1(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro App Request form fields", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []pro.AppRequestFormInputField{}
	if resp != nil {
		items = resp.Results
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, appRequestFormFieldName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, m := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = m.Title

		id := helpers.StringValueFromIntPtr(m.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, appRequestFormFieldIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := AppRequestFormFieldResourceModel{
				ID:          id,
				Title:       types.StringValue(m.Title),
				Description: types.StringPointerValue(m.Description),
				Priority:    types.Int64Value(int64(m.Priority)),
				Timeouts:    helpers.NewResourceTimeoutsNullValue(appRequestFormFieldTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro App Request form fields", map[string]any{
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

// appRequestFormFieldName is the name accessor passed to filters.ApplyClassicFilter.
func appRequestFormFieldName(m pro.AppRequestFormInputField) string {
	return m.Title
}
