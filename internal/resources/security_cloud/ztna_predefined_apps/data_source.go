// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ztna_predefined_apps implements the
// jamfplatform_security_cloud_ztna_predefined_apps data source, a read-only view of
// the Jamf-curated Zero Trust Network Access app templates.
//
// A predefined app is Jamf's own definition of a well-known SaaS application —
// Slack, Salesforce, Workday — bundling the hostnames that application uses. They
// are the same for every entitled tenant and cannot be created, changed or deleted,
// which is why this package holds only a plural data source: the API exposes no
// per-id endpoint to build a singular one on, and a list resource over a fixed
// catalogue nobody manages would be ceremony without a payoff.
//
// The reason it exists is discovery. A Zero Trust Network Access app built from a
// template names it by an opaque UUID, and that ID is only visible in the admin UI
// or through this data source. Reading the hostnames alongside it is what makes the
// choice reviewable: a template can carry a dozen or more, and adopting one means
// adopting all of them.
//
// Note that the consuming construct is not built. This provider does not manage
// Zero Trust Network Access apps yet, so today the identifiers read here are for
// reference and for pre-staging a configuration — an output, or a value an
// administrator carries into the admin UI — not something another resource can be
// wired to.
//
// Wire-probed 2026-08-29 in production EU: 30 templates, unpaginated, `totalCount`
// equal to the number of results, names unique, and returned in an order that is
// neither sorted nor documented — so the order is passed through as the server
// gives it rather than presented as meaningful.
package ztna_predefined_apps

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultReadTimeout caps how long the predefined apps read will wait.
const defaultReadTimeout = 60 * time.Second

// dataSourceID is the fixed ID this data source reports. The catalogue is the same
// for every read, so there is nothing to derive an ID from.
const dataSourceID = "ztna_predefined_apps"

// PredefinedAppsDataSource implements the Terraform data source for the
// Jamf-curated Zero Trust Network Access app templates.
type PredefinedAppsDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &PredefinedAppsDataSource{}

// NewPredefinedAppsDataSource returns a new instance of PredefinedAppsDataSource.
func NewPredefinedAppsDataSource() datasource.DataSource {
	return &PredefinedAppsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PredefinedAppsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_predefined_apps"
}

// Schema returns the data source schema.
func (d *PredefinedAppsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the predefined Zero Trust Network Access app templates available in Jamf " +
			"Security Cloud: the built-in definitions of well-known applications such as `Slack` or " +
			"`Salesforce`, each bundling the hostnames that application uses. The catalogue is centrally " +
			"curated, identical for every entitled tenant, and cannot be changed.\n\n" +
			"Use this to read a template's identifier without hard-coding it, and to review the hostnames the " +
			"template brings with it.\n\n" +
			"Wire the identifier into a `jamfplatform_security_cloud_ztna_app`'s `predefined_app_id` to " +
			"manage an application based on the definition. Only one application per definition is allowed " +
			"on a tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"predefined_apps": schema.ListNestedAttribute{
				MarkdownDescription: "The predefined app templates available to this tenant, in the order Jamf " +
					"Security Cloud returns them.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Template identifier. This is the value a Zero Trust Network " +
								"Access app built from this template refers to.",
							Computed: true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the application the template covers, for example " +
								"`Slack`.",
							Computed: true,
						},
						"hostnames": schema.ListAttribute{
							MarkdownDescription: "The hostnames an app built from this template covers. " +
								"Adopting a template adopts all of them.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *PredefinedAppsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_predefined_apps")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the predefined app catalogue and populates Terraform state.
func (d *PredefinedAppsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PredefinedAppsDataSourceModel
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

	apps, err := d.client.ListZtnaPredefinedAppsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud predefined ZTNA apps", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(dataSourceID)
	data.PredefinedApps = make([]PredefinedAppResultModel, 0, len(apps.Results))
	for _, a := range apps.Results {
		hostnames, hostnameDiags := hostnameListValue(ctx, a.Hostnames)
		resp.Diagnostics.Append(hostnameDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.PredefinedApps = append(data.PredefinedApps, PredefinedAppResultModel{
			ID:        types.StringValue(a.ID),
			Name:      types.StringValue(a.Name),
			Hostnames: hostnames,
		})
	}

	tflog.Trace(ctx, "read Jamf Security Cloud predefined ZTNA apps data source", map[string]any{"count": len(data.PredefinedApps)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// hostnameListValue converts a template's hostnames into a list, normalising a nil
// slice to an empty list rather than a null one.
//
// types.ListValueFrom reflects a nil []string into a NULL list, and a `for`
// expression over a null list is a plan-time error — so a template carrying no
// hostnames would hand the operator an error instead of an empty result. This
// matches the sibling dns_hostname_mappings data source, which normalises the same
// way for the same reason.
//
// Every one of the 30 templates in the 2026-08-29 production EU probe carried
// hostnames, so a zero-hostname template may not exist today. This is consistency
// with the sibling and cheap insurance, not a fix for an observed failure.
func hostnameListValue(ctx context.Context, hostnames []string) (types.List, diag.Diagnostics) {
	if hostnames == nil {
		return types.ListValueFrom(ctx, types.StringType, []string{})
	}
	return types.ListValueFrom(ctx, types.StringType, hostnames)
}
