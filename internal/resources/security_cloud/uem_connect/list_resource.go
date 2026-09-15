// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

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
	_ list.ListResource              = &UEMConnectListResource{}
	_ list.ListResourceWithConfigure = &UEMConnectListResource{}
)

// UEMConnectListResource implements Terraform query support for the Jamf Security
// Cloud UEM Connect integration.
//
// A tenant holds at most one integration, so this streams either one result or
// none, and it exists for discovery rather than for filtering. The integration's
// identifier is a value no operator would recognise or have written down, which
// makes importing an integration set up in the admin UI a hunt: read the data
// source, output the ID, then hand-copy it into an import block. `terraform query`
// against this list resource writes that import block instead.
//
// The plural data source that would normally accompany a list resource is
// deliberately absent — with one integration per tenant it would return exactly
// what the singular data source already returns.
type UEMConnectListResource struct {
	client *securitycloud.Client
}

// NewUEMConnectListResource returns a list resource for UEM Connect queries.
func NewUEMConnectListResource() list.ListResource {
	return &UEMConnectListResource{}
}

// Metadata sets the list resource type name.
func (r *UEMConnectListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_uem_connect"
}

// Configure wires the Jamf Security Cloud client into the list resource.
func (r *UEMConnectListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_uem_connect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *UEMConnectListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Finds the Jamf Security Cloud UEM Connect integration on the tenant, for generating an " +
			"import block with `terraform query`. A tenant holds at most one, so there is nothing to filter and " +
			"this takes no configuration." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List streams the integration's identity back to Terraform.
func (r *UEMConnectListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config UEMConnectListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	page, err := r.client.ListUemConnectorsV1(ctx)
	if err != nil {
		var listDiags diag.Diagnostics
		if !appendCreateDiagnostics(&listDiags, err) {
			listDiags.AddError("Unable to list the Jamf Security Cloud UEM Connect integration", helpers.APIErrorDetail(err))
		}
		stream.Results = list.ListResultsStreamDiagnostics(listDiags)
		return
	}

	// A tenant with no integration is an empty result, not an error. Unlike the
	// data source — whose reference cannot be satisfied by nothing — a query that
	// finds nothing has answered the question it was asked.
	if page == nil || len(page.Results) == 0 {
		tflog.Debug(ctx, "No Jamf Security Cloud UEM Connect integration on this tenant")
		stream.Results = list.NoListResults
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(page.Results)) {
		maxResults = int64(len(page.Results))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, connector := range page.Results {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		// The integration has no name of its own, so the display name is the Jamf
		// Pro instance it syncs with — the thing an operator would recognise it by.
		result.DisplayName = connector.URL

		id := types.StringValue(connector.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, uemConnectIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := UEMConnectResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(uemConnectTimeoutAttributeTypes),
			}
			// Populated as an import would be: a query has no configuration to say
			// which optional blocks are managed, so it captures everything the
			// tenant holds. Same reasoning as the import path in Read.
			result.Diagnostics.Append(assignUEMConnectResourceModel(&state, &connector, true)...)
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

	tflog.Debug(ctx, "Listed the Jamf Security Cloud UEM Connect integration", map[string]any{
		"limit":    req.Limit,
		"returned": len(results),
	})

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}
