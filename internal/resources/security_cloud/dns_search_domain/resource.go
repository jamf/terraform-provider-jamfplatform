// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package dns_search_domain implements the
// jamfplatform_security_cloud_dns_search_domain resource and data source, backed by
// the Jamf Security Cloud custom DNS API.
//
// The search domain is the suffix Jamf Security Cloud appends to an unqualified
// host name, so that an app which only accepts a short name still reaches the right
// host: with the search domain set to `example.com`, a user asking for `product`
// is directed to `product.example.com`.
//
// Attribute names follow the admin UI rather than the wire, per STYLE_GUIDE
// §Attribute names mirror the Jamf Pro admin UI. Because the guide also forbids
// comments inside function bodies, the wire mapping lives here:
//
//	Terraform attribute   Wire field
//	-------------------   ----------
//	domain_name           suffix
//
// # Shape: one per tenant, with a real clear
//
// There is exactly one search domain per tenant and no identifier for it, so this
// resource follows STYLE_GUIDE §Singleton resources for its fixed
// `helpers.SingletonID`, identity schema, import validation and nil-client guards.
// It diverges on the two points that section assumes away, because the wire does
// not:
//
//  1. Delete is real. §Singleton resources says "the record cannot be deleted";
//     `DELETE /dns/search-domains` clears it and answers 204, and answers 204 again
//     when nothing was set. So Delete calls the endpoint rather than no-opping, and
//     CheckDestroy is the ordinary contract (assert gone) rather than the inverted
//     singleton one.
//  2. Absence is observable. `GET` on an unset search domain is a 404 carrying
//     `SEARCH_DOMAIN_NOT_SET`, not an empty 200, so Read can and does treat it as
//     deleted and drop the resource from state.
//
// # Create refuses to clobber
//
// `PUT` is an unconditional upsert: it overwrites whatever was there and reports no
// conflict, so nothing on the wire distinguishes "creating the tenant's search
// domain" from "silently replacing the one an administrator set by hand". Create
// therefore reads first and refuses when a search domain already exists, pointing
// the operator at import. Without that, a first apply quietly takes over — and two
// Terraform configurations both declaring this resource would fight with neither
// reporting anything.
//
// Wire-probed against production EU on 2026-08-29 under a tenant-scoped
// integration. The admin UI renders the saved value as a one-row table with a
// Remove link and keeps its input box, which reads like a multi-value list; it is
// not one. A second write replaces the first.
package dns_search_domain

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// SearchDomainResource implements the Terraform resource for the Jamf Security
// Cloud search domain.
type SearchDomainResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                = &SearchDomainResource{}
	_ resource.ResourceWithImportState = &SearchDomainResource{}
	_ resource.ResourceWithIdentity    = &SearchDomainResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewSearchDomainResource returns a new instance of SearchDomainResource.
func NewSearchDomainResource() resource.Resource {
	return &SearchDomainResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *SearchDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_search_domain"
}

// IdentitySchema defines the identifier used for import. There is one search domain
// per tenant, so the identifier is always helpers.SingletonID.
func (r *SearchDomainResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Always \"singleton\", since a tenant holds one search domain.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the search domain resource.
func (r *SearchDomainResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the **\"Search domain\"** under Custom DNS in the Jamf Security Cloud admin " +
			"UI: the domain that completes an incomplete host name for apps that only accept short host " +
			"names. With the search domain set to `example.com`, a user who asks for `product` is directed to " +
			"`product.example.com`.\n\n" +
			"There is one search domain per tenant, so only one instance of this resource should exist in your " +
			"configuration. Destroying it clears the search domain for the whole tenant." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton`, since a tenant holds one search domain.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"domain_name": schema.StringAttribute{
				MarkdownDescription: "**\"Domain name\"** in the Jamf Security Cloud admin UI. Up to 253 " +
					"characters. A single domain, not a list: writing a new value replaces the previous one. " +
					"Wildcards are not accepted, and letter case is stored exactly as written.",
				Required: true,
				Validators: []validator.String{
					commonvalidators.DNSHostname(),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *SearchDomainResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_search_domain")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import of the tenant's single search domain.
//
// The identifier must be helpers.SingletonID. Normalising anything else silently
// would hide a mis-typed import behind a Read that happens to succeed, since the
// endpoint takes no identifier and would return the tenant's search domain
// whatever was typed.
//
// helpers.ImportSingletonState is what accepts the identifier from either import
// form: `terraform import` and an `id =` import block fill req.ID, while an
// `identity = { id = … }` block leaves it empty and puts the value in
// req.Identity. This resource advertises an IdentitySchema, so it has to honour
// both.
//
//	terraform import jamfplatform_security_cloud_dns_search_domain.<name> singleton
func (r *SearchDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_security_cloud_dns_search_domain")
}
