// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	ssodomainactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/account/sso_domain"
	deviceactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/device"
	appinstalleractions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/pro/app_installers"
	jamfprotectactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/pro/jamf_protect"
	maintenanceactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/pro/maintenance"
	msuactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/pro/managed_software_updates"
	mdmactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/pro/mdm"
	patchactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/pro/patch"
	uemconnectactions "github.com/jamf/terraform-provider-jamfplatform/internal/actions/security_cloud/uem_connect"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	mcxforcedpayload "github.com/jamf/terraform-provider-jamfplatform/internal/functions/mcx_forced_payload"
	"github.com/jamf/terraform-provider-jamfplatform/internal/functions/mobileconfig"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
	ssoconnection "github.com/jamf/terraform-provider-jamfplatform/internal/resources/account/sso_connection"
	ssodomain "github.com/jamf/terraform-provider-jamfplatform/internal/resources/account/sso_domain"
	aigovernancepolicy "github.com/jamf/terraform-provider-jamfplatform/internal/resources/ai_governance/policy"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/ai_governance/tool"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/component"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/components"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/cbengine/baselines"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/cbengine/benchmark"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/cbengine/benchmarks"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/cbengine/rules"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/device"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/device_group"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/device_groups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/devices"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/access_management_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/account"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/account_group"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/account_privileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/activation_code"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/advanced_computer_search"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/advanced_mobile_device_search"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/advanced_user_search"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/advanced_volume_purchasing_content_search"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/allowed_file_extension"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/app_installer"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/app_installer_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/app_installer_title"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/app_request_form_field"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/app_request_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/app_store_country_codes"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/automated_device_enrollment"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/automated_device_enrollment_public_key"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/building"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/category"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/class"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/cloud_distribution_point"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/cloud_identity_provider"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/computer_check_in_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/computer_extension_attribute"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/computer_inventory_collection_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/computer_invitation"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/computer_prestage_enrollment"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/department"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/directory_binding"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/disk_encryption_configuration"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/dock_item"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/ebook"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/enrollment_customization"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/file_share_distribution_point"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/gsx_connection"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/ibeacon"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/icon"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/impact_alert_notification_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/inventory_preload_record"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/jamf_connect"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/jamf_parent_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/jamf_pro_server_url"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/jamf_protect"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/jamf_teacher_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/ldap_server"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/licensed_software"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/local_admin_password_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/location"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/login_page"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mac_app_store_app"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/macos_configuration_profile"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/macos_onboarding"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/managed_software_updates"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mdm_profile_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_app"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_configuration_profile"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_enrollment_profile"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_extension_attribute"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_invitation"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_prestage_enrollment"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/mobile_device_provisioning_profile"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/network_segment"
	pkg "github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/package"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/patch_external_source"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/patch_internal_source"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/patch_policy"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/patch_software_title"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/pki_adcs"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/pki_certificate_authority"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/pki_digicert"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/pki_json_web_token_configuration"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/pki_venafi"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/policy"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/printer"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/re_enrollment_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/removable_mac_address"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/restricted_software"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/return_to_service"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/script"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/self_service_branding_image"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/self_service_branding_ios"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/self_service_branding_macos"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/self_service_macos_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/self_service_plus_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/service_discovery_enrollment"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/site"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/smtp_server"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/sso_failover_url"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/sso_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/supervision_identity"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/tenant_id"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/user"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/user_extension_attribute"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/user_group"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/user_initiated_enrollment_settings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/volume_purchasing_notification"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/vpp_assignment"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/vpp_invitation"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/webhook"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/activation_profile"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/content_categories"
	securityclouddevicegroup "github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/device_group"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/dns_hostname_mappings"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/dns_search_domain"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/dns_zone"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/uem_connect"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/ztna_app"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/ztna_gateway"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/ztna_grouped_gateway"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/ztna_predefined_apps"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/security_cloud/ztna_shared_gateways"
)

// Constants for environment variable names.
const (
	envBaseURL              = "JAMFPLATFORM_BASE_URL"
	envClientID             = "JAMFPLATFORM_CLIENT_ID"
	envClientSecret         = "JAMFPLATFORM_CLIENT_SECRET"
	envEnvironmentID        = "JAMFPLATFORM_ENVIRONMENT_ID"
	envTenantID             = "JAMFPLATFORM_TENANT_ID"
	envMinRequestIntervalMs = "JAMFPLATFORM_MIN_REQUEST_INTERVAL_MS"
	envImpactAlerts         = "JAMFPLATFORM_IMPACT_ALERTS"

	envCustomHeaders           = "JAMFPLATFORM_CUSTOM_HEADERS"
	envAuthorizationHeaderName = "JAMFPLATFORM_AUTHORIZATION_HEADER_NAME"
)

// Ensure JamfPlatformProvider satisfies the various provider interfaces.
var _ provider.Provider = &JamfPlatformProvider{}
var _ provider.ProviderWithListResources = &JamfPlatformProvider{}
var _ provider.ProviderWithActions = &JamfPlatformProvider{}

// JamfPlatformProvider defines the provider implementation.
type JamfPlatformProvider struct {
	version string
}

// JamfPlatformProviderModel describes the provider data model.
type JamfPlatformProviderModel struct {
	BaseURL              types.String `tfsdk:"base_url"`
	ClientID             types.String `tfsdk:"client_id"`
	ClientSecret         types.String `tfsdk:"client_secret"`
	EnvironmentID        types.String `tfsdk:"environment_id"`
	TenantID             types.String `tfsdk:"tenant_id"`
	MinRequestIntervalMs types.Int64  `tfsdk:"min_request_interval_ms"`
	ImpactAlerts         types.Bool   `tfsdk:"impact_alerts"`

	CustomHeaders           types.Map    `tfsdk:"custom_headers"`
	AuthorizationHeaderName types.String `tfsdk:"authorization_header_name"`
}

func (p *JamfPlatformProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "jamfplatform"
	resp.Version = p.version
}

func (p *JamfPlatformProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf(
			"> **⚠️ This provider now publishes as `jamf/jamfplatform`.** Earlier releases carried `jamf-concepts/jamfplatform`, and Terraform records the namespace in state, so upgrading from one of them needs `terraform state replace-provider jamf-concepts/jamfplatform jamf/jamfplatform` in every workspace and state file before `terraform init -upgrade` will succeed. See [Moving to the jamf namespace](guides/namespace-migration).\n\n"+
				"Provider for [Jamf Platform API Services](https://developer.jamf.com/platform-api/reference/getting-started-with-platform-api). "+
				"Configure `base_url` and credentials via the provider block, environment variables, or Terraform variables.\n\n"+
				"**📘 New here? Start with the getting-started guide:** [Managing the Jamf Platform with Terraform: the Jamf Platform provider](https://concepts.jamf.com/en/guides/infrastructure-as-code/managing-the-jamf-platform-with-terraform-the-jamf-platform-provider/) on Jamf Concepts walks through installing Terraform, creating API credentials, configuring the provider, writing your first device groups, compliance benchmarks and blueprints, applying a configuration, and bringing an existing tenant under management.\n\n"+
				"> **⚠️ Upgrading from any pre-GA version — `v0.28.1` or earlier, or a `v0.29.0` release candidate?** The Jamf Platform API has reached general availability, and this release targets the GA gateway. Beta API integration credentials are revoked whichever pre-GA version you are coming from, so register a replacement integration in Jamf Account and replace `tenant_id` with `environment_id`. On `v0.28.1` or earlier, set `base_url` to `https://{region}.api.jamfcloud.com` in the same change as the provider upgrade; a release candidate already points there. Earlier versions reach only the beta host, which no longer serves the API, and several constructs were removed along with the endpoints they called. See [Upgrading to the Platform API GA](guides/platform-api-ga).\n\n"+
				"**Supported Jamf products and tenant version targets**\n\n"+
				"| Product | Resource namespace | Built against API as of |\n"+
				"|---------|--------------------|--------------------------|\n"+
				"| Jamf Pro | `jamfplatform_pro_*` | %s |\n"+
				"| Jamf Platform Services (Blueprints, Device Groups, Devices, Device Actions, Compliance Benchmarks) | no product prefix, such as `jamfplatform_blueprints_blueprint` | continuously deployed |\n"+
				"| Jamf Security Cloud (Custom DNS, ZTNA, UEM Connect, device groups, activation profiles) | `jamfplatform_security_cloud_*` | continuously deployed |\n"+
				"| Jamf AI Governance | `jamfplatform_ai_governance_*` | continuously deployed |\n"+
				"| Jamf Account (single sign-on) | `jamfplatform_account_*` | continuously deployed |\n\n"+
				"Jamf Pro is the only product versioned at the tenant. A tenant below the listed version raises an advisory warning at apply time, and a resource that needs a newer endpoint declares its own floor and fails when it is configured rather than mid-apply. The rest target continuously-deployed platform services, and no version is fetched for a configuration that uses only those.\n\n"+
				"Which namespaces an API integration reaches depends on the scope it was created with: Blueprints, Compliance Benchmarks and Jamf AI Governance need a platform environment, and `jamfplatform_account_*` needs organization management and is the only family that scope reaches. Every resource and data source reports the scope it needs when it is configured. See `environment_id` below.\n\n"+
				"This provider builds on the work of [Deployment Theory](https://github.com/deploymenttheory)'s [terraform-provider-jamfpro](https://github.com/deploymenttheory/terraform-provider-jamfpro) — first released in early 2024, the most widely adopted community Terraform provider for Jamf.",
			providerdata.ProviderMinJamfProVersion,
		),
		Attributes: map[string]schema.Attribute{
			"base_url": schema.StringAttribute{
				Optional:    true,
				Description: "Required. The regional Jamf Platform gateway root: `https://us.api.jamfcloud.com`, `https://eu.api.jamfcloud.com` or `https://apac.api.jamfcloud.com`. Give the host alone. The gateway serves the token endpoint and every API namespace at the root, so a path such as `/api` makes authentication fail. A `base_url` naming the pre-GA `{region}.apigw.jamf.com` beta gateway is refused at configure time, because that host still accepts credentials and then serves no API namespace. Must be set either here or via the JAMFPLATFORM_BASE_URL environment variable. Marked Optional in the schema so it can be sourced from the environment; the provider errors at configure time if it is set in neither place.",
			},
			"client_id": schema.StringAttribute{
				Optional:    true,
				Description: "Required. OAuth client ID for Jamf Platform API. Must be set either here or via the JAMFPLATFORM_CLIENT_ID environment variable. Marked Optional in the schema so it can be sourced from the environment; the provider errors at configure time if it is set in neither place.",
			},
			"client_secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Required. OAuth client secret for Jamf Platform API. Must be set either here or via the JAMFPLATFORM_CLIENT_SECRET environment variable. Marked Optional in the schema so it can be sourced from the environment; the provider errors at configure time if it is set in neither place.",
			},
			"environment_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "**Preferred.** ID of the **\"Platform environment\"** your API integration " +
					"targets, a group of tenants across product types with interconnected capabilities. Can also " +
					"be set via the `JAMFPLATFORM_ENVIRONMENT_ID` environment variable. This is the scope new " +
					"integrations should be created with, and the only one that reaches Blueprints, Compliance " +
					"Benchmarks and Jamf AI Governance; the provider refuses a tenant-scoped integration for " +
					"those when you configure it. Mutually exclusive with the legacy `tenant_id`. An API " +
					"integration targets " +
					"one or the other, so setting both is an error, and supplying the ID that does not match the " +
					"integration is refused with `403 OWNERSHIP_FORBIDDEN` even when both IDs belong to the same " +
					"customer. Pick the one your integration was created for rather than treating them as two " +
					"spellings of the same thing. Setting neither leaves the provider organization-scoped, " +
					"matching an **\"Organization management\"** integration: the scope the " +
					"`jamfplatform_account_*` resources require, and the only one that reaches them. Each " +
					"resource and data source reports the scope it needs when it is configured, so a mismatch is " +
					"named at plan time rather than failing mid-apply.",
			},
			"tenant_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "**Legacy.** Prefer `environment_id`. UUID of the single **\"Tenant\"** your API integration targets: one Jamf Pro, Jamf School, Jamf Protect or Jamf Security Cloud tenant. " +
					"Can also be set via the `JAMFPLATFORM_TENANT_ID` environment variable. " +
					"Tenant scope is the legacy method for targeting integrations without a platform environment. " +
					"It stays supported, and a few surfaces are still only reachable this way, but new configurations should use `environment_id`. " +
					"This scope does not reach Blueprints, Compliance Benchmarks or Jamf AI Governance, so set `environment_id` if your configuration uses any of them. " +
					"Mutually exclusive with `environment_id`; see that attribute for what happens when the ID and the integration disagree.",
			},
			"min_request_interval_ms": schema.Int64Attribute{
				Optional:    true,
				Description: "Minimum elapsed time, in milliseconds, between the start of consecutive outbound API requests, pacing all traffic through the shared client. Defaults to 0 (no pacing). Because it gates request starts rather than limiting concurrency, it is a throughput ceiling on the whole provider: at 100ms no plan exceeds 10 requests per second however many operations Terraform runs in parallel. Raise it to give a heavily loaded server breathing room, at the cost of slowing every plan and apply. Can also be set via the JAMFPLATFORM_MIN_REQUEST_INTERVAL_MS environment variable.",
			},
			"custom_headers": schema.MapAttribute{
				Optional:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				MarkdownDescription: "Extra HTTP headers to send on every request the provider makes, including " +
					"the token request it uses to authenticate. Keyed by header name; header names are " +
					"case-insensitive, so `x-proxy-route` and `X-Proxy-Route` are the same header and only one " +
					"of the two may appear. For deployments whose traffic reaches Jamf through a reverse proxy " +
					"that authenticates callers itself, or routes on a tag the provider knows nothing about. " +
					"Marked sensitive in full, because these headers commonly carry a credential and Terraform " +
					"cannot redact one entry of a map. Can also be set via the `JAMFPLATFORM_CUSTOM_HEADERS` " +
					"environment variable, as one `Name: value` pair per line, which a CI runner can inject " +
					"without a Terraform variable. When this attribute is set the variable is not read at all. " +
					"Five names are refused, all because a supplied header replaces rather than adds to what the " +
					"provider sends. `X-Environment-Id` and `X-Tenant-Id` are set by the provider from " +
					"`environment_id` and `tenant_id`, and the gateway answers an overridden scope with the same " +
					"error a wrong credential gets. `Cookie` would displace the session cookie Jamf Cloud uses " +
					"to keep this client on one application node. A proxy that needs a cookie of its own can set " +
					"one on a response, which the provider stores and sends back on its own. `Content-Type` is " +
					"chosen per request: JSON, merge-patch JSON, XML on some Jamf Pro requests, or multipart " +
					"with a generated boundary for a file upload. One value here overrides all four and leaves " +
					"an upload without its boundary. `Accept` is `application/xml` on those same Jamf Pro " +
					"requests, and setting it elsewhere is no safer: Jamf Security Cloud's UEM Connect service " +
					"answers `Accept: application/xml` with an XML body the provider would decode as JSON. " +
					"Supplying `Authorization` is supported and is the point of the feature. Pair it with " +
					"`authorization_header_name` so the proxy's credential and the Jamf credential both reach " +
					"the request. See the [Reverse proxy guide](../guides/reverse-proxy) for the whole " +
					"arrangement.",
			},
			"authorization_header_name": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Send the Jamf credential in this header instead of `Authorization`. For a " +
					"reverse proxy that consumes `Authorization` for its own service account and expects the " +
					"Jamf credential under a name it chose. By default it is unset and the credential stays in " +
					"`Authorization`, which is what talking to Jamf directly needs. Can also be set via the " +
					"`JAMFPLATFORM_AUTHORIZATION_HEADER_NAME` environment variable. Only the credential the " +
					"provider sends on API requests moves; the credentials that obtain it are unaffected, so " +
					"authentication keeps working. `Authorization`, `X-Environment-Id`, `X-Tenant-Id`, `Accept`, " +
					"`Content-Type`, and any name also present in `custom_headers` are refused. Each leaves the " +
					"request with no usable credential (or, for the two media-type headers, one the gateway " +
					"cannot parse) and fails in terms that name something else as the cause. See the [Reverse " +
					"proxy guide](../guides/reverse-proxy).",
			},
			"impact_alerts": schema.BoolAttribute{
				Optional: true,
				MarkdownDescription: "Show **impact alerts** during `terraform plan`: an advisory warning on each deployable or scopeable object this plan changes, reporting how many computers or mobile devices the change affects. It is the same signal Jamf Pro shows on Save. " +
					"Off by default; alerts never block a plan. " +
					"Can also be set via the JAMFPLATFORM_IMPACT_ALERTS environment variable, which accepts the values of Go's `strconv.ParseBool` (`true`, `false`, `1`, `0`, …). An unparseable value leaves alerts off and raises a configure-time warning. This attribute, when set, always takes precedence over the variable. " +
					"This does not change any Jamf Pro setting; to configure the alerts Jamf Pro shows in its own web interface, use `jamfplatform_pro_impact_alert_notification_settings`. " +
					"See the [Impact alerts guide](../guides/impact-alerts) for how to read the figures, how to consume them in CI, and what they cost.",
			},
		},
	}
}

func (p *JamfPlatformProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data JamfPlatformProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := data.BaseURL.ValueString()
	if baseURL == "" {
		baseURL = getenv(envBaseURL)
	}
	if baseURL == "" {
		resp.Diagnostics.AddError(
			"Missing Required Provider Configuration",
			"base_url must be set either in the provider block or via the JAMFPLATFORM_BASE_URL environment variable.",
		)
		return
	}
	if summary, detail := betaGatewayError(baseURL); summary != "" {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if summary, detail := baseURLPathWarning(baseURL); summary != "" {
		resp.Diagnostics.AddWarning(summary, detail)
	}

	clientID := data.ClientID.ValueString()
	if clientID == "" {
		clientID = getenv(envClientID)
	}
	if clientID == "" {
		resp.Diagnostics.AddError(
			"Missing Required Provider Configuration",
			"client_id must be set either in the provider block or via the JAMFPLATFORM_CLIENT_ID environment variable.",
		)
		return
	}

	clientSecret := data.ClientSecret.ValueString()
	if clientSecret == "" {
		clientSecret = getenv(envClientSecret)
	}
	if clientSecret == "" {
		resp.Diagnostics.AddError(
			"Missing Required Provider Configuration",
			"client_secret must be set either in the provider block or via the JAMFPLATFORM_CLIENT_SECRET environment variable.",
		)
		return
	}

	scopeKind, scopeID, scopeDiags := resolveScope(data.EnvironmentID, data.TenantID)
	resp.Diagnostics.Append(scopeDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := []jamfplatform.Option{
		jamfplatform.WithUserAgent("terraform-provider-jamfplatform/" + p.version),
	}
	switch scopeKind {
	case providerdata.ScopeEnvironment:
		opts = append(opts, jamfplatform.WithEnvironmentID(scopeID))
	case providerdata.ScopeTenant:
		opts = append(opts, jamfplatform.WithTenantID(scopeID))
	}
	// Inter-request pacing, defaulting to none — see resolveMinRequestInterval.
	//
	// Eventual-consistency retries are NOT done by the transport — resources that
	// need them poll explicitly (see the apps and device_group Delete paths).
	minInterval, err := resolveMinRequestInterval(data.MinRequestIntervalMs)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid JAMFPLATFORM_MIN_REQUEST_INTERVAL_MS",
			fmt.Sprintf("Expected an integer number of milliseconds, got %q: %s", getenv(envMinRequestIntervalMs), helpers.APIErrorDetail(err)),
		)
		return
	}
	opts = append(opts, jamfplatform.WithMinRequestInterval(minInterval))
	customHeaders, headerDiags := resolveCustomHeaders(data.CustomHeaders)
	resp.Diagnostics.Append(headerDiags...)
	authHeaderName, authHeaderDiags := resolveAuthorizationHeaderName(data.AuthorizationHeaderName, customHeaders)
	resp.Diagnostics.Append(authHeaderDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(customHeaders) > 0 {
		opts = append(opts, jamfplatform.WithHeaders(customHeaders))
	}
	if authHeaderName != "" {
		opts = append(opts, jamfplatform.WithAuthorizationHeaderName(authHeaderName))
	}
	if shouldEnableHTTPLogging() {
		opts = append(opts, jamfplatform.WithLogger(NewTerraformLogger()))
	}
	apiClient := jamfplatform.NewClient(baseURL, clientID, clientSecret, opts...)

	if err := apiClient.ValidateCredentials(ctx); err != nil {
		summary, detail := authFailureDiagnostic(baseURL, err)
		resp.Diagnostics.AddError(summary, detail)
		return
	}

	pd := providerdata.New(apiClient)

	tflog.Info(ctx, "Jamf Platform provider configured", map[string]any{
		"provider_version":     p.version,
		"jamf_pro_api_version": jamfplatform.JamfProAPIVersion,
		"provider_pro_floor":   providerdata.ProviderMinJamfProVersion,
		"integration_scope":    pd.Scope().String(),
		"custom_headers":       sortedHeaderNames(customHeaders),
		"credential_header":    credentialHeaderName(authHeaderName),
	})
	impactEnabled, impactWarn := impactAlertsEnabled(data.ImpactAlerts)
	if impactWarn != "" {
		// Advisory by design: a typo in the env value must not fail the plan,
		// but the operator has to be told why no alerts appeared.
		resp.Diagnostics.AddWarning("Invalid JAMFPLATFORM_IMPACT_ALERTS", impactWarn)
	}
	if impactEnabled {
		pd.EnableImpactAlerts()
	}
	resp.DataSourceData = pd
	resp.ResourceData = pd
	resp.ListResourceData = pd
	resp.ActionData = pd
}

// Functions registers the provider-defined functions exposed under the
// jamfplatform:: namespace.
func (p *JamfPlatformProvider) Functions(_ context.Context) []func() function.Function {
	return []func() function.Function{
		mcxforcedpayload.NewFunction,
		mobileconfig.NewFunction,
	}
}

func (p *JamfPlatformProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		account.NewAccountResource,
		account_group.NewAccountGroupResource,
		automated_device_enrollment.NewAutomatedDeviceEnrollmentResource,
		benchmark.NewBenchmarkResource,
		blueprint.NewBlueprintResource,
		allowed_file_extension.NewAllowedFileExtensionResource,
		building.NewBuildingResource,
		category.NewCategoryResource,
		computer_extension_attribute.NewComputerExtensionAttributeResource,
		mobile_device_extension_attribute.NewMobileDeviceExtensionAttributeResource,
		user_extension_attribute.NewUserExtensionAttributeResource,
		cloud_distribution_point.NewCloudDistributionPointResource,
		file_share_distribution_point.NewFileShareDistributionPointResource,
		cloud_identity_provider.NewCloudIdentityProviderResource,
		department.NewDepartmentResource,
		device_group.NewDeviceGroupResource,
		directory_binding.NewDirectoryBindingResource,
		ldap_server.NewLdapServerResource,
		local_admin_password_settings.NewLocalAdminPasswordSettingsResource,
		disk_encryption_configuration.NewDiskEncryptionConfigurationResource,
		computer_invitation.NewComputerInvitationResource,
		mobile_device_invitation.NewMobileDeviceInvitationResource,
		computer_prestage_enrollment.NewComputerPrestageEnrollmentResource,
		mobile_device_prestage_enrollment.NewMobileDevicePrestageEnrollmentResource,
		return_to_service.NewReturnToServiceResource,
		dock_item.NewDockItemResource,
		enrollment_customization.NewEnrollmentCustomizationResource,
		mobile_device_enrollment_profile.NewEnrollmentProfileResource,
		supervision_identity.NewSupervisionIdentityResource,
		ibeacon.NewIbeaconResource,
		icon.NewIconResource,
		inventory_preload_record.NewInventoryPreloadRecordResource,
		licensed_software.NewLicensedSoftwareResource,
		app_installer.NewAppInstallerResource,
		app_request_form_field.NewAppRequestFormFieldResource,
		app_request_settings.NewAppRequestSettingsResource,
		ebook.NewEbookResource,
		mac_app_store_app.NewMacAppResource,
		mobile_device_app.NewMobileAppResource,
		mobile_device_provisioning_profile.NewProvisioningProfileResource,
		macos_configuration_profile.NewResource,
		mobile_device_configuration_profile.NewResource,
		network_segment.NewNetworkSegmentResource,
		pkg.NewPackageResource,
		patch_external_source.NewPatchExternalSourceResource,
		patch_policy.NewPatchPolicyResource,
		patch_software_title.NewPatchSoftwareTitleResource,
		policy.NewPolicyResource,
		restricted_software.NewRestrictedSoftwareResource,
		printer.NewPrinterResource,
		removable_mac_address.NewRemovableMacAddressResource,
		re_enrollment_settings.NewReEnrollmentSettingsResource,
		access_management_settings.NewAccessManagementSettingsResource,
		user_initiated_enrollment_settings.NewUserInitiatedEnrollmentSettingsResource,
		script.NewScriptResource,
		advanced_computer_search.NewAdvancedComputerSearchResource,
		advanced_mobile_device_search.NewAdvancedMobileDeviceSearchResource,
		advanced_user_search.NewAdvancedUserSearchResource,
		advanced_volume_purchasing_content_search.NewAdvancedVolumePurchasingContentSearchResource,
		self_service_plus_settings.NewSelfServicePlusSettingsResource,
		app_installer_settings.NewAppInstallerSettingsResource,
		activation_code.NewActivationCodeResource,
		computer_check_in_settings.NewComputerCheckInSettingsResource,
		computer_inventory_collection_settings.NewComputerInventoryCollectionSettingsResource,
		gsx_connection.NewGsxConnectionSettingsResource,
		pki_venafi.NewPkiVenafiResource,
		pki_digicert.NewDigicertResource,
		pki_adcs.NewAdcsResource,
		pki_json_web_token_configuration.NewJSONWebTokenConfigurationResource,
		impact_alert_notification_settings.NewImpactAlertNotificationSettingsResource,
		managed_software_updates.NewManagedSoftwareUpdateResource,
		jamf_connect.NewJamfConnectResource,
		jamf_parent_settings.NewJamfParentSettingsResource,
		jamf_protect.NewJamfProtectResource,
		jamf_teacher_settings.NewJamfTeacherSettingsResource,
		login_page.NewLoginPageSettingsResource,
		macos_onboarding.NewOnboardingResource,
		mdm_profile_settings.NewMDMProfileSettingsResource,
		self_service_branding_image.NewSelfServiceBrandingImageResource,
		self_service_branding_ios.NewSelfServiceBrandingIosResource,
		self_service_branding_macos.NewSelfServiceBrandingMacosResource,
		self_service_macos_settings.NewSelfServiceMacosSettingsResource,
		service_discovery_enrollment.NewServiceDiscoveryEnrollmentResource,
		smtp_server.NewSmtpServerResource,
		site.NewSiteResource,
		sso_failover_url.NewSsoFailoverURLResource,
		sso_settings.NewSsoSettingsResource,
		class.NewClassResource,
		user_group.NewUserGroupResource,
		location.NewVolumePurchasingLocationResource,
		volume_purchasing_notification.NewVolumePurchasingNotificationResource,
		vpp_assignment.NewVPPAssignmentResource,
		vpp_invitation.NewVPPInvitationResource,
		webhook.NewWebhookResource,
		ssodomain.NewDomainResource,
		ssoconnection.NewConnectionResource,
		aigovernancepolicy.NewPolicyResource,
		activation_profile.NewActivationProfileResource,
		securityclouddevicegroup.NewDeviceGroupResource,
		dns_hostname_mappings.NewHostnameMappingsResource,
		dns_search_domain.NewSearchDomainResource,
		dns_zone.NewDNSZoneResource,
		uem_connect.NewUEMConnectResource,
		ztna_app.NewZtnaAppResource,
		ztna_gateway.NewGatewayResource,
		ztna_grouped_gateway.NewGroupedGatewayResource,
	}
}

func (p *JamfPlatformProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		account.NewAccountDataSource,
		account_group.NewAccountGroupDataSource,
		account_privileges.NewAccountPrivilegesDataSource,
		automated_device_enrollment.NewAutomatedDeviceEnrollmentDataSource,
		automated_device_enrollment_public_key.NewAutomatedDeviceEnrollmentPublicKeyDataSource,
		blueprint.NewBlueprintDataSource,
		blueprints.NewBlueprintsDataSource,
		component.NewComponentDataSource,
		components.NewComponentsDataSource,
		baselines.NewBaselinesDataSource,
		rules.NewRulesDataSource,
		benchmark.NewBenchmarkDataSource,
		benchmarks.NewBenchmarksDataSource,
		allowed_file_extension.NewAllowedFileExtensionDataSource,
		building.NewBuildingDataSource,
		building.NewBuildingsDataSource,
		category.NewCategoriesDataSource,
		category.NewCategoryDataSource,
		ssodomain.NewDomainDataSource,
		ssodomain.NewDomainsDataSource,
		ssoconnection.NewConnectionDataSource,
		ssoconnection.NewConnectionsDataSource,
		aigovernancepolicy.NewPolicyDataSource,
		aigovernancepolicy.NewPoliciesDataSource,
		tool.NewToolDataSource,
		tool.NewToolsDataSource,
		content_categories.NewContentCategoriesDataSource,
		securityclouddevicegroup.NewDeviceGroupDataSource,
		securityclouddevicegroup.NewDeviceGroupsDataSource,
		dns_hostname_mappings.NewHostnameMappingsDataSource,
		dns_search_domain.NewSearchDomainDataSource,
		dns_zone.NewDNSZoneDataSource,
		dns_zone.NewDNSZonesDataSource,
		uem_connect.NewUEMConnectDataSource,
		ztna_app.NewZtnaAppDataSource,
		ztna_app.NewZtnaAppsDataSource,
		ztna_gateway.NewGatewayDataSource,
		ztna_gateway.NewGatewaysDataSource,
		ztna_grouped_gateway.NewGroupedGatewayDataSource,
		ztna_grouped_gateway.NewGroupedGatewaysDataSource,
		ztna_predefined_apps.NewPredefinedAppsDataSource,
		ztna_shared_gateways.NewSharedGatewaysDataSource,
		computer_extension_attribute.NewComputerExtensionAttributeDataSource,
		mobile_device_extension_attribute.NewMobileDeviceExtensionAttributeDataSource,
		user_extension_attribute.NewUserExtensionAttributeDataSource,
		cloud_distribution_point.NewCloudDistributionPointDataSource,
		file_share_distribution_point.NewFileShareDistributionPointDataSource,
		cloud_identity_provider.NewCloudIdentityProviderDataSource,
		cloud_identity_provider.NewCloudIdentityProviderDefaultsDataSource,
		cloud_identity_provider.NewCloudIdentityProvidersDataSource,
		department.NewDepartmentDataSource,
		department.NewDepartmentsDataSource,
		device_group.NewDeviceGroupDataSource,
		device_groups.NewDeviceGroupsDataSource,
		device.NewDeviceDataSource,
		devices.NewDevicesDataSource,
		directory_binding.NewDirectoryBindingDataSource,
		ldap_server.NewLdapServerDataSource,
		local_admin_password_settings.NewLocalAdminPasswordSettingsDataSource,
		disk_encryption_configuration.NewDiskEncryptionConfigurationDataSource,
		computer_invitation.NewComputerInvitationDataSource,
		mobile_device_invitation.NewMobileDeviceInvitationDataSource,
		computer_prestage_enrollment.NewComputerPrestageEnrollmentDataSource,
		mobile_device_prestage_enrollment.NewMobileDevicePrestageEnrollmentDataSource,
		return_to_service.NewReturnToServiceDataSource,
		dock_item.NewDockItemDataSource,
		enrollment_customization.NewEnrollmentCustomizationDataSource,
		mobile_device_enrollment_profile.NewEnrollmentProfileDataSource,
		supervision_identity.NewSupervisionIdentityDataSource,
		tenant_id.NewTenantIDDataSource,
		ibeacon.NewIbeaconDataSource,
		inventory_preload_record.NewInventoryPreloadRecordDataSource,
		licensed_software.NewLicensedSoftwareDataSource,
		app_installer.NewAppInstallerDataSource,
		app_installer.NewAppInstallersDataSource,
		app_installer_title.NewAppInstallerTitleDataSource,
		app_installer_title.NewAppInstallerTitlesDataSource,
		app_request_form_field.NewAppRequestFormFieldDataSource,
		app_store_country_codes.NewAppStoreCountryCodesDataSource,
		ebook.NewEbookDataSource,
		mac_app_store_app.NewMacAppDataSource,
		mobile_device_app.NewMobileAppDataSource,
		mobile_device_provisioning_profile.NewProvisioningProfileDataSource,
		macos_configuration_profile.NewDataSource,
		mobile_device_configuration_profile.NewDataSource,
		network_segment.NewNetworkSegmentDataSource,
		network_segment.NewNetworkSegmentsDataSource,
		pkg.NewPackageDataSource,
		policy.NewPolicyDataSource,
		restricted_software.NewRestrictedSoftwareDataSource,
		printer.NewPrinterDataSource,
		removable_mac_address.NewRemovableMacAddressDataSource,
		re_enrollment_settings.NewReEnrollmentSettingsDataSource,
		access_management_settings.NewAccessManagementSettingsDataSource,
		user_initiated_enrollment_settings.NewUserInitiatedEnrollmentSettingsDataSource,
		script.NewScriptDataSource,
		script.NewScriptsDataSource,
		self_service_plus_settings.NewSelfServicePlusSettingsDataSource,
		app_installer_settings.NewAppInstallerSettingsDataSource,
		activation_code.NewActivationCodeDataSource,
		computer_check_in_settings.NewComputerCheckInSettingsDataSource,
		computer_inventory_collection_settings.NewComputerInventoryCollectionSettingsDataSource,
		gsx_connection.NewGsxConnectionSettingsDataSource,
		pki_certificate_authority.NewCertificateAuthorityDataSource,
		pki_venafi.NewPkiVenafiDataSource,
		pki_digicert.NewDigicertDataSource,
		pki_adcs.NewAdcsDataSource,
		pki_json_web_token_configuration.NewJSONWebTokenConfigurationDataSource,
		impact_alert_notification_settings.NewImpactAlertNotificationSettingsDataSource,
		jamf_connect.NewJamfConnectDataSource,
		jamf_pro_server_url.NewJamfProServerURLDataSource,
		jamf_protect.NewJamfProtectPlansDataSource,
		login_page.NewLoginPageSettingsDataSource,
		macos_onboarding.NewOnboardingDataSource,
		macos_onboarding.NewOnboardingEligibleItemsDataSource,
		mdm_profile_settings.NewMDMProfileSettingsDataSource,
		self_service_branding_ios.NewSelfServiceBrandingIosDataSource,
		self_service_branding_macos.NewSelfServiceBrandingMacosDataSource,
		self_service_macos_settings.NewSelfServiceMacosSettingsDataSource,
		service_discovery_enrollment.NewServiceDiscoveryEnrollmentDataSource,
		smtp_server.NewSmtpServerDataSource,
		patch_external_source.NewPatchExternalSourceDataSource,
		patch_internal_source.NewPatchInternalSourceDataSource,
		patch_policy.NewPatchPolicyDataSource,
		patch_software_title.NewPatchSoftwareTitleDataSource,
		site.NewSiteDataSource,
		sso_failover_url.NewSsoFailoverURLDataSource,
		sso_settings.NewSsoDependenciesDataSource,
		sso_settings.NewSsoSettingsDataSource,
		sso_settings.NewSsoSpMetadataDataSource,
		site.NewSitesDataSource,
		class.NewClassDataSource,
		user_group.NewUserGroupDataSource,
		user_group.NewUserGroupsDataSource,
		user.NewUserDataSource,
		user.NewUsersDataSource,
		advanced_computer_search.NewAdvancedComputerSearchDataSource,
		advanced_mobile_device_search.NewAdvancedMobileDeviceSearchDataSource,
		advanced_user_search.NewAdvancedUserSearchDataSource,
		advanced_volume_purchasing_content_search.NewAdvancedVolumePurchasingContentSearchDataSource,
		location.NewVolumePurchasingLocationDataSource,
		volume_purchasing_notification.NewVolumePurchasingNotificationDataSource,
		vpp_assignment.NewVPPAssignmentDataSource,
		vpp_invitation.NewVPPInvitationDataSource,
		webhook.NewWebhookDataSource,
	}
}

func (p *JamfPlatformProvider) ListResources(ctx context.Context) []func() list.ListResource {
	return []func() list.ListResource{
		account.NewAccountListResource,
		account_group.NewAccountGroupListResource,
		automated_device_enrollment.NewAutomatedDeviceEnrollmentListResource,
		benchmark.NewBenchmarkListResource,
		blueprint.NewBlueprintListResource,
		allowed_file_extension.NewAllowedFileExtensionListResource,
		building.NewBuildingListResource,
		category.NewCategoryListResource,
		securityclouddevicegroup.NewDeviceGroupListResource,
		ssodomain.NewDomainListResource,
		ssoconnection.NewConnectionListResource,
		aigovernancepolicy.NewPolicyListResource,
		dns_zone.NewDNSZoneListResource,
		uem_connect.NewUEMConnectListResource,
		ztna_app.NewZtnaAppListResource,
		ztna_gateway.NewGatewayListResource,
		ztna_grouped_gateway.NewGroupedGatewayListResource,
		file_share_distribution_point.NewFileShareDistributionPointListResource,
		computer_extension_attribute.NewComputerExtensionAttributeListResource,
		mobile_device_extension_attribute.NewMobileDeviceExtensionAttributeListResource,
		user_extension_attribute.NewUserExtensionAttributeListResource,
		cloud_identity_provider.NewCloudIdentityProviderListResource,
		department.NewDepartmentListResource,
		device_group.NewDeviceGroupListResource,
		directory_binding.NewDirectoryBindingListResource,
		ldap_server.NewLdapServerListResource,
		disk_encryption_configuration.NewDiskEncryptionConfigurationListResource,
		computer_invitation.NewComputerInvitationListResource,
		mobile_device_invitation.NewMobileDeviceInvitationListResource,
		computer_prestage_enrollment.NewComputerPrestageEnrollmentListResource,
		mobile_device_prestage_enrollment.NewMobileDevicePrestageEnrollmentListResource,
		return_to_service.NewReturnToServiceListResource,
		dock_item.NewDockItemListResource,
		enrollment_customization.NewEnrollmentCustomizationListResource,
		mobile_device_enrollment_profile.NewEnrollmentProfileListResource,
		supervision_identity.NewSupervisionIdentityListResource,
		ibeacon.NewIbeaconListResource,
		inventory_preload_record.NewInventoryPreloadRecordListResource,
		pki_json_web_token_configuration.NewJSONWebTokenConfigurationListResource,
		licensed_software.NewLicensedSoftwareListResource,
		app_installer.NewAppInstallerListResource,
		app_request_form_field.NewAppRequestFormFieldListResource,
		ebook.NewEbookListResource,
		mac_app_store_app.NewMacAppListResource,
		mobile_device_app.NewMobileAppListResource,
		mobile_device_provisioning_profile.NewProvisioningProfileListResource,
		macos_configuration_profile.NewListResource,
		mobile_device_configuration_profile.NewListResource,
		network_segment.NewNetworkSegmentListResource,
		pkg.NewPackageListResource,
		policy.NewPolicyListResource,
		restricted_software.NewRestrictedSoftwareListResource,
		printer.NewPrinterListResource,
		removable_mac_address.NewRemovableMacAddressListResource,
		script.NewScriptListResource,
		patch_external_source.NewPatchExternalSourceListResource,
		patch_policy.NewPatchPolicyListResource,
		patch_software_title.NewPatchSoftwareTitleListResource,
		site.NewSiteListResource,
		class.NewClassListResource,
		user_group.NewUserGroupListResource,
		advanced_computer_search.NewAdvancedComputerSearchListResource,
		advanced_mobile_device_search.NewAdvancedMobileDeviceSearchListResource,
		advanced_user_search.NewAdvancedUserSearchListResource,
		advanced_volume_purchasing_content_search.NewAdvancedVolumePurchasingContentSearchListResource,
		location.NewVolumePurchasingLocationListResource,
		volume_purchasing_notification.NewVolumePurchasingNotificationListResource,
		vpp_assignment.NewVPPAssignmentListResource,
		vpp_invitation.NewVPPInvitationListResource,
		webhook.NewWebhookListResource,
	}
}

func (p *JamfPlatformProvider) Actions(ctx context.Context) []func() action.Action {
	return []func() action.Action{
		deviceactions.NewEraseAction,
		deviceactions.NewRestartAction,
		deviceactions.NewShutdownAction,
		deviceactions.NewUnmanageAction,
		msuactions.NewPlanAction,
		msuactions.NewAbandonFeatureToggleAction,
		mdmactions.NewSendBlankPushAction,
		mdmactions.NewRenewMdmProfileAction,
		mdmactions.NewFlushMdmCommandsAction,
		maintenanceactions.NewRedeployManagementFrameworkAction,
		uemconnectactions.NewSynchronizeAction,
		uemconnectactions.NewDeployActivationProfileAction,
		maintenanceactions.NewFlushPolicyLogsAction,
		patchactions.NewRetryPatchPolicyLogsAction,
		appinstalleractions.NewRetryInstallationsAction,
		appinstalleractions.NewRetryAllInstallationsAction,
		appinstalleractions.NewUpdateVersionAction,
		jamfprotectactions.NewSyncPlansAction,
		jamfprotectactions.NewRetryDeploymentAction,
		ssodomainactions.NewVerifySSODomainAction,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &JamfPlatformProvider{
			version: version,
		}
	}
}

// getenv is a helper to get an environment variable, returns empty string if not set.
func getenv(key string) string {
	v, _ := os.LookupEnv(key)
	return v
}

// shouldEnableHTTPLogging checks TF_LOG to determine whether HTTP logging should be wired up.
// impactAlertsEnabled resolves the impact_alerts setting, the attribute always
// taking precedence over the environment variable (which is then not consulted
// at all). An unparseable env value leaves the feature off and returns a warning
// for Configure to surface: impact alerts are advisory, so a typo here must not
// stop a plan that would otherwise succeed — but it must not silently do nothing
// either. An empty value is treated as unset, matching the other env variables.
func impactAlertsEnabled(attr types.Bool) (enabled bool, warning string) {
	if !attr.IsNull() && !attr.IsUnknown() {
		return attr.ValueBool(), ""
	}
	v, ok := os.LookupEnv(envImpactAlerts)
	if !ok || strings.TrimSpace(v) == "" {
		return false, ""
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, fmt.Sprintf(
			"%s is set to %q, which is not a valid boolean, so impact alerts stay off. "+
				"Accepted values are those of Go's strconv.ParseBool: 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False. "+
				"The impact_alerts provider attribute, when set, takes precedence over this variable.",
			envImpactAlerts, v)
	}
	return enabled, ""
}

// resolveMinRequestInterval resolves the min_request_interval_ms setting, the
// attribute always taking precedence over the environment variable (which is
// then not consulted at all). An empty env value is treated as unset, matching
// the other env variables; an unparseable one is an error for Configure to
// surface, since silently picking a different pace would misreport what the
// operator asked for.
//
// The default is 0 — no pacing at all. The interval gates request *starts*,
// making it a throughput ceiling rather than a concurrency limit: at 100ms it
// caps the whole provider at 10 requests per second however many operations
// Terraform runs in parallel — measurably the dominant cost of a read-heavy
// plan. Serially it buys almost nothing either, since a typical Jamf request
// already outlasts the interval. Hence a default of 0, which is always passed
// to the client so it overrides the SDK's own default interval; set the
// attribute or the env variable to put pacing back. Features that fan out reads
// bound their own concurrency instead — see impact.dependencySweepConcurrency.
func resolveMinRequestInterval(attr types.Int64) (time.Duration, error) {
	var ms int64
	switch {
	case !attr.IsNull() && !attr.IsUnknown():
		ms = attr.ValueInt64()
	case getenv(envMinRequestIntervalMs) != "":
		parsed, err := strconv.ParseInt(getenv(envMinRequestIntervalMs), 10, 64)
		if err != nil {
			return 0, err
		}
		ms = parsed
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func shouldEnableHTTPLogging() bool {
	level, ok := os.LookupEnv("TF_LOG")
	if !ok {
		return false
	}

	switch strings.ToLower(level) {
	case "debug", "trace":
		return true
	default:
		return false
	}
}
