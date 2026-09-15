// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package volume_purchasing_notification

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

// defaultListTimeout caps how long the list operation waits on the endpoint.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &VolumePurchasingNotificationListResource{}
	_ list.ListResourceWithConfigure = &VolumePurchasingNotificationListResource{}
)

// NewVolumePurchasingNotificationListResource returns a list resource.
func NewVolumePurchasingNotificationListResource() list.ListResource {
	return &VolumePurchasingNotificationListResource{}
}

// VolumePurchasingNotificationListResource implements Terraform query list support.
// The endpoint has no server-side name filter, so the optional `filter` block is
// applied client-side as a case-insensitive name substring. List items carry
// identity (id) + display name only, so when IncludeResource is requested (config
// generation) each notification is fetched individually and hydrated through the
// shared Read state-builder — matching the resource's import fidelity.
type VolumePurchasingNotificationListResource struct {
	client *pro.Client
}

// Metadata sets the list resource type name.
func (r *VolumePurchasingNotificationListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_volume_purchasing_notification"
}

// Configure wires the Jamf Pro client into the list resource.
func (r *VolumePurchasingNotificationListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_volume_purchasing_notification")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *VolumePurchasingNotificationListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Volume Purchasing notifications. Supply an optional case-insensitive `name_substring` filter applied locally after the full list is fetched. List entries surface as identity-only (id and display name); full detail requires a per-notification read." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams notification identities back to Terraform.
func (r *VolumePurchasingNotificationListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config VolumePurchasingNotificationListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	entries, err := r.client.ListVolumePurchasingSubscriptionsV1(listCtx, nil)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Volume Purchasing notifications", helpers.APIErrorDetail(err)),
		})
		return
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	entries = filters.ApplyClassicFilter(entries, filter, volumePurchasingNotificationListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(entries)) {
		maxResults = int64(len(entries))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, e := range entries {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = e.Name

		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, volumePurchasingNotificationIdentityModel{ID: helpers.StringPointerValueOrNull(&e.ID)})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetVolumePurchasingSubscriptionV1(itemCtx, e.ID)
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping volume purchasing notification from generated config after per-item read failure", map[string]any{
					"id":    e.ID,
					"error": err.Error(),
				})
				continue
			}
			state := VolumePurchasingNotificationResourceModel{
				ID:       helpers.StringPointerValueOrNull(&e.ID),
				Timeouts: helpers.NewResourceTimeoutsNullValue(volumePurchasingNotificationTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignVolumePurchasingNotificationResourceModel(ctx, &state, got)...)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Volume Purchasing notifications", map[string]any{
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

// volumePurchasingNotificationListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func volumePurchasingNotificationListItemName(e pro.VolumePurchasingSubscription) string {
	return e.Name
}
