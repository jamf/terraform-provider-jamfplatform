// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ztna_app implements the jamfplatform_security_cloud_ztna_app resource,
// data sources and list resource backed by the Jamf Security Cloud ZTNA API.
//
// A ZTNA app is one row on the admin UI's **Access policy** page: an enterprise
// application, identified by the host names and address ranges its traffic matches,
// together with who may reach it, how their traffic is routed, and what the device
// must prove first. Jamf's own vocabulary shifts between the two words — the page is
// titled "Access policy" and its button says "Create policy", while the object it
// creates is an "application" with an "App name" and an "App type" — so the resource
// is named for the object and described as the policy.
//
// An application takes one of two forms, and the form is immutable:
//
//   - **Predefined** — `predefined_app_id` names one of the Jamf-maintained SaaS
//     definitions the jamfplatform_security_cloud_ztna_predefined_apps data source
//     lists. The definition owns the name and contributes its own host names, which
//     the admin UI shows under a "Default" heading and which never appear in this
//     resource's `hostnames`. A tenant may hold only one application per definition.
//   - **Custom** — no `predefined_app_id`, so `name` is required and every host name
//     is the operator's own. Jamf's API calls this an "Enterprise" application in
//     one error message; the admin UI calls it "Custom".
//
// Attribute names follow the admin UI rather than the wire wherever the two diverge,
// per STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI, and three
// enumerated values are translated to their UI labels — see mappings.go. Because the
// guide also forbids comments inside function bodies, the wire mapping lives here:
//
//	Terraform attribute                    Wire field
//	------------------------------------   -------------------------------------------
//	category                               categoryName
//	direct_ips_and_subnets                 bareIps
//	all_device_groups                      assignments.inclusions.allUsers
//	device_group_ids                       assignments.inclusions.groups
//	routing.traffic_routing                routing.type
//	routing.routing_mode                   routing.dnsIpResolutionType
//	routing_overrides                      groupOverrides.routingOverrides
//	routing_overrides[].device_group_ids   groupOverrides.routingOverrides[].groupIds
//	security.managed_device                security.deviceManagementBasedAccess
//	security.managed_device.enabled        security.deviceManagementBasedAccess.enabled
//	security.device_risk                   security.riskControls
//	security.device_risk.deny_at_risk_level security.riskControls.levelThreshold
//	security.jamf_trust                    security.dohIntegration
//	security.jamf_trust.enabled            security.dohIntegration.blocking
//	*.device_push_notifications            *.notificationsEnabled
//	app_type                               (none — derived from predefinedAppId)
//
// Two of those deserve their reasoning stated, because neither is guessable from the
// wire name. `security.jamf_trust` is `dohIntegration`: the admin UI's third Security
// card is "Access requires Jamf Trust to be enabled", and the mapping was confirmed
// on 2026-08-30 by writing an application with that field set and nothing else, then
// reading the card back off the UI. And `routing.routing_mode` is
// `dnsIpResolutionType` — the UI's "Routing mode" section, whose "Standard" and
// "Legacy" options are the IPv6 and IPv4 resolution modes respectively.
//
// `app_type` corresponds to no wire field at all. It is the admin UI's "App type"
// column, derived from whether `predefined_app_id` is set, and exists so a
// configuration can branch on the form without null-checking a UUID.
package ztna_app

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Server defaults for the security cards, reproduced as schema defaults.
//
// Each card is a whole wire sub-object with non-pointer members, so a card the
// configuration declares must supply every value: there is no way to send
// "enabled" without also sending a risk level. Defaults keep each leaf known at
// plan time, which is what makes a partially written card safe — without them an
// omitted `deny_at_risk_level` would be unknown at plan time and the payload would
// carry an empty string the enum refuses.
//
// The values are Jamf's own, observed on 2026-08-30 by creating an application with
// no `security` at all and reading back what the server stored. `deny_at_risk_level`
// defaults to the strictest level and persists even while the card is disabled.
const (
	defaultSecurityEnabled          = false
	defaultDevicePushNotifications  = true
	defaultDenyAtRiskLevelWireValue = securitycloud.RiskControlsLevelThresholdHigh
	minCollectionSize               = 1
	defaultCreateTimeout            = 60 * time.Second
	defaultReadTimeout              = 60 * time.Second
	defaultUpdateTimeout            = 60 * time.Second
	defaultDeleteTimeout            = 60 * time.Second
)

// ZtnaAppResource implements the Terraform resource for Jamf Security Cloud ZTNA
// access policy applications.
type ZtnaAppResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                     = &ZtnaAppResource{}
	_ resource.ResourceWithImportState      = &ZtnaAppResource{}
	_ resource.ResourceWithIdentity         = &ZtnaAppResource{}
	_ resource.ResourceWithConfigValidators = &ZtnaAppResource{}
)

// NewZtnaAppResource returns a new instance of ZtnaAppResource.
func NewZtnaAppResource() resource.Resource {
	return &ZtnaAppResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ZtnaAppResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_app"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ZtnaAppResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Application ID used to uniquely reference the access policy application.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the ZTNA app resource.
func (r *ZtnaAppResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Security Cloud access policy application, one entry on the " +
			"**Access policy** page. An application is defined by the host names and address ranges its " +
			"traffic matches; defining one is what lets Jamf Security Cloud apply access policy and reporting " +
			"to that traffic.\n\n" +
			"An application is either **predefined**, based on one of the Jamf-maintained definitions the " +
			"`jamfplatform_security_cloud_ztna_predefined_apps` data source lists, or **custom**, defined " +
			"entirely by you. Set `predefined_app_id` for the first and `name` for the second. The choice " +
			"cannot be changed afterwards, so Terraform replaces the application instead.\n\n" +
			"Host names and address ranges may belong to only one application across the whole tenant, so two " +
			"applications cannot claim the same one. Misconfiguring an application can cut end users off from " +
			"the resources it covers.\n\n" +
			"See the [Jamf Security Cloud guide](../guides/security-cloud) for what that tenant-wide uniqueness " +
			"means in practice: how overlapping host names are resolved, and why renaming an application by " +
			"replacing it fails. The guide also carries a worked example taking one internal application all " +
			"the way from a device group to a device that can reach it." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Application ID assigned by Jamf Security Cloud.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"App name\"** in the Jamf Security Cloud admin UI. Required for a " +
					"custom application, and not accepted for a predefined one, which takes its name from the " +
					"Jamf-maintained definition. Application names are not required " +
					"to be unique, so prefer the application ID when referencing one elsewhere.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"predefined_app_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Jamf-maintained application definition this application is " +
					"based on, from the `jamfplatform_security_cloud_ztna_predefined_apps` data source. " +
					"Setting it makes this a predefined application: the definition owns the name and " +
					"contributes its own host names, which the admin UI shows as \"Default\" and which do not " +
					"appear in `hostnames`. Additional host names can still be added. Only one application " +
					"per definition is allowed on a tenant. Changing this in either direction replaces the " +
					"application, because the choice of form is fixed once made.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"app_type": schema.StringAttribute{
				MarkdownDescription: "**\"App type\"** in the Jamf Security Cloud admin UI: " +
					markdownList(appTypeValues()) + ". Follows from whether `predefined_app_id` is set.",
				Computed: true,
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "**\"Category\"** in the Jamf Security Cloud admin UI: how this " +
					"application is classified for reporting. Must match a category Jamf Security Cloud " +
					"defines: use the `display_name` from the " +
					"`jamfplatform_security_cloud_content_categories` data source, not its `name`. " +
					"`Uncategorized` is the admin UI's own default.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"hostnames": schema.SetAttribute{
				MarkdownDescription: "**\"Hostname\"** entries under Traffic matching in the Jamf Security " +
					"Cloud admin UI: the host names whose traffic belongs to this application. A wildcard " +
					"may replace the whole leading label (`*.example.com`), and `*` on its own matches " +
					"everything. Entries must be mutually exclusive: `*.example.com` already covers " +
					"`sub.example.com`, and the parent domain has to be listed separately from its " +
					"subdomains. Host names must be lower-case with no trailing dot, because Jamf Security " +
					"Cloud stores them that way. A host name already claimed by another application is " +
					"rejected. For a predefined application these are additions to the definition's own " +
					"host names, not a replacement for them. Removing an entry stops Jamf Security Cloud " +
					"treating its traffic as part of this application; omitting the attribute clears the list.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(minCollectionSize),
					setvalidator.ValueStringsAre(
						commonvalidators.DNSHostnameOrWildcard(),
						normalisedHostname(),
					),
				},
			},
			"direct_ips_and_subnets": schema.SetAttribute{
				MarkdownDescription: "**\"Direct IPs and subnets\"** in the Jamf Security Cloud admin UI: " +
					"address ranges for applications that cannot be reached by host name. Use this only when " +
					"the application does not support connecting by the host names above; it also requires a " +
					"current version of Jamf Trust. Each entry is an IPv4 range in CIDR notation such as " +
					"`10.1.2.0/24`; a bare address and an IPv6 range are both rejected. A range must name its " +
					"own network address rather than a host inside it: write `10.1.2.0/24`, not " +
					"`10.1.2.3/24`, because Jamf Security Cloud stores only the network. A range already " +
					"claimed by another application is rejected. Removing an entry stops Jamf Security Cloud " +
					"treating its traffic as part of this application; omitting the attribute clears the list.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(minCollectionSize),
					setvalidator.ValueStringsAre(
						commonvalidators.IPv4CIDR(),
					),
				},
			},
			"all_device_groups": schema.BoolAttribute{
				MarkdownDescription: "**\"All device groups\"** under Device group permissions in the Jamf " +
					"Security Cloud admin UI. `true` lets users reach this application from any device in " +
					"the fleet. `false` restricts it to the groups in `device_group_ids`, which the admin UI " +
					"calls \"Selected device groups\".",
				Required: true,
			},
			"device_group_ids": schema.SetAttribute{
				MarkdownDescription: "IDs of the device groups that may reach this application, from the " +
					"`jamfplatform_security_cloud_device_group` resource or data source. Applies only when " +
					"`all_device_groups` is false. Leaving it unset with `all_device_groups` false is " +
					"accepted and means no device can reach the application.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(minCollectionSize),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"routing": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Application traffic routing\"** in the Jamf Security Cloud admin " +
					"UI: how authorised devices reach this application's servers.",
				Required:   true,
				Attributes: routingSchemaAttributes(),
			},
			"routing_overrides": schema.ListNestedAttribute{
				MarkdownDescription: "**\"Custom group assignments\"** in the Jamf Security Cloud admin UI: " +
					"per-group routing that overrides `routing` for the groups it names. A device group may " +
					"appear in only one override, and unless `all_device_groups` is true it must also be in " +
					"`device_group_ids`. Omitting the attribute clears the overrides.",
				Optional: true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(minCollectionSize),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"device_group_ids": schema.SetAttribute{
							MarkdownDescription: "IDs of the device groups this override applies to.",
							Required:            true,
							ElementType:         types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(minCollectionSize),
								setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
							},
						},
						"routing": schema.SingleNestedAttribute{
							MarkdownDescription: "Routing applied to the groups named above, in place of the " +
								"application's own.",
							Required:   true,
							Attributes: routingSchemaAttributes(),
						},
					},
				},
			},
			"security": schema.SingleNestedAttribute{
				MarkdownDescription: "The **Security** tab in the Jamf Security Cloud admin UI: what a " +
					"device must prove before it may reach this application. Each block corresponds to one " +
					"card on that tab. A block left out is one Jamf Security Cloud keeps its own setting " +
					"for; include a block to take control of it.\n\n" +
					"Unlike every other attribute here, *removing* a block you had previously applied stops " +
					"Terraform managing that requirement without turning it off; Jamf Security Cloud keeps " +
					"it as last applied. Set `enabled = false` to lift a requirement.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"managed_device": schema.SingleNestedAttribute{
						MarkdownDescription: "**\"Access requires device to be managed\"** in the Jamf Security " +
							"Cloud admin UI. Access is denied unless the device is enrolled in device " +
							"management. Omit the block to leave any existing values untouched (they are not " +
							"cleared on update).",
						Optional:   true,
						Attributes: securityControlSchemaAttributes(),
					},
					"device_risk": schema.SingleNestedAttribute{
						MarkdownDescription: "**\"Access requires device risk validation\"** in the Jamf " +
							"Security Cloud admin UI. Access is denied to devices at or above a risk level. " +
							"Omit the block to leave any existing values untouched (they are not cleared on " +
							"update).",
						Optional: true,
						Attributes: map[string]schema.Attribute{
							"enabled":                   securityEnabledAttribute(),
							"deny_at_risk_level":        denyAtRiskLevelAttribute(),
							"device_push_notifications": devicePushNotificationsAttribute(),
						},
					},
					"jamf_trust": schema.SingleNestedAttribute{
						MarkdownDescription: "**\"Access requires Jamf Trust to be enabled\"** in the Jamf " +
							"Security Cloud admin UI. Access is denied unless the device is protecting its " +
							"traffic through Jamf Trust. Omit the block to leave any existing values " +
							"untouched (they are not cleared on update).",
						Optional:   true,
						Attributes: securityControlSchemaAttributes(),
					},
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

// routingSchemaAttributes returns the attributes of a routing block. One builder
// serves the application's own routing and each per-group override, because the wire
// sends the same object in both positions and the admin UI shows the same three
// controls in both.
func routingSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"traffic_routing": schema.StringAttribute{
			MarkdownDescription: "**\"Application traffic routing\"** in the Jamf Security Cloud admin UI: " +
				markdownList(routingModeValues()) + ". \"" +
				routingModeLabels[securitycloud.RoutingTypeCustom] + "\" sends traffic through the access " +
				"gateway named in `gateway_id` and requires `routing_mode` alongside it. \"" +
				routingModeLabels[securitycloud.RoutingTypeDirect] + "\" leaves traffic to the device's own " +
				"routing and accepts neither. To reach private infrastructure over an IPsec tunnel, configure " +
				"the tunnel on the gateway rather than here.",
			Required: true,
			Validators: []validator.String{
				stringvalidator.OneOf(routingModeValues()...),
			},
		},
		"gateway_id": schema.StringAttribute{
			MarkdownDescription: "**\"Access gateway\"** in the Jamf Security Cloud admin UI: the ID of the " +
				"gateway this application's traffic is routed through. Accepts a Jamf-managed shared gateway " +
				"from the `jamfplatform_security_cloud_ztna_shared_gateways` data source (\"Nearest Data " +
				"Center\" or one of the shared IP pools), one of your own gateways, or a grouped gateway. " +
				"Required when routing via ZTNA and not accepted for direct routing.",
			Optional: true,
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
		"routing_mode": schema.StringAttribute{
			MarkdownDescription: "**\"Routing mode\"** in the Jamf Security Cloud admin UI: " +
				markdownList(dnsResolutionValues()) + ". \"Standard\" is recommended and resolves " +
				"addresses as IPv6; choose \"Legacy\", which is IPv4, only for devices or applications known " +
				"to be incompatible with IPv6. Required when routing via ZTNA and not accepted for direct " +
				"routing.",
			Optional: true,
			Validators: []validator.String{
				stringvalidator.OneOf(dnsResolutionValues()...),
			},
		},
	}
}

// securityControlSchemaAttributes returns the attributes of a security card that
// carries only a toggle and its notification setting.
func securityControlSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"enabled":                   securityEnabledAttribute(),
		"device_push_notifications": devicePushNotificationsAttribute(),
	}
}

// securityEnabledAttribute returns the toggle attribute shared by all three security
// cards.
func securityEnabledAttribute() schema.Attribute {
	return schema.BoolAttribute{
		MarkdownDescription: "Whether this requirement is enforced. Defaults to `false`, matching Jamf " +
			"Security Cloud's own default for a new application.",
		Optional: true,
		Computed: true,
		Default:  booldefault.StaticBool(defaultSecurityEnabled),
	}
}

// devicePushNotificationsAttribute returns the notification attribute shared by all
// three security cards.
func devicePushNotificationsAttribute() schema.Attribute {
	return schema.BoolAttribute{
		MarkdownDescription: "**\"Device push notifications\"** in the Jamf Security Cloud admin UI: " +
			"whether the user is told when access is denied by this requirement. Defaults to `true`, " +
			"matching Jamf Security Cloud's own default.",
		Optional: true,
		Computed: true,
		Default:  booldefault.StaticBool(defaultDevicePushNotifications),
	}
}

// denyAtRiskLevelAttribute returns the risk threshold attribute.
func denyAtRiskLevelAttribute() schema.Attribute {
	return schema.StringAttribute{
		MarkdownDescription: "**\"Deny access to devices starting at the following risk level\"** in the " +
			"Jamf Security Cloud admin UI: " + markdownList(riskLevelValues()) + ". Defaults to `" +
			labelFor(defaultDenyAtRiskLevelWireValue, riskLevelLabels) + "`, matching Jamf Security " +
			"Cloud's own default. Jamf Security Cloud keeps this value even while the requirement is not " +
			"enforced.",
		Optional: true,
		Computed: true,
		Default:  stringdefault.StaticString(labelFor(defaultDenyAtRiskLevelWireValue, riskLevelLabels)),
		Validators: []validator.String{
			stringvalidator.OneOf(riskLevelValues()...),
		},
	}
}

// ConfigValidators returns the cross-field rules the server either cannot report
// usefully or does not report at all. See validators.go for what each one exists for.
func (r *ZtnaAppResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		appFormValidator{},
		routingCombinationValidator{},
		deviceGroupAssignmentValidator{},
		hostnameOverlapValidator{},
	}
}

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *ZtnaAppResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_app")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Security Cloud ZTNA app ID.
func (r *ZtnaAppResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
