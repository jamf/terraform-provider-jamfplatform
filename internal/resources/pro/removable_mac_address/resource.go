// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package removable_mac_address implements the jamfplatform_pro_removable_mac_address
// resource, data source, and list resource backed by the Jamf ProClassic
// removable Mac addresses API.
package removable_mac_address

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the classic /removablemacaddresses endpoint predates the provider's overall
// floor (11.0.0). The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// RemovableMacAddressResource implements the Terraform resource for Jamf ProClassic
// removable Mac addresses.
type RemovableMacAddressResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &RemovableMacAddressResource{}
var _ resource.ResourceWithImportState = &RemovableMacAddressResource{}
var _ resource.ResourceWithIdentity = &RemovableMacAddressResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewRemovableMacAddressResource returns a new instance of RemovableMacAddressResource.
func NewRemovableMacAddressResource() resource.Resource {
	return &RemovableMacAddressResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *RemovableMacAddressResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_removable_mac_address"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *RemovableMacAddressResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro removable MAC address ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the removable MAC address resource.
func (r *RemovableMacAddressResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro removable MAC address. Removable MAC addresses are the tenant-wide list of hardware addresses Jamf Pro ignores when matching a computer to its inventory record. Use them when a USB or Thunderbolt Ethernet adapter is shared across machines and would otherwise cause inventory mis-matches." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Removable MAC address ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// Wire field is `name`; the UI labels this column "MAC Address" so the
			// attribute is named mac_address (STYLE_GUIDE §127). Stored verbatim by the
			// server — no case-folding or separator canonicalisation — so it never drifts.
			"mac_address": schema.StringAttribute{
				MarkdownDescription: "MAC address that Jamf Pro ignores when matching a computer to its inventory record (for example, a shared USB or Thunderbolt Ethernet adapter). Stored exactly as entered. Must be unique and not empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
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
func (r *RemovableMacAddressResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_removable_mac_address")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro removable MAC address ID.
func (r *RemovableMacAddressResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
