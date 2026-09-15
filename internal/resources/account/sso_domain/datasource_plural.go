// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPluralReadTimeout caps how long the plural SSO domains read will wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The domain
// collection takes no filter, so every read returns the same set and there is
// nothing to derive an ID from.
const pluralDataSourceID = "sso_domains"

// DomainsDataSource implements the Terraform data source for listing every DNS
// domain a Jamf Account organization holds.
type DomainsDataSource struct {
	client *account.Client
}

var _ datasource.DataSource = &DomainsDataSource{}

// NewDomainsDataSource returns a new instance of DomainsDataSource.
func NewDomainsDataSource() datasource.DataSource {
	return &DomainsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DomainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_domains"
}

// Schema returns the plural data source schema.
func (d *DomainsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every DNS domain your Jamf Account organization holds, including any shared " +
			"with it by another organization. Jamf Account exposes no search arguments for domains, so this data " +
			"source takes none. Filter the result in Terraform.\n\n" +
			"Assignment information is not included: the connections a domain is in use by are read one domain at " +
			"a time, so use the singular `jamfplatform_account_sso_domain` data source for that." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"sso_domains": schema.ListNestedAttribute{
				MarkdownDescription: "The domains the organization holds, in the order Jamf Account returns them.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier Jamf Account assigned to the claim.",
							Computed:            true,
						},
						"domain": schema.StringAttribute{
							MarkdownDescription: "The DNS domain, in lower case.",
							Computed:            true,
						},
						"verification_status": schema.StringAttribute{
							MarkdownDescription: "Verification state of the claim: " + verificationStatusDocs() + ".",
							Computed:            true,
						},
						"verification_key": schema.StringAttribute{
							MarkdownDescription: "Token Jamf Account minted for this claim, published as the " +
								"value of a TXT record on the domain to prove ownership.",
							Computed: true,
						},
						"verification_txt_record": schema.StringAttribute{
							MarkdownDescription: "Complete TXT record value to publish at the root of the domain.",
							Computed:            true,
						},
						"parent_domain_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the verified parent domain a subdomain inherits " +
								"its verification from.",
							Computed: true,
						},
						"shared": schema.BoolAttribute{
							MarkdownDescription: "Whether the domain is owned by another Jamf Account " +
								"organization and shared with yours. A shared domain can be assigned to a " +
								"connection but cannot be changed or withdrawn, so it cannot be managed as a " +
								"`jamfplatform_account_sso_domain` resource.",
							Computed: true,
						},
						"account_id": schema.StringAttribute{
							MarkdownDescription: "Identifier of the Jamf account the domain belongs to.",
							Computed:            true,
						},
						"created_by": schema.StringAttribute{
							MarkdownDescription: "Name of the Jamf Account user who added the domain.",
							Computed:            true,
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
							MarkdownDescription: "When ownership was last verified successfully.",
							Computed:            true,
						},
						"verification_expires_at": schema.StringAttribute{
							MarkdownDescription: "When the current verification lapses.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *DomainsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_domains")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every claimed domain and populates Terraform state.
func (d *DomainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DomainsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
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

	data.ID = types.StringValue(pluralDataSourceID)
	data.SSODomains = make([]DomainsDataSourceResultModel, 0, len(domains))
	for _, domain := range domains {
		data.SSODomains = append(data.SSODomains, buildDomainsResultModel(domain))
	}

	tflog.Trace(ctx, "read Jamf Account SSO domains data source", map[string]any{"count": len(data.SSODomains)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
