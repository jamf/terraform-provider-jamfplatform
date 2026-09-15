// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ibeacon implements the jamfplatform_pro_ibeacon resource, data
// source, and list resource backed by the Jamf ProClassic ibeacons API.
package ibeacon

import (
	"context"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// uuidRegexCompiled is the pre-compiled regexp for the canonical iBeacon UUID
// form, used by the stringvalidator.RegexMatches call in the resource schema.
var uuidRegexCompiled = regexp.MustCompile(uuidRegex)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: classic /ibeacons predates the provider's overall floor. The provider-level
// advisory still fires through providerdata.ConfigureProClassic when the tenant is
// below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// uuidRegex matches the canonical 8-4-4-4-12 hex form of an iBeacon UUID
// (Jamf-Pro stores the UUID as a plain string; the canonical form is what
// the Jamf UI emits). Case-insensitive on the hex digits.
const uuidRegex = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`

// IbeaconResource implements the Terraform resource for Jamf Pro iBeacons.
type IbeaconResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &IbeaconResource{}
var _ resource.ResourceWithImportState = &IbeaconResource{}
var _ resource.ResourceWithIdentity = &IbeaconResource{}
var _ resource.ResourceWithConfigValidators = &IbeaconResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewIbeaconResource returns a new instance of IbeaconResource.
func NewIbeaconResource() resource.Resource {
	return &IbeaconResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *IbeaconResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ibeacon"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *IbeaconResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro iBeacon ID used to uniquely reference the iBeacon region.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the iBeacon resource.
func (r *IbeaconResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro iBeacon region. iBeacons are Bluetooth Low Energy regions identified by a UUID plus an optional major/minor pair; Jamf Pro policies and configuration profiles can be scoped to clients that are inside or outside an iBeacon region. To match any major value, set `include_any_major_value = true` and leave `major` unset; do the same for `include_any_minor_value` and `minor`. The two toggles are independent, so you can match a specific major with any minor. The resource enforces this mutual exclusivity at plan time." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "iBeacon ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "iBeacon display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "iBeacon UUID in canonical 8-4-4-4-12 hex form (e.g. `759b0599-64e0-416a-8d31-d8e93482a4d7`).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidRegexCompiled, "must be a UUID in canonical 8-4-4-4-12 hex form, e.g. 759b0599-64e0-416a-8d31-d8e93482a4d7"),
				},
			},
			"major": schema.Int64Attribute{
				MarkdownDescription: "Major value of the iBeacon region. Must be 0–65535. Required when `include_any_major_value` is omitted or set to `false`. Must NOT be set when `include_any_major_value = true`.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"minor": schema.Int64Attribute{
				MarkdownDescription: "Minor value of the iBeacon region. Must be 0–65535. Required when `include_any_minor_value` is omitted or set to `false`. Must NOT be set when `include_any_minor_value = true`.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"include_any_major_value": schema.BoolAttribute{
				MarkdownDescription: "When `true`, the iBeacon matches any major value. When `true`, `major` must NOT be set. Defaults to `false`. Independent of `include_any_minor_value`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"include_any_minor_value": schema.BoolAttribute{
				MarkdownDescription: "When `true`, the iBeacon matches any minor value. When `true`, `minor` must NOT be set. Defaults to `false`. Independent of `include_any_major_value`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
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
func (r *IbeaconResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ibeacon")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro iBeacon ID.
func (r *IbeaconResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ConfigValidators registers the plan-time cross-field validator. The
// apply-time helper validateIbeaconPlan in helpers.go remains as
// defence-in-depth (catches values that only become known during apply).
func (r *IbeaconResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		includeAnyMajorMinorConfigValidator{},
	}
}
