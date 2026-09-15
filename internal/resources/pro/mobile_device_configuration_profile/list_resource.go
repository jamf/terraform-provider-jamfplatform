// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_configuration_profile

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultListTimeout = 120 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &ListResource{}
var _ list.ListResourceWithConfigure = &ListResource{}

// NewListResource returns a list resource for mobile device configuration profile queries.
func NewListResource() list.ListResource {
	return &ListResource{}
}

// ListResource queries mobile device configuration profiles. List items carry
// only id + name, so when IncludeResource is requested (config generation) each
// profile is fetched individually and hydrated through the shared Read
// state-builder with includeUnmanaged=true, fully populating general, scope,
// and self_service so the generated config is complete rather than
// general-only.
type ListResource struct {
	client *proclassic.Client
}

// ListResourceConfigModel is the config model for list queries.
type ListResourceConfigModel struct {
	Filter *filters.ClassicFilterModel `tfsdk:"filter"`
}

// Metadata sets the list resource type name.
func (r *ListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_configuration_profile"
}

// Configure wires the SDK client.
func (r *ListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_configuration_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *ListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists mobile device configuration profiles in the tenant. Supply an optional case-insensitive `name_substring` filter. Filtering happens in the provider, so every profile is fetched before the filter runs. List entries return identity only; use the `jamfplatform_pro_mobile_device_configuration_profile` data source for per-profile detail." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams profile identities back to Terraform.
func (r *ListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run after `terraform init` completes.",
			),
		})
		return
	}

	var config ListResourceConfigModel
	if diags := req.Config.Get(ctx, &config); diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListMobileDeviceConfigurationProfiles(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list mobile device configuration profiles", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.MobileDeviceConfigurationProfilesItemConfigurationProfile{}
	if resp != nil {
		items = resp.ConfigurationProfiles
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, itemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)
	// skippedByGate collects profiles the import fidelity gate dropped, reported
	// as a single warning once the stream is assembled.
	var skippedByGate []string
	for _, it := range items {
		if int64(len(results)) >= maxResults {
			break
		}
		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(it.Name)
		id := helpers.StringValueFromIntPtr(it.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, identityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			got, err := r.client.GetMobileDeviceConfigurationProfileByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping mobile device configuration profile from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			// Import fidelity gate. Config generation is the bulk on-ramp to import, so
			// a profile Jamf Pro cannot store back must not be emitted as apply-ready
			// HCL. Drop it from the stream and name it in one consolidated warning
			// below; failing the stream instead would abandon config generation for
			// every other profile in the tenant over one unmanageable profile.
			var payload []byte
			if got != nil && got.General != nil && got.General.Payloads != nil {
				payload = []byte(string(*got.General.Payloads))
			}
			if payloadhelpers.ImportGateSkip(payload, payloadhelpers.PlatformMobileDevice) {
				skippedByGate = append(skippedByGate, helpers.DerefString(it.Name))
				continue
			}
			state := ResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignResourceModel(ctx, &state, got, true)...)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Pro mobile device configuration profiles", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"limit":          req.Limit,
		"returned":       len(results),
	})

	gateWarning := payloadhelpers.ImportGateSkipWarning(skippedByGate, "jamfplatform_pro_mobile_device_configuration_profile")

	if len(results) == 0 {
		if len(gateWarning) > 0 {
			stream.Results = list.ListResultsStreamDiagnostics(gateWarning)
			return
		}
		stream.Results = list.NoListResults
		return
	}
	// One warning for the whole query, carried by the first streamed result.
	results[0].Diagnostics.Append(gateWarning...)
	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}

func itemName(p proclassic.MobileDeviceConfigurationProfilesItemConfigurationProfile) string {
	return helpers.DerefString(p.Name)
}
