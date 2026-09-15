// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package icon implements the jamfplatform_pro_icon resource backed by the
// Jamf Pro icon upload API.
package icon

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

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty string skips the version check — the icon endpoint has been stable since
// well before the provider's overall floor (11.0.0), so no per-resource gate is needed.
const minJamfProVersion = ""

// IconResource implements the Terraform resource for Jamf Pro icons.
type IconResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &IconResource{}
	_ resource.ResourceWithImportState = &IconResource{}
	_ resource.ResourceWithIdentity    = &IconResource{}
	_ resource.ResourceWithModifyPlan  = &IconResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
)

// iconIdentityModel is the identity struct for Jamf Pro icon resources.
type iconIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// NewIconResource returns a new instance of IconResource.
func NewIconResource() resource.Resource {
	return &IconResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *IconResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_icon"
}

// IdentitySchema defines the identifier used for import.
func (r *IconResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro icon ID used to uniquely reference the icon.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the icon resource.
func (r *IconResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Manages a Jamf Pro icon. Icons are uploaded to Jamf Pro and referenced by ID from Self Service branding configurations.

### Change detection

Jamf Pro has no icon update endpoint, so changing the image replaces the resource. ` + "`source_hash`" + ` holds a SHA-256 of the bytes the provider uploaded, computed during apply from the exact bytes it sends. Any plan that creates or replaces an icon therefore shows ` + "`source_hash = (known after apply)`" + `.

` + "`icon_file_source`" + ` accepts a local filesystem path or an ` + "`http(s)://`" + ` URL, and the provider detects a change differently for each.

The provider reads and hashes a local path on every plan. Edit the file and the next plan shows the replacement.

For a URL it waits until apply, and compares the URL string at plan time instead. Re-point the URL and the next plan replaces the icon. Publish a new image behind an unchanged URL and the provider will not see it. It cannot hash a URL during a plan, because remote content is not stable between reads: Apple's iTunes artwork CDN answers one URL with different bytes from one request to the next, so a plan-time hash proposes replacements nobody asked for and fails applies. To track a vendor's image, download it and point ` + "`icon_file_source`" + ` at the committed copy.

### Destroy behaviour

Jamf Pro cannot delete an icon record. ` + "`terraform destroy`" + ` and replacements remove the resource from Terraform state only; the icon stays on the tenant.

### Import workflow

1. ` + "`terraform import jamfplatform_pro_icon.example 42`" + `. The provider downloads the icon bytes from the CDN URL and stores their SHA-256 in state.
2. Download the icon locally from the URL stored in state, for example ` + "`curl -o ./icon.png \"$(terraform state show jamfplatform_pro_icon.example | awk '/^[[:space:]]*url/{print $3}' | tr -d '\\\"')\"`" + `. Take it from that URL rather than reusing the file you uploaded: Jamf Pro re-encodes uploaded PNGs, with different zlib compression or metadata, so the bytes the CDN serves back are not byte-identical to the ones you sent.
3. Add ` + "`icon_file_source = \"./icon.png\"`" + ` to your configuration.
4. ` + "`terraform plan`" + ` shows an in-place update on ` + "`icon_file_source`" + ` (null to path) and replaces nothing, because the local bytes now match what import stored.

Point ` + "`icon_file_source`" + ` at your original upload instead of the CDN-downloaded copy and the first plan after import replaces the icon, since the local bytes hash to something other than what import stored. Give an imported icon a URL source and the first plan replaces it too: the provider compares URLs by string, and import leaves it no path to compare against.` + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro icon ID assigned on upload. Changes when the resource is replaced.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"icon_file_source": schema.StringAttribute{
				MarkdownDescription: "Local filesystem path or `http(s)://` URL to the icon image. The provider reads a local path on every plan and again on apply. It reads a URL on apply only, so a plan compares the URL string rather than the bytes it serves.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"source_hash": schema.StringAttribute{
				MarkdownDescription: "SHA-256 of the uploaded icon bytes, prefixed `sha256:`. The provider computes it during apply from the bytes it sends, so it reads `(known after apply)` on any plan that creates or replaces the icon.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "CDN URL returned by Jamf Pro after upload.",
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *IconResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_icon")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ModifyPlan decides whether a configuration change replaces the icon, and
// leaves source_hash unresolved on every plan that creates one.
//
// Jamf Pro has no icon update endpoint, so the only way to change an icon's
// image is to replace the resource. source_hash carries that decision because
// icon_file_source cannot: a path can be re-pointed at byte-identical content,
// and content can change underneath an unchanged path.
//
// Where the hash comes from depends on the source, because the two differ in
// whether two reads of the same source answer with the same bytes:
//
//   - A create leaves source_hash Unknown. Create computes the hash from the
//     exact bytes it uploads, so a source that answers two reads differently
//     can no longer plan one value and apply another (issue #373). A local path
//     is still opened and hashed here and the hash discarded, so an unreadable
//     one fails at plan rather than part-way through an apply.
//   - A local path on an existing icon is read and hashed here, and a differing
//     hash replaces the resource. Local bytes are stable, so the operator sees
//     the replacement in terraform plan.
//   - An http(s):// URL on an existing icon is not fetched here. Remote content
//     is not stable between reads: Apple's iTunes artwork CDN answers one URL
//     with different bytes from one request to the next, so hashing it would
//     propose a replacement on plans where nothing had changed. The URL string
//     is compared instead, which does not detect content published behind an
//     unchanged URL.
//
// The destroy path returns before touching the source; a destroy needs no bytes.
func (r *IconResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan IconResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.IconFileSource.IsNull() || plan.IconFileSource.IsUnknown() || plan.IconFileSource.ValueString() == "" {
		return
	}

	source := plan.IconFileSource.ValueString()
	localSource := !files.URLSource(source)

	var hash string
	if localSource {
		hashed, err := files.HashLocalSource(ctx, source)
		if err != nil {
			resp.Diagnostics.AddError("Error reading icon source during plan", helpers.APIErrorDetail(err))
			return
		}
		hash = hashed
	}

	if req.State.Raw.IsNull() {
		return
	}

	var state IconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if localSource {
		if hash == state.SourceHash.ValueString() {
			return
		}
		planIconReplacement(ctx, resp, &plan, path.Root("source_hash"))
		return
	}

	if source == state.IconFileSource.ValueString() {
		return
	}
	planIconReplacement(ctx, resp, &plan, path.Root("icon_file_source"))
}

// ImportState handles import by the Jamf Pro icon ID.
func (r *IconResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
