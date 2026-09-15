// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package vpp_invitation implements the jamfplatform_pro_vpp_invitation resource,
// data source, and list resource backed by the Jamf ProClassic /vppinvitations
// API — user-based Volume Purchasing (VPP) invitations that register users with a
// VPP account so apps and books can be assigned to them. Device-based Apps &
// Books locations are a separate construct (jamfplatform_pro_volume_purchasing_location).
//
// Server semantics (wire-probed 2026-06-08):
//   - Create: POST id/0 returns 201 with an id-only body — GET-after to populate.
//   - Update: PUT returns 201 with NO body — GET-after. Writes MERGE (omitting a
//     field/collection retains it; a present scope collection is full-replaced).
//   - General scalars are always-emitted so a removed value clears. Scope
//     follows per-category granular ownership: a declared category (including
//     `[]`, which clears) is owned by Terraform; an omitted (null) category is
//     preserved via a scope-only read-merge-write in Update (a scope PUT
//     replaces the whole subtree once any category element is present, even
//     empty — same classic wire family as /vppassignments, probed 2026-07-08;
//     this endpoint was not individually probed to avoid triggering real
//     invitation emails).
//   - distribution_method is one of three exact strings; "Send emails" requires
//     sender_name / sender_email_address / subject / message, and only then are
//     those fields (plus require_login) stored.
//   - limitations.user_groups and exclusions.user_groups are NAME-keyed
//     directory-service groups (PUT-by-id → 409); both get the plan-time DS
//     preflight. jss_users / jss_user_groups are id-keyed Jamf objects.
//   - invitation_usages is read-only server-tracked state.
package vpp_invitation

import (
	"context"
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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/ldapgroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const minJamfProVersion = ""

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// VPPInvitationResource implements the Terraform resource.
type VPPInvitationResource struct {
	client *proclassic.Client
	// ldapSearcher backs the plan-time scope directory-service user-group
	// preflight (Pro v1 LDAP search). Nil when the Pro client is unavailable.
	ldapSearcher ldapgroups.Searcher
}

var (
	_ resource.Resource                     = &VPPInvitationResource{}
	_ resource.ResourceWithImportState      = &VPPInvitationResource{}
	_ resource.ResourceWithIdentity         = &VPPInvitationResource{}
	_ resource.ResourceWithConfigValidators = &VPPInvitationResource{}
	_ resource.ResourceWithModifyPlan       = &VPPInvitationResource{}
)

// NewVPPInvitationResource returns a new instance.
func NewVPPInvitationResource() resource.Resource {
	return &VPPInvitationResource{}
}

func (r *VPPInvitationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_vpp_invitation"
}

func (r *VPPInvitationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro VPP invitation ID.",
				RequiredForImport: true,
			},
		},
	}
}

func (r *VPPInvitationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro VPP invitation, a user-based Volume Purchasing invitation that registers users with a VPP (Apple Business/School Manager) account so apps and books can be assigned to them.\n\n" +
			"Related: device-based Apps & Books locations are managed by `jamfplatform_pro_volume_purchasing_location`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "VPP invitation ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name for the invitation. Must not be empty.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"vpp_account_id": schema.StringAttribute{
				MarkdownDescription: "ID of the VPP (Apple Business/School Manager) account this invitation registers users with. Required. The VPP account itself is not managed by this provider.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"distribution_method": schema.StringAttribute{
				MarkdownDescription: "How the invitation is distributed. One of: `Prompt users to accept/make available in Self Service`, `Make available in Self Service only`, `Send emails`. `Send emails` requires `sender_name`, `sender_email_address`, `subject`, and `message`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(distributionMethods...)},
			},
			"auto_register_managed_users": schema.BoolAttribute{
				MarkdownDescription: "Automatically register users who have Managed Apple IDs with volume purchasing. Defaults to `true`.\n\n" +
					"Requires the referenced VPP location to have automatic registration enabled (`auto_register_managed_users = true` on `jamfplatform_pro_volume_purchasing_location`); otherwise Jamf Pro rejects `true` with \"not enabled on Vpp Location\". Set to `false` for locations without it.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"sender_name": schema.StringAttribute{
				MarkdownDescription: "Sender display name for the invitation email. Required (and only used) when `distribution_method` is `Send emails`.",
				Optional:            true,
			},
			"sender_email_address": schema.StringAttribute{
				MarkdownDescription: "Sender email address for the invitation email. Required (and only used) when `distribution_method` is `Send emails`.",
				Optional:            true,
			},
			"subject": schema.StringAttribute{
				MarkdownDescription: "Subject of the invitation email. Required (and only used) when `distribution_method` is `Send emails`.",
				Optional:            true,
			},
			"message": schema.StringAttribute{
				MarkdownDescription: "Body of the invitation email. Use `%@` where the registration URL should be inserted. Required (and only used) when `distribution_method` is `Send emails`.",
				Optional:            true,
			},
			"require_login": schema.BoolAttribute{
				MarkdownDescription: "Require users to log in with a directory-service or Jamf Pro account before enrolling. Only applies (and is only stored) when `distribution_method` is `Send emails`. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "User-based scope. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and updates preserve whatever is configured outside Terraform.",
				Optional:            true,
				Attributes:          scope.UserScopeAttributes(),
			},
			"invitation_usages": schema.ListNestedAttribute{
				MarkdownDescription: "Read-only per-user registration status Jamf Pro tracks for this invitation.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     computedString("Usage record ID."),
						"name":                   computedString("User name."),
						"email_address":          computedString("User email address."),
						"status":                 computedString("Registration status."),
						"last_action_date_utc":   computedString("Last action timestamp (UTC)."),
						"last_action_date_epoch": computedString("Last action timestamp (epoch milliseconds)."),
						"vpp_account":            computedString("VPP account name."),
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Read: true, Update: true, Delete: true}),
		},
	}
}

func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
}

func (r *VPPInvitationResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		emailModeRequiresFieldsValidator{},
	}
}

func (r *VPPInvitationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_invitation")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client

	// Pro (v1) client for the scope directory-service group preflight.
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_invitation")
	resp.Diagnostics.Append(proDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if proClient != nil {
		r.ldapSearcher = proClient
	}
}

// ModifyPlan runs the plan-time directory-service user-group preflight on the
// scope limitations / exclusions name sets — surfacing an unknown group as a
// clear plan error instead of the apply-time 409. Best-effort: search errors /
// unconfigured LDAP downgrade to a warning. No-op on destroy and when no scope
// groups are declared.
func (r *VPPInvitationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}
	var plan VPPInvitationResourceModel
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

func (r *VPPInvitationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
