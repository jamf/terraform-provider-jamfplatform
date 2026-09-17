// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package static_computer_group implements jamfplatform_pro_static_computer_group
// as a resource, a singular data source, a plural data source and a list
// resource. A static computer group holds the computers an administrator
// assigns to it, rather than the computers a set of criteria selects.
//
// internal/common/progroups carries the wire laws the four Jamf Pro
// device-group constructs share, and the helpers that hold them. Read it first.
// Three things belong to this construct alone.
//
// Membership is written through Jamf Pro's v3 group surface and read through the
// classic one, and this is the only classic call anywhere in the family. Jamf
// Pro has no v1, v2 or v3 static-computer-group membership read: all three
// /computer-groups/static-group-membership/{id} routes answer 403
// BAD_PERMISSIONS, which by this repository's law (CLAUDE.md §Jamf Security
// Cloud) means unrouted rather than unprivileged, and the smart equivalent
// answers 200 on the same credential. GET
// /pro/v3/computer-groups/static-groups/{id} then reports name, description and
// siteId and no membership at all. So GET /JSSResource/computergroups/id/{id} is
// the only source for who is in the group. Every write stays on pro/.
//
// The mandatory full-replace assignments key is why an update can need two
// requests. `assignments` must be present on create and on update alike, a
// populated value replaces the whole membership and [] clears it, so the
// endpoint offers no body meaning "leave the membership alone". An update that
// does not manage membership therefore reads the current members first and sends
// them back unchanged, and a failed pre-read fails the apply rather than falling
// back to a value that would empty the group.
//
// Neither identifier costs a bridging request on the happy path. The create asks
// for platform identifiers, so it answers with the platform UUID, and the
// read-after-create runs against that UUID and reports the Jamf Pro id in its
// body. Only import and the plural reads start with a Jamf Pro id alone, and
// progroups.PlatformIDsByJamfProID serves those from one request for a whole
// page.
//
// Wire names, for tracing a field seen in a Jamf Pro response back to an
// attribute:
//
//	| Terraform attribute   | Wire field                                  |
//	|-----------------------|---------------------------------------------|
//	| assigned_computer_ids | assignments (write), computers/computer/id (read) |
//	| site_id               | siteId                                      |
//	| platform_id           | groupPlatformId                             |
//	| member_count          | count (search results only)                 |
package static_computer_group

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty because there is no floor to source. The group
// endpoints this construct calls carry no "available since" note, and the
// version work that does apply is the refusal in
// progroups.RequireSupportedJamfProVersion, which every Configure below runs.
const minJamfProVersion = ""

const (
	defaultCreateTimeout = 120 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 120 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// StaticComputerGroupResource implements the Terraform resource.
//
// Two clients, for the reason the package doc gives: writes and the scalar read
// go through client, and membership is read through classicClient because Jamf
// Pro publishes no other way to read it.
type StaticComputerGroupResource struct {
	client        *pro.Client
	classicClient *proclassic.Client
	pd            *providerdata.Data
}

var (
	_ resource.Resource                = &StaticComputerGroupResource{}
	_ resource.ResourceWithConfigure   = &StaticComputerGroupResource{}
	_ resource.ResourceWithImportState = &StaticComputerGroupResource{}
	_ resource.ResourceWithIdentity    = &StaticComputerGroupResource{}
	_ resource.ResourceWithModifyPlan  = &StaticComputerGroupResource{}
)

// NewStaticComputerGroupResource returns a new StaticComputerGroupResource.
func NewStaticComputerGroupResource() resource.Resource {
	return &StaticComputerGroupResource{}
}

// Metadata sets the resource type name.
func (r *StaticComputerGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_computer_group"
}

// IdentitySchema declares the identifier import and list results carry.
func (r *StaticComputerGroupResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro identifier for the static computer group.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the resource schema.
func (r *StaticComputerGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(progroups.DeviceTypeComputer, progroups.GroupKindStatic) +
			"\n\nMembership is the set of computers you list in `assigned_computer_ids`. Nothing is derived from criteria, so the group holds exactly what the configuration says and no more." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro identifier for the group, and the value `terraform import` takes.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform. Use it to target the group from a blueprint or a compliance benchmark, and anywhere else a `jamfplatform_device_*` construct asks for one. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name. Jamf Pro requires it to be unique among computer groups.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Note Jamf Pro keeps with the group. Set it to an empty string to clear it; dropping the attribute from your configuration keeps whatever Jamf Pro already holds.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site the group belongs to. `-1` means no site, and is the default.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(progroups.NoSiteID),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"assigned_computer_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro computer identifiers to hold in the group, as strings. Omit the attribute and the group keeps the computers Jamf Pro already holds. Terraform manages that by reading the membership and sending it straight back, because Jamf Pro gives no way to leave it untouched, so a computer you assign or remove between that read and that write goes back the way it was. An update changing only the name or the site can undo a change you made in Jamf Pro while it ran. Set it to `[]` to empty the group. A populated set is the whole membership, and a computer you remove from it leaves the group. A group that belongs to a site accepts only computers assigned to that same site.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
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

// Configure wires both clients and the shared provider data onto the resource.
func (r *StaticComputerGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_computer_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	classicClient, classicDiags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_computer_group")
	resp.Diagnostics.Append(classicDiags...)
	if resp.Diagnostics.HasError() || classicClient == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_computer_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.classicClient = classicClient
	r.pd = pd
}

// ImportState imports by the Jamf Pro identifier.
func (r *StaticComputerGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
