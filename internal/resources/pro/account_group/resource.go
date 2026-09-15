// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package account_group implements the jamfplatform_pro_account_group resource
// (ProClassic /accounts/groupid), data source, and list resource (Pro v1
// /account-groups). These are Jamf Pro ADMINISTRATOR permission groups — the
// groups whose members can sign in to Jamf Pro — and are unrelated to the
// jamfplatform_pro_user_group inventory construct.
package account_group

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/accountprivileges"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty: the classic /accounts endpoint predates the
// provider's overall floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// AccountGroupResource implements the Terraform resource for Jamf Pro
// administrator account groups.
type AccountGroupResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &AccountGroupResource{}
var _ resource.ResourceWithImportState = &AccountGroupResource{}
var _ resource.ResourceWithIdentity = &AccountGroupResource{}
var _ resource.ResourceWithModifyPlan = &AccountGroupResource{}

// NewAccountGroupResource returns a new instance of AccountGroupResource.
func NewAccountGroupResource() resource.Resource {
	return &AccountGroupResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AccountGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_account_group"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *AccountGroupResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro account group ID.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the account group resource.
func (r *AccountGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro **administrator account group**: a permission group whose members can sign in to Jamf Pro. This is NOT the `jamfplatform_pro_user_group` inventory construct, which groups end-user and device records." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Account group ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Group display name (UI \"Display Name\"). Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"access_level": schema.StringAttribute{
				MarkdownDescription: "Access level granted to members. One of `Full Access` or `Site Access`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(accessLevelValues...),
				},
			},
			"privilege_set": schema.StringAttribute{
				MarkdownDescription: "Privilege set. One of `Administrator`, `Auditor`, `Enrollment Only`, or `Custom`. The `privileges` block is only applied when this is `Custom`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(privilegeSetValues...),
				},
			},
			"site_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the site this group is scoped to. `-1` means no site (the default). Only meaningful when `access_level` is `Site Access`. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"site_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the scoped site. Read-only.",
				Computed:            true,
			},
			"ldap_server_id": schema.Int64Attribute{
				MarkdownDescription: "ID of the LDAP / cloud-identity-provider server backing this group, for directory-sourced membership. Omit for a Jamf-Pro-local group.",
				Optional:            true,
			},
			"ldap_server_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the backing directory server. Read-only.",
				Computed:            true,
			},
			"members": schema.SetAttribute{
				MarkdownDescription: "Account IDs that are members of this group. This is the authoritative side of admin-account-to-group membership (the account resource cannot read its own group membership back from Jamf Pro). For an LDAP-backed group, membership is directory-sourced; leave unset to let the directory manage it. Leave unset to not manage membership; set to `[]` to clear it.",
				Optional:            true,
				ElementType:         types.Int64Type,
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

// Configure wires the Jamf ProClassic client into the resource.
func (r *AccountGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_account_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro account group ID.
func (r *AccountGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan validates declared privileges at plan time against the tenant's
// privilege catalog (discovered from an Administrator account/group). Jamf Pro
// silently ignores unrecognised privileges, so without this a typo would surface
// as a perpetual diff. On discovery failure a loud warning is emitted
// (validation skipped) rather than blocking the plan.
//
// Skipped on destroy, when the privileges block is absent, and when
// privilege_set is not Custom — the same rule managesPrivileges applies to the
// write. A preset set never puts the grid on the wire, and one sent alongside a
// preset set is discarded (see customPrivilegeSet for the 2026-09-08 wire
// evidence), so refusing a privilege there would block a plan over a value Jamf
// Pro was never going to read. jamfplatform_pro_account gates its own ModifyPlan
// the same way.
func (r *AccountGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}
	var plan AccountGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || plan.Privileges == nil || plan.Privileges.IsEmpty() {
		return
	}
	if !customPrivilegeSet(plan.PrivilegeSet) {
		return
	}

	privPath := path.Root("privileges")
	catalog, err := accountprivileges.Discover(ctx, r.client)
	if err != nil {
		resp.Diagnostics.Append(accountprivileges.DiscoveryFailureWarning(privPath, err)...)
		return
	}
	resp.Diagnostics.Append(accountprivileges.Validate(ctx, catalog, plan.Privileges, privPath)...)
}
