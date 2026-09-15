// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package vpp_assignment implements the jamfplatform_pro_vpp_assignment resource,
// data source, and list resource backed by the Jamf ProClassic /vppassignments
// API — user-based Volume Purchasing (VPP) content assignment that assigns
// account-owned apps and books to Jamf users / user groups. The sibling
// jamfplatform_pro_vpp_invitation registers users with a VPP account; this
// resource assigns the actual content. Device-based Apps & Books locations are a
// separate construct (jamfplatform_pro_volume_purchasing_location).
//
// Server semantics (wire-probed 2026-06-08):
//   - Create: POST id/0 returns 201 with an id-only body — GET-after to populate.
//   - Update: PUT returns 201 with NO body — GET-after. Writes MERGE (omitting a
//     field/collection retains it; a present content collection is full-replaced;
//     an empty collection element clears it).
//   - General scalars (name, vpp_admin_account_id) are always-emitted.
//   - Content collections (ios_app_adam_ids / mac_app_adam_ids / ebook_adam_ids)
//     are OPT-OUT: null omits the block (retain), empty clears it, populated
//     full-replaces it. The three collections are independent. Content item name
//     is server-resolved — only adam_id is sent.
//   - Scope follows per-category granular ownership: a declared category
//     (including `[]`, which clears) is owned by Terraform; an omitted (null)
//     category is preserved via a scope-only read-merge-write in Update
//     (wire-probed 2026-07-08: a scope PUT replaces the whole subtree once any
//     category element is present, even empty). limitations / exclusions
//     directory_service user groups are NAME-keyed and get the plan-time DS
//     preflight; jss_users / jss_user_groups are id-keyed Jamf objects.
package vpp_assignment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
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

// VPPAssignmentResource implements the Terraform resource.
type VPPAssignmentResource struct {
	client *proclassic.Client
	// ldapSearcher backs the plan-time scope directory-service user-group
	// preflight (Pro v1 LDAP search). Nil when the Pro client is unavailable.
	ldapSearcher ldapgroups.Searcher
}

var (
	_ resource.Resource                = &VPPAssignmentResource{}
	_ resource.ResourceWithImportState = &VPPAssignmentResource{}
	_ resource.ResourceWithIdentity    = &VPPAssignmentResource{}
	_ resource.ResourceWithModifyPlan  = &VPPAssignmentResource{}
)

// NewVPPAssignmentResource returns a new instance.
func NewVPPAssignmentResource() resource.Resource {
	return &VPPAssignmentResource{}
}

func (r *VPPAssignmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_vpp_assignment"
}

func (r *VPPAssignmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro VPP assignment ID.",
				RequiredForImport: true,
			},
		},
	}
}

func (r *VPPAssignmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro VPP assignment, a user-based Volume Purchasing assignment that assigns account-owned apps and books to Jamf Pro users and user groups.\n\n" +
			"Related: `jamfplatform_pro_vpp_invitation` registers users with a VPP account; device-based Apps & Books locations are managed by `jamfplatform_pro_volume_purchasing_location`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "VPP assignment ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name for the assignment. Must not be empty.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"vpp_admin_account_id": schema.StringAttribute{
				MarkdownDescription: "ID of the VPP (Apple Business/School Manager) account (\"Location\") whose content this assignment distributes. Required. The VPP account itself is not managed by this provider. Changing this updates the assignment in place, but Jamf Pro refuses a move to a different account.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"vpp_admin_account_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the VPP account, resolved by Jamf Pro from `vpp_admin_account_id`.",
				Computed:            true,
			},
			"ios_app_adam_ids": schema.SetAttribute{
				MarkdownDescription: "Apple catalog adam IDs of the iOS apps to assign. Omit to leave the assignment's current iOS apps untouched; set to `[]` to clear all iOS apps; otherwise the listed apps replace the assignment's iOS apps. Jamf Pro resolves the app names.",
				Optional:            true,
				ElementType:         types.Int64Type,
			},
			"mac_app_adam_ids": schema.SetAttribute{
				MarkdownDescription: "Apple catalog adam IDs of the Mac apps to assign. Omit to leave the assignment's current Mac apps untouched; set to `[]` to clear all Mac apps; otherwise the listed apps replace the assignment's Mac apps. Jamf Pro resolves the app names.",
				Optional:            true,
				ElementType:         types.Int64Type,
			},
			"ebook_adam_ids": schema.SetAttribute{
				MarkdownDescription: "Apple catalog adam IDs of the books to assign. Omit to leave the assignment's current books untouched; set to `[]` to clear all books; otherwise the listed books replace the assignment's books. Jamf Pro resolves the book names.\n\n" +
					"Un-assigning a book removes it from the assignment, but Apple does not return or refund the underlying book license to the account. Book licenses are consumed irrevocably.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"scope": schema.SingleNestedAttribute{
				MarkdownDescription: "User-based scope. Each category is independently owned: declare it (including `[]`, which clears it) and Terraform manages its members; omit it and updates preserve whatever is configured outside Terraform.",
				Optional:            true,
				Attributes:          scope.UserScopeAttributes(),
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Read: true, Update: true, Delete: true}),
		},
	}
}

func (r *VPPAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_assignment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client

	// Pro (v1) client for the scope directory-service group preflight.
	proClient, proDiags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_vpp_assignment")
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
func (r *VPPAssignmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.ldapSearcher == nil || req.Plan.Raw.IsNull() {
		return
	}
	var plan VPPAssignmentResourceModel
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

func (r *VPPAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
