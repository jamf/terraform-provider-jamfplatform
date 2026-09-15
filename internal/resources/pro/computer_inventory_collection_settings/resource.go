// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package computer_inventory_collection_settings implements the
// jamfplatform_pro_computer_inventory_collection_settings singleton resource and data
// source backed by the Jamf Pro Computer Inventory Collection Settings V2 API.
package computer_inventory_collection_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the computer-inventory-collection-settings V2 endpoint is present at the
// provider's overall floor, so no per-resource gate is needed.
const minJamfProVersion = ""

// ComputerInventoryCollectionSettingsResource implements the singleton resource for
// Jamf Pro computer inventory collection settings (V2). Backed by an Update-only
// settings API (PATCH; no Create/Delete on the remote): Create funnels into Update;
// Delete is a no-op that only removes the object from Terraform state. The custom
// application search-path collection is managed via dedicated create/delete endpoints
// (there is no path-update endpoint), reconciled by path string in Create/Update.
type ComputerInventoryCollectionSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &ComputerInventoryCollectionSettingsResource{}
var _ resource.ResourceWithImportState = &ComputerInventoryCollectionSettingsResource{}
var _ resource.ResourceWithIdentity = &ComputerInventoryCollectionSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewComputerInventoryCollectionSettingsResource returns a new instance of the resource.
func NewComputerInventoryCollectionSettingsResource() resource.Resource {
	return &ComputerInventoryCollectionSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ComputerInventoryCollectionSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_inventory_collection_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *ComputerInventoryCollectionSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Computer inventory collection settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// optionalComputedBool returns the canonical Optional+Computed bool attribute used for
// every collection-preference toggle. These are plain (non-nested) scalars, so
// UseStateForUnknown is the correct plan modifier. The settings update is a merge-patch,
// so an omitted toggle keeps its stored value; the standard description sentence is
// appended here so all fourteen toggles document the same contract — see
// STYLE_GUIDE.md §Full-replace endpoints & shared backing stores.
func optionalComputedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc + " Omit to leave the current value untouched; set `true`/`false` to change it.",
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

// optionalComputedBoolValidated is optionalComputedBool with attribute-level validators
// attached (used for the account sub-options gated by collect_local_user_accounts).
func optionalComputedBoolValidated(desc string, validators ...validator.Bool) schema.BoolAttribute {
	a := optionalComputedBool(desc)
	a.Validators = validators
	return a
}

// Schema returns the Terraform schema for the computer inventory collection settings resource.
func (r *ComputerInventoryCollectionSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro computer inventory collection settings (Settings → Computer Management → Inventory Collection). " +
			"One record per tenant. " +
			"`application_search_paths` covers custom application search paths only. Jamf Pro does not expose Fonts or Plug-ins custom paths here, and the scope is fixed to `APP`. " +
			"Import with `terraform import jamfplatform_pro_computer_inventory_collection_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// Inventory Collection (General tab)
			"collect_local_user_accounts":                      optionalComputedBool("Collect local user accounts."),
			"include_home_directory_sizes":                     optionalComputedBoolValidated("Include home directory sizes when collecting local user accounts. Sub-option of `collect_local_user_accounts`: may only be `true` when `collect_local_user_accounts` is `true`.", requiresAccountCollection{}),
			"include_hidden_accounts":                          optionalComputedBoolValidated("Include hidden accounts when collecting local user accounts. Sub-option of `collect_local_user_accounts`: may only be `true` when `collect_local_user_accounts` is `true`.", requiresAccountCollection{}),
			"collect_printers":                                 optionalComputedBool("Collect printers."),
			"collect_active_services":                          optionalComputedBool("Collect active services."),
			"collect_synced_mobile_device_backup_dates":        optionalComputedBool("Collect last backup date/time for managed mobile devices that are synced to computers."),
			"collect_user_and_location_from_directory_service": optionalComputedBool("Collect user and location information from Directory Service."),
			"collect_package_receipts":                         optionalComputedBool("Collect package receipts."),
			"collect_available_software_updates":               optionalComputedBool("Collect available software updates."),
			"collect_unmanaged_certificates":                   optionalComputedBool("Collect unmanaged certificates."),
			"monitor_beacon_regions":                           optionalComputedBool("Monitor Beacon regions."),
			"allow_jamf_binary_user_and_location_changes":      optionalComputedBool("Allow local administrators to use the jamf binary recon verb to change User and Location inventory information in Jamf Pro (Advanced)."),

			// Software tab → Applications
			"collect_application_usage_information": optionalComputedBool("Collect Application Usage Information."),
			"use_unix_user_paths":                   optionalComputedBool("Enable inventory collection of applications in user (UNIX) home-directory paths."),

			// Read-only server-managed preference (no admin-UI control)
			"include_software_id": schema.BoolAttribute{
				MarkdownDescription: "Whether the inventory submission includes a software identifier. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			"application_search_paths": schema.SetAttribute{
				MarkdownDescription: "Custom application search paths used when collecting applications (Software → Applications → Custom Search Paths). " +
					"Built-in paths such as `/Applications/` and `/System/Applications/` are managed by Jamf Pro and are not included here. " +
					"Changing an entry replaces it, because Jamf Pro cannot update a path in place. " +
					"Omit the attribute to leave the tenant's custom paths unmanaged; set it to `[]` to remove all custom application paths.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *ComputerInventoryCollectionSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_inventory_collection_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error.
//
//	terraform import jamfplatform_pro_computer_inventory_collection_settings.<name> singleton
func (r *ComputerInventoryCollectionSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_computer_inventory_collection_settings")
}
