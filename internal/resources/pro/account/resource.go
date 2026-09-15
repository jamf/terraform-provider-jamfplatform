// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package account implements the jamfplatform_pro_account resource, data source,
// and list resource for Jamf Pro ADMINISTRATOR login accounts (the people who
// sign in to Jamf Pro). This is NOT the jamfplatform_pro_user inventory
// construct (end-user/device-assignment records).
//
// The resource is a hybrid across two Jamf APIs:
//   - Pro v1 /accounts: create (the only path that can set accountType, incl.
//     FEDERATED), base-field read, base-field update, delete.
//   - ProClassic /accounts/userid: the Custom privilege grid (read + write),
//     which the Pro API cannot express.
//
// Base-field updates route through Pro PUT; privilege-only updates route through
// classic. account_type is RequiresReplace (classic cannot change it; only Pro
// create sets it).
package account

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const minJamfProVersion = ""

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// AccountResource implements the Terraform resource for Jamf Pro admin accounts.
// It holds both clients: pro for base CRUD, proclassic for the privilege grid.
type AccountResource struct {
	proClient     *pro.Client
	classicClient *proclassic.Client
}

var _ resource.Resource = &AccountResource{}
var _ resource.ResourceWithImportState = &AccountResource{}
var _ resource.ResourceWithIdentity = &AccountResource{}
var _ resource.ResourceWithModifyPlan = &AccountResource{}

// NewAccountResource returns a new instance of AccountResource.
func NewAccountResource() resource.Resource {
	return &AccountResource{}
}

// Metadata sets the resource type name.
func (r *AccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_account"
}

// IdentitySchema defines the import/list identifier.
func (r *AccountResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro account ID.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the account resource.
func (r *AccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro **administrator login account**: a person who signs in to Jamf Pro. This is NOT the `jamfplatform_pro_user` inventory construct, which holds end-user and device records. Assign a Custom privilege grid through the `privileges` block. Base account fields (username, full name, email, access level and so on) are updated in place. Changing `account_type` forces the account to be replaced." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Account ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Account username (UI \"Username\"). Must not be empty.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"full_name": schema.StringAttribute{
				MarkdownDescription: "Full name (UI \"Full Name\"). Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email_address": schema.StringAttribute{
				MarkdownDescription: "Email address (UI \"Email Address\"). Must be unique across accounts; Jamf Pro rejects a duplicate on create. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"access_level": schema.StringAttribute{
				MarkdownDescription: "Access level (UI \"Access Level\"). One of `Full Access`, `Site Access`, or `Group Access`. Account-level `privileges` only apply when this is `Full Access` and `privilege_set` is `Custom`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(accessLevelValues...)},
			},
			"privilege_set": schema.StringAttribute{
				MarkdownDescription: "Privilege set (UI \"Privilege Set\"). One of `Administrator`, `Auditor`, `Enrollment Only`, or `Custom`. The `privileges` block is only applied when this is `Custom`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(privilegeSetValues...)},
			},
			"access_status": schema.StringAttribute{
				MarkdownDescription: "Account status (UI \"Access Status\"). One of `Enabled` or `Disabled`. Omit to leave the current value untouched; set `Enabled` or `Disabled` to change it.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.OneOf(accessStatusValues...)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"account_type": schema.StringAttribute{
				MarkdownDescription: "Account type. `DEFAULT` for a local or directory account; `FEDERATED` for an SSO/identity-provider account. Immutable, so changing it forces the account to be replaced.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.OneOf(accountTypeValues...)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
			},
			"ldap_server_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the backing LDAP / cloud-identity-provider server for a directory account. `-1` (the default) means a Jamf-Pro-local account. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"site_id": schema.Int64Attribute{
				MarkdownDescription: "Scoped site ID. `-1` means no site. Only meaningful for `Site Access` / `Group Access`. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"force_password_change": schema.BoolAttribute{
				MarkdownDescription: "Whether the user must change their password at next login (UI \"Force change at next login\"). Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Plaintext account password. `WriteOnly`: sent to Jamf Pro on writes, never persisted in Terraform state, and never returned by Jamf Pro. Required when creating a local (non-directory, non-federated) account. To rotate, change the value and bump `password_wo_version`.",
				Optional:            true,
				WriteOnly:           true,
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Set to `1` on create; bump it (any change) to force a base-field update that re-sends `password`. Unset/unchanged means \"leave the stored password alone\".",
				Optional:            true,
			},
			"privileges": accountprivileges.SchemaBlock(),
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// Configure wires both the Pro and ProClassic clients (one underlying
// jamfplatform.Client serves both surfaces).
func (r *AccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	proClient, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	classicClient, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.proClient = proClient
	r.classicClient = classicClient
}

// ImportState handles import by account ID.
func (r *AccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan validates declared privileges at plan time against the tenant's
// catalog (discovered from an Administrator account/group). Skipped on destroy,
// when privileges are absent, or when the privilege set is not Custom (Jamf Pro
// only honours the grid for Custom + Full Access accounts).
func (r *AccountResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.classicClient == nil {
		return
	}
	var plan AccountResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Privileges == nil || plan.Privileges.IsEmpty() {
		return
	}
	if plan.PrivilegeSet.ValueString() != "Custom" {
		return
	}

	privPath := path.Root("privileges")
	catalog, err := accountprivileges.Discover(ctx, r.classicClient)
	if err != nil {
		resp.Diagnostics.Append(accountprivileges.DiscoveryFailureWarning(privPath, err)...)
		return
	}
	resp.Diagnostics.Append(accountprivileges.Validate(ctx, catalog, plan.Privileges, privPath)...)
}

// custPrivApplicable reports whether account-level privileges apply to this
// account: Jamf Pro only honours them for Custom privilege set + Full Access.
func custPrivApplicable(privilegeSet, accessLevel types.String) bool {
	return privilegeSet.ValueString() == "Custom" && accessLevel.ValueString() == "Full Access"
}
