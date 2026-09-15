// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package file_share_distribution_point implements the
// jamfplatform_pro_file_share_distribution_point resource, data source, and
// list resource backed by the Jamf Pro distribution points API. These are the
// on-prem SMB / AFP file servers Jamf Pro distributes packages from
// (Settings → Server → File share distribution points). This is a
// multi-instance CRUD resource — distinct from the one-per-tenant
// jamfplatform_pro_cloud_distribution_point singleton.
package file_share_distribution_point

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: file share distribution points are a long-standing feature
// that predates the provider's overall floor, so no per-resource gate is
// needed. Matches every other settings sibling.
const minJamfProVersion = ""

// Wire enum values for file_sharing_connection_type and https_security_type,
// aliased from the SDK's generated vocabularies rather than restated, so a spec
// change reaches them. These are the API's spellings, not the admin-UI labels.
const (
	connectionTypeAFP  = pro.DistributionPointFileSharingConnectionTypeAfp
	connectionTypeSMB  = pro.DistributionPointFileSharingConnectionTypeSmb
	connectionTypeNone = pro.DistributionPointFileSharingConnectionTypeNone

	httpsSecurityUsernamePassword = pro.DistributionPointHttpsSecurityTypeUsernamePassword
	httpsSecurityNone             = pro.DistributionPointHttpsSecurityTypeNone

	// Failover (backup_distribution_point_id) sentinels. A real failover is the
	// id of another file share distribution point (a positive integer); the two
	// negative values are special. Randomized load sharing is only valid with a
	// real file-share failover — neither sentinel.
	noneBackupSentinel  = "-1" // No failover distribution point.
	cloudBackupSentinel = "-2" // The Jamf Cloud distribution point.
)

// FileShareDistributionPointResource implements the Terraform resource for
// Jamf Pro file share distribution points.
type FileShareDistributionPointResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                     = &FileShareDistributionPointResource{}
	_ resource.ResourceWithImportState      = &FileShareDistributionPointResource{}
	_ resource.ResourceWithIdentity         = &FileShareDistributionPointResource{}
	_ resource.ResourceWithConfigValidators = &FileShareDistributionPointResource{}
	_ resource.ResourceWithModifyPlan       = &FileShareDistributionPointResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewFileShareDistributionPointResource returns a new instance.
func NewFileShareDistributionPointResource() resource.Resource {
	return &FileShareDistributionPointResource{}
}

// Metadata sets the resource type name.
func (r *FileShareDistributionPointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_file_share_distribution_point"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *FileShareDistributionPointResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro distribution point ID used to uniquely reference the distribution point.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *FileShareDistributionPointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro file share distribution point: an on-premises SMB or AFP file server (optionally with HTTPS downloads) that Jamf Pro distributes packages from (Settings → Server → File share distribution points). This is a multi-instance resource; for the hosted Jamf Cloud distribution point use `jamfplatform_pro_cloud_distribution_point`. The three plaintext passwords (`read_write_password`, `read_only_password`, `https_password`) are Terraform `WriteOnly` attributes, sent to Jamf Pro but never stored in state. Pair each with its `*_wo_version` companion to rotate the stored password: bump the integer to force an update that re-sends the current password." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Distribution point ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the distribution point. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"server_name": schema.StringAttribute{
				MarkdownDescription: "Hostname or IP address of the distribution point server. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"file_sharing_connection_type": schema.StringAttribute{
				MarkdownDescription: "File sharing protocol the distribution point uses (the **Protocol** field). One of `AFP`, `SMB`, or `NONE`. Use `NONE` for a distribution point that serves packages over HTTPS only; `https_enabled` must then be `true`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(connectionTypeAFP, connectionTypeSMB, connectionTypeNone),
				},
			},
			"principal": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the principal distribution point (the **Use as principal distribution point** option). Only one distribution point can be the principal at a time. Designating a second one moves the designation, silently clearing it from the previous principal. If two distribution points both set `principal = true`, the one that loses the designation will show a persistent diff; set it on exactly one. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"backup_distribution_point_id": schema.StringAttribute{
				MarkdownDescription: "Failover distribution point (the **Failover distribution point** option). Set it to `-2` for the Jamf Cloud distribution point, or to the ID of another file share distribution point, which you can reference with `jamfplatform_pro_file_share_distribution_point.other.id`. If the referenced distribution point is deleted, this resets to `-1`. Omit to leave the current value untouched; set `-1` to clear the failover.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_load_balancing": schema.BoolAttribute{
				MarkdownDescription: "Whether to randomly distribute the load between this distribution point and its failover (the **Enable randomized load sharing** option). Only valid when `backup_distribution_point_id` points at another file share distribution point. Load sharing does not apply when the failover is none (`-1`) or the Jamf Cloud distribution point (`-2`). Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"share_name": schema.StringAttribute{
				MarkdownDescription: "Name of the file share (the **Share name** field). Required by Jamf Pro when `file_sharing_connection_type` is `AFP` or `SMB`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "Port used for file sharing (the **Port** field; typically 548 for AFP, 445 for SMB). Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"workgroup": schema.StringAttribute{
				MarkdownDescription: "Workgroup or domain for the file share (the **Workgroup or domain** field). Applies to SMB shares. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"read_write_username": schema.StringAttribute{
				MarkdownDescription: "Username for the read/write account (the **Read/Write Account** username). Required by Jamf Pro when `file_sharing_connection_type` is `AFP` or `SMB`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"read_write_password": schema.StringAttribute{
				MarkdownDescription: "Password for the read/write account (the **Read/Write Account** password). `WriteOnly`: sent to Jamf Pro but never stored in Terraform state. Required by Jamf Pro when `file_sharing_connection_type` is `AFP` or `SMB`. Rotate it by bumping `read_write_password_wo_version`.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"read_write_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `read_write_password`. Bump this integer (any change) to force an update that re-sends `read_write_password`. Set it to `1` when you first set the password. Leaving it unchanged keeps the stored password as-is.",
				Optional:            true,
			},
			"read_only_username": schema.StringAttribute{
				MarkdownDescription: "Username for the read-only account (the **Read-only Account** username). Required by Jamf Pro when `file_sharing_connection_type` is `AFP` or `SMB`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"read_only_password": schema.StringAttribute{
				MarkdownDescription: "Password for the read-only account (the **Read-only Account** password). `WriteOnly`: sent to Jamf Pro but never stored in Terraform state. Required by Jamf Pro when `file_sharing_connection_type` is `AFP` or `SMB`. Rotate it by bumping `read_only_password_wo_version`.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"read_only_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `read_only_password`. Bump this integer (any change) to force an update that re-sends `read_only_password`. Set it to `1` when you first set the password. Leaving it unchanged keeps the stored password as-is.",
				Optional:            true,
			},
			"https_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether packages may be downloaded over HTTPS (the **Use HTTPS downloads** option). Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"https_port": schema.Int64Attribute{
				MarkdownDescription: "Port used for HTTPS downloads (the **Port** field on the HTTPS tab; typically 443). Required by Jamf Pro when `https_enabled` is `true`. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"https_context": schema.StringAttribute{
				MarkdownDescription: "Context path appended to the server for HTTPS downloads (the **Context** field). Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"https_security_type": schema.StringAttribute{
				MarkdownDescription: "Authentication type for HTTPS downloads (the **Authentication type** field). One of `USERNAME_PASSWORD` or `NONE`. When `USERNAME_PASSWORD`, Jamf Pro requires `https_username` and `https_password`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(httpsSecurityUsernamePassword, httpsSecurityNone),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"https_username": schema.StringAttribute{
				MarkdownDescription: "Username for the HTTPS account (the **HTTPS Account** username). Required by Jamf Pro when `https_security_type` is `USERNAME_PASSWORD`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"https_password": schema.StringAttribute{
				MarkdownDescription: "Password for the HTTPS account (the **HTTPS Account** password). `WriteOnly`: sent to Jamf Pro but never stored in Terraform state. Required by Jamf Pro when `https_security_type` is `USERNAME_PASSWORD`. Rotate it by bumping `https_password_wo_version`.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"https_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `https_password`. Bump this integer (any change) to force an update that re-sends `https_password`. Set it to `1` when you first set the password. Leaving it unchanged keeps the stored password as-is.",
				Optional:            true,
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

// ConfigValidators returns the plan-time cross-field validators.
func (r *FileShareDistributionPointResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		transportConfigValidator{},
		loadBalancingConfigValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *FileShareDistributionPointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_file_share_distribution_point")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro distribution point ID.
func (r *FileShareDistributionPointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan predicts the cleared file-sharing fields when the connection type
// is NONE. The server blanks share_name / port / workgroup / the account
// usernames whenever fileSharingConnectionType is NONE; without this, an omitted
// (UseStateForUnknown-carried) prior value — e.g. a port left over from an SMB
// configuration being switched to NONE — would be planned as a known value the
// server then nulls, tripping "provider produced inconsistent result after
// apply". (STYLE_GUIDE §discriminator-gated field clearing — the predict-cleared
// half of the pattern; the input builder is the don't-send half.)
func (r *FileShareDistributionPointResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy
	}

	var connType types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("file_sharing_connection_type"), &connType)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if connType.IsUnknown() || connType.ValueString() != connectionTypeNone {
		return
	}

	for _, p := range []path.Path{
		path.Root("share_name"),
		path.Root("workgroup"),
		path.Root("read_write_username"),
		path.Root("read_only_username"),
	} {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, p, types.StringNull())...)
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("port"), types.Int64Null())...)
}
