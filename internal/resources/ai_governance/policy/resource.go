// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package policy implements the jamfplatform_ai_governance_policy resource and data sources backed
// by the Jamf Platform AI Governance policies API.
//
// An AI policy is the managed configuration for one AI tool — Claude Code, Claude Desktop or
// OpenAI Codex today — which Jamf Pro then delivers to Macs through a blueprint. The settings body
// is the tool vendor's own, not Jamf's: its shape is declared by a JSON Schema the platform serves
// per tool per schema version, so the provider carries it as a JSON string rather than as typed
// attributes. internal/common/aischemas explains why, and validates the body at plan time.
//
// Two behaviours drive the whole design and are not obvious from the schema:
//
// A policy has a draft and a published history. Creating or updating a policy saves a draft and
// deploys nothing; publishing snapshots the draft as an immutable version, and a blueprint pins a
// version number rather than the policy. So `publish` defaults to true — an applied policy that
// cannot be deployed is not what `terraform apply` should mean. The platform diffs the settings
// itself, so an update that changes nothing raises no draft and the publish that follows is a
// no-op rather than a wasted version.
//
// Deleting a policy archives it, and archiving is not blocked by a blueprint that references it:
// the platform accepts the delete and leaves the blueprint pointing at a version it will no longer
// serve. Nothing the provider can do about that at the policy end — re-point or destroy the
// blueprint component first. See docs/guides/ai-governance-policies.md.
//
// Attribute names follow the UI and the product documentation rather than the wire wherever they
// diverge, per STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI. Because the guide also
// forbids comments inside function bodies, the wire mapping lives here:
//
//	Terraform attribute   Wire field
//	-------------------   --------------------
//	settings_json         settings
//	published_version     currentVersionNumber
//	has_draft             hasDraft
//	schema_drift          schemaDrift
//	publish               (none — drives POST /policies/{id}/publish)
//
// `tool_id` keeps the wire spelling deliberately. The policy detail page labels it "Product", but
// Jamf's own AI Governance documentation calls these AI tools throughout, the catalogue endpoint is
// /tools, and the platform reports an unknown one as TOOL_ID_UNKNOWN — so there is no single UI name
// to defer to, and matching the diagnostic the practitioner will see wins.
//
// `status` is read but exposed as no attribute: a policy reported as archived is treated as absent,
// which is defence against a service release that starts honouring its own specification. Probing on
// 2026-08-30 only ever saw an archived policy answer 404, but the specification declares archived as
// a status a read may report, and the difference decides whether a deleted policy plans as gone or as
// unchanged.
//
// Three wire fields are deliberately unmapped. `version` is a row-revision counter that increments
// on writes that change nothing, and the SDK's policy detail type does not carry it, so exposing it
// would need an SDK change before a provider one. `createdBy` and `updatedBy` are opaque composite
// actor identifiers that name the auth realm and appear nowhere in the UI.
package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/aischemas"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// Field bounds. The platform reports an over-long name or description as VALIDATION_FAILED naming
// the field, so both are checked at plan time instead of surfacing mid-apply.
const (
	maxNameLength        = 255
	maxDescriptionLength = 1000
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// PolicyResource implements the Terraform resource for Jamf AI Governance policies.
//
// privateOverride is a test seam and is nil in production, where every operation uses the private
// state the framework supplies. privateState says why one is needed: the framework's private-state
// type is internal with no exported constructor, so a request or response built in a unit test
// carries none, and the precondition an update sends would otherwise be pinned by nothing.
type PolicyResource struct {
	client          *aigovernance.Client
	schemas         *aischemas.Cache
	privateOverride privateState
}

var (
	_ resource.Resource                = &PolicyResource{}
	_ resource.ResourceWithImportState = &PolicyResource{}
	_ resource.ResourceWithIdentity    = &PolicyResource{}
	_ resource.ResourceWithModifyPlan  = &PolicyResource{}
)

// NewPolicyResource returns a new instance of PolicyResource.
func NewPolicyResource() resource.Resource {
	return &PolicyResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_governance_policy"
}

// IdentitySchema defines the identifier used for import.
func (r *PolicyResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Policy ID used to uniquely reference the policy.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the AI policy resource.
func (r *PolicyResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf AI Governance policy, the managed configuration for one AI tool " +
			"such as Claude Code, Claude Desktop or OpenAI Codex.\n\nA policy carries a draft and a history of " +
			"published versions. Applying a change saves the draft and, unless `publish` is disabled, publishes " +
			"it as a new version. Deploying a published version to devices is a separate step: add an **AI " +
			"Governance** component to a blueprint and reference the policy's `id` and `published_version`. " +
			"Nothing reaches a device until a blueprint that names the policy is deployed.\n\nThe " +
			"`settings_json` body is the tool vendor's own configuration format, checked during `terraform plan` " +
			"against the schema the platform serves for the tool and `schema_version`.\n\nSettings are written " +
			"whole, and Jamf holds no merge for them, so an update is made conditional on the policy still being " +
			"at the version last read. If something else changed the policy in the meantime, Jamf refuses the " +
			"update and a new plan shows you the change. A policy Jamf created before it started keeping that " +
			"counter has none, so Terraform updates it without the check. See the [AI Governance " +
			"policies guide](../guides/ai-governance-policies) for where each tool's settings are " +
			"documented." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Policy ID assigned by the platform. Reference this from a blueprint's AI Governance component.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Name\"** in the Jamf Account admin UI. Policy names are not required to be " +
					"unique, so prefer the policy ID when referencing a policy elsewhere.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, maxNameLength),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "**\"Description\"** in the Jamf Account admin UI: what this policy is for. " +
					"Unlike most optional values in this provider, taking it out of your configuration clears " +
					"the stored description rather than leaving it alone.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(maxDescriptionLength),
				},
			},
			"tool_id": schema.StringAttribute{
				MarkdownDescription: "**\"Product\"** in the Jamf Account admin UI: the identifier of the AI tool this " +
					"policy configures, such as `com.anthropic.claudecode`. Read the available identifiers from the " +
					"`jamfplatform_ai_governance_tools` data source. Changing the tool replaces the policy, because a " +
					"policy's tool is fixed once it exists.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"schema_version": schema.StringAttribute{
				MarkdownDescription: "The version of the tool's settings format that `settings_json` is written " +
					"against, such as `2026-08-14`. Must be one of the versions the platform offers for the " +
					"tool. Read them from the `jamfplatform_ai_governance_tool` data source. A policy left on an " +
					"older version than the tool's current one still works, and reports `schema_drift`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"settings_json": schema.StringAttribute{
				MarkdownDescription: "The tool's settings as a JSON object string. Author it with `jsonencode({ ... })`, " +
					"`file(\"settings.json\")`, or by copying the configuration exported from the Jamf Account admin " +
					"UI. Only the settings you include are managed; the tool's own defaults apply to the rest.\n\n" +
					"Formatting and key order are not significant: the value is compared as JSON, so reindenting it " +
					"produces no change. Contents are checked during `terraform plan` against the tool's published " +
					"schema for `schema_version`: a setting of the wrong type or outside its accepted values is an " +
					"error, and a setting the schema does not declare is a warning, because a tool may accept settings " +
					"added after that schema version was published.\n\n" +
					"Keep credentials out of these settings: the value is held in Terraform state and shown in plan " +
					"output in cleartext. Where a tool needs an API key, prefer a setting that names a command to " +
					"fetch the key over one that carries the key itself.",
				Required:   true,
				CustomType: jsonObjectType{},
				Validators: []validator.String{
					jsonObjectValidator(),
				},
			},
			"publish": schema.BoolAttribute{
				MarkdownDescription: "Whether to publish a new version after saving changes. Defaults to `true`, so an " +
					"applied policy is always deployable. Publishing only happens when something actually changed; " +
					"an apply that alters nothing does not mint a version. Set to `false` to stage changes as a draft " +
					"and publish them in the Jamf Account admin UI instead; `has_draft` then reports that unpublished " +
					"changes exist.\n\n" +
					"While this is enabled, a draft that already exists is published by the next apply, whether it " +
					"was left behind by a publish that failed or saved in the Jamf Account admin UI. Such a plan " +
					"shows `has_draft` and `published_version` as known after apply even when nothing else changed.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"published_version": schema.Int64Attribute{
				MarkdownDescription: "**\"Published version\"** in the Jamf Account admin UI: the number of the " +
					"most recently published version, counting from 1. Null until the policy is first published. " +
					"This is the value a blueprint's AI Governance component pins.\n\nAny change to the policy " +
					"plans this as known after apply, because whether a version is minted depends on how the " +
					"platform compares the settings, and on whether anyone published in the admin UI meanwhile. " +
					"An apply that publishes nothing leaves the number unchanged.",
				Computed: true,
			},
			"has_draft": schema.BoolAttribute{
				MarkdownDescription: "Whether the policy holds changes that have not been published. `false` after a " +
					"successful apply with `publish` enabled. A draft that outlives an apply is published by the next " +
					"apply while `publish` is enabled, whether publishing it failed or someone saved one in the Jamf " +
					"Account admin UI.",
				Computed: true,
			},
			"schema_drift": schema.BoolAttribute{
				MarkdownDescription: "Whether `schema_version` is behind the version the platform now offers for " +
					"the tool. The policy keeps working; moving it forward means setting `schema_version` to the " +
					"current version and reconciling `settings_json` with it.",
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the policy was created, in RFC 3339 format.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				MarkdownDescription: "When the policy was last changed, in RFC 3339 format.",
				Computed:            true,
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

// Configure wires the AI Governance client and the shared schema cache into the resource.
//
// AI Governance is a continuously-deployed platform service with no customer-tenant version, so
// there is no version gate. Environment is the only scope that reaches it, and both alternatives
// were probed on 2026-08-30: with no scope header the platform answers REQUEST_CONTEXT_NOT_PROVIDED,
// and under tenant scope it answers BAD_PERMISSIONS while a Jamf Pro request carrying the same
// tenant header succeeds — so the header is accepted and the refusal belongs to this namespace. By
// the rule this provider applies to that code (see CLAUDE.md), an unmapped route and a privilege gap
// are indistinguishable, so tenant scope is treated as unreachable and widening the gate to it needs
// a fresh probe, not a one-word edit.
func (r *PolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_ai_governance_policy", providerdata.AIGovernanceScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client = aigovernance.New(pd.Client)
	r.schemas = pd.AISchemaCache()
}

// ImportState handles import by the AI Governance policy ID.
func (r *PolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// policyModel is the Terraform model for an AI Governance policy.
type policyModel struct {
	ID               types.String    `tfsdk:"id"`
	Name             types.String    `tfsdk:"name"`
	Description      types.String    `tfsdk:"description"`
	ToolID           types.String    `tfsdk:"tool_id"`
	SchemaVersion    types.String    `tfsdk:"schema_version"`
	SettingsJSON     jsonObjectValue `tfsdk:"settings_json"`
	Publish          types.Bool      `tfsdk:"publish"`
	PublishedVersion types.Int64     `tfsdk:"published_version"`
	HasDraft         types.Bool      `tfsdk:"has_draft"`
	SchemaDrift      types.Bool      `tfsdk:"schema_drift"`
	CreatedAt        types.String    `tfsdk:"created_at"`
	UpdatedAt        types.String    `tfsdk:"updated_at"`
	Timeouts         timeouts.Value  `tfsdk:"timeouts"`
}
