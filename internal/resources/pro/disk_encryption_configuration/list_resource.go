// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation will wait on the
// classic /diskencryptionconfigurations endpoint. The list resource
// schema does not expose a user-overridable timeout.
const defaultListTimeout = 90 * time.Second

// defaultItemReadTimeout bounds each per-item hydration GET issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow item cannot
// exhaust a shared deadline. An item whose read fails or times out is dropped
// from the generated config rather than aborting the whole type.
const defaultItemReadTimeout = 30 * time.Second

var (
	_ list.ListResource              = &DiskEncryptionConfigurationListResource{}
	_ list.ListResourceWithConfigure = &DiskEncryptionConfigurationListResource{}
)

// NewDiskEncryptionConfigurationListResource returns a list resource for
// Jamf Pro disk encryption configuration queries.
func NewDiskEncryptionConfigurationListResource() list.ListResource {
	return &DiskEncryptionConfigurationListResource{}
}

// DiskEncryptionConfigurationListResource implements Terraform query list
// support. Classic /diskencryptionconfigurations accepts no query
// parameters, so the optional `filter` block is applied client-side via
// filters.ApplyClassicFilter after the full list is fetched. The list
// endpoint returns only id+name per row, so when IncludeResource=true we
// follow up with a per-item GET to populate the full record — N+1 path
// mirroring directory_binding's list resource.
type DiskEncryptionConfigurationListResource struct {
	client *proclassic.Client
}

// Metadata sets the list resource type name.
func (r *DiskEncryptionConfigurationListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_disk_encryption_configuration"
}

// Configure wires the Jamf ProClassic client into the list resource.
func (r *DiskEncryptionConfigurationListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_disk_encryption_configuration")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the supported list filters.
func (r *DiskEncryptionConfigurationListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists Jamf Pro disk encryption configurations. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched. By default each row returns `id` and `name`. Set `include_resource = true` to fetch the full record for each item." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"filter": filters.ClassicListFilterAttribute(),
		},
	}
}

// List executes the query and streams disk encryption configuration
// identities back to Terraform.
func (r *DiskEncryptionConfigurationListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config DiskEncryptionConfigurationListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, defaultListTimeout)
	defer cancel()

	resp, err := r.client.ListDiskEncryptionConfigurations(listCtx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Pro disk encryption configurations", helpers.APIErrorDetail(err)),
		})
		return
	}

	items := []proclassic.IDName{}
	if resp != nil {
		items = resp.DiskEncryptionConfigurations
	}

	filter := filters.ClassicFilterModel{}
	if config.Filter != nil {
		filter = *config.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, diskEncryptionConfigurationListItemName)

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(items)) {
		maxResults = int64(len(items))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, c := range items {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = helpers.DerefString(c.Name)

		id := helpers.StringValueFromIntPtr(c.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, diskEncryptionConfigurationIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			// /diskencryptionconfigurations list response carries only
			// id+name. Follow up with a singular GET to populate the
			// full record rather than emitting nulls.
			itemCtx, cancel := context.WithTimeout(ctx, defaultItemReadTimeout)
			full, err := r.client.GetDiskEncryptionConfigurationByID(itemCtx, id.ValueString())
			cancel()
			if err != nil {
				tflog.Warn(ctx, "Skipping disk encryption configuration from generated config after per-item read failure", map[string]any{
					"id":    id.ValueString(),
					"error": err.Error(),
				})
				continue
			}
			state := DiskEncryptionConfigurationResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(diskEncryptionConfigurationTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignDiskEncryptionConfigurationResourceModel(&state, full)...)
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

	tflog.Debug(ctx, "Listed Jamf Pro disk encryption configurations", map[string]any{
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

// diskEncryptionConfigurationListItemName is the name accessor passed to
// filters.ApplyClassicFilter.
func diskEncryptionConfigurationListItemName(c proclassic.IDName) string {
	return helpers.DerefString(c.Name)
}
