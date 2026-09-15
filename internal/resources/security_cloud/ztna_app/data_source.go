// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ZtnaAppDataSource implements the Terraform data source for a single Jamf Security
// Cloud ZTNA access policy application.
type ZtnaAppDataSource struct {
	client *securitycloud.Client
}

var (
	_ datasource.DataSource                     = &ZtnaAppDataSource{}
	_ datasource.DataSourceWithConfigValidators = &ZtnaAppDataSource{}
)

// NewZtnaAppDataSource returns a new instance of ZtnaAppDataSource.
func NewZtnaAppDataSource() datasource.DataSource {
	return &ZtnaAppDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ZtnaAppDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_app"
}

// Schema returns the data source schema.
func (d *ZtnaAppDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Security Cloud access policy application by ID, by name, or by " +
			"the Jamf-maintained definition it is based on.\n\n" +
			"Which key to use follows from the application's form. A **custom** application has a name, but " +
			"names are not required to be unique, so a name matching more than one application is an error. " +
			"A **predefined** application has no name of its own at all (Jamf Security Cloud reports it as " +
			"null), so look one up by `predefined_app_id`, of which a tenant may hold only one." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Application ID to look up. Exactly one of `id`, `name` or " +
					"`predefined_app_id` must be set.",
				Optional: true,
				Computed: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Application name to look up. Custom applications only; a predefined " +
					"application has no name. Exactly one of `id`, `name` or `predefined_app_id` must be set.",
				Optional: true,
				Computed: true,
			},
			"predefined_app_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Jamf-maintained application definition to look up the " +
					"application for. Exactly one of `id`, `name` or `predefined_app_id` must be set.",
				Optional: true,
				Computed: true,
			},
			"app_type": schema.StringAttribute{
				MarkdownDescription: "Whether the application is predefined or custom: " +
					markdownList(appTypeValues()) + ". Follows from whether `predefined_app_id` is set.",
				Computed: true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Category the application is classified under.",
				Computed:            true,
			},
			"hostnames": schema.ListAttribute{
				MarkdownDescription: "Host names whose traffic belongs to this application. For a predefined " +
					"application these are the additions to the definition's own host names, which are not " +
					"reported here.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"direct_ips_and_subnets": schema.ListAttribute{
				MarkdownDescription: "Address ranges whose traffic belongs to this application.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"all_device_groups": schema.BoolAttribute{
				MarkdownDescription: "Whether every device group may reach this application.",
				Computed:            true,
			},
			"device_group_ids": schema.ListAttribute{
				MarkdownDescription: "IDs of the device groups that may reach this application.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"routing": schema.SingleNestedAttribute{
				MarkdownDescription: "How authorised devices reach this application's servers.",
				Computed:            true,
				Attributes:          dsRoutingSchemaAttributes(),
			},
			"routing_overrides": schema.ListNestedAttribute{
				MarkdownDescription: "Per-group routing that overrides the application's own.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"device_group_ids": schema.ListAttribute{
							MarkdownDescription: "IDs of the device groups this override applies to.",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"routing": schema.SingleNestedAttribute{
							MarkdownDescription: "Routing applied to the groups named above.",
							Computed:            true,
							Attributes:          dsRoutingSchemaAttributes(),
						},
					},
				},
			},
			"security": schema.SingleNestedAttribute{
				MarkdownDescription: "What a device must prove before it may reach this application. Unlike " +
					"the resource, all three requirements are always reported, because Jamf Security Cloud " +
					"always holds a setting for each.",
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"managed_device": schema.SingleNestedAttribute{
						MarkdownDescription: "Whether access requires the device to be managed.",
						Computed:            true,
						Attributes:          dsSecurityControlSchemaAttributes(),
					},
					"device_risk": schema.SingleNestedAttribute{
						MarkdownDescription: "Whether access requires device risk validation.",
						Computed:            true,
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								MarkdownDescription: "Whether the requirement is enforced.",
								Computed:            true,
							},
							"deny_at_risk_level": schema.StringAttribute{
								MarkdownDescription: "Risk level at which access is denied, lowest first: " +
									markdownList(riskLevelValues()) + ". Jamf Security Cloud keeps this value even " +
									"while the requirement is not enforced.",
								Computed: true,
							},
							"device_push_notifications": schema.BoolAttribute{
								MarkdownDescription: "Whether the user is told when access is denied.",
								Computed:            true,
							},
						},
					},
					"jamf_trust": schema.SingleNestedAttribute{
						MarkdownDescription: "Whether access requires Jamf Trust to be enabled.",
						Computed:            true,
						Attributes:          dsSecurityControlSchemaAttributes(),
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// dsRoutingSchemaAttributes returns the read-only attributes of a routing block.
//
// The documented value lists are generated from the same `*Values()` helpers the
// resource's OneOf validators use. These spellings are the provider's own admin-UI
// labels rather than the wire values a reader could look up in Jamf's API reference,
// and nothing validates a read-path comparison, so an operator guessing at them gets
// an empty result rather than an error.
func dsRoutingSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"traffic_routing": schema.StringAttribute{
			MarkdownDescription: "Whether traffic is routed via ZTNA or left to the device: " +
				markdownList(routingModeValues()) + ". \"" +
				routingModeLabels[securitycloud.RoutingTypeCustom] + "\" routes through the access gateway " +
				"reported in `gateway_id`; \"" + routingModeLabels[securitycloud.RoutingTypeDirect] +
				"\" leaves traffic to the device's own routing and reports neither `gateway_id` nor " +
				"`routing_mode`.",
			Computed: true,
		},
		"gateway_id": schema.StringAttribute{
			MarkdownDescription: "ID of the access gateway traffic is routed through. Null for direct " +
				"device routing.",
			Computed: true,
		},
		"routing_mode": schema.StringAttribute{
			MarkdownDescription: "How ZTNA resolves this application's addresses: " +
				markdownList(dnsResolutionValues()) + ". \"" +
				dnsResolutionLabels[securitycloud.RoutingDnsIpResolutionTypeIPv6] + "\" is IPv6 and \"" +
				dnsResolutionLabels[securitycloud.RoutingDnsIpResolutionTypeIPv4] + "\" is IPv4. Null for " +
				"direct device routing.",
			Computed: true,
		},
	}
}

// dsSecurityControlSchemaAttributes returns the read-only attributes of a security
// card that carries only a toggle and its notification setting.
func dsSecurityControlSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enabled": schema.BoolAttribute{
			MarkdownDescription: "Whether the requirement is enforced.",
			Computed:            true,
		},
		"device_push_notifications": schema.BoolAttribute{
			MarkdownDescription: "Whether the user is told when access is denied.",
			Computed:            true,
		},
	}
}

// ConfigValidators enforces that exactly one lookup key is supplied.
func (d *ZtnaAppDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
			path.MatchRoot("predefined_app_id"),
		),
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the shared
// providerdata.ConfigureSecurityCloud helper.
func (d *ZtnaAppDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an application by ID, name or predefined app ID and populates
// Terraform state.
//
// The default time budget follows the request shape rather than the data source. A
// lookup by `id` is a single GET, so it gets defaultReadTimeout; a lookup by `name`
// or `predefined_app_id` has no endpoint of its own and walks the paged application
// list at a hundred per request, so it gets the same defaultPagedReadTimeout the
// plural data source uses for the identical call. An explicit `timeouts.read`
// overrides both.
func (d *ZtnaAppDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ZtnaAppDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lookupTimeout := defaultReadTimeout
	if data.ID.IsNull() {
		lookupTimeout = defaultPagedReadTimeout
	}
	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), lookupTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	for _, key := range []struct {
		name  string
		value types.String
	}{
		{"id", data.ID},
		{"name", data.Name},
		{"predefined_app_id", data.PredefinedAppID},
	} {
		if !key.value.IsNull() && key.value.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root(key.name),
				"Lookup key is empty",
				"`"+key.name+"` is set to an empty string, which ExactlyOneOf still counts as configured, so "+
					"another key cannot be used instead. This usually means a variable or a reference "+
					"resolved to \"\" — give it a value, or remove it and set a different key.",
			)
			return
		}
	}

	app, found := d.lookup(readCtx, data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() || !found {
		return
	}

	resp.Diagnostics.Append(assignAppDataSourceModel(ctx, &data, app)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Security Cloud access policy application data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// lookup resolves the configured key to one application.
//
// The name and predefined-app-ID paths cost ceil(N/100) sequential GETs, because
// ListZtnaAppsV1 pages internally at a hundred applications per request and the whole
// collection is scanned client-side.
//
// Those paths list every application rather than going through the SDK's
// ResolveZtnaAppV1ByName, for two reasons. Application names are not unique on this
// endpoint (wire-verified 2026-08-30), so an ambiguous match has
// to be reported rather than silently resolved to whichever the server listed first;
// and a predefined application has a null name, so the name field cannot address one
// at all.
func (d *ZtnaAppDataSource) lookup(ctx context.Context, data ZtnaAppDataSourceModel, diags *diag.Diagnostics) (*securitycloud.App, bool) {
	if !data.ID.IsNull() {
		app, err := d.client.GetZtnaAppV1(ctx, data.ID.ValueString())
		if err != nil {
			if helpers.IsNotFoundError(err) {
				diags.AddAttributeError(
					path.Root("id"),
					"Unable to find Jamf Security Cloud access policy application",
					"No access policy application on this tenant has the ID \""+data.ID.ValueString()+"\".",
				)
				return nil, false
			}
			diags.AddError("Unable to find Jamf Security Cloud access policy application", helpers.APIErrorDetail(err))
			return nil, false
		}
		return app, true
	}

	apps, err := d.client.ListZtnaAppsV1(ctx)
	if err != nil {
		diags.AddError("Unable to list Jamf Security Cloud access policy applications", helpers.APIErrorDetail(err))
		return nil, false
	}

	if !data.PredefinedAppID.IsNull() {
		wanted := data.PredefinedAppID.ValueString()
		for i := range apps {
			if apps[i].PredefinedAppID != nil && *apps[i].PredefinedAppID == wanted {
				return &apps[i], true
			}
		}
		diags.AddAttributeError(
			path.Root("predefined_app_id"),
			"Unable to find Jamf Security Cloud access policy application",
			"No access policy application on this tenant is based on the definition \""+wanted+"\". Use the "+
				"`jamfplatform_security_cloud_ztna_predefined_apps` data source to check the ID, and the "+
				"`jamfplatform_security_cloud_ztna_apps` data source to list what exists.",
		)
		return nil, false
	}

	wanted := data.Name.ValueString()
	var matches []int
	for i := range apps {
		if apps[i].Name != nil && *apps[i].Name == wanted {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 1:
		return &apps[matches[0]], true
	case 0:
		diags.AddAttributeError(
			path.Root("name"),
			"Unable to find Jamf Security Cloud access policy application",
			"No access policy application on this tenant is named \""+wanted+"\". Names are matched exactly, "+
				"so one differing only in capitalisation or surrounding whitespace will not be found, and a "+
				"predefined application has no name at all — look one of those up by `predefined_app_id`. "+
				"Use the `jamfplatform_security_cloud_ztna_apps` data source to list what exists.",
		)
		return nil, false
	default:
		ids := make([]string, 0, len(matches))
		for _, i := range matches {
			ids = append(ids, apps[i].ID)
		}
		sort.Strings(ids)
		diags.AddError(
			"Multiple Jamf Security Cloud access policy applications share that name",
			"Jamf Security Cloud does not require application names to be unique, and "+wanted+" names more "+
				"than one. Look the application up by `id` instead. Matching application IDs: "+
				strings.Join(ids, ", "),
		)
		return nil, false
	}
}
