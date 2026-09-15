// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

var (
	_ list.ListResource              = &DomainListResource{}
	_ list.ListResourceWithConfigure = &DomainListResource{}
)

// NewDomainListResource returns a list resource for Jamf Account SSO domain
// queries.
func NewDomainListResource() list.ListResource {
	return &DomainListResource{}
}

// DomainListResource implements Terraform query and bulk-import support for Jamf
// Account SSO domains.
//
// The domain collection accepts neither a filter nor a sort expression, so there
// is no filter block and no ordering to pin: the resource streams the domains the
// organization holds, in the order Jamf returns them.
//
// One entry class is dropped. The collection also returns the domains other
// organizations have shared in, which this organization may assign to an SSO
// connection but cannot change or withdraw, so they are not manageable as
// jamfplatform_account_sso_domain and streaming them would offer a bulk import
// whose every entry is stuck in state; the data sources are how a shared domain
// is read. `req.Limit` still caps the results rather than the scan: it is
// clamped against the number of domains returned, which is an upper bound on the
// number kept, and the loop counts kept entries — so a limit of 5 against ten
// domains of which six are shared yields all four owned ones rather than
// stopping early.
type DomainListResource struct {
	client *account.Client
}

// Metadata sets the list resource type name.
func (r *DomainListResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_domain"
}

// Configure wires the Jamf Account client into the list resource via the shared
// providerdata.ConfigureAccount helper.
func (r *DomainListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_domain")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ListResourceConfigSchema describes the (empty) list configuration.
func (r *DomainListResource) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Lists the DNS domains your Jamf Account organization has claimed, for `terraform query` and " +
			"for importing existing claims in bulk. Jamf Account exposes no search arguments for domains, so this " +
			"list resource takes no filter configuration.\n\n" +
			"Domains shared into your organization by another Jamf Account organization are excluded. They cannot " +
			"be changed or withdrawn, so they cannot be managed as `jamfplatform_account_sso_domain` and importing " +
			"one would leave an entry that no destroy can remove. Use the " +
			"`jamfplatform_account_sso_domains` data source to see every domain including the shared ones." +
			listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{},
	}
}

// List executes the query and streams SSO domain identities back to Terraform.
func (r *DomainListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config DomainListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	domains, err := r.client.ListDomains(ctx)
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic("Unable to list Jamf Account SSO domains", helpers.APIErrorDetail(err)),
		})
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(domains)) {
		maxResults = int64(len(domains))
	}

	results := make([]list.ListResult, 0, maxResults)

	for _, domain := range domains {
		if int64(len(results)) >= maxResults {
			break
		}

		if domain.SharedDomain {
			continue
		}

		result := req.NewListResult(ctx)
		result.DisplayName = domain.Domain

		identity := domainIdentityModel{Domain: types.StringValue(domain.Domain)}
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, identity)...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			state := DomainResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(domainTimeoutAttributeTypes),
			}
			assignDomainResourceModel(&state, &domain)
			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
	}

	tflog.Debug(ctx, "Listed Jamf Account SSO domains", map[string]any{
		"limit":    req.Limit,
		"scanned":  len(domains),
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
