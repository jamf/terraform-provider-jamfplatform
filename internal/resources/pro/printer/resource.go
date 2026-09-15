// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package printer implements the jamfplatform_pro_printer resource, data
// source, and list resource backed by the Jamf ProClassic /printers API.
package printer

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /printers predates the provider's overall floor.
// The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below
// ProviderMinJamfProVersion.
const minJamfProVersion = ""

// PrinterResource implements the Terraform resource for Jamf Pro printers.
type PrinterResource struct {
	client *proclassic.Client
	// impact backs the plan-time impact alert reporting how many computers this
	// object reaches through the policies that use it. nil when the provider's
	// impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var (
	_ resource.Resource                     = &PrinterResource{}
	_ resource.ResourceWithImportState      = &PrinterResource{}
	_ resource.ResourceWithIdentity         = &PrinterResource{}
	_ resource.ResourceWithConfigValidators = &PrinterResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPrinterResource returns a new instance of PrinterResource.
func NewPrinterResource() resource.Resource {
	return &PrinterResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PrinterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_printer"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *PrinterResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro printer ID used to uniquely reference the printer.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the printer resource.
func (r *PrinterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro printer. Printers are reusable definitions Jamf Pro policies use to map an IPP / LPD / SMB printer, and its PPD, onto Mac computers. Cross-field rules bind the `use_generic` toggle to the PPD trio (`ppd`, `ppd_path`, `ppd_contents`) and are enforced at plan time. Removing `category`, `uri`, `cups_name`, `location`, `model`, `info`, `notes`, `ppd` or `os_requirements` from your configuration clears the stored value on the next update; only `ppd_path` and `ppd_contents` are left untouched when omitted. See each attribute for details." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Printer ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Printer display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"category": schema.StringAttribute{
				MarkdownDescription: "Display name of an existing Jamf Pro category to assign this printer to. The category must already exist in the tenant; supplying an unknown name produces a 409 Conflict at apply time. Leave unset to mean \"no category assigned.\"",
				Optional:            true,
				Validators: []validator.String{
					noLiteralSentinelValidator{},
				},
			},
			"uri": schema.StringAttribute{
				MarkdownDescription: "Device URI of the printer (e.g. `ipp://10.1.20.120/`, `lpd://printer.local/queue1`).",
				Optional:            true,
			},
			"cups_name": schema.StringAttribute{
				MarkdownDescription: "CUPS queue name.",
				Optional:            true,
			},
			"location": schema.StringAttribute{
				MarkdownDescription: "Physical location of the printer (free text).",
				Optional:            true,
			},
			"model": schema.StringAttribute{
				MarkdownDescription: "Printer model (free text).",
				Optional:            true,
			},
			"info": schema.StringAttribute{
				MarkdownDescription: "Free-text information shown to administrators when the printer is mapped or unmapped.",
				Optional:            true,
			},
			"notes": schema.StringAttribute{
				MarkdownDescription: "Free-text notes about the printer (e.g. who created it, when, why).",
				Optional:            true,
			},
			"make_default": schema.BoolAttribute{
				MarkdownDescription: "Whether to set this printer as the default printer on target Macs when mapped. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"use_generic": schema.BoolAttribute{
				MarkdownDescription: "Whether to use the bundled macOS Generic.ppd. When `true` (the default) Jamf Pro uses the generic PPD and clears any `ppd`, `ppd_path`, or `ppd_contents` you supplied. When `false` you must supply a concrete `ppd_path`. `ppd` and `ppd_contents` alone are not sufficient; Jamf Pro falls back to the generic PPD if `ppd_path` is missing.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"ppd": schema.StringAttribute{
				MarkdownDescription: "Short name of the PPD file (e.g. `HP DeskJet 2600 series.ppd`). Only valid when `use_generic = false`. Plan-time error if set with `use_generic = true`.",
				Optional:            true,
			},
			"ppd_path": schema.StringAttribute{
				MarkdownDescription: "Filesystem path to the PPD file on target Macs (e.g. `/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd`). Required when `use_generic = false`; without it Jamf Pro silently falls back to the generic PPD. Plan-time error if set with `use_generic = true`. Computed when unset: under the generic configuration Jamf Pro populates it with the bundled Generic.ppd path. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ppd_contents": schema.StringAttribute{
				MarkdownDescription: "Inline contents of the PPD file. Only valid when `use_generic = false`. Jamf Pro strips trailing whitespace from this field on every round-trip; the provider's custom type treats two values as semantically equal when they differ only by trailing whitespace, so `ppd_contents = file(\"some.ppd\")` does not produce drift on subsequent plans. PPD bodies are driver descriptors rather than secrets, so `terraform plan` shows the full text. Wrap the value in `sensitive(...)` in config if you would like Terraform to redact it. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				CustomType:          trimmedStringType{},
				Optional:            true,
				Computed:            true,
			},
			"shared": schema.BoolAttribute{
				MarkdownDescription: "Whether the printer is shared.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"os_requirements": schema.StringAttribute{
				MarkdownDescription: "Operating-system version requirement for this printer (admin-UI Limitations tab). Free-text, typically a comma-separated list of macOS versions (e.g. `\"13.5.2, 16.6\"`).",
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

// ConfigValidators returns the cross-field validators evaluated against the
// user's config at plan time.
func (r *PrinterResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		useGenericPPDConfigValidator{},
	}
}

// Configure wires the Jamf ProClassic client into the resource via the
// shared providerdata.ConfigureProClassic helper.
func (r *PrinterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_printer")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro printer ID.
func (r *PrinterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
