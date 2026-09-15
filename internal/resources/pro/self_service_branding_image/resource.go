// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package self_service_branding_image implements the
// jamfplatform_pro_self_service_branding_image resource backed by the Jamf Pro
// Self Service branding image upload API. Uploaded images are referenced by ID
// from jamfplatform_pro_self_service_branding_macos (icon_id /
// banner_image_id) and jamfplatform_pro_self_service_branding_ios (icon_id).
//
// Two things about this store were probed on 2026-09-04 against Jamf Pro in EU,
// and both differ from what the general icon store does, so do not carry either
// answer across:
//
//   - It is append-only rather than one-per-tenant. Three uploads in a row
//     answered 201 with ids 1, 2 and 3, including two of byte-identical
//     content, so there is no conflict to translate on a second create.
//   - It stores an image verbatim. A 512x512 PNG uploaded and downloaded again
//     hashed identically, where the same image through UploadIconV1 came back
//     re-encoded at 83 times the size. The import workflow in the schema
//     description rests on that: an operator can point image_file_source at
//     their own copy, which is the opposite of the icon resource's advice.
package self_service_branding_image

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/files"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty string skips the version check — the Self Service branding
// image endpoint predates the provider's overall floor (11.0.0).
const minJamfProVersion = ""

// SelfServiceBrandingImageResource implements the Terraform resource for Self
// Service branding images.
type SelfServiceBrandingImageResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &SelfServiceBrandingImageResource{}
	_ resource.ResourceWithImportState = &SelfServiceBrandingImageResource{}
	_ resource.ResourceWithIdentity    = &SelfServiceBrandingImageResource{}
	_ resource.ResourceWithModifyPlan  = &SelfServiceBrandingImageResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
)

// selfServiceBrandingImageIdentityModel is the identity struct for import.
type selfServiceBrandingImageIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// NewSelfServiceBrandingImageResource returns a new instance of the resource.
func NewSelfServiceBrandingImageResource() resource.Resource {
	return &SelfServiceBrandingImageResource{}
}

// Metadata sets the resource type name.
func (r *SelfServiceBrandingImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_branding_image"
}

// IdentitySchema defines the identifier used for import.
func (r *SelfServiceBrandingImageResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Self Service branding image ID used to uniquely reference the image.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the branding image resource.
func (r *SelfServiceBrandingImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Uploads an image to Jamf Pro for use in Self Service branding. The resulting ` + "`id`" + ` is referenced by ` + "`jamfplatform_pro_self_service_branding_macos`" + ` (` + "`icon_id`" + ` / ` + "`banner_image_id`" + `) and ` + "`jamfplatform_pro_self_service_branding_ios`" + ` (` + "`icon_id`" + `).

The Self Service branding image store is **separate** from the general Jamf Pro icon store (` + "`jamfplatform_pro_icon`" + `). The same numeric ID refers to a different image in each store, so a branding configuration must reference an ID minted by this resource rather than a ` + "`jamfplatform_pro_icon`" + ` ID.

### Change detection

Jamf Pro has no branding-image update endpoint, so changing the image replaces the resource. ` + "`source_hash`" + ` holds a SHA-256 of the uploaded bytes. The provider computes it during apply from the exact bytes it sends, so any plan that creates or replaces an image shows ` + "`source_hash = (known after apply)`" + `.

` + "`image_file_source`" + ` accepts a local filesystem path or an ` + "`http(s)://`" + ` URL, and the provider detects a change differently for each.

The provider reads and hashes a local path on every plan. Edit the file and the next plan shows the replacement.

For a URL it waits until apply, and compares the URL string at plan time instead. Re-point the URL and the next plan replaces the image. It cannot hash a URL during a plan, because remote content is not stable between reads. A CDN can answer one URL with different bytes from one request to the next, so a plan-time hash proposes replacements nobody asked for and fails applies. The cost is that the provider will not see a new file published behind an unchanged URL; to track a vendor's artwork, download it and point ` + "`image_file_source`" + ` at the committed copy.

### Dimensions

The Jamf Pro admin UI recommends 180×180 for an icon and 1500×235 for a home page banner. PNG or GIF.

### Destroy behaviour

Jamf Pro cannot delete a branding image. ` + "`terraform destroy`" + ` and replacements remove the resource from Terraform state only; the image stays on the tenant.

### Import

` + "`terraform import jamfplatform_pro_self_service_branding_image.example 81`" + `. The provider downloads the image bytes from Jamf Pro and stores their SHA-256.

Point ` + "`image_file_source`" + ` at the file you uploaded. This store hands an image back exactly as you sent it, so the hash import records matches your own copy and the first plan after import is an in-place update. The general icon store (` + "`jamfplatform_pro_icon`" + `) re-encodes a PNG, which is why its import workflow tells you to download the stored copy instead; that advice does not apply here.` + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro Self Service branding image ID, derived from the upload URL. Changes when the resource is replaced.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"image_file_source": schema.StringAttribute{
				MarkdownDescription: "Local filesystem path or `http(s)://` URL to the image. The provider reads a local path on every plan and again on apply. It reads a URL on apply only, so a plan compares the URL string rather than the bytes it serves.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"source_hash": schema.StringAttribute{
				MarkdownDescription: "SHA-256 of the uploaded image bytes, prefixed `sha256:`. The provider computes it during apply from the bytes it sends, so it reads `(known after apply)` on any plan that creates or replaces the image.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "Download URL returned by Jamf Pro after upload.",
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

// Configure wires the Jamf Pro client into the resource.
func (r *SelfServiceBrandingImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_branding_image")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ModifyPlan decides whether a configuration change replaces the image, and
// leaves source_hash unresolved on every plan that creates one.
//
// Jamf Pro exposes an upload and a download for this store and nothing else, so
// the only way to change an image is to replace the resource, and the id is
// derived from the upload URL so a replacement mints a new one. source_hash
// carries that decision because image_file_source cannot: a path can be
// re-pointed at byte-identical content, and content can change underneath an
// unchanged path.
//
// Where the hash comes from depends on the source, because the two differ in
// whether two reads of the same source answer with the same bytes:
//
//   - A create leaves source_hash Unknown. Create computes the hash from the
//     exact bytes it uploads, so a source that answers two reads differently
//     can no longer plan one value and apply another (issue #373, reproduced on
//     this resource on 2026-09-04). A local path is still opened and hashed
//     here and the hash discarded, so an unreadable one fails at plan rather
//     than part-way through an apply.
//   - A local path on an existing image is read and hashed here, and a
//     differing hash replaces the resource. Local bytes are stable, so the
//     operator sees the replacement in terraform plan.
//   - An http(s):// URL on an existing image is not fetched here. Remote
//     content is not stable between reads, so hashing it would propose a
//     replacement on plans where nothing had changed. The URL string is
//     compared instead, which does not detect content published behind an
//     unchanged URL.
//
// The destroy path returns before touching the source; a destroy needs no bytes.
func (r *SelfServiceBrandingImageResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan SelfServiceBrandingImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.ImageFileSource.IsNull() || plan.ImageFileSource.IsUnknown() || plan.ImageFileSource.ValueString() == "" {
		return
	}

	source := plan.ImageFileSource.ValueString()
	localSource := !files.URLSource(source)

	var hash string
	if localSource {
		hashed, err := files.HashLocalSource(ctx, source)
		if err != nil {
			resp.Diagnostics.AddError("Error reading branding image source during plan", helpers.APIErrorDetail(err))
			return
		}
		hash = hashed
	}

	if req.State.Raw.IsNull() {
		return
	}

	var state SelfServiceBrandingImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if localSource {
		if hash == state.SourceHash.ValueString() {
			return
		}
		planImageReplacement(ctx, resp, &plan, path.Root("source_hash"))
		return
	}

	if source == state.ImageFileSource.ValueString() {
		return
	}
	planImageReplacement(ctx, resp, &plan, path.Root("image_file_source"))
}

// ImportState handles import by the Jamf Pro branding image ID.
func (r *SelfServiceBrandingImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
