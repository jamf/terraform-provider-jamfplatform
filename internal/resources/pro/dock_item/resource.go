// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package dock_item implements the jamfplatform_pro_dock_item resource, data
// source, and list resource backed by the Jamf ProClassic /dockitems API.
package dock_item

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: classic /dockitems predates the provider's overall floor. The provider-level
// advisory still fires through providerdata.ConfigureProClassic when the tenant is
// below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// Dock item type enum values accepted by the classic /dockitems endpoint,
// aliased from the SDK. Note the classic vocabulary is title-cased —
// pro.DockItemType, the Pro JSON one, spells the same three values APP / FILE /
// FOLDER, so the two are not interchangeable.
const (
	DockItemTypeApp    = proclassic.DockItemTypeApp
	DockItemTypeFile   = proclassic.DockItemTypeFile
	DockItemTypeFolder = proclassic.DockItemTypeFolder
)

// DockItemResource implements the Terraform resource for Jamf Pro dock items.
type DockItemResource struct {
	client *proclassic.Client
	// impact backs the plan-time impact alert reporting how many computers this
	// object reaches through the policies that use it. nil when the provider's
	// impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var _ resource.Resource = &DockItemResource{}
var _ resource.ResourceWithImportState = &DockItemResource{}
var _ resource.ResourceWithIdentity = &DockItemResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDockItemResource returns a new instance of DockItemResource.
func NewDockItemResource() resource.Resource {
	return &DockItemResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DockItemResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_dock_item"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *DockItemResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro dock item ID used to uniquely reference the dock item.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the dock item resource.
func (r *DockItemResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro dock item. Dock items are reusable references that Jamf Pro policies use to add an application, file, or folder to a Mac's Dock. The PLIST `contents` field is derived from `name`, `type`, and `path` by Jamf Pro and exposed read-only." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Dock item ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Dock item display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Dock item type. Must be one of `App`, `File`, `Folder`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(DockItemTypeApp, DockItemTypeFile, DockItemTypeFolder),
				},
			},
			"path": schema.StringAttribute{
				MarkdownDescription: "Path to the dock item. The Jamf Pro admin UI hints at `file://localhost/Applications/App%20Store.app/` style URIs for apps; raw filesystem paths (`/My/App.app`) are also accepted. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"contents": schema.StringAttribute{
				MarkdownDescription: "Property-list representation of the dock tile. Jamf Pro derives it from `name`, `type`, and `path` on every write. Returned by Jamf Pro; not user-settable.",
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *DockItemResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_dock_item")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro dock item ID.
func (r *DockItemResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
