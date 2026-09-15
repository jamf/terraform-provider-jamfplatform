// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

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

// defaultZoneSort is the sort expression every zone list read sends. `sort` is
// the only query parameter the endpoint accepts — there is no filter — and
// pinning it to ascending name order makes the streamed results deterministic
// across runs instead of leaving the order to the server's default.
const defaultZoneSort = "name:asc"

var (
	_ list.ListResource              = &DNSZoneListResource{}
	_ list.ListResourceWithConfigure = &DNSZoneListResource{}
)

// NewDNSZoneListResource returns a list resource for Jamf Security Cloud custom
// DNS zone queries.
func NewDNSZoneListResource() list.ListResource {
	return &DNSZoneListResource{}
}

// DNSZoneListResource implements Terraform query list support for Jamf Security
// Cloud custom DNS zones. The zone list endpoint accepts only a sort expression,
// so there is no filter block — the resource returns every zone on the tenant.
type DNSZoneListResource struct {
	client *securitycloud.Client
}

// Metadata sets the list resource type name.
func (r *DNSZoneListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_zone"
}

// Configure wires the Jamf Security Cloud client into the list resource via the
// shared providerdata.ConfigureSecurityCloud helper.
func (r *DNSZoneListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_zone")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *DNSZoneListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists every Jamf Security Cloud custom DNS zone on the tenant. Jamf Security Cloud exposes no " +
			"filter parameters for zones, so this list resource takes no filter configuration." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams DNS zone identities back to Terraform.
func (r *DNSZoneListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config DNSZoneListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	zones, err := r.client.ListDnsZonesV1(ctx, defaultZoneSort)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Security Cloud DNS zones", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(zones.Results)) {
		maxResults = int64(len(zones.Results))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, z := range zones.Results {
		if int64(len(results)) >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = z.Name

		id := types.StringValue(z.ID)
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, dnsZoneIdentityModel{ID: id})...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := DNSZoneResourceModel{
				ID:       id,
				Timeouts: helpers.NewResourceTimeoutsNullValue(dnsZoneTimeoutAttributeTypes),
			}
			result.Diagnostics.Append(assignDNSZoneResourceModel(ctx, &state, &z)...)
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

	tflog.Debug(ctx, "Listed Jamf Security Cloud DNS zones", map[string]any{
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
