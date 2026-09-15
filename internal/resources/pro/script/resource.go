// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package script implements the jamfplatform_pro_script resource, data source, and
// list resource backed by the Jamf Pro scripts API.
package script

import (
	"context"
	"fmt"
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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty string skips the per-resource version check — the scripts endpoint has been
// stable since well before the provider's overall floor (11.0.0).
const minJamfProVersion = ""

// ScriptResource implements the Terraform resource for Jamf Pro scripts.
type ScriptResource struct {
	client *pro.Client
	// impact backs the plan-time impact alert reporting how many computers this
	// object reaches through the policies that use it. nil when the provider's
	// impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var _ resource.Resource = &ScriptResource{}
var _ resource.ResourceWithImportState = &ScriptResource{}
var _ resource.ResourceWithIdentity = &ScriptResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// priorityValues enumerates the accepted Jamf Pro script priority values, taken
// from the SDK's generated helper so the OneOf validator cannot drift from the
// API.
var priorityValues = pro.ScriptPriorityValues()

// NewScriptResource returns a new instance of ScriptResource.
func NewScriptResource() resource.Resource {
	return &ScriptResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ScriptResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_script"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ScriptResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro script ID used to uniquely reference the script.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the script resource.
func (r *ScriptResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro script. Scripts execute on managed devices through policies or Self Service workflows. Jamf Pro reserves parameter slots 1–3; the labels you control are `parameter_4` through `parameter_11`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Script ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Script display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"category_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Jamf Pro category this script belongs to. Look it up via the `jamfplatform_pro_category` data source. Jamf Pro reports `-1` when the script has no category. Omit to leave the current value untouched; there is no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"category_name": schema.StringAttribute{
				// No UseStateForUnknown: category_name is derived from the
				// mutable category_id, so it must go Unknown (not pin the stale
				// value) when category_id changes, or the post-apply consistency
				// check trips. See STYLE_GUIDE §886.
				MarkdownDescription: "Display name of the category referenced by `category_id`. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"info": schema.StringAttribute{
				MarkdownDescription: "Informational text shown to end users (e.g. in Self Service) describing the script. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"notes": schema.StringAttribute{
				MarkdownDescription: "Administrator-only notes about the script. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"os_requirements": schema.StringAttribute{
				MarkdownDescription: "Comma-separated macOS versions the script supports (e.g. `13.0.x,14.0.x`). Empty allows all. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"priority": schema.StringAttribute{
				MarkdownDescription: "Execution order relative to other policy actions. Valid values: `BEFORE`, `AFTER`, `AT_REBOOT`. Defaults to `AFTER`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(priorityValues...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parameter_4":  optionalParameterAttribute(4),
			"parameter_5":  optionalParameterAttribute(5),
			"parameter_6":  optionalParameterAttribute(6),
			"parameter_7":  optionalParameterAttribute(7),
			"parameter_8":  optionalParameterAttribute(8),
			"parameter_9":  optionalParameterAttribute(9),
			"parameter_10": optionalParameterAttribute(10),
			"parameter_11": optionalParameterAttribute(11),
			"script_contents": schema.StringAttribute{
				MarkdownDescription: "Script contents as plain text (shell, Python, and so on). Omit to leave the existing contents untouched: Terraform will not clear them, so the body can be co-managed in the Jamf Pro UI. Set to `\"\"` to clear. A declared value is owned by Terraform and reverts out-of-band edits.",
				Optional:            true,
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

// optionalParameterAttribute returns the schema for a single script parameter slot.
// Jamf Pro reserves parameters 1–3 (called out on the resource description), so
// user-managed slots run 4–11.
func optionalParameterAttribute(slot int) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: fmt.Sprintf("Label for script parameter slot %d. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.", slot),
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *ScriptResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_script")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro script ID.
func (r *ScriptResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
