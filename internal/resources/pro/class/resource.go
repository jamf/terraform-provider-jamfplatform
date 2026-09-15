// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package class implements the jamfplatform_pro_class resource, data source, and
// list resource backed by the Jamf ProClassic /classes API. The construct name
// mirrors the Jamf Pro admin UI ("Classes" under the Computers sidebar — Apple
// Classroom). Class membership has its own shape (students/teachers by username,
// group IDs, device groups) and does not consume the shared scope helper.
package class

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /classes predates the provider's overall floor. The
// provider-level advisory still fires through providerdata.ConfigureProClassic
// when the tenant is below the floor.
const minJamfProVersion = ""

// ClassResource implements the Terraform resource for Jamf Pro classes.
type ClassResource struct {
	client *proclassic.Client
	// impact backs the plan-time impact alert on device group targeting. nil when
	// the provider's impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var _ resource.Resource = &ClassResource{}
var _ resource.ResourceWithImportState = &ClassResource{}
var _ resource.ResourceWithIdentity = &ClassResource{}
var _ resource.ResourceWithModifyPlan = &ClassResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewClassResource returns a new instance of ClassResource.
func NewClassResource() resource.Resource {
	return &ClassResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ClassResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_class"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ClassResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro class ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource. Attribute names mirror
// the Jamf Pro admin UI labels; user-facing descriptions are UI-aligned.
func (r *ClassResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro class: the \"Classes\" item under the Computers sidebar in the Jamf Pro admin UI, used by Apple Classroom and Apple School Manager. A class groups students and teachers by username, and student, teacher and mobile device groups by ID. Membership is authoritative, so each set is applied in full on every change. Classes synchronised from a roster in Apple School Manager are managed by that sync and should not be managed with this resource." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Class ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Must be unique within the tenant.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "**\"Description\"** in the Jamf Pro admin UI. Free-text description for the class. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "**\"Site\"** in the Jamf Pro admin UI. Site ID scoping the class. Use `-1` for \"None\", the default.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(noSiteID),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"site_name": schema.StringAttribute{
				// No UseStateForUnknown: site_name derives from site_id, so it
				// must go Unknown (not pin the stale value) whenever site_id
				// changes, or the post-apply consistency check trips.
				MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "How the class was created in Jamf Pro (for example a manually created class versus one synchronised from a roster). Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			// Authoritative membership: the full set is applied on every change,
			// so removing an element removes the member and omitting the attribute
			// (or setting it to []) leaves the class with none. Jamf Pro stores
			// usernames case-insensitively and may echo a different casing; the
			// provider preserves the configured casing to avoid spurious drift.
			"students": schema.SetAttribute{
				MarkdownDescription: "**\"Students\"** in the Jamf Pro admin UI. Usernames of the students assigned to the class. The full set is applied on every change: removing a username removes the member, and omitting the attribute leaves the class with no students. Unrecognised usernames are created as Jamf Pro users.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"teachers": schema.SetAttribute{
				MarkdownDescription: "**\"Teachers\"** in the Jamf Pro admin UI. Usernames of the teachers assigned to the class. The full set is applied on every change: removing a username removes the member, and omitting the attribute leaves the class with no teachers. Unrecognised usernames are created as Jamf Pro users.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"student_group_ids": schema.SetAttribute{
				MarkdownDescription: "**\"Student Groups\"** in the Jamf Pro admin UI. Jamf Pro user group IDs, as strings, assigned as student groups. The full set is applied on every change. Referenced IDs must already exist.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"teacher_group_ids": schema.SetAttribute{
				MarkdownDescription: "**\"Teacher Groups\"** in the Jamf Pro admin UI. Jamf Pro user group IDs, as strings, assigned as teacher groups. The full set is applied on every change. Referenced IDs must already exist.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"mobile_device_group_ids": schema.SetAttribute{
				MarkdownDescription: "**\"Mobile Device Groups\"** in the Jamf Pro admin UI. Jamf Pro mobile device group IDs, as strings, assigned to the class. The full set is applied on every change. Referenced IDs must already exist.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"student_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro user IDs (as strings) for the students, resolved by Jamf Pro from the supplied usernames. Read-only.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"teacher_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro user IDs (as strings) for the teachers, resolved by Jamf Pro from the supplied usernames. Read-only.",
				Computed:            true,
				ElementType:         types.StringType,
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
func (r *ClassResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_class")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
}

// ImportState handles import by the Jamf Pro class ID.
func (r *ClassResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
