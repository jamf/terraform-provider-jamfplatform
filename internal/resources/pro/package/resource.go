// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: /v1/packages predates the provider's overall floor; the
// shared providerdata.ConfigurePro helper still surfaces the global floor
// advisory when the tenant is older.
const minJamfProVersion = ""

// PackageResource implements the Terraform resource for Jamf Pro packages.
type PackageResource struct {
	client *pro.Client
	// impact backs the plan-time impact alert reporting how many computers this
	// object reaches through the policies that use it. nil when the provider's
	// impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var (
	_ resource.Resource                = &PackageResource{}
	_ resource.ResourceWithImportState = &PackageResource{}
	_ resource.ResourceWithIdentity    = &PackageResource{}
)

const (
	// Upload + verification can take minutes on multi-GB binaries — 30m
	// matches the v1 spike default. Read/Delete remain on the standard
	// 60s budget; no upload path runs.
	defaultCreateTimeout = 30 * time.Minute
	defaultUpdateTimeout = 30 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPackageResource returns a new instance of PackageResource.
func NewPackageResource() resource.Resource {
	return &PackageResource{}
}

// Metadata sets the resource type name.
func (r *PackageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_package"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *PackageResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro package ID used to uniquely reference the package record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the package resource.
func (r *PackageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro package. A package record carries the metadata Jamf Pro pairs with a file on a distribution point: name, category, restart requirement, operating system requirement, info and notes, an optional manifest and optional hashes.\n\n### Operating modes\n\nThe mode is inferred from your configuration.\n\n- Cloud distribution point upload. Set `package_file_source` to a local path or an `https?://` URL. The provider creates the package record, uploads the file to the Jamf Cloud Distribution Point, and waits until Jamf Pro finishes calculating the file hashes and `cloud_transfer_status` becomes `READY`. In this mode Jamf Pro populates the hash attributes (`sha3_512`, `sha256`, `md5`, `size`, `hash_type`, `hash_value`), and setting any of them in configuration is rejected before the change runs.\n- Distribution point with supplied hashes. Omit `package_file_source` and set the hash attributes directly. Jamf Pro stores the values as given, without validating them. Use this when the file lives on a distribution point you manage and the hashes are calculated elsewhere.\n- Metadata only. Omit `package_file_source` and every hash attribute. The provider manages the package record alone, and no file is uploaded.\n\n### Uploads and updates\n\n- `package_file_source_checksum` (optional) is a SHA-3-512 value checked against the file locally before anything is uploaded. A mismatch fails the apply without uploading, which catches on-disk corruption.\n- `size` is read-only. Jamf Pro calculates it from the uploaded file and ignores any value set in configuration, including on metadata-only records.\n- Changing only metadata (`info`, `notes`, `priority` and so on) updates the record without re-uploading the file. The file is re-uploaded only when its contents change.\n- `manifest_file_source` (optional) uploads a `.plist` manifest for the package. Setting it uploads the manifest, and clearing it removes the manifest from Jamf Pro. The manifest is likewise re-uploaded only when its contents change.\n- Both `package_file_source` and `manifest_file_source` accept `http(s)://` URLs. The provider downloads the URL to a temporary file, up to 8 GiB and following at most 10 redirects, before uploading." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Package ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Required.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"file_name": schema.StringAttribute{
				MarkdownDescription: "**\"Filename\"** in the Jamf Pro admin UI. The on-disk filename Jamf Pro associates with the binary on the distribution point. Required.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"category_id": schema.StringAttribute{
				MarkdownDescription: "**\"Category\"** in the Jamf Pro admin UI. Defaults to `\"-1\"` (None).",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"info": schema.StringAttribute{
				MarkdownDescription: "**\"Info\"** in the Jamf Pro admin UI. Free-form metadata field. Jamf Pro returns an empty string when the field is unset, and the provider reconciles that to keep state stable.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"notes": schema.StringAttribute{
				MarkdownDescription: "**\"Notes\"** in the Jamf Pro admin UI. Free-form notes field. Jamf Pro returns an empty string when the field is unset, and the provider reconciles that to keep state stable.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "**\"Priority\"** in the Jamf Pro admin UI. Defaults to `10`. Lower values install first when multiple packages are queued.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.Between(1, 20),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"fill_user_template": schema.BoolAttribute{
				MarkdownDescription: "**\"Fill user templates (FUT)\"** in the Jamf Pro admin UI. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"fill_existing_users": schema.BoolAttribute{
				MarkdownDescription: "**\"Fill existing user home directories (FEU)\"** in the Jamf Pro admin UI. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"reboot_required": schema.BoolAttribute{
				MarkdownDescription: "**\"Requires restart\"** in the Jamf Pro admin UI. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"os_requirements": schema.StringAttribute{
				MarkdownDescription: "**\"Operating system requirement\"** in the Jamf Pro admin UI (Limitations tab). Comma-separated string, e.g. `\"13.5.2, 13.6.x, 14.3\"`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"available_in_software_update": schema.BoolAttribute{
				MarkdownDescription: "**\"Install only if available in Software Update\"** in the Jamf Pro admin UI. Defaults to `false`. This is not the separate OS-installer flag, which this resource does not expose.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			// Upload-source inputs (no wire field — pure provider plumbing).
			"package_file_source": schema.StringAttribute{
				MarkdownDescription: "Optional local path or `http(s)://` URL pointing to the package file. Set it to upload the file to the Jamf Cloud Distribution Point on create or update. A cloud distribution point must be configured on the tenant; without one the change is refused before any bytes are uploaded. When omitted, the resource manages only the package record. Cannot be combined with the hash attributes; setting both is rejected before the change runs.",
				Optional:            true,
			},
			"package_file_source_checksum": schema.StringAttribute{
				MarkdownDescription: "Optional SHA-3-512 hex digest checked against the file locally before it is uploaded. A mismatch fails the apply without uploading. Cannot be combined with `stream_url_directly`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("stream_url_directly")),
				},
			},
			"stream_url_directly": schema.BoolAttribute{
				MarkdownDescription: "When `true` and `package_file_source` is an `http(s)://` URL, the file is sent straight to the Jamf Cloud Distribution Point as it downloads, instead of being saved to a temporary file first. This avoids using local disk, but disables retry on rate-limiting, pre-upload checksum validation (so it cannot be combined with `package_file_source_checksum`), and recovery if the download is interrupted partway. Use it on disk-constrained machines with very large files. Ignored for local-path sources.",
				Optional:            true,
			},
			"manifest_file_source": schema.StringAttribute{
				MarkdownDescription: "**\"Manifest file\"** in the Jamf Pro admin UI. Optional local path or `http(s)://` URL to a `.plist` manifest. The provider uploads when this is set and the stored `manifest` body differs from the freshly-loaded source content. Clearing this attribute deletes the manifest in Jamf Pro.",
				Optional:            true,
			},

			// Server-echoed manifest fields (Computed).
			"manifest": schema.StringAttribute{
				MarkdownDescription: "The raw plist body Jamf Pro stores for the package manifest. Empty when no manifest is uploaded. Direct-equality compared against the freshly-loaded `manifest_file_source` to decide whether to re-upload.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("manifest_file_source")),
				},
			},
			"manifest_file_name": schema.StringAttribute{
				MarkdownDescription: "The filename Jamf Pro echoed back for the most recent manifest upload.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("manifest_file_source")),
				},
			},

			// Hash attrs: Optional+Computed — Jamf Pro calculates them after a
			// cloud distribution point upload, or the user supplies them when
			// managing a record without an uploaded file.
			"sha3_512": schema.StringAttribute{
				MarkdownDescription: "SHA-3-512 hex digest of the package file. Calculated by Jamf Pro after a cloud distribution point upload, or set directly when managing the record without an uploaded file. Cannot be combined with `package_file_source`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("package_file_source")),
				},
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("package_file_source")),
				},
			},
			"sha256": schema.StringAttribute{
				MarkdownDescription: "SHA-256 hex digest of the package file. Calculated by Jamf Pro after a cloud distribution point upload, or set directly when managing the record without an uploaded file. Cannot be combined with `package_file_source`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("package_file_source")),
				},
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("package_file_source")),
				},
			},
			"md5": schema.StringAttribute{
				MarkdownDescription: "MD5 hex digest of the package file. Calculated by Jamf Pro after a cloud distribution point upload, or set directly when managing the record without an uploaded file. Cannot be combined with `package_file_source`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("package_file_source")),
				},
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("package_file_source")),
				},
			},
			"hash_type": schema.StringAttribute{
				MarkdownDescription: "Hash algorithm for the package. One of " + markdownValueList(AllowedHashTypeValues) + ". A record that has never had a file uploaded reads back `SHA_512`. Cannot be combined with `package_file_source` (a cloud distribution point upload sets this to `SHA3_512`).",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(AllowedHashTypeValues...),
					stringvalidator.ConflictsWith(path.MatchRoot("package_file_source")),
				},
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("package_file_source")),
				},
			},
			"hash_value": schema.StringAttribute{
				MarkdownDescription: "Primary hash Jamf Pro uses to match the package on a distribution point. Set to the SHA-3-512 of the file after a cloud distribution point upload, or supply a digest matching `hash_type` when managing the record directly. Cannot be combined with `package_file_source`, and requires `hash_type` when set.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("package_file_source")),
					stringvalidator.AlsoRequires(path.MatchRoot("hash_type")),
				},
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("package_file_source")),
				},
			},

			// Read-only — Jamf Pro derives size from the uploaded binary and
			// silently drops any value supplied on create or update (wire-probed
			// platform-nmartin 2026-06-25). Every metadata update blanks the
			// server-managed value, which the resource re-derives from a cloud
			// distribution point refresh afterwards; modelled as plain Computed
			// (no state-forwarding plan modifier) per STYLE_GUIDE §Server-derived
			// computed fields so it can recompute when the binary changes without
			// tripping "inconsistent result after apply".
			"size": schema.StringAttribute{
				MarkdownDescription: "Package binary size in bytes. Read-only: Jamf Pro calculates it from the uploaded package file, so any value set in configuration is ignored. A package managed as metadata only, with no uploaded file, leaves this empty. It shows as `(known after apply)` on any update that changes the package, because Jamf Pro recalculates the size after the change is saved.",
				Computed:            true,
			},
			"install_language": schema.StringAttribute{
				MarkdownDescription: "Locale tag, default `\"en_US\"`. Returned by Jamf Pro; not user-settable. Not exposed in the Jamf Pro admin UI; surfaced as read-only to avoid drift on refresh.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parent_package_id": schema.StringAttribute{
				MarkdownDescription: "Parent package ID, default `\"-1\"` (no parent). Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"self_healing_action": schema.StringAttribute{
				MarkdownDescription: "Self-healing action, default `\"nothing\"`. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"self_heal_notify": schema.BoolAttribute{
				MarkdownDescription: "Self-healing notification flag. Default `false`. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"cloud_transfer_status": schema.StringAttribute{
				MarkdownDescription: "Cloud distribution point transfer status. Jamf Pro updates it as it processes an uploaded file, reaching `\"READY\"` once the upload is complete. Empty for a record with no uploaded file.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					resetIfSourceChangedString(path.MatchRoot("package_file_source")),
				},
			},
			"indexed": schema.BoolAttribute{
				MarkdownDescription: "Distribution-point indexing telemetry. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"format": schema.StringAttribute{
				MarkdownDescription: "Distribution-point format string. Returned by Jamf Pro; not user-settable.",
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
func (r *PackageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_package")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro package ID.
func (r *PackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
