// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package policy implements the jamfplatform_pro_policy resource, data
// source, and list resource backed by the Jamf ProClassic policies API. It
// is the canary consumer of internal/common/scope for the Phase 5 fan-out
// across every scope-bearing classic resource.
package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /policies predates the provider's overall floor.
const minJamfProVersion = ""

// PolicyResource implements the Terraform resource for Jamf Pro classic policies.
type PolicyResource struct {
	client *proclassic.Client
	// ldapSearcher backs the plan-time scope directory-service user-group
	// preflight (ModifyPlan). The LDAP group search is a Pro (v1) endpoint, so
	// it is a separate client from the ProClassic CRUD client. nil until Configure.
	ldapSearcher ldapgroups.Searcher
	// impact backs the plan-time impact alert on scope changes. nil when the
	// provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var _ resource.Resource = &PolicyResource{}
var _ resource.ResourceWithImportState = &PolicyResource{}
var _ resource.ResourceWithIdentity = &PolicyResource{}
var _ resource.ResourceWithModifyPlan = &PolicyResource{}
var _ resource.ResourceWithConfigValidators = &PolicyResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPolicyResource returns a new instance of PolicyResource.
func NewPolicyResource() resource.Resource {
	return &PolicyResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PolicyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_policy"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *PolicyResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro policy ID used to uniquely reference the policy.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the policy resource. STYLE_GUIDE
// §Schema rules: keep inline and as flat as possible. The 13-section policy
// schema is large by necessity — every section mirrors the SDK Policy type.
// Attribute names mirror the Jamf Pro admin UI labels (STYLE_GUIDE
// §"Attribute names mirror the Jamf Pro admin UI"); the underlying wire
// element names are noted in attribute descriptions where they differ.
func (r *PolicyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             2,
		MarkdownDescription: "Manages a Jamf Pro policy. Top-level blocks mirror the admin UI's tabs and Options sidebar: `general`, `scope`, `self_service`, `user_interaction`, and the Options payloads `packages`, `scripts`, `printers`, `disk_encryption`, `dock_items`, `local_accounts`, `management_account`, `directory_bindings`, `efi_password`, `restart_options`, `maintenance`, `files_and_processes`. Scope targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.x.jamf_pro_id` to bridge from Platform Services. The four account-maintenance payloads (`local_accounts`, `management_account`, `directory_bindings`, `efi_password`) are flattened peers of the UI sections; internally Jamf Pro stores them as a single `account_maintenance` object. The legacy Software Update and Conditional Access policy sections are **intentionally not modelled**. Both are obsolete in Jamf Pro, superseded by MDM-driven app installs, OS update scheduling and the patch-management surface. To drive OS or app updates from Terraform, reach for the patch / DDM resources instead.\n\nUpdates are merged rather than replaced. Removing a whole optional block (`scope`, `self_service`, `packages`, `scripts`, `printers`, `dock_items`, `local_accounts`, `management_account`, `directory_bindings`, `efi_password`, `restart_options`, `maintenance`, `files_and_processes`, `user_interaction` or `disk_encryption`) from your configuration does not clear it: Jamf Pro keeps the values you set previously. To clear a block, null its individual fields instead of deleting the block." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Policy ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"general": schema.SingleNestedAttribute{
				MarkdownDescription: "Policy general settings. `name` is required; every other field is optional. Read-only fields (`category_name`, `site_name`, `id`) are returned by Jamf Pro.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "Policy ID under `general`. Matches the top-level `id`. Returned by Jamf Pro.",
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "Policy display name. Must be unique within the tenant.",
						Required:            true,
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"enabled":                       optComputedBool("Whether the policy is enabled. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"trigger":                       optComputedString("Aggregate trigger label (`EVENT`, `USER_INITIATED`, etc.). Omit to leave the current value untouched."),
					"trigger_checkin":               optComputedBool("Fire on managed check-in. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"trigger_enrollment_complete":   optComputedBool("Fire when device enrollment completes. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"trigger_login":                 optComputedBool("Fire on user login. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"trigger_network_state_changed": optComputedBool("Fire when the device's network state changes. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"trigger_startup":               optComputedBool("Fire on device startup. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"trigger_other":                 optComputedString("Custom event name to trigger the policy. Omit to leave the current value untouched."),
					"frequency":                     optComputedString("How often the policy runs. Valid values include `Once per computer`, `Once per user per computer`, `Once per user`, `Once every day`, `Once every week`, `Once every month`, `Ongoing`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it."),
					"retry_event": schema.StringAttribute{
						MarkdownDescription: "When to retry a failed run: `none`, `trigger` (the policy's own trigger), or `check-in`. Requires `frequency = \"Once per computer\"`. Jamf Pro clears a policy's retry configuration under any other frequency. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(proclassic.PolicyPostGeneralRetryEventValues()...),
						},
					},
					"retry_attempts":                  optComputedInt("Maximum number of retry attempts; `-1` means no retries. Requires `frequency = \"Once per computer\"`. Jamf Pro clears a policy's retry configuration under any other frequency. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it."),
					"notify_on_each_failed_retry":     optComputedBool("Notify the administrator on each failed retry. Requires `frequency = \"Once per computer\"`. Jamf Pro clears a policy's retry configuration under any other frequency. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"limit_to_jamf_pro_assigned_user": optComputedBool("Restrict the policy to the Jamf Pro-assigned user only. Mirrors Options > General > Client-Side Limitations > Limit to Jamf Pro-assigned user. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"target_drive":                    optComputedString("Drive target (e.g. `/`). Omit to leave the current value untouched."),
					"offline":                         optComputedBool("Allow execution while the device is offline. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"network_requirements": schema.StringAttribute{
						MarkdownDescription: "Network connection the policy requires. `Any` places no requirement; `Ethernet` restricts the policy to a wired connection. Shown in the admin UI as Options ▸ General ▸ Client-Side Limitations ▸ Network Requirements. Also drives the read-only `network_limitations.minimum_network_connection`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(proclassic.PolicyPostGeneralNetworkRequirementsValues()...),
						},
					},
					"category_id": optComputedString("Jamf Pro category ID. Use `-1` to clear. Omit to leave the current value untouched."),
					"category_name": schema.StringAttribute{
						// No UseStateForUnknown: derived from the mutable category_id, so it
						// must go Unknown when category_id changes. See STYLE_GUIDE §886.
						MarkdownDescription: "Category display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"site_id": optComputedString("Jamf Pro site ID scoping the policy. Use `-1` for \"no site\". Omit to leave the current value untouched."),
					"site_name": schema.StringAttribute{
						// No UseStateForUnknown: derived from the mutable site_id, so it
						// must go Unknown when site_id changes. See STYLE_GUIDE §886.
						MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"date_time_limitations": schema.SingleNestedAttribute{
						MarkdownDescription: "Optional schedule limitations for when the policy may run. Only the user-authored date/time inputs are surfaced. The derived epoch and UTC siblings Jamf Pro also stores are deterministic transforms of `activation_date` / `expiration_date`, reproducible client-side with Terraform stdlib functions such as `formatdate`.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"activation_date": schema.StringAttribute{
								MarkdownDescription: "Activation date in 24-hour `YYYY-MM-DD HH:MM:SS` form (e.g. `2027-06-01 14:30:00`).",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
								Validators: []validator.String{
									stringvalidator.RegexMatches(
										activationExpirationDatePattern,
										"Value must be 24-hour YYYY-MM-DD HH:MM:SS (e.g. 2027-06-01 14:30:00)",
									),
								},
							},
							"expiration_date": schema.StringAttribute{
								MarkdownDescription: "Expiration date in 24-hour `YYYY-MM-DD HH:MM:SS` form (e.g. `2027-12-31 23:59:59`).",
								Optional:            true,
								Computed:            true,
								PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
								Validators: []validator.String{
									stringvalidator.RegexMatches(
										activationExpirationDatePattern,
										"Value must be 24-hour YYYY-MM-DD HH:MM:SS (e.g. 2027-12-31 23:59:59)",
									),
								},
							},
							"no_execute_on": schema.SetAttribute{
								MarkdownDescription: "Day-of-week labels on which the policy must not execute. Three-letter abbreviations: `Sun`, `Mon`, `Tue`, `Wed`, `Thu`, `Fri`, `Sat`.",
								ElementType:         types.StringType,
								Optional:            true,
								Computed:            true,
								Validators: []validator.Set{
									setvalidator.ValueStringsAre(
										stringvalidator.OneOf(proclassic.PolicyGeneralDateTimeLimitationsNoExecuteOnDayValues()...),
									),
								},
							},
							"no_execute_start": noExecuteTimeAttribute("start", "5:00 PM"),
							"no_execute_end":   noExecuteTimeAttribute("end", "7:00 AM"),
						},
					},
					"network_limitations": schema.SingleNestedAttribute{
						MarkdownDescription: "Read-only view of the network conditions under which the policy may run. Jamf Pro derives all three attributes from `general.network_requirements` and `scope.limitations.network_segment_ids`. Declare the block as `{}` to read the derived values into state.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"minimum_network_connection": serverProjectedString("Minimum network connection, which Jamf Pro derives from `general.network_requirements`: `No Minimum` when that is `Any`, `Ethernet` when it is `Ethernet`. Set `general.network_requirements` to change it."),
							"any_ip_address":             serverProjectedBool("Whether the policy runs on any IP address. Jamf Pro sets it to `true` while `scope.limitations.network_segment_ids` is empty, and `false` once that list names a segment."),
							"network_segment_ids": schema.SetAttribute{
								MarkdownDescription: "Network segment IDs the policy may run on, as Jamf Pro IDs. A read-only view of `scope.limitations.network_segment_ids`, which is the same list. Set it there.",
								ElementType:         types.StringType,
								Computed:            true,
							},
						},
					},
					"override_default_settings": schema.SingleNestedAttribute{
						MarkdownDescription: "Read-only view of the per-policy overrides of tenant-wide defaults, matching the admin UI's \"Override Default Settings\" panel. Jamf Pro derives each attribute from a setting that lives elsewhere. Declare the block as `{}` to read the derived values into state.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"target_drive":       serverProjectedString("Target drive, mirroring `general.target_drive`. Set it there."),
							"distribution_point": serverProjectedString("Distribution point the packages download from, mirroring `packages.distribution_point`. Set it there."),
							"force_afp_smb":      serverProjectedBool("Whether file sharing is forced over AFP/SMB instead of HTTP, matching the admin UI's Options ▸ Packages checkbox of the same name. Change it in the admin UI: Jamf Pro offers no way to set it from Terraform."),
							"sus":                serverProjectedString("Software update server the policy installs updates from, matching the admin UI's Options ▸ Software Update setting. This provider does not model that section, and Jamf Pro offers no way to set the value from Terraform, so change it in the admin UI."),
						},
					},
				},
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "Policy scope. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and it is left as configured outside Terraform, with updates preserving it. Targets are flat sets of Jamf Pro IDs; interpolate `jamfplatform_device_group.<x>.jamf_pro_id` to bridge from Platform Services UUIDs. Setting `all_computers = true` forbids `computer_ids`, `computer_group_ids`, `building_ids`, `department_ids`. Setting `all_jss_users = true` forbids `user_ids` and `user_group_ids`. `user_ids` / `user_group_ids` map to the admin UI's \"Users\" / \"User Groups\" lists.",
				Optional:            true,
				Attributes:          scope.ComputerScopeAttributes(scope.ComputerScopeOptions{IncludeIbeacons: true}),
			},
			"self_service": schema.SingleNestedAttribute{
				MarkdownDescription: "Self Service integration. Pair `display_notifications` with `notification_location` to control whether and where Self Service surfaces a notification when the policy becomes available. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"use_for_self_service":          optComputedBool("Expose the policy in Self Service. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"self_service_display_name":     optComputedString("Self Service display name (defaults to the policy name). Omit to leave the current value untouched."),
					"install_button_text":           optComputedString("Install-button label. Defaults to `Install`. Omit to leave the current value untouched."),
					"reinstall_button_text":         optComputedString("Re-install-button label. Defaults to `Reinstall`. Omit to leave the current value untouched."),
					"self_service_description":      optComputedString("Self Service description. Markdown supported. Omit to leave the current value untouched."),
					"ensure_users_view_description": optComputedBool("Force users to view the description before installing. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"include_in_featured_category":  optComputedBool("Feature the policy on the Self Service main page. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"display_notifications":         optComputedBool("Whether Self Service surfaces a notification when the policy becomes available. Pair with `notification_location` to set the delivery target. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"notification_location": schema.StringAttribute{
						MarkdownDescription: "Notification delivery location. Valid values: `Self Service`, `Self Service and Notification Center`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf("Self Service", "Self Service and Notification Center"),
						},
					},
					"notification_subject": optComputedString("Notification subject line. Omit to leave the current value untouched."),
					"notification_message": optComputedString("Notification body text. Omit to leave the current value untouched."),
					"self_service_icon": schema.SingleNestedAttribute{
						MarkdownDescription: "Self Service icon. The icon binary is uploaded out-of-band; the provider surfaces the resolved id, URI, and filename. Uploading the icon bytes inline is not currently supported. Open an issue if you need it.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"id":       optComputedString("Icon ID assigned by Jamf Pro."),
							"uri":      optComputedString("Icon URI. Returned by Jamf Pro."),
							"filename": optComputedString("Icon filename. Returned by Jamf Pro."),
						},
					},
					"categories": schema.SetNestedAttribute{
						MarkdownDescription: "Self Service categories under which the policy appears. Each entry carries its own `display_in` / `feature_in` flags, mirroring the admin UI's parallel \"Display in\" / \"Feature in\" columns. A policy may appear in multiple categories. Declaring one or more entries replaces the stored list. An empty list reads as an omission. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{MarkdownDescription: "Category ID.", Required: true},
								// `name` is server-derived from the (mutable) `id`. It MUST be
								// plain Computed — NOT Optional+Computed+UseNonNullStateForUnknown.
								// In a Set, flipping a sibling element's flag changes that
								// element's hash, so the framework can't pair it to prior state;
								// a USFU-carried known `name` then mis-correlates and trips
								// "produced an inconsistent result after apply … does not correlate".
								// Leaving `name` Unknown at plan defers Set correlation cleanly.
								"name":       schema.StringAttribute{MarkdownDescription: "Category display name. Returned by Jamf Pro.", Computed: true},
								"display_in": optComputedBool("Display the policy in this category."),
								"feature_in": optComputedBool("Feature the policy in this category."),
							},
						},
					},
				},
			},
			"packages": schema.SingleNestedAttribute{
				MarkdownDescription: "Packages to install / cache / remove. Mirrors the admin UI's Options ▸ Packages section. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"distribution_point": schema.StringAttribute{
						MarkdownDescription: "Name of the file share distribution point the policy uses. On create Jamf Pro falls back to the tenant default. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"packages": schema.SetNestedAttribute{
						MarkdownDescription: "Set of package assignments. Each item identifies the package by ID; `name` is returned by Jamf Pro. `action` is one of `Install`, `Cache`, `Install Cached`, `Uninstall`. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":             schema.StringAttribute{MarkdownDescription: "Package ID.", Required: true},
								"name":           optComputedString("Package display name. Returned by Jamf Pro."),
								"action":         optComputedString("Package action."),
								"fut":            optComputedBool("Fill user template at install time."),
								"feu":            optComputedBool("Fill existing user accounts."),
								"update_autorun": optComputedBool("Update autorun data."),
							},
						},
					},
				},
			},
			"scripts": schema.SingleNestedAttribute{
				MarkdownDescription: "Scripts to run as part of the policy. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"scripts": schema.SetNestedAttribute{
						MarkdownDescription: "Set of script assignments. `priority` is one of `Before`, `After`, `At Reboot`. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":           schema.StringAttribute{MarkdownDescription: "Script ID.", Required: true},
								"name":         optComputedString("Script display name. Returned by Jamf Pro."),
								"priority":     optComputedString("Run order."),
								"parameter_4":  optComputedString("Parameter 4 passed to the script."),
								"parameter_5":  optComputedString("Parameter 5 passed to the script."),
								"parameter_6":  optComputedString("Parameter 6 passed to the script."),
								"parameter_7":  optComputedString("Parameter 7 passed to the script."),
								"parameter_8":  optComputedString("Parameter 8 passed to the script."),
								"parameter_9":  optComputedString("Parameter 9 passed to the script."),
								"parameter_10": optComputedString("Parameter 10 passed to the script."),
								"parameter_11": optComputedString("Parameter 11 passed to the script."),
							},
						},
					},
				},
			},
			"printers": schema.SingleNestedAttribute{
				MarkdownDescription: "Printers to install or remove. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"printers": schema.SetNestedAttribute{
						MarkdownDescription: "Set of printer assignments. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":   schema.StringAttribute{MarkdownDescription: "Printer ID.", Required: true},
								"name": optComputedString("Printer display name. Returned by Jamf Pro."),
								"action": schema.StringAttribute{
									MarkdownDescription: "Action. Mirrors the admin UI dropdown: `Map` (install printer) or `Unmap` (uninstall printer).",
									Optional:            true,
									Computed:            true,
									PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
									Validators: []validator.String{
										stringvalidator.OneOf("Map", "Unmap"),
									},
								},
								"make_default": optComputedBool("Make this printer the device's default."),
							},
						},
					},
				},
			},
			"dock_items": schema.SingleNestedAttribute{
				MarkdownDescription: "Dock items to add or remove. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"dock_items": schema.SetNestedAttribute{
						MarkdownDescription: "Set of dock item assignments. Omit to leave any existing entries untouched; they are not cleared on update.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id":     schema.StringAttribute{MarkdownDescription: "Dock item ID.", Required: true},
								"name":   optComputedString("Dock item display name. Returned by Jamf Pro."),
								"action": optComputedString("Action (`Add To Beginning`, `Add To End`, `Remove`)."),
							},
						},
					},
				},
			},
			// account_maintenance is flattened into four UI-aligned peer blocks
			// to mirror the admin UI's Options sidebar (Local Accounts /
			// Management Accounts / Directory Bindings / EFI Password). The
			// classic wire still nests all four under a single
			// <account_maintenance> object — the input/state builders join
			// these four fields on write and split them on read.
			"local_accounts": schema.ListNestedAttribute{
				MarkdownDescription: "Local account operations (admin UI: Options ▸ Local Accounts). Each `password` is a Terraform `WriteOnly` attribute: sent to Jamf Pro on writes, never persisted in state. Pair it with `password_wo_version` to rotate. Modelled as a List rather than a Set so the `WriteOnly` attribute is permitted inside each element; Jamf Pro matches accounts by `username`, and the order has no semantic effect. Omit to leave any existing entries untouched; they are not cleared on update.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							MarkdownDescription: "Account action. Valid values: `Create`, `Reset`, `Delete`, `DisableFileVault`. The admin UI labels the last action \"Disable FileVault\"; supply `DisableFileVault` here, with no trailing `2`. Jamf Pro silently strips `DisableFileVault` account entries on both create and update: they do not round-trip, and the policy reports no such account afterwards. That holds regardless of whether the username exists, which sibling accounts are present, or what extra fields are sent. Only the Jamf Pro web UI can persist this action. Avoid `DisableFileVault` in Terraform-managed policies; it produces a perpetual diff.",
							Optional:            true,
							Computed:            true,
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
							Validators: []validator.String{
								stringvalidator.OneOf(proclassic.PolicyAccountMaintenanceAccountsAccountItemActionValues()...),
							},
						},
						"username": optComputedString("Account username."),
						"realname": optComputedString("Account real (full) name."),
						"password": schema.StringAttribute{
							MarkdownDescription: "Plaintext password used by `Create` and `Reset` actions. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**. Pair with `password_wo_version` to rotate the stored password. `local_accounts` is a List, so bumping `password_wo_version` surfaces in `terraform plan` as an in-place change to the list element at the matching index. Jamf Pro matches accounts by `username`.",
							Optional:            true,
							Sensitive:           true,
							WriteOnly:           true,
						},
						"password_wo_version": schema.Int64Attribute{
							MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer (any change) to force a new apply that re-sends `password` to Jamf Pro for this account. Set `password_wo_version = 1` on create. Leaving it unset or unchanged signals \"leave the stored password alone\": the provider omits the password from the next update, so Jamf Pro retains the existing value.",
							Optional:            true,
						},
						"permanently_delete_home_directory": optComputedBool("Permanently delete the home directory when `action = \"Delete\"`. When true, the home is removed; when false (or unset), the home is archived to `archive_home_directory_to`. Mirrors the admin UI checkbox \"Permanently delete home directory\"."),
						"archive_home_directory_to":         optComputedString("Destination for the archived home directory. Only meaningful when `permanently_delete_home_directory = false`."),
						"home":                              optComputedString("Home directory path. **Required in configuration when `action = \"Create\"`**: Jamf Pro refuses a create-account entry without one, answering `409 Problem with create account fields` and naming no field, so the provider checks for it when you plan. The check reads configuration rather than state, so `home` cannot be dropped from a `Create` entry and left to the Computed carry-over, even on a policy that already applied."),
						"hint":                              optComputedString("Password hint."),
						"picture":                           optComputedString("Account picture path."),
						"admin":                             optComputedBool("Whether the account is an admin."),
						"filevault_enabled":                 optComputedBool("Whether FileVault 2 is enabled for the account."),
						"secure_token_allowed":              optComputedBool("Whether the account is allowed to hold a Secure Token."),
					},
				},
			},
			"management_account": schema.SingleNestedAttribute{
				MarkdownDescription: "Management account configuration (admin UI: Options ▸ Management Accounts). Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"action": optComputedString("Management account action (e.g. `doNotChange`, `rotate`). Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it."),
					"managed_password": schema.StringAttribute{
						MarkdownDescription: "Plaintext managed password. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**. Pair with `managed_password_wo_version` to rotate the stored password.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"managed_password_wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `managed_password`. Bump this integer (any change) to force a new apply that re-sends `managed_password` to Jamf Pro. Set `managed_password_wo_version = 1` on create. Leaving it unset or unchanged signals \"leave the stored password alone\": the provider omits the password from the next update, so Jamf Pro retains the existing value.",
						Optional:            true,
					},
				},
			},
			"directory_bindings": schema.SetNestedAttribute{
				MarkdownDescription: "Directory binding assignments (admin UI: Options ▸ Directory Bindings). Omit to leave any existing entries untouched; they are not cleared on update.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{MarkdownDescription: "Directory binding ID.", Required: true},
						"name": optComputedString("Directory binding display name. Returned by Jamf Pro."),
					},
				},
			},
			"efi_password": schema.SingleNestedAttribute{
				MarkdownDescription: "Open Firmware / EFI password configuration (admin UI: Options ▸ EFI Password). Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"of_mode": schema.StringAttribute{
						MarkdownDescription: "Open Firmware / EFI password mode. `command` requires a password to boot from anything but the startup disk; `none` clears the policy's EFI password payload. Jamf Pro accepts no other value: given one it resets the whole block to `none` and **clears the stored password**, without reporting an error. The provider checks the value at plan time.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(proclassic.PolicyAccountMaintenanceOpenFirmwareEfiPasswordOfModeValues()...),
						},
					},
					"of_password": schema.StringAttribute{
						MarkdownDescription: "Plaintext Open Firmware / EFI password. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**. Pair with `of_password_wo_version` to rotate the stored password.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"of_password_wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `of_password`. Bump this integer (any change) to force a new apply that re-sends `of_password` to Jamf Pro. Set `of_password_wo_version = 1` on create. Leaving it unset or unchanged signals \"leave the stored password alone\": the provider omits the password from the next update, so Jamf Pro retains the existing value.",
						Optional:            true,
					},
				},
			},
			"restart_options": schema.SingleNestedAttribute{
				MarkdownDescription: "Reboot configuration after the policy completes. Mirrors the admin UI's Options ▸ Restart Options section. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"message":      optComputedString("Reboot prompt message. Omit to leave the current value untouched."),
					"startup_disk": optComputedString("Startup disk label. Omit to leave the current value untouched."),
					"specify_startup": schema.StringAttribute{
						MarkdownDescription: "Reboot-method discriminator. Empty string is the default: a standard reboot with no explicit method. `Standard Restart` matches the admin UI radio option. `MDM Restart with Kernel Cache Rebuild` issues an MDM-driven restart that rebuilds the kernel cache. The admin UI surfaces a separate \"KEXT PATH\" text input alongside the radio, but Jamf Pro does not echo that value back, so it is not exposed here. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf("", "Standard Restart", "MDM Restart with Kernel Cache Rebuild"),
						},
					},
					"no_user_logged_in":              optComputedString("Action when no user is logged in. Omit to leave the current value untouched."),
					"user_logged_in":                 optComputedString("Action when a user is logged in. Omit to leave the current value untouched."),
					"delay_minutes":                  optComputedInt("Minutes to wait before forcing reboot. Mirrors the admin UI \"Delay\" input. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it."),
					"start_reboot_timer_immediately": optComputedBool("Start the reboot countdown immediately. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"file_vault_2_reboot":            optComputedBool("Trigger a FileVault 2 reboot. Omit to leave the current value untouched; set `true`/`false` to change it."),
				},
			},
			"maintenance": schema.SingleNestedAttribute{
				MarkdownDescription: "Maintenance tasks to run as part of the policy. Attribute names mirror the Jamf Pro admin UI checkbox labels. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"update_inventory":        optComputedBool("Update inventory. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"reset_computer_names":    optComputedBool("Reset computer names. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"install_cached_packages": optComputedBool("Install cached packages. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"fix_disk_permissions":    optComputedBool("Fix disk permissions. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"fix_byhost_files":        optComputedBool("Fix ByHost files. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"flush_system_caches":     optComputedBool("Flush system caches. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"flush_user_caches":       optComputedBool("Flush user caches. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"verify_startup_disk":     optComputedBool("Verify startup disk. Omit to leave the current value untouched; set `true`/`false` to change it."),
				},
			},
			"files_and_processes": schema.SingleNestedAttribute{
				MarkdownDescription: "File and process operations (admin UI: Options ▸ Files and Processes). Attribute names mirror the Jamf Pro admin UI labels. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"search_by_path":         optComputedString("Path to search for. Mirrors the admin UI \"Search for File by Path\" input. Omit to leave the current value untouched."),
					"delete_file_if_found":   optComputedBool("Delete files matching the search criteria if found. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"search_by_filename":     optComputedString("File name to search for. Mirrors the admin UI \"Search for File by Filename\" input. Omit to leave the current value untouched."),
					"update_locate_database": optComputedBool("Update the locate database before searching. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"search_by_spotlight":    optComputedString("Spotlight query. Mirrors the admin UI \"Search for File Using Spotlight\" input. Omit to leave the current value untouched."),
					"search_for_process":     optComputedString("Process name to search for. Omit to leave the current value untouched."),
					"kill_process_if_found":  optComputedBool("Kill processes matching the search if found. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"execute_command":        optComputedString("Command to execute. Mirrors the admin UI \"Execute Command\" input. Omit to leave the current value untouched."),
				},
			},
			"user_interaction": schema.SingleNestedAttribute{
				MarkdownDescription: "User interaction prompts shown around policy execution. The \"Deferral Type\" dropdown (None / Date / Duration) is modelled as `deferral_type`, with `deferral_until_utc` (Date form) and `deferral_days` (Duration form) as type-specific siblings. Switching between deferral types is an in-place change. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"start_message": optComputedString("Message displayed before the policy runs. Mirrors the admin UI \"Start Message\" input."),
					"deferral_type": schema.StringAttribute{
						MarkdownDescription: "User deferral mode. Mirrors the admin UI \"Deferral Type\" dropdown. One of:\n  - `none`: no deferral allowed, and the policy runs without prompting.\n  - `date`: deferral allowed until `deferral_until_utc`; the policy runs after that cut-off regardless.\n  - `duration`: deferral allowed for `deferral_days` days from the first prompt.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf("none", "date", "duration"),
							DeferralTypeCompanionsValidator(),
						},
					},
					// deferral_until_utc and deferral_days are plain Optional (no
					// Computed, no UseNonNullStateForUnknown) — value-discriminated
					// siblings of deferral_type. The companions must clear to null
					// in the plan on a type transition (e.g. Date→Duration removes
					// `deferral_until_utc`); UseNonNullStateForUnknown would
					// resurrect the prior-step value and trip the cross-field
					// validator. The server never independently surfaces these
					// (their wire values are always derived from deferral_type),
					// so Computed buys nothing.
					"deferral_until_utc": schema.StringAttribute{
						MarkdownDescription: "Date/time at which deferrals are prohibited and the policy runs. ISO-8601 with millisecond precision and a four-digit numeric offset (e.g. `2027-01-01T01:00:00.000+0000`). Required when `deferral_type = \"date\"`; forbidden otherwise.",
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(
								deferralUntilUtcPattern,
								"Value must be ISO-8601 with millisecond precision and a four-digit numeric offset (e.g. 2027-01-01T01:00:00.000+0000)",
							),
						},
					},
					"deferral_days": schema.Int64Attribute{
						MarkdownDescription: "Number of days the user may defer the policy after the first prompt. Mirrors the admin UI \"Duration\" input (in days). Required when `deferral_type = \"duration\"`; forbidden otherwise.",
						Optional:            true,
					},
					"complete_message": optComputedString("Message displayed after the policy completes. Mirrors the admin UI \"Complete Message\" input."),
				},
			},
			"disk_encryption": schema.SingleNestedAttribute{
				MarkdownDescription: "Disk encryption configuration to apply. Omit the block to leave any existing values untouched (they are not cleared on update).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"action":                           optComputedString("Disk encryption action (`apply`, `remediate`, `none`). Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it."),
					"disk_encryption_configuration_id": optComputedInt("Disk encryption configuration ID to apply. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it."),
					"auth_restart":                     optComputedBool("Use authenticated restart. Omit to leave the current value untouched; set `true`/`false` to change it."),
					"remediate_key_type":               optComputedString("Key type for remediation (`Individual`, `Institutional`, `Individual And Institutional`). Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it."),
					"remediate_disk_encryption_configuration_id": optComputedInt("Disk encryption configuration ID used to remediate. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it."),
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *PolicyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_policy")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client

	// Pro (v1) client for the scope directory-service group preflight.
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_policy")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if proClient != nil {
		r.ldapSearcher = proClient
	}

	// Impact alerts are advisory and produce no diagnostics of their own here;
	// a nil cache means they are switched off.
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
}

// ImportState handles import by the Jamf Pro policy ID.
func (r *PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ConfigValidators returns the resource's plan-time cross-field checks.
func (r *PolicyResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		retryRequiresOncePerComputerValidator{},
		createAccountRequiresHomeValidator{},
	}
}

// ModifyPlan runs the plan-time directory-service user-group preflight on the
// policy scope limitations/exclusions — surfacing an unknown group as a clear
// plan error instead of the apply-time 409 ("Problem matching limitation user
// group"). Best-effort: search errors / unconfigured LDAP downgrade to a
// warning. No-op on destroy and when no scope groups are declared.
func (r *PolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportScopeImpact(ctx, req, resp)

	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}
	var plan PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Scope == nil {
		return
	}
	scopeRoot := path.Root("scope")
	if plan.Scope.Limitations != nil {
		resp.Diagnostics.Append(scope.ValidateDirectoryServiceUserGroupNames(
			ctx, r.ldapSearcher, plan.Scope.Limitations.DirectoryServiceUserGroupNames,
			scopeRoot.AtName("limitations").AtName("directory_service_user_group_names"),
		)...)
	}
	if plan.Scope.Exclusions != nil {
		resp.Diagnostics.Append(scope.ValidateDirectoryServiceUserGroupNames(
			ctx, r.ldapSearcher, plan.Scope.Exclusions.DirectoryServiceUserGroupNames,
			scopeRoot.AtName("exclusions").AtName("directory_service_user_group_names"),
		)...)
	}
}

// reportScopeImpact emits the plan-time impact alert for a scope change. A policy
// is a deployable object in Jamf Pro's terms, so the alert reports how many
// computers the change reaches.
func (r *PolicyResource) reportScopeImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	impact.ReportPlan(ctx, req, resp, impact.PlanReport{
		Cache: r.impact,
		Path:  path.Root("scope"),
		Label: "policy",
	}, func(ctx context.Context, m *PolicyResourceModel) impact.Scope {
		return scope.ComputerImpactScope(ctx, m.Scope)
	})
}

// optComputedString returns an Optional+Computed StringAttribute with the
// UseNonNullStateForUnknown plan modifier for server-augmented fields.
// Field-level construction helper, not a schema-section decomposition —
// STYLE_GUIDE §Schema keeps the section bodies inline.
//
// Why UseNonNullStateForUnknown and not UseStateForUnknown: these helpers
// are used both at top level and inside nested list elements. When a list
// element is appended (list-length growth), the new index has no prior
// state at its path — prior StateValue is Null. UseStateForUnknown copies
// that Null into the plan; if the server returns a real value for the
// field, the post-apply consistency check trips ("Provider produced
// inconsistent result after apply"). When a Sensitive sibling lives on
// the same nested element (e.g. a WriteOnly password), the error path
// is redacted up to the nearest non-sensitive ancestor, masking the
// real attribute. UseNonNullStateForUnknown leaves the plan Unknown when
// prior StateValue is Null, so the framework accepts whatever the
// server returns. Behavior is identical to UseStateForUnknown for the
// non-Null prior-state case (singletons, already-set values).
// noExecuteTimeAttribute builds the schema for one end of the daily no-execute
// window.
//
// Deliberately plain: from a configuration's point of view this is an ordinary
// 12-hour time, which is what it accepts and what it reads back. The
// contortion needed to make Jamf Pro actually store it lives in no_execute.go
// and is not a practitioner's problem.
func noExecuteTimeAttribute(edge, example string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: fmt.Sprintf(
			"Daily %s of the no-execute window, in 12-hour `h:MM AM` / `h:MM PM` form with the hour 1–12 and no leading zero (e.g. `%s`). "+
				"Shown in the admin UI under Options ▸ General ▸ Server-Side Limitations. Applies on the days named by `no_execute_on`.",
			edge, example,
		),
		Optional:      true,
		Computed:      true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
		Validators: []validator.String{
			stringvalidator.RegexMatches(
				noExecuteTimePattern,
				fmt.Sprintf("Value must be 12-hour h:MM AM / h:MM PM (e.g. %s)", example),
			),
		},
	}
}

// serverProjectedString is the read for a `general` attribute Jamf Pro derives
// from somewhere else and refuses to accept a write for. Computed-only: the
// value is always echoed, so it is read straight from the wire and reports
// drift, but Terraform will not let a practitioner set it. Each call site's
// description names the attribute that actually writes it.
//
// The write really is refused rather than merely normalised — wire-probed
// against Jamf Pro 11.31.1 on two tenants, on create and on update, including
// on a policy that already held a non-default value, and identically through
// raw XML and the SDK. See issue #387.
func serverProjectedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Computed:            true,
	}
}

// serverProjectedBool is the bool sibling of serverProjectedString.
func serverProjectedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Computed:            true,
	}
}

func optComputedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
	}
}

// optComputedBool is the bool sibling of optComputedString. Uses
// UseNonNullStateForUnknown for the same nested-list-growth reason —
// see the optComputedString doc comment.
func optComputedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
	}
}

// optComputedInt is the int64 sibling of optComputedString. Uses
// UseNonNullStateForUnknown for the same nested-list-growth reason —
// see the optComputedString doc comment.
func optComputedInt(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseNonNullStateForUnknown()},
	}
}
