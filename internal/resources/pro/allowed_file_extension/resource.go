// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package allowed_file_extension implements the jamfplatform_pro_allowed_file_extension
// resource, data source, and list resource backed by the Jamf ProClassic allowed file
// extensions API.
package allowed_file_extension

import (
	"context"
	"regexp"
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
// Empty: the classic /allowedfileextensions endpoint predates the provider's overall
// floor (11.0.0). The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// noSurroundingWhitespace rejects values with leading or trailing whitespace. The server
// silently trims surrounding whitespace on write (wire-probed: " jpg " is stored as
// "jpg"), so accepting such a value would produce an "inconsistent result after apply"
// once the trimmed echo lands in state. The attribute is Required (non-Computed), so a
// plan-modifier rewrite is not an option (STYLE_GUIDE §327) — drift is surfaced at plan
// time by this validator instead. Internal characters and case are unconstrained.
var noSurroundingWhitespace = regexp.MustCompile(`^\S(.*\S)?$`)

// AllowedFileExtensionResource implements the Terraform resource for Jamf ProClassic
// allowed file extensions.
type AllowedFileExtensionResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &AllowedFileExtensionResource{}
var _ resource.ResourceWithImportState = &AllowedFileExtensionResource{}
var _ resource.ResourceWithIdentity = &AllowedFileExtensionResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAllowedFileExtensionResource returns a new instance of AllowedFileExtensionResource.
func NewAllowedFileExtensionResource() resource.Resource {
	return &AllowedFileExtensionResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AllowedFileExtensionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_allowed_file_extension"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *AllowedFileExtensionResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro allowed file extension ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the allowed file extension resource.
func (r *AllowedFileExtensionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro allowed file extension. Allowed file extensions are the tenant-wide list of file extensions Jamf Pro permits for attachments uploaded to inventory records (computers, mobile devices, and users)." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Allowed file extension ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// Wire field is `extension`; the UI labels this "File extension" — the
			// attribute keeps the same spelling (STYLE_GUIDE §127). The server stores the
			// value with case and any leading dot preserved (no canonicalisation there),
			// but trims surrounding whitespace, so the validator rejects surrounding
			// whitespace to keep plan and state aligned. There is no update path for this
			// record, so any change forces replacement.
			"extension": schema.StringAttribute{
				MarkdownDescription: "File extension Jamf Pro permits for attachments uploaded to inventory records (for example, `jpg`). Stored exactly as entered, with case and any leading dot preserved; leading or trailing whitespace is not allowed. Must be unique. Changing it forces the record to be replaced.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(noSurroundingWhitespace, "must not contain leading or trailing whitespace"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
func (r *AllowedFileExtensionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_allowed_file_extension")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro allowed file extension ID.
func (r *AllowedFileExtensionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
