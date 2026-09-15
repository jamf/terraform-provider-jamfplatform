// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// SearchDomainDataSource implements the Terraform data source for the Jamf Security
// Cloud search domain.
type SearchDomainDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &SearchDomainDataSource{}

// NewSearchDomainDataSource returns a new instance of SearchDomainDataSource.
func NewSearchDomainDataSource() datasource.DataSource {
	return &SearchDomainDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *SearchDomainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_dns_search_domain"
}

// Schema returns the data source schema. It takes no arguments: there is one search
// domain per tenant and nothing to select it by.
func (d *SearchDomainDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the **\"Search domain\"** under Custom DNS in the Jamf Security Cloud admin " +
			"UI: the domain that completes an incomplete host name for apps that only accept short host " +
			"names.\n\n" +
			"There is one search domain per tenant, so this data source takes no arguments. Reading it when no " +
			"search domain is configured is an error." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `singleton`, since a tenant holds one search domain.",
				Computed:            true,
			},
			"domain_name": schema.StringAttribute{
				MarkdownDescription: "**\"Domain name\"** in the Jamf Security Cloud admin UI: the configured " +
					"search domain.",
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *SearchDomainDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_dns_search_domain")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// noSearchDomainConfiguredError is the diagnostic the data source raises when the
// tenant holds no search domain, kept in one place so the two paths that reach it
// cannot drift apart.
func noSearchDomainConfiguredError() (string, string) {
	return "No Jamf Security Cloud search domain configured",
		"This tenant has no search domain set, so there is nothing to read. Set one in the Jamf Security " +
			"Cloud admin UI under Custom DNS, or manage it with the " +
			"jamfplatform_security_cloud_dns_search_domain resource."
}

// Read fetches the tenant's search domain and populates Terraform state.
//
// An unconfigured search domain answers 404, which is an error here rather than an
// empty result: a data source that silently produced an empty string would feed that
// into whatever referenced it.
//
// A successful read carrying no value raises the same error, which is what makes the
// promise above hold for both shapes. That one is defence rather than an observed
// answer — absence presents as a 404, and Create's preflight guards the same shape
// on the write path.
func (d *SearchDomainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(providerNotConfiguredError())
		return
	}

	var data SearchDomainDataSourceModel
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

	got, err := d.client.GetDnsSearchDomainV1(readCtx)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddError(noSearchDomainConfiguredError())
			return
		}
		resp.Diagnostics.AddError("Unable to read Jamf Security Cloud search domain", helpers.APIErrorDetail(err))
		return
	}

	if got == nil || got.Suffix == "" {
		resp.Diagnostics.AddError(noSearchDomainConfiguredError())
		return
	}

	assignSearchDomainDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Security Cloud search domain data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
