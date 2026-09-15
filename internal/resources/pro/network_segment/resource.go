// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package network_segment implements the jamfplatform_pro_network_segment resource,
// data source, and list resource backed by the Jamf ProClassic networksegments API.
package network_segment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: classic /networksegments endpoint predates the provider's overall floor (11.0.0).
// The provider-level advisory still fires through providerdata.ConfigureProClassic when
// the tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// NetworkSegmentResource implements the Terraform resource for Jamf ProClassic network segments.
type NetworkSegmentResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &NetworkSegmentResource{}
var _ resource.ResourceWithImportState = &NetworkSegmentResource{}
var _ resource.ResourceWithIdentity = &NetworkSegmentResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewNetworkSegmentResource returns a new instance of NetworkSegmentResource.
func NewNetworkSegmentResource() resource.Resource {
	return &NetworkSegmentResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *NetworkSegmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_network_segment"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *NetworkSegmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro network segment ID used to uniquely reference the network segment.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the network segment resource.
func (r *NetworkSegmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro network segment. Network segments are IP ranges used to scope Jamf Pro policies, configuration profiles, and other objects to clients whose IP falls within the segment. Optionally a network segment can override the building/department assignment of devices that join it." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Network segment ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Network segment display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"starting_address": schema.StringAttribute{
				MarkdownDescription: "Starting IPv4 address of the network segment (inclusive). Jamf Pro network segments are IPv4-only.",
				Required:            true,
				Validators: []validator.String{
					ipv4Address(),
				},
			},
			"ending_address": schema.StringAttribute{
				MarkdownDescription: "Ending IPv4 address of the network segment (inclusive). Jamf Pro network segments are IPv4-only.",
				Required:            true,
				Validators: []validator.String{
					ipv4Address(),
				},
			},
			"building": schema.StringAttribute{
				MarkdownDescription: "Optional building name to associate with the network segment. Used together with `override_buildings = true` to override the building of devices that join this segment.",
				Optional:            true,
			},
			"department": schema.StringAttribute{
				MarkdownDescription: "Optional department name to associate with the network segment. Used together with `override_departments = true` to override the department of devices that join this segment.",
				Optional:            true,
			},
			"override_buildings": schema.BoolAttribute{
				MarkdownDescription: "When true, devices that join this segment have their building set to the value of `building`. Also requires `building` to be set.",
				Optional:            true,
				Validators: []validator.Bool{
					boolvalidator.AlsoRequires(path.MatchRoot("building")),
				},
			},
			"override_departments": schema.BoolAttribute{
				MarkdownDescription: "When true, devices that join this segment have their department set to the value of `department`. Also requires `department` to be set.",
				Optional:            true,
				Validators: []validator.Bool{
					boolvalidator.AlsoRequires(path.MatchRoot("department")),
				},
			},
			"distribution_point": schema.StringAttribute{
				MarkdownDescription: "Distribution point assigned by Jamf Pro for this segment. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"distribution_server": schema.StringAttribute{
				MarkdownDescription: "Distribution server assigned by Jamf Pro for this segment. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"swu_server": schema.StringAttribute{
				MarkdownDescription: "Software update server assigned by Jamf Pro for this segment. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "NetBoot/imaging URL assigned by Jamf Pro for this segment. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *NetworkSegmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_network_segment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro network segment ID.
func (r *NetworkSegmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
