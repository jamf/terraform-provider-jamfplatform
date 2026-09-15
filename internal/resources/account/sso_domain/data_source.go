// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DomainDataSource implements the Terraform data source for a single Jamf Account
// SSO domain.
type DomainDataSource struct {
	client *account.Client
}

var _ datasource.DataSource = &DomainDataSource{}

// NewDomainDataSource returns a new instance of DomainDataSource.
func NewDomainDataSource() datasource.DataSource {
	return &DomainDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DomainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_domain"
}

// Schema returns the data source schema.
//
// The lookup key is the domain name and there is no alternative by identifier.
// Jamf Account exposes no read of a single claim, so an identifier lookup would
// have to scan the collection just the same — and the identifier is neither shown
// to a practitioner nor stable across a withdraw-and-reclaim, so offering it
// would be offering the worse of two keys.
func (d *DomainDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a DNS domain your Jamf Account organization holds, and the SSO " +
			"connections it is currently assigned to.\n\n" +
			"The assignment list is the read to do before destroying a claim: withdrawing a domain also removes " +
			"it from every connection that names it, and nothing warns you.\n\n" +
			"This is also the construct for a domain another Jamf Account organization owns and shares with " +
			"yours (`shared` is `true`). Such a domain can be assigned to a connection but cannot be changed or " +
			"withdrawn, so the `jamfplatform_account_sso_domain` resource refuses to manage one; reading it here " +
			"takes no ownership of it." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "Domain name to look up, such as `example.com`. Lower case only, and a bare " +
					"name: no scheme, no path, no port and no user part.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxDomainLength),
					DomainName(),
				},
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier Jamf Account assigned to the claim.",
				Computed:            true,
			},
			"verification_status": schema.StringAttribute{
				MarkdownDescription: "Verification state of the claim: " + verificationStatusDocs() + ".",
				Computed:            true,
			},
			"verification_key": schema.StringAttribute{
				MarkdownDescription: "Token Jamf Account minted for this claim, published as the value of a TXT " +
					"record on the domain to prove ownership.",
				Computed: true,
			},
			"verification_txt_record": schema.StringAttribute{
				MarkdownDescription: "Complete TXT record value to publish at the root of the domain: the " +
					"`jamf-site-verification=` prefix followed by `verification_key`.",
				Computed: true,
			},
			"parent_domain_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the verified parent domain a subdomain inherits its " +
					"verification from. Null for a domain verified in its own right.",
				Computed: true,
			},
			"shared": schema.BoolAttribute{
				MarkdownDescription: "Whether the domain is owned by another Jamf Account organization and shared " +
					"with yours. A shared domain can be assigned to a connection but cannot be changed or " +
					"withdrawn, so it can be read here but not managed as a " +
					"`jamfplatform_account_sso_domain` resource.",
				Computed: true,
			},
			"account_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the Jamf account the domain belongs to. For a shared domain " +
					"this is the owning organization's account rather than yours.",
				Computed: true,
			},
			"created_by": schema.StringAttribute{
				MarkdownDescription: "Name of the Jamf Account user who added the domain. Populated only for " +
					"domains added through the Jamf Account console.",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the domain was claimed.",
				Computed:            true,
			},
			"last_modified_at": schema.StringAttribute{
				MarkdownDescription: "When the claim last changed.",
				Computed:            true,
			},
			"last_verified_at": schema.StringAttribute{
				MarkdownDescription: "When ownership was last verified successfully. Null for a domain that has " +
					"never verified.",
				Computed: true,
			},
			"verification_expires_at": schema.StringAttribute{
				MarkdownDescription: "When the current verification lapses.",
				Computed:            true,
			},
			"assigned_connections": schema.ListNestedAttribute{
				MarkdownDescription: "SSO connections this domain is currently assigned to. Empty for a verified " +
					"domain no connection uses yet.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"connection_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the SSO connection.",
							Computed:            true,
						},
						"connection_organization_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the identity organization the connection is " +
								"assigned through. Managed by Jamf Account; informational only.",
							Computed: true,
						},
						"region": schema.StringAttribute{
							MarkdownDescription: "**\"Hosting region\"** in the Jamf Account console: the region " +
								"the connection's identity provider details live in and its sign-in traffic is " +
								"routed through.",
							Computed: true,
						},
					},
				},
			},
			"jamf_id_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether people on this domain can still sign in with a Jamf ID alongside the " +
					"identity provider.",
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *DomainDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_domain")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a claim by domain name, together with its assignment record, and
// populates Terraform state.
func (d *DomainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DomainDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	domains, err := d.client.ListDomains(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Account SSO domains", helpers.APIErrorDetail(err))
		return
	}

	found := findDomain(domains, data.Domain.ValueString())
	if found == nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("domain"),
			"Unable to find Jamf Account SSO domain",
			"Your Jamf Account organization has not claimed \""+data.Domain.ValueString()+"\". Use the "+
				"`jamfplatform_account_sso_domains` data source to list the domains it holds, including any "+
				"shared with it by another organization.",
		)
		return
	}

	allocation, err := d.client.GetDomainAllocation(readCtx, found.Domain)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Jamf Account SSO domain assignments",
			"The domain \""+found.Domain+"\" is claimed, but the connections it is assigned to could not be "+
				"read. Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}

	resp.Diagnostics.Append(assignDomainDataSourceModel(&data, found, allocation)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Account SSO domain data source", map[string]any{"domain": data.Domain.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
