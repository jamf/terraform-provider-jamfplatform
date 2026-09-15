// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package uem_connect implements the jamfplatform_security_cloud_uem_connect
// resource and data source, backed by the Jamf Security Cloud UEM Connect API.
//
// UEM Connect is the link between Jamf Security Cloud and a Jamf Pro instance:
// device inventory and group membership sync from Jamf Pro into Jamf Security
// Cloud, and device risk can be signalled back the other way. A tenant holds at
// most one, so a second create is refused.
//
// # Two authentication forms, and why the strategy is not an attribute
//
// The integration authenticates to Jamf Pro one of two ways, selected by which
// configuration block is present rather than by a strategy attribute:
//
//	platform_tenant   Jamf Security Cloud provisions its own API role and
//	                  integration on the named tenant. No secret is configured.
//	oauth             Credentials for an API integration the operator created on
//	                  the target instance themselves.
//
// The wire carries an authStrategy field, but it does not round-trip: a connector
// created as M2M reads back as JAMF_PRO_OAUTH, because provisioning leaves an
// OAuth client behind and that is the steady state. Surfacing it as an attribute
// would mean either committing a value the server contradicts, or a plan modifier
// rewriting what the user wrote. Deriving it from block presence — the shape
// ztna_gateway uses for its two immutable forms — avoids both.
//
// # Immutability
//
// There is no update operation for the connection itself; only sync settings and
// enablement can be written after create. So vendor, server address and both
// authentication blocks force replacement — including oauth.client_secret_wo_version,
// because no endpoint accepts credentials for an integration that already exists, so
// re-sending a rotated secret means creating the integration again.
//
// uem_server_url is the one of those that uses RequiresReplaceIfConfigured rather
// than RequiresReplace, and the difference is load-bearing rather than stylistic.
// It is the only Optional+Computed member of the set: on the platform_tenant form
// it is never configured, because Jamf Security Cloud resolves the address from
// the tenant and the resource refuses the pair. The framework marks an
// unconfigured Computed attribute unknown on every plan where anything else
// changes, and plain RequiresReplace tests only whether the planned value equals
// the prior one — an unknown never does. Ordered ahead of UseStateForUnknown, as
// it was, it therefore fired on every in-place edit of a platform_tenant
// connector: toggling enabled or editing a field mapping planned a destroy and
// recreate, which per the schema's own warning strands the JSC Connector API
// integration on the Jamf Pro tenant. RequiresReplaceIfConfigured asks the
// question actually intended — did the practitioner change an address they wrote
// — so it is inert on the form that never writes one, and independent of where it
// sits in the slice. Confirmed by driving the framework's plan pipeline: a no-op
// plan was always clean, an enabled-only change planned a replacement before and
// none after, and a changed configured address still replaces.
// TestUEMServerURL_ReplacementFollowsWhatThePractitionerConfigured holds the line.
//
// The form itself is guarded elsewhere and does not rely on this: both
// platform_tenant and oauth carry objectplanmodifier.RequiresReplace, so
// switching between them replaces regardless.
//
// # Wire mapping
//
// Attribute names follow the admin UI rather than the wire wherever the two
// diverge, per STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI. Because
// the guide also forbids comments inside function bodies, the mapping lives here:
//
//	Terraform attribute                        Wire field
//	----------------------------------------   ------------------------------------
//	uem_server_url                                 url
//	platform_tenant.tenant_id                  tenantId
//	oauth.client_id                            deviceSyncAuth.clientId
//	oauth.client_secret                        deviceSyncAuth.clientSecret
//	enabled                                    enabled
//	scheduled_sync_enabled                     scheduled
//	sync_refresh_interval_minutes                      refreshRateMinutes
//	uem_auto_delete_behavior                       syncConfig.autoDeviceDeletion
//	unmanaged_sync_threshold                   deviceUnmanagedThreshold
//	device_risk_uem_signaling_enabled              deviceRiskTagging
//	disable_sync_on_auth_error                 syncConfig.disableSyncOnAuthError
//	concurrent_device_sync_enabled             concurrentSyncEnabled
//	user_data_field_mapping.device_name             deviceFieldMappings.deviceNameMapping
//	user_data_field_mapping.user_name               deviceFieldMappings.userNameMapping
//	user_data_field_mapping.user_id                 deviceFieldMappings.userIdMapping
//	user_data_field_mapping.phone_number            deviceFieldMappings.phoneNumberMapping
//	user_data_field_mapping.email.source            deviceFieldMappings.userEmailMapping.type
//	user_data_field_mapping.email.prefix            deviceFieldMappings.userEmailMapping.fieldPrefix
//	user_data_field_mapping.email.suffix            deviceFieldMappings.userEmailMapping.fieldSuffix
//	user_data_field_mapping.email.only_if_email_missing
//	                                           deviceFieldMappings.userEmailMapping.useOnlyIfEmailMissing
//	group_membership_mapping.enabled                      groupSettings.groupMappingEnabled
//	group_membership_mapping.default_security_cloud_group_id      groupSettings.defaultGroupId
//	group_membership_mapping.mappings[].uem_group_id      groupSettings.groupMappings[].emmGroupId
//	group_membership_mapping.mappings[].security_cloud_group_id   groupSettings.groupMappings[].wanderaGroupId
//
// `emm` and `wandera` are retired internal product names the service still
// serializes; they do not reach the schema. `type` becomes `source` because the
// value names where the address is read from rather than a kind of thing, and
// `type` next to a `prefix` and a `suffix` reads like a discriminator.
//
// # Surfaces with no API
//
// Three things on the admin UI's UEM Connect screens cannot be managed here,
// because nothing exposes them: the webhook token under Advanced settings,
// vulnerability management enrolment, and the event log. Their absence is a gap in
// the API rather than an omission in this package.
package uem_connect

import (
	"context"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
	commonvalidators "github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Defaults Jamf Security Cloud applies. Wire-probed against a real Jamf Security
// Cloud tenant on 2026-09-01, superseding a 2026-08-28 probe whose findings the
// service has since changed under: the sync interval then accepted any value of 1
// or more with no ceiling, and the unmanaged threshold then stored what it was
// sent. Neither holds — the interval is now an enforced enum and the threshold is
// discarded outright.
//
// The interval's accepted set is not restated here; it comes from the SDK's
// generated SyncSettingsRefreshRateMinutesValues.
const (
	defaultSyncRefreshIntervalMinutes = 1440
	defaultUEMAutoDeleteBehaviour     = "remove_deleted_or_retired"
)

// jamfProGroupIDPattern is the form Jamf Security Cloud requires of a Jamf Pro
// group identifier. Checked at plan time because the server's refusal names no
// field, so an unattributed error would leave the user hunting which of several
// mappings is wrong.
var jamfProGroupIDPattern = regexp.MustCompile(`^(computer|mobile)_[0-9]+$`)

// serverURLPattern is the shape a Jamf Pro address has to take. Checked at plan
// time because the server's only signal is UEM_CONNECTION_FAILED after a failed
// connection test, which reports a missing scheme, an unreachable instance and bad
// credentials as one indistinguishable reason.
var serverURLPattern = regexp.MustCompile(`^https?://\S+$`)

var (
	_ resource.Resource                     = &UEMConnectResource{}
	_ resource.ResourceWithImportState      = &UEMConnectResource{}
	_ resource.ResourceWithIdentity         = &UEMConnectResource{}
	_ resource.ResourceWithConfigValidators = &UEMConnectResource{}
)

const (
	defaultCreateTimeout = 120 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewUEMConnectResource returns a new instance of UEMConnectResource.
func NewUEMConnectResource() resource.Resource {
	return &UEMConnectResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *UEMConnectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_uem_connect"
}

// IdentitySchema defines the identifier used for import.
func (r *UEMConnectResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "UEM Connect integration ID used to uniquely reference the integration.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators enforces that exactly one authentication form is configured,
// and that the OAuth form carries the address it needs.
func (r *UEMConnectResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("platform_tenant"),
			path.MatchRoot("oauth"),
		),
		resourcevalidator.RequiredTogether(
			path.MatchRoot("oauth"),
			path.MatchRoot("uem_server_url"),
		),
		resourcevalidator.Conflicting(
			path.MatchRoot("platform_tenant"),
			path.MatchRoot("uem_server_url"),
		),
	}
}

// Schema returns the Terraform schema for the UEM Connect resource.
func (r *UEMConnectResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Security Cloud **UEM Connect** integration, which syncs device " +
			"inventory and group membership from Jamf Pro into Jamf Security Cloud and can signal device risk " +
			"back to Jamf Pro.\n\n" +
			"A tenant holds one UEM Connect integration. Creating a second is refused, so where an integration " +
			"already exists, import it rather than declaring a new one.\n\n" +
			"Choose one of two ways to authenticate to Jamf Pro. With `platform_tenant`, Jamf Security Cloud " +
			"creates and manages its own credentials on the named tenant and no secret is configured here. " +
			"Prefer it. With `oauth`, supply the client ID and secret of an API integration you created on the " +
			"Jamf Pro instance yourself.\n\n" +
			"~> **`platform_tenant` leaves credentials behind when the integration is destroyed.** Jamf Security " +
			"Cloud creates a Jamf Pro API integration named `JSC Connector` to authenticate with, and that " +
			"integration survives the destroy. Neither Jamf Security Cloud nor this provider removes it, and " +
			"this provider holds no Jamf Pro credentials for that tenant to remove it with. Each " +
			"create/destroy cycle therefore leaves one more enabled client credential on the Jamf Pro " +
			"instance, each carrying the 31-privilege `JSC Connector` role, which includes writing " +
			"configuration profiles, computer and mobile device records, extension attributes and group " +
			"memberships. Audit **Settings → API roles and clients** on the Jamf Pro instance after any " +
			"destroy and delete the orphans. Observed 2026-09-01 on a test instance: 97 enabled `JSC " +
			"Connector` integrations against zero live connectors, 88% of every integration on it.\n\n" +
			"The connection is fixed once created: changing the vendor, the address or the way it authenticates " +
			"replaces the integration, which briefly interrupts syncing.\n\n" +
			"After importing, run `terraform plan` before you apply: `user_data_field_mapping` and " +
			"`group_membership_mapping` are captured from the tenant even where your configuration does not " +
			"declare them, so the plan proposes removing both. That removal is real: Jamf Security Cloud " +
			"writes the sync settings whole and resets whatever you leave out. Copy the values the plan " +
			"shows you into your configuration to keep them. See the " +
			"[Importing existing objects guide](../guides/importing).\n\n" +
			"See the [Jamf Security Cloud guide](../guides/security-cloud) for how a Jamf Pro group is named in a " +
			"membership mapping, why the order of the mappings decides which group a device joins, and what an " +
			"import leaves you to reconcile." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Integration ID assigned by Jamf Security Cloud.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uem_vendor": schema.StringAttribute{
				MarkdownDescription: "**\"UEM vendor\"** in the Jamf Security Cloud admin UI. Only `JAMF_PRO` is " +
					"supported by this provider.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(vendorJamfPro),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"uem_server_url": schema.StringAttribute{
				MarkdownDescription: "**\"UEM server URL\"** in the Jamf Security Cloud admin UI: the full address " +
					"of the Jamf Pro instance, including `https://`. Required with `oauth`. Must not be set " +
					"with `platform_tenant`, which resolves the address from the tenant itself.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						serverURLPattern,
						"must be the full address of the Jamf Pro instance including its scheme, for example `https://your-instance.jamfcloud.com`",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform_tenant": schema.SingleNestedAttribute{
				MarkdownDescription: "Authenticate by naming the Jamf Pro tenant, letting Jamf Security Cloud " +
					"provision and manage its own credentials there. Mutually exclusive with `oauth`.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"tenant_id": schema.StringAttribute{
						MarkdownDescription: "Identifier of the Jamf Pro tenant to sync with. " +
							"`jamfplatform_pro_tenant_id` reads it for the tenant a provider is scoped to.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"oauth": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"OAuth authentication\"** in the Jamf Security Cloud admin UI: " +
					"authenticate with credentials from an API integration you created on the Jamf Pro instance. " +
					"Mutually exclusive with `platform_tenant`, and requires `uem_server_url`.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"client_id": schema.StringAttribute{
						MarkdownDescription: "**\"Client ID\"** in the Jamf Security Cloud admin UI: the client ID " +
							"of the API integration on the Jamf Pro instance.",
						Required: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"client_secret": schema.StringAttribute{
						MarkdownDescription: "**\"Client secret\"** in the Jamf Security Cloud admin UI. Never " +
							"returned once stored, so it is write-only: it is sent when supplied and never held " +
							"in state. Change `client_secret_wo_version` to send a new one.",
						Required:  true,
						WriteOnly: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"client_secret_wo_version": schema.Int64Attribute{
						MarkdownDescription: "Increment after rotating the credential in Jamf Pro to send the new " +
							"`client_secret`. The secret itself is not stored, so there is nothing to compare " +
							"against and no other way to trigger a rotation.\n\n" +
							"Jamf Security Cloud offers no way to change an existing integration's " +
							"credentials, so a rotation **replaces** the integration, which briefly interrupts " +
							"syncing.",
						Optional: true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.RequiresReplace(),
						},
					},
				},
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the integration syncs. Disabling it stops scheduled and manual syncs " +
					"without removing the integration or the devices already synced.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"scheduled_sync_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Security Cloud syncs on the schedule set by " +
					"`sync_refresh_interval_minutes`. With this off, only a manually triggered sync runs.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"sync_refresh_interval_minutes": schema.Int64Attribute{
				MarkdownDescription: "**\"Sync refresh interval\"** in the Jamf Security Cloud admin UI, in " +
					"minutes. Jamf Security Cloud accepts only the intervals its admin UI offers and rejects " +
					"anything else, so the accepted values are 60, 120, 240, 480, 720 and 1440. Defaults to 1440 " +
					"(24 hours).",
				Optional: true,
				Computed: true,
				Default:  int64default.StaticInt64(defaultSyncRefreshIntervalMinutes),
				Validators: []validator.Int64{
					int64validator.OneOf(securitycloud.SyncSettingsRefreshRateMinutesValues()...),
				},
			},
			"uem_auto_delete_behavior": schema.StringAttribute{
				MarkdownDescription: "**\"Configure UEM auto-delete behavior\"** in the Jamf Security Cloud admin " +
					"UI: what happens in Jamf Security Cloud to devices that leave Jamf Pro.\n\n" +
					"- `keep_deleted_or_retired`: keep deleted or retired devices in Jamf Security Cloud.\n" +
					"- `remove_deleted_or_retired`: remove deleted or retired devices from Jamf Security Cloud.\n" +
					"- `remove_deleted_or_unmanaged`: also remove devices Jamf Pro reports as no longer managed.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(defaultUEMAutoDeleteBehaviour),
				Validators: []validator.String{
					stringvalidator.OneOf(uemAutoDeleteBehaviourValues()...),
				},
			},
			"unmanaged_sync_threshold": schema.Int64Attribute{
				MarkdownDescription: "Days since a device last checked in before Jamf Security Cloud treats it as " +
					"unmanaged. Read-only, and reported as `0`: Jamf Security Cloud does not apply this setting to a " +
					"Jamf Pro connection, taking device status from Jamf Pro directly instead, so there is nothing " +
					"for this provider to set. Confirmed on 2026-09-01: every value sent for a `JAMF_PRO` connector " +
					"is accepted and then discarded.",
				Computed: true,
			},
			"device_risk_uem_signaling_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable device risk UEM signaling\"** in the Jamf Security Cloud admin " +
					"UI: whether a device's risk level is sent back to Jamf Pro.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"disable_sync_on_auth_error": schema.BoolAttribute{
				MarkdownDescription: "**\"Disable syncs when the credentials are expired or the user account is " +
					"locked\"** in the Jamf Security Cloud admin UI. Leaving this on stops repeated failing syncs " +
					"after the credentials stop working.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"concurrent_device_sync_enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Sync multiple devices simultaneously for faster inventory updates\"** in " +
					"the Jamf Security Cloud admin UI.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"user_data_field_mapping": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"User data field mapping\"** in the Jamf Security Cloud admin UI: which " +
					"Jamf Pro attribute each Jamf Security Cloud device field is populated from. Omit the whole " +
					"block for the defaults, which is what the admin UI's \"Use default data field mapping\" " +
					"checkbox selects.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"device_name": schema.StringAttribute{
						MarkdownDescription: "**\"Device name\"** in the Jamf Security Cloud admin UI. Defaults to " +
							"`" + defaultDeviceNameMapping + "`.",
						Optional: true,
						Computed: true,
						Validators: []validator.String{
							stringvalidator.OneOf(deviceNameMappingValues...),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"user_name": schema.StringAttribute{
						MarkdownDescription: "**\"User name\"** in the Jamf Security Cloud admin UI. `NO_CHANGE` " +
							"leaves the field as Jamf Security Cloud already has it. Defaults to `" +
							defaultUserNameMapping + "`.",
						Optional: true,
						Computed: true,
						Validators: []validator.String{
							stringvalidator.OneOf(userNameMappingValues...),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"user_id": schema.StringAttribute{
						MarkdownDescription: "**\"User ID\"** in the Jamf Security Cloud admin UI. `NO_CHANGE` " +
							"leaves the field as Jamf Security Cloud already has it. Defaults to `" +
							defaultUserIDMapping + "`.",
						Optional: true,
						Computed: true,
						Validators: []validator.String{
							stringvalidator.OneOf(userIDMappingValues...),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"phone_number": schema.StringAttribute{
						MarkdownDescription: "**\"Phone number\"** in the Jamf Security Cloud admin UI. " +
							"`NO_PHONE_NUMBER` leaves the field empty. Defaults to `" +
							defaultPhoneNumberMapping + "`.",
						Optional: true,
						Computed: true,
						Validators: []validator.String{
							stringvalidator.OneOf(phoneNumberMappingValues...),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"email": schema.SingleNestedAttribute{
						MarkdownDescription: "**\"Email\"** in the Jamf Security Cloud admin UI: how a device's " +
							"email address is derived.",
						Optional: true,
						Attributes: map[string]schema.Attribute{
							"source": schema.StringAttribute{
								MarkdownDescription: "The Jamf Pro attribute the address is read from. " +
									"`EMAIL_ADDRESS` uses Jamf Pro's email attribute as-is; any other value reads " +
									"that attribute and builds an address from it with `prefix` and `suffix`. " +
									"Defaults to `" + defaultUserEmailMappingType + "`.",
								Optional: true,
								Computed: true,
								Validators: []validator.String{
									stringvalidator.OneOf(userEmailMappingTypeValues...),
								},
								PlanModifiers: []planmodifier.String{
									stringplanmodifier.UseNonNullStateForUnknown(),
								},
							},
							"prefix": schema.StringAttribute{
								MarkdownDescription: "Prepended to the value read from Jamf Pro. Ignored when " +
									"`source` is `EMAIL_ADDRESS`.",
								Optional: true,
							},
							"suffix": schema.StringAttribute{
								MarkdownDescription: "Appended to the value read from Jamf Pro to form a full " +
									"address; an `@` is inserted when the suffix does not start with one. Ignored " +
									"when `source` is `EMAIL_ADDRESS`.",
								Optional: true,
							},
							"only_if_email_missing": schema.BoolAttribute{
								MarkdownDescription: "Apply this rule only where Jamf Pro's own email attribute is " +
									"empty, rather than always.",
								Optional: true,
								Computed: true,
								PlanModifiers: []planmodifier.Bool{
									boolplanmodifier.UseNonNullStateForUnknown(),
								},
							},
						},
					},
				},
			},
			"group_membership_mapping": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Group membership mapping\"** in the Jamf Security Cloud admin UI. Links " +
					"Jamf Pro groups to Jamf Security Cloud device groups so membership syncs between the two. " +
					"Devices matching no mapping join the default group.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						MarkdownDescription: "**\"Enable group membership mapping\"** in the Jamf Security Cloud " +
							"admin UI.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"default_security_cloud_group_id": schema.StringAttribute{
						MarkdownDescription: "**\"Default mapping\"** in the Jamf Security Cloud admin UI: the " +
							"Jamf Security Cloud device group devices join when no mapping matches. Leave unset " +
							"for the built-in Default Group.",
						Optional: true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"mappings": schema.ListNestedAttribute{
						MarkdownDescription: "Group assignments, evaluated in order: a device joins the group of " +
							"the first entry it matches, so put the most specific first. An empty list clears " +
							"every mapping, and so does omitting this while still declaring " +
							"`group_membership_mapping`, because the block replaces what it does not mention.",
						Optional: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"uem_group_id": schema.StringAttribute{
									MarkdownDescription: "**\"UEM group\"** in the Jamf Security Cloud admin UI: " +
										"the Jamf Pro group, written as `computer_` or `mobile_` followed by the " +
										"group's number, for example `computer_12`.\n\n" +
										"This composes out of a `jamfplatform_device_group`, whose `device_type` " +
										"is already `computer` or `mobile` and whose `jamf_pro_id` is the " +
										"number:\n\n" +
										"    uem_group_id = \"${jamfplatform_device_group.x.device_type}_${jamfplatform_device_group.x.jamf_pro_id}\"\n\n" +
										"Jamf Security Cloud does not check that the group exists, so a wrong " +
										"number is accepted and never matches.",
									Required: true,
									Validators: []validator.String{
										stringvalidator.RegexMatches(
											jamfProGroupIDPattern,
											"must be `computer_` or `mobile_` followed by the Jamf Pro group's number, for example `computer_12`",
										),
									},
								},
								"security_cloud_group_id": schema.StringAttribute{
									MarkdownDescription: "**\"Jamf Security Cloud group\"** in the Jamf Security " +
										"Cloud admin UI: the ID of the device group members of the Jamf Pro group " +
										"are assigned to. Jamf Security Cloud does not check that the group " +
										"exists.",
									Required: true,
									Validators: []validator.String{
										stringvalidator.LengthAtLeast(1),
									},
								},
							},
						},
						Validators: []validator.List{
							commonvalidators.UniqueStringFieldList("uem_group_id"),
						},
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

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *UEMConnectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_uem_connect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Security Cloud UEM Connect integration
// ID.
func (r *UEMConnectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
