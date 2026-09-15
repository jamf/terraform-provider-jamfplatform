// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package activation_profile implements the
// jamfplatform_security_cloud_activation_profile resource, backed by the Jamf
// Security Cloud enrollment API.
//
// An activation profile is the enrollment credential end users activate Jamf
// Trust against: it mints an opaque code, which the console turns into a
// shareable link and which jamfplatform_security_cloud_activation_profile_deploy
// hands to a UEM. This resource is the half that mints one.
//
// Attribute names follow the console rather than the wire, per STYLE_GUIDE
// §Attribute names mirror the Jamf Pro admin UI. Because the guide also forbids
// comments inside function bodies, the wire mapping lives here:
//
//	Terraform attribute                Wire field
//	--------------------------------   -----------------------------------------
//	name                               name
//	platforms                          platforms ("ios"/"mac" -> "iOS"/"MAC")
//	capabilities.content_controls      capabilities.dataPolicy
//	capabilities.network_security      capabilities.networkSecurity
//	                                   AND capabilities.vulnerabilityManagement
//	capabilities.note                  capabilities.note
//	device_group_id                    groupId
//	paused                             (no field - the pause/resume operations)
//	id                                 code
//
// Two mappings are not one-to-one and both were established by creating profiles
// through the API and reading them in the console on 2026-09-01.
//
// `network_security` writes *two* wire fields. The API declares `networkSecurity`
// and `vulnerabilityManagement` separately and the server refuses any request
// where they disagree, without the schema saying so. The console shows a single
// "Network security" checkbox, and enabling both wire fields is exactly what
// lights it. Modelling one attribute rather than two plus an equality validator
// makes the state the server rejects unrepresentable instead of merely invalid.
//
// `origin` is not surfaced at all. Its declared enum has one member, PUBLIC_API,
// and it identifies the caller rather than configuring the profile, so it is
// request plumbing rather than user configuration. Note the wire accepts more
// values than the spec declares — `RADAR` covers console-created profiles — which
// is why the list operation cannot enumerate a tenant and why this package ships
// no plural data source.
//
// The code itself is treated as a secret rather than a plain identifier. Jamf
// Security Cloud mints it, never reads it back, and holding it is on its own
// enough to enrol a device into the tenant — the exact shape STYLE_GUIDE
// §Server-minted secrets describes — so `id` is Computed + Sensitive, and the
// published example's output carries `sensitive = true` to match. Dropping
// either would put a live enrollment credential in cleartext plan output, build
// logs and `-json` artefacts, so neither is a simplification.
//
// Three limits follow from the read model being `{"code": "..."}` and nothing
// else, and they shape every operation below:
//
// Read cannot refresh a single configured attribute, so every one of them is
// RequiresReplace and state is the only record of what was sent. Drift on a
// configured field is undetectable.
//
// Delete is a soft delete the read surface does not reflect: after it succeeds
// the item GET still answers 200 and the collection still returns the code. So
// Delete is fire-and-trust per STYLE_GUIDE §Delete semantics, and out-of-band
// deletion never appears as drift. A non-destructive liveness oracle does exist —
// re-asserting the profile's pause state answers 204 when live, 409 when deleted
// and 404 when the code never existed — but it is deliberately not used in Read,
// because Read runs during refresh and a plan must not issue writes.
//
// Import is not implemented. A passthrough import would set the code and leave
// every other attribute null, and because those attributes are all
// RequiresReplace the next plan would replace — destroying the profile it had
// just adopted. Verified against a fully-configured console profile: its GET
// returns the bare code and nothing more, so there is nothing an importer could
// populate.
package activation_profile

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Bounds Jamf Security Cloud enforces on an activation profile write,
// wire-probed against the EU gateway on 2026-09-01. Each is checked at plan time
// because the server reports a violation as a bare 400 mid-apply.
const (
	maxNameLength = 100
	maxNoteLength = 255
	minPlatforms  = 1
	maxPlatforms  = 2
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// ActivationProfileResource implements the Terraform resource for Jamf Security
// Cloud activation profiles.
type ActivationProfileResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                     = &ActivationProfileResource{}
	_ resource.ResourceWithConfigValidators = &ActivationProfileResource{}
)

// NewActivationProfileResource returns a new activation profile resource.
func NewActivationProfileResource() resource.Resource {
	return &ActivationProfileResource{}
}

// Metadata sets the Terraform type name.
func (r *ActivationProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_activation_profile"
}

// Schema returns the Terraform schema for the activation profile resource.
func (r *ActivationProfileResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Security Cloud activation profile, the enrollment credential end users " +
			"activate Jamf Trust against. Creating one mints an activation code, which you can hand to a UEM with " +
			"`jamfplatform_security_cloud_activation_profile_deploy` or distribute as a link.\n\n" +
			"This resource manages a deliberately small part of an activation profile. Jamf Security Cloud offers " +
			"considerably more on an activation profile than it accepts here: the end user application, " +
			"authentication and identity provider, in-app secure DNS control, customizable block pages, expiration " +
			"date and device location settings are all configurable in Jamf Security Cloud and not through this " +
			"resource. A profile created here takes the Jamf Security Cloud default for each of them, and Terraform " +
			"can neither set nor detect changes to them.\n\n" +
			"Three limitations are worth understanding before adopting this resource, all of them consequences of " +
			"Jamf Security Cloud returning only the activation code when a profile is read:\n\n" +
			"- **Changing any setting replaces the profile**, which mints a new activation code and invalidates the " +
			"old one. Anything already distributed with the previous code stops working.\n" +
			"- **Terraform cannot detect changes made outside Terraform.** A profile edited or deleted in Jamf " +
			"Security Cloud continues to appear healthy, and `terraform plan` reports no changes.\n" +
			"- **Destroying a profile cannot be confirmed, and does not remove it from the Jamf Security Cloud " +
			"list.** Jamf Security Cloud retains destroyed profiles, so the list grows with every profile Terraform " +
			"has ever created.\n\n" +
			"Importing is not supported: Jamf Security Cloud does not return enough of an existing profile for " +
			"Terraform to adopt it without immediately replacing it.\n\n" +
			"See the [Jamf Security Cloud guide](../guides/security-cloud) for how activation profiles reach " +
			"devices." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Activation code identifying this profile. Jamf Security Cloud generates it, " +
					"and it is the value an enrollment link or a UEM deployment carries. Treated as sensitive: " +
					"the code on its own is enough to enrol a device, so Terraform redacts it from plan and " +
					"apply output. An output or variable that carries it must be marked sensitive too.",
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name shown for this activation profile in Jamf Security Cloud. Changing the " +
					"name replaces the profile and mints a new activation code.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxNameLength),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"platforms": schema.SetAttribute{
				MarkdownDescription: "Device platforms this activation profile targets: `ios`, `mac`, or both. " +
					"Jamf Security Cloud requires at least one and accepts at most two. Jamf Security Cloud does " +
					"not show this selection anywhere on the profile, and the platforms a profile actually " +
					"supports follow from the service capabilities you enable rather than from this " +
					"value.\n\nThe `jamfplatform_security_cloud_activation_profile_deploy` action spells the Mac " +
					"platform `macos` in its own `os` argument. The two are deliberately different vocabularies: " +
					"this attribute selects platforms, while that argument selects one deployment form " +
					"(`ios_byod`, `ios_supervised`, `ios_unsupervised` or `macos`).",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeBetween(minPlatforms, maxPlatforms),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(platformLabels()...)),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"capabilities": schema.SingleNestedAttribute{
				MarkdownDescription: "Service capabilities enabled for devices enrolled with this profile. At " +
					"least one capability must be enabled. Changing them replaces the profile.",
				Required: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"content_controls": schema.BoolAttribute{
						MarkdownDescription: "**\"Content controls\"** in Jamf Security Cloud. Manage network " +
							"activity for content filtering and cost control.",
						Optional: true,
						Computed: true,
						Default:  booldefault.StaticBool(false),
					},
					"network_security": schema.BoolAttribute{
						MarkdownDescription: "**\"Network security\"** in Jamf Security Cloud. Protect device " +
							"network connections from cyber threats.",
						Optional: true,
						Computed: true,
						Default:  booldefault.StaticBool(false),
					},
					"note": schema.StringAttribute{
						MarkdownDescription: "Free-text note stored alongside the capability selection. Jamf " +
							"Security Cloud does not display it on the profile.",
						Optional: true,
						Validators: []validator.String{
							stringvalidator.LengthAtMost(maxNoteLength),
						},
					},
				},
			},
			"device_group_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the Jamf Security Cloud device group that devices enrolling " +
					"with this profile are added to. Jamf Security Cloud does not check that the group exists, so " +
					"an identifier that does not match a group leaves devices in no group at all. If a UEM " +
					"integration is configured, a UEM sync overwrites the group chosen here.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"paused": schema.BoolAttribute{
				MarkdownDescription: "Whether the profile is paused. A paused profile stops accepting new " +
					"enrollments. This is the only setting that can be changed without replacing the profile. " +
					"Jamf Security Cloud does not report a profile's paused state when read, so a profile paused " +
					"or resumed outside Terraform is not detected as a change.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
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

// ConfigValidators enforces the one capability rule the server applies as a
// business rule rather than field validation.
//
// Jamf Security Cloud refuses a profile with no capability enabled, and it does
// so in its own error envelope rather than the documented shape — nothing in the
// response body names a field or parses reliably — so the check has to happen at
// plan time to be useful at all.
func (r *ActivationProfileResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		atLeastOneCapabilityValidator{},
	}
}

// Configure stores the configured Jamf Security Cloud client.
func (r *ActivationProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_activation_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}
