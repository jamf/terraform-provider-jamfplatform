// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package computer_check_in_settings implements the jamfplatform_pro_computer_check_in_settings
// singleton resource and data source backed by the Jamf Pro Client Check-In settings API.
package computer_check_in_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the Client Check-In settings endpoint is present at the provider's overall
// floor, so no per-resource gate is needed.
const minJamfProVersion = ""

// ComputerCheckInSettingsResource implements the singleton resource for Jamf Pro Client
// Check-In settings. Backed by an Update-only API (no Create/Delete on the remote):
// Create funnels into Update; Delete is a no-op that only removes the object from
// Terraform state.
type ComputerCheckInSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &ComputerCheckInSettingsResource{}
var _ resource.ResourceWithImportState = &ComputerCheckInSettingsResource{}
var _ resource.ResourceWithIdentity = &ComputerCheckInSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewComputerCheckInSettingsResource returns a new instance of ComputerCheckInSettingsResource.
func NewComputerCheckInSettingsResource() resource.Resource {
	return &ComputerCheckInSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ComputerCheckInSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_check_in_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *ComputerCheckInSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Client Check-In settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Client Check-In settings resource.
func (r *ComputerCheckInSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro Client Check-In settings (Settings > Computers > Check-in). " +
			"One record per tenant. " +
			"A startup or login toggle you omit keeps its current Jamf Pro value, including on the first apply: this resource adopts the existing settings and changes only the toggles you declare. A boolean has no \"unset\" state, so omit it to preserve the current value, or set `true` or `false` to change it. `check_in_frequency` is required. " +
			"Import with `terraform import jamfplatform_pro_computer_check_in_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"check_in_frequency": schema.Int64Attribute{
				MarkdownDescription: "Recurring Check-in Frequency, in minutes. One of `5`, `15`, `30`, or `60`.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.OneOf(5, 15, 30, 60),
				},
			},
			"create_startup_script": schema.BoolAttribute{
				MarkdownDescription: "Create a startup script. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"startup_log": schema.BoolAttribute{
				MarkdownDescription: "Log Computer Usage Information at startup. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"startup_policies": schema.BoolAttribute{
				MarkdownDescription: "Check for policies triggered by startup. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"startup_ssh": schema.BoolAttribute{
				MarkdownDescription: "Ensure SSH is enabled. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"create_login_hook": schema.BoolAttribute{
				MarkdownDescription: "Create a login event. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"login_hook_log": schema.BoolAttribute{
				MarkdownDescription: "Log Computer Usage Information at login. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"login_hook_policies": schema.BoolAttribute{
				MarkdownDescription: "Check for policies triggered by login. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_network_state_change_triggers": schema.BoolAttribute{
				MarkdownDescription: "Allow Network State Change Triggers. Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
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
func (r *ComputerCheckInSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_check_in_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error so users do
// not accidentally end up with mis-keyed state that the resource silently normalizes
// on the next Read.
//
//	terraform import jamfplatform_pro_computer_check_in_settings.<name> singleton
func (r *ComputerCheckInSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_computer_check_in_settings")
}
