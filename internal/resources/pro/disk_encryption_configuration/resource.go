// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package disk_encryption_configuration implements the
// jamfplatform_pro_disk_encryption_configuration resource, data source,
// and list resource backed by the Jamf ProClassic
// /diskencryptionconfigurations API.
package disk_encryption_configuration

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /diskencryptionconfigurations predates the
// provider's overall floor. The provider-level advisory still fires
// through providerdata.ConfigureProClassic when the tenant is below
// ProviderMinJamfProVersion.
const minJamfProVersion = ""

// DiskEncryptionConfigurationResource implements the Terraform resource for
// Jamf Pro disk encryption configurations.
type DiskEncryptionConfigurationResource struct {
	client *proclassic.Client
	// impact backs the plan-time impact alert reporting how many computers this
	// object reaches through the policies that use it. nil when the provider's
	// impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var (
	_ resource.Resource                     = &DiskEncryptionConfigurationResource{}
	_ resource.ResourceWithImportState      = &DiskEncryptionConfigurationResource{}
	_ resource.ResourceWithIdentity         = &DiskEncryptionConfigurationResource{}
	_ resource.ResourceWithConfigValidators = &DiskEncryptionConfigurationResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDiskEncryptionConfigurationResource returns a new instance of the
// resource.
func NewDiskEncryptionConfigurationResource() resource.Resource {
	return &DiskEncryptionConfigurationResource{}
}

// Metadata sets the resource type name.
func (r *DiskEncryptionConfigurationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_disk_encryption_configuration"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *DiskEncryptionConfigurationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro disk encryption configuration ID used to uniquely reference the configuration.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *DiskEncryptionConfigurationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro disk encryption configuration. Disk encryption configurations describe how Jamf-managed Macs derive a FileVault recovery key. The top-level fields (`name`, `key_type`, `file_vault_enabled_users`) are paired with an optional `institutional_recovery_key` block carrying the recovery certificate when `key_type` selects `Institutional` or `Individual and Institutional`.\n\n- `key_type` values use lowercase `and` in `Individual and Institutional`; see the attribute description for the full list.\n- `certificate_type` is required whenever `institutional_recovery_key` is supplied. Jamf Pro rejects the block otherwise with `Certificate type is required if a recovery key is specified`.\n- `institutional_recovery_key.password` is a Terraform `WriteOnly` attribute, sent to Jamf Pro on writes but never persisted in Terraform state. Pair it with `institutional_recovery_key.password_wo_version` to trigger rotation: bump the integer to force a new update carrying the current `password` value. Jamf Pro never returns the plaintext on read.\n- **Clearing the recovery key is not supported by Jamf Pro.** Once the `institutional_recovery_key` block is set, removing it or moving `key_type` from `Institutional` / `Individual and Institutional` back to `Individual` leaves the stored certificate in place. Destroy and recreate the resource to clear the recovery key material." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Disk encryption configuration ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Disk encryption configuration name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"key_type": schema.StringAttribute{
				MarkdownDescription: "**\"Recovery Key Type\"** in the Jamf Pro admin UI. Selects which recovery key Jamf Pro provisions when the Mac enables FileVault. Accepted values (must be supplied verbatim): `\"Individual\"`, `\"Institutional\"`, `\"Individual and Institutional\"` (lowercase `and`).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(allKeyTypeWireValues...),
				},
			},
			"file_vault_enabled_users": schema.StringAttribute{
				MarkdownDescription: "**\"Enabled FileVault 2 User\"** in the Jamf Pro admin UI. Account allowed to unlock FileVault. Accepted values: `\"Current or Next User\"`, `\"Management Account\"`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(allFileVaultEnabledUsersValues...),
				},
			},
			"institutional_recovery_key": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Institutional Recovery Key\"** in the Jamf Pro admin UI. Optional block carrying the recovery certificate that issues FileVault keys when `key_type` is `Institutional` or `Individual and Institutional`. The block is required in those modes; supplying it for `Individual` is allowed but meaningless, because Jamf Pro stores the certificate and never issues it.\n\nThis block stays Optional-only (not Computed) because the framework cannot fit an Unknown value into a typed pointer model.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						MarkdownDescription: "Subject DN extracted from the uploaded certificate (`data`). Returned by Jamf Pro; not user-settable. Sample: `C=US, O=jamf-tf-provider, CN=tf-audit-probe`.",
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"certificate_type": schema.StringAttribute{
						MarkdownDescription: "Certificate format. Required by Jamf Pro whenever a recovery key is supplied; Jamf Pro otherwise reports `Certificate type is required if a recovery key is specified`. Accepted values: `\"PKCS12\"` (private-key-containing .p12 upload), `\"DER\"` (public-cert binary), `\"PEM\"` (public-cert text). Use `\"PKCS12\"` (with `password` set) when `key_type` is `Institutional` or `Individual and Institutional`: only PKCS12 carries the private key Jamf needs to derive per-Mac recovery keys.",
						Required:            true,
					},
					"password": schema.StringAttribute{
						MarkdownDescription: "PKCS12 import password. `WriteOnly`: sent to Jamf Pro on writes, never persisted in Terraform state. Required when uploading a `.p12` certificate (the `data` payload contains the private key wrapped by this password). Omit for `.cer` / `.pem` uploads. Pair with `password_wo_version` to rotate the stored password.",
						Optional:            true,
						Sensitive:           true,
						WriteOnly:           true,
					},
					"password_wo_version": schema.Int64Attribute{
						MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer (any change) to force a new update that re-sends `password` to Jamf Pro. Initial create should set `password_wo_version = 1`. Leaving this attribute unset or unchanged signals \"leave the stored password alone\": the provider omits the password from the next update, so Jamf Pro retains the existing value.",
						Optional:            true,
					},
					"data": schema.StringAttribute{
						MarkdownDescription: "Base64-encoded recovery certificate payload. Required whenever the IRK block is supplied. Jamf Pro accepts `.p12` (PKCS12 with private key, required for the IRK to issue keys), `.cer` (DER binary), and `.pem` (PEM text). For PKCS12 uploads, also set `password`. Round-trips exactly on read. Marked sensitive because PKCS12 payloads contain the wrapped private key.",
						Required:            true,
						Sensitive:           true,
					},
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

// ConfigValidators returns the cross-field validators evaluated against the
// user's config at plan time.
func (r *DiskEncryptionConfigurationResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		institutionalKeyTypeRequiresIRKConfigValidator{},
	}
}

// Configure wires the Jamf ProClassic client into the resource.
func (r *DiskEncryptionConfigurationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_disk_encryption_configuration")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro disk encryption configuration ID.
func (r *DiskEncryptionConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
