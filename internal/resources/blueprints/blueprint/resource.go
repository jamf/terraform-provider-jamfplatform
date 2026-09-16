// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

// BlueprintResource implements the Terraform resource for Jamf Blueprint.
type BlueprintResource struct {
	client *blueprints.Client
	// impact backs the plan-time impact alert on device group targeting. nil when
	// the provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &BlueprintResource{}
var _ resource.ResourceWithImportState = &BlueprintResource{}
var _ resource.ResourceWithIdentity = &BlueprintResource{}
var _ resource.ResourceWithModifyPlan = &BlueprintResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// uuidRegex matches UUID strings used to validate device group IDs.
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// PLATFORM-DEPRECATED remove-after=2026-10-22 replaced-by=component_blocks — the flat top-level
// component attributes (and the top-level activation_conditions) are superseded by named, ordered
// component_blocks. On or after the date, batch-remove every attribute carrying
// componentAttrDeprecation plus this const, and land the schema Version bump + UpgradeState. See
// STYLE_GUIDE "Platform Services Terraform schema deprecation — 90-day window".
const componentAttrDeprecation = "Deprecated: use `component_blocks` instead. Superseded by named, ordered component blocks; may be removed on or after 2026-10-22."

// NewBlueprintResource returns a new instance of BlueprintResource.
func NewBlueprintResource() resource.Resource {
	return &BlueprintResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *BlueprintResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blueprints_blueprint"
}

// Schema returns the Terraform schema for the blueprint resource.
func (r *BlueprintResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             4,
		MarkdownDescription: "Manages a Jamf blueprint. Requires **Blueprints API** access." + resourcePrivileges,
		Attributes:          blueprintSchemaAttributes(ctx),
	}
}

// blueprintSchemaAttributes returns the current top-level attribute set. It is a function rather
// than an inline literal in Schema so blueprintSchemaV3 can derive the prior schema from it and
// change only the one attribute that changed, instead of transcribing every other.
func blueprintSchemaAttributes(ctx context.Context) map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "The unique identifier for the blueprint.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "Blueprint name.",
			Required:            true,
		},
		"description": schema.StringAttribute{
			MarkdownDescription: "Blueprint description. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
			Optional:            true,
		},
		"deployed": schema.BoolAttribute{
			MarkdownDescription: "Whether the blueprint should be deployed. If set to `true`, the provider will deploy the blueprint (and redeploy if it becomes `OUT_OF_DATE`). If set to `false`, the provider will undeploy the blueprint.",
			Required:            true,
		},
		"device_groups": schema.SetAttribute{
			MarkdownDescription: "Set of device group Platform IDs to target. Specified as a set of strings in UUID format.",
			Required:            true,
			ElementType:         types.StringType,
			Validators: []validator.Set{
				setvalidator.SizeAtLeast(1),
				setvalidator.ValueStringsAre(stringvalidator.RegexMatches(
					uuidRegex,
					"Each device group ID must be a valid UUID",
				)),
			},
		},
		"activation_conditions": schema.StringAttribute{
			MarkdownDescription: activationConditionsDescription("the blueprint") +
				" When using `component_blocks`, set the condition on each block instead of here.",
			Optional:           true,
			DeprecationMessage: componentAttrDeprecation,
			Validators: []validator.String{
				stringvalidator.LengthAtMost(10000),
			},
		},
		"created": schema.StringAttribute{
			MarkdownDescription: "Creation timestamp.",
			Computed:            true,
		},
		"updated": schema.StringAttribute{
			MarkdownDescription: "Last updated timestamp.",
			Computed:            true,
		},
		"deployment_state": schema.StringAttribute{
			MarkdownDescription: "Current deployment state.",
			Computed:            true,
		},
		"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
			Create: true,
			Read:   true,
			Update: true,
			Delete: true,
		}),
		"legacy_payloads": schema.DynamicAttribute{
			MarkdownDescription: "Legacy configuration profile payloads as a list of objects. Each object must have a `payload_type` key (Apple reverse-domain payload type, e.g. `com.apple.applicationaccess`) and an optional `settings` object containing the payload key-value pairs. Jamf Pro names each payload after the blueprint. Payload validation behaves as described on `component_blocks` → `legacy_payloads` → `settings`.",
			Optional:            true,
			DeprecationMessage:  componentAttrDeprecation,
			Validators: []validator.Dynamic{
				flatLegacyPayloadSchemaValidator(),
			},
		},
		"component_blocks": schema.ListNestedAttribute{
			MarkdownDescription: "Ordered list of component blocks. Each block appears as a step in the Jamf Blueprints editor, with its own name, " +
				"its own activation condition, and its own set of components. Blocks are applied in the order listed. " +
				"Use `component_blocks` instead of the deprecated top-level component attributes; the two cannot be combined.",
			Optional: true,
			Validators: []validator.List{
				listvalidator.ConflictsWith(flatComponentAttributePaths()...),
			},
			NestedObject: schema.NestedAttributeObject{
				Attributes: componentBlockAttributes(),
			},
		},
	}

	maps.Copy(attributes, sharedComponentAttributes(componentAttrDeprecation))

	return attributes
}

// activationConditionsDescription returns the shared MarkdownDescription for an activation-condition
// field, parameterised by what the condition applies to ("the blueprint" or "this block").
func activationConditionsDescription(appliesTo string) string {
	return "Optional activation condition expression that further restricts which scoped devices " + appliesTo + " applies to; " +
		"when omitted, " + appliesTo + " applies to every device in the targeted device groups. " +
		"See the [Activation Condition Expression Reference](https://learn.jamf.com/r/en-US/jamf-pro-blueprints-configuration-guide/Activation_Condition_Expression_Reference) for the syntax; " +
		"the easiest way to author one is to build the rule in the **Activation conditions** editor in the Jamf UI, switch to the **Text** view, and copy the expression here. " +
		"Device groups are referenced by Platform UUID, so ordinary interpolation keeps a condition in sync with a managed group, " +
		"e.g. `\"ANY @property(jamf.device.groups) IN {'${jamfplatform_device_group.example.id}'}\"`."
}

// sharedComponentAttributes returns the component attribute set shared by the deprecated flat
// top-level schema and each component block — the strongly-typed components plus raw_component, all
// of which the framework allows inside a collection. legacy_payloads is NOT included: it is a
// dynamic type at the top level (illegal inside a collection), so each caller supplies its own
// legacy_payloads shape. Passing a non-empty deprecation string marks every entry Deprecated (the
// flat top-level use); the block passes "" so the same attributes are current there.
func sharedComponentAttributes(deprecation string) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"raw_component": schema.SetNestedAttribute{
			MarkdownDescription: "Raw component configuration using key-value pairs. " +
				"Use it for a component this resource has no attribute for, or to manage one without its " +
				"attribute's checks. A component managed by its own attribute cannot also be declared here.",
			Optional:           true,
			DeprecationMessage: deprecation,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"identifier": schema.StringAttribute{
						MarkdownDescription: "Component identifier (e.g., `com.jamf.ddm.disk-management`).",
						Required:            true,
					},
					"configuration": schema.MapAttribute{
						MarkdownDescription: "Component configuration as key-value pairs. Each component has its own unique configuration options.",
						Optional:            true,
						ElementType:         types.StringType,
					},
				},
			},
		},
		"audio_accessory_settings": schema.SingleNestedAttribute{
			MarkdownDescription: "Audio accessory settings component for managing temporary pairing and unpairing policies.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.AudioAccessorySettingsComponentSchema(),
		},
		"custom_declarations": schema.SingleNestedAttribute{
			MarkdownDescription: "**\"Custom Declarations\"** in the Jamf Pro blueprint editor. " +
				"Manages custom declarative device management declarations with system or user channel types. " +
				"Prefer `apple_declarations`, which delivers the same declarations and renders them as typed forms in Jamf Pro.",
			Optional:           true,
			DeprecationMessage: deprecation,
			Attributes:         components.CustomDeclarationsComponentSchema(),
			Validators:         []validator.Object{customDeclarationsSchemaValidator()},
		},
		"disk_management_settings": schema.SingleNestedAttribute{
			MarkdownDescription: "Disk management settings component for controlling external and network storage restrictions.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.DiskManagementPolicyComponentSchema(),
		},
		"math_settings": schema.SingleNestedAttribute{
			MarkdownDescription: "Math settings component for managing calculator modes and system behavior.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.MathSettingsComponentSchema(),
		},
		"passcode_policy": schema.SingleNestedAttribute{
			MarkdownDescription: "Passcode policy component for managing device passcode requirements and restrictions.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.PasscodePolicyComponentSchema(),
		},
		"safari_bookmarks": schema.SingleNestedAttribute{
			MarkdownDescription: "Safari bookmarks component for managing Safari managed bookmarks and bookmark groups.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.SafariBookmarksComponentSchema(),
		},
		"safari_extensions": schema.SingleNestedAttribute{
			MarkdownDescription: "Safari extensions component for managing Safari extension permissions and states.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.SafariExtensionsComponentSchema(),
		},
		"safari_settings": schema.SingleNestedAttribute{
			MarkdownDescription: "Safari settings component for managing Safari browser behavior and security settings.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.SafariSettingsComponentSchema(),
		},
		"service_background_tasks": schema.SingleNestedAttribute{
			MarkdownDescription: "Service background tasks component for managing background service tasks and launchd configurations.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.ServiceBackgroundTasksComponentSchema(),
		},
		"service_configuration_files": schema.SingleNestedAttribute{
			MarkdownDescription: "Service configuration files component for managing configuration files for system services.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.ServiceConfigurationFilesComponentSchema(),
		},
		"software_update": schema.SingleNestedAttribute{
			MarkdownDescription: "Software update component for enforcing OS updates on devices.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.SoftwareUpdateComponentSchema(),
		},
		"software_update_settings": schema.SingleNestedAttribute{
			MarkdownDescription: "Software update settings component for configuring system update behavior and policies.",
			Optional:            true,
			DeprecationMessage:  deprecation,
			Attributes:          components.SoftwareUpdateSettingsComponentSchema(),
		},
	}
}

// componentBlockAttributes returns the schema attributes for one component block: block metadata
// (name, per-block activation condition) plus the shared, non-deprecated component set.
func componentBlockAttributes() map[string]schema.Attribute {
	attributes := map[string]schema.Attribute{
		"name": schema.StringAttribute{
			MarkdownDescription: "Name shown for this component block in the Jamf Blueprints editor (e.g. `Passcode Policy`). When omitted, the platform assigns a default name.",
			Optional:            true,
		},
		// apple_declarations is deliberately NOT in sharedComponentAttributes: the flat top-level
		// authoring style is deprecated and removed on or after 2026-10-22, so a component
		// introduced now is offered only inside a block.
		"apple_declarations": schema.ListNestedAttribute{
			MarkdownDescription: "**\"All Declarations\"** in the Jamf Pro blueprint editor. " +
				"Delivers any Apple declarative device management declaration, and the provider checks each payload " +
				"against Apple's published schemas during `plan`. Prefer this over `custom_declarations`: Jamf Pro " +
				"renders these as typed forms generated from Apple's schemas, where a custom declaration is an " +
				"opaque JSON blob. Ordered: a payload may reference another declaration in this list with " +
				"`$PAYLOAD_n`, where `n` is the referenced declaration's 1-based position.",
			Optional:   true,
			Validators: []validator.List{appleDeclarationsSchemaValidator()},
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"channel": schema.StringAttribute{
						MarkdownDescription: "The channel the declaration applies to. Valid values are `SYSTEM` (the device channel) and `USER`.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf(blueprints.DeclarationChannelTypeSystem, blueprints.DeclarationChannelTypeUser),
						},
					},
					"type": schema.StringAttribute{
						MarkdownDescription: "The Apple declaration type, for example `com.apple.configuration.passcode.settings`. " +
							"Matched exactly: Jamf Pro delivers nothing for a type spelled differently, including in case.",
						Required: true,
					},
					"payload": schema.StringAttribute{
						MarkdownDescription: "The declaration's payload as a JSON object string. Write it with `jsonencode({ ... })`, " +
							"or read a `.json` file with `file(\"${path.module}/example.json\")`. The provider keeps the formatting and " +
							"key order you wrote. Keys are Apple's own, spelled as Apple declares them." +
							components.AppleDeclarationsBehaviour,
						Required: true,
					},
				},
			},
		},
		"activation_conditions": schema.StringAttribute{
			MarkdownDescription: activationConditionsDescription("this block"),
			Optional:            true,
			Validators: []validator.String{
				stringvalidator.LengthAtMost(10000),
			},
		},
		"legacy_payloads": schema.ListNestedAttribute{
			MarkdownDescription: "Legacy configuration profile payloads in this block. Jamf Pro names each payload after the blueprint.",
			Optional:            true,
			Validators: []validator.List{
				blockLegacyPayloadSchemaValidator(),
			},
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"payload_type": schema.StringAttribute{
						MarkdownDescription: "Apple reverse-domain payload type, e.g. `com.apple.applicationaccess`.",
						Required:            true,
					},
					"settings": schema.StringAttribute{
						MarkdownDescription: "Payload key-value settings as a JSON object string. Author with `jsonencode({ ... })`. " + legacyPayloadSettingsBehaviour,
						Optional:            true,
					},
				},
			},
		},
	}

	attributes["ai_governance"] = schema.SingleNestedAttribute{
		MarkdownDescription: "AI Governance component. Delivers published AI policy versions, such as managed " +
			"Claude Code or OpenAI Codex settings, to the devices this blueprint targets. See the " +
			"[AI Governance policies guide](../guides/ai-governance-policies).",
		Optional:   true,
		Attributes: components.AIGovernanceComponentSchema(),
	}

	maps.Copy(attributes, sharedComponentAttributes(""))

	return attributes
}

// flatComponentAttributePaths lists the root paths of the deprecated flat component attributes,
// used to make `component_blocks` mutually exclusive with the flat top-level authoring style.
func flatComponentAttributePaths() []path.Expression {
	names := []string{
		"activation_conditions",
		"legacy_payloads",
		"raw_component",
		"audio_accessory_settings",
		"custom_declarations",
		"disk_management_settings",
		"math_settings",
		"passcode_policy",
		"safari_bookmarks",
		"safari_extensions",
		"safari_settings",
		"service_background_tasks",
		"service_configuration_files",
		"software_update",
		"software_update_settings",
	}
	paths := make([]path.Expression, 0, len(names))
	for _, name := range names {
		paths = append(paths, path.MatchRoot(name))
	}
	return paths
}

// IdentitySchema defines the blueprint identity used across CRUD and list.
func (r *BlueprintResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Blueprint ID used to uniquely reference Jamf blueprints.",
				RequiredForImport: true,
			},
		},
	}
}

// Configure sets up the API client for the resource from the provider configuration.
func (r *BlueprintResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_blueprints_blueprint", providerdata.BlueprintsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client = blueprints.New(pd.Client)
	r.impact = pd.ImpactCache()
}

// ModifyPlan marks the server-managed `updated` and `deployment_state`
// attributes as unknown whenever a change is planned. The service re-stamps
// `updated` on every write (and may transition `deployment_state` while a
// deployment reconciles), so their post-apply values cannot be predicted from
// prior state; without this, an in-place update fails with "Provider produced
// inconsistent result after apply" on the `updated` attribute. Create (nil prior
// state) and destroy (nil plan) are skipped — the attributes are already unknown
// on create and there is no planned state to modify on destroy — and a plan with
// no change is left untouched so the resource keeps showing an empty plan.
//
// It also refuses a flat-mode apply that would delete component blocks the diff cannot show — see
// checkFlatModeBlockLoss.
func (r *BlueprintResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Runs ahead of the create/destroy guard below: a blueprint entering or
	// leaving management changes what devices receive, so both are worth an
	// impact alert even though neither needs the unknown-marking that follows.
	r.reportDeviceGroupImpact(ctx, req, resp)

	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	if req.Plan.Raw.Equal(req.State.Raw) {
		return
	}

	resp.Diagnostics.Append(r.checkFlatModeBlockLoss(ctx, req.Plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("updated"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("deployment_state"), types.StringUnknown())...)
}

// checkFlatModeBlockLoss blocks a plan that would delete component blocks without showing them in
// the diff. The deprecated flat top-level attributes can only carry one block, so a read maps just
// the first one into state (see updateFlatComponentsFromAPI) — any further block is absent from
// prior state, invisible in the diff, and still deleted by the apply, which rewrites every block at
// once. Blocks that Terraform cannot show it is about to destroy are worth an extra read per plan,
// so this re-reads the blueprint whenever a flat-mode configuration has a planned change.
//
// The gate is the *planned configuration*, not prior state: a configuration that has moved to
// `component_blocks` is migrating and takes explicit ownership of every block it declares, so it is
// allowed through. That is also why the error enumerates the current blocks — it is the material the
// user needs to write that migration without dropping one.
//
// Block mode needs no guard: every block lands in state, so removals are already plan-visible.
func (r *BlueprintResource) checkFlatModeBlockLoss(ctx context.Context, plannedState tfsdk.Plan) diag.Diagnostics {
	var diags diag.Diagnostics

	if r.client == nil {
		return diags
	}

	var plan BlueprintResourceModel
	diags.Append(plannedState.Get(ctx, &plan)...)
	if diags.HasError() {
		return diags
	}

	if len(plan.ComponentBlocks) > 0 || !plan.hasFlatComponents() {
		return diags
	}

	id := plan.ID.ValueString()
	if id == "" {
		return diags
	}

	blueprint, err := r.client.GetBlueprint(ctx, id)
	if err != nil {
		diags.AddWarning(
			"Could not check this blueprint for component blocks Terraform cannot represent",
			"The configuration uses the deprecated top-level component attributes, which can only represent the first component block. "+
				"The provider could not re-read the blueprint to check whether it has more: "+err.Error()+
				". If it does, applying would delete them without showing the deletion in this plan. Migrate to `component_blocks` to manage every block.",
		)
		return diags
	}

	if len(blueprint.Steps) <= 1 {
		return diags
	}

	diags.AddError(
		"Blueprint has component blocks the configuration cannot represent",
		fmt.Sprintf("This blueprint has %d component blocks, but the configuration uses the deprecated top-level component attributes, which can only represent the first. "+
			"Applying would delete the rest, and because they cannot be held in state that deletion does not appear in this plan.\n\n"+
			"Component blocks currently on this blueprint:\n%s\n"+
			"Migrate to `component_blocks` and declare every block listed above, then apply. Leave a block out only if you mean to delete it.",
			len(blueprint.Steps), describeBlueprintBlocks(blueprint.Steps)),
	)

	return diags
}

// ImportState handles the import of existing Blueprint resources.
func (r *BlueprintResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
