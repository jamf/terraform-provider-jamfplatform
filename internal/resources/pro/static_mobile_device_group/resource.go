// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package static_mobile_device_group implements the
// jamfplatform_pro_static_mobile_device_group resource, its singular and plural
// data sources, and the jamfplatform_pro_static_mobile_device_groups list
// resource, over the Jamf Pro /pro/v2 mobile device group endpoints.
//
// The family's shared wire law lives in internal/common/progroups; read that
// package's doc comment first. What follows is only what is specific to the
// static mobile device group.
//
// # Terraform attribute to wire field
//
//	id                          groupId
//	name                        groupName
//	description                 groupDescription
//	site_id                     siteId
//	assigned_mobile_device_ids  assignments[].mobileDeviceId
//	member_count                count (read only; the write body has no counterpart)
//
// platform_id has no field of its own. It is the value the create returns in
// place of the Jamf Pro identifier when the create asks for it, and every route
// in this family accepts either identifier in the path.
//
// # The write mixes two opposite semantics in one body
//
// A PATCH replaces the scalars and delta-merges the membership, and the two
// halves of the same body therefore behave in contradictory ways. Everything
// interesting about this package follows from that; assignmentsForUpdate is
// where it is handled and crud.go's annotation block states the four probed
// facts.
//
// The upshot for a reader: this is the one static group in the provider whose
// update cannot be built from the plan alone. Making the configuration
// authoritative means reading the current membership first and sending the
// difference, so an update that manages membership costs two requests and a
// failed pre-read fails the apply rather than degrading to sending additions
// only.
package static_mobile_device_group

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is empty because there is no floor to source: the static
// mobile device group endpoints predate every Jamf Pro release the provider
// supports, and the version work this family needs is a refusal rather than a
// minimum. progroups.RequireSupportedJamfProVersion carries it, rejecting the
// window in which a group criterion value came back as an identifier.
const minJamfProVersion = ""

const (
	deviceType = progroups.DeviceTypeMobile
	groupKind  = progroups.GroupKindStatic
)

// Two calls on the managed-membership update path, so create and update get
// double the read and delete budget.
const (
	defaultCreateTimeout = 120 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 120 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// StaticMobileDeviceGroupResource implements the Terraform resource. pd carries
// the impact-alert cache and the once-per-provider warning latches the platform
// identifier bridge uses, so it is held alongside the client rather than
// re-derived.
type StaticMobileDeviceGroupResource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ resource.Resource                = &StaticMobileDeviceGroupResource{}
	_ resource.ResourceWithImportState = &StaticMobileDeviceGroupResource{}
	_ resource.ResourceWithIdentity    = &StaticMobileDeviceGroupResource{}
	_ resource.ResourceWithModifyPlan  = &StaticMobileDeviceGroupResource{}
)

// NewStaticMobileDeviceGroupResource returns a new resource instance.
func NewStaticMobileDeviceGroupResource() resource.Resource {
	return &StaticMobileDeviceGroupResource{}
}

// Metadata sets the resource type name.
func (r *StaticMobileDeviceGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_mobile_device_group"
}

// IdentitySchema declares the identifier used for import and list results.
func (r *StaticMobileDeviceGroupResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro identifier of the static mobile device group.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the resource schema.
func (r *StaticMobileDeviceGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(deviceType, groupKind) +
			" Membership is explicit: name each mobile device by its Jamf Pro identifier in `assigned_mobile_device_ids`. Leave that attribute out and Terraform does not manage membership, so whoever is in the group stays in it; set it to `[]` to empty the group. A group that belongs to a site accepts only mobile devices assigned to that same site." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro identifier for the group.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform. A blueprint or a compliance benchmark targets a group by this value, as does anything else outside Jamf Pro that scopes to one. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group display name. Jamf Pro requires it to be unique across mobile device groups.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Note Jamf Pro keeps with the group.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site the group belongs to. `-1` means no site, and is the default. A group in a site holds only mobile devices assigned to that same site.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(progroups.NoSiteID),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"assigned_mobile_device_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro identifiers of the mobile devices in the group, as strings. Omit the attribute and Terraform leaves membership alone, so devices assigned in Jamf Pro stay where they are. Set it to `[]` to empty the group. A populated set is the whole membership, and a device you remove from it leaves the group.",
				Optional:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"member_count": schema.Int64Attribute{
				MarkdownDescription: "Mobile devices Jamf Pro counts in the group. Read-only.",
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

// Configure wires the Jamf Pro client and provider data into the resource.
func (r *StaticMobileDeviceGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_mobile_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_mobile_device_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
	r.pd = pd
}

// ImportState imports by the Jamf Pro identifier.
func (r *StaticMobileDeviceGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
