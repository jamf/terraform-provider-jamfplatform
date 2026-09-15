// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package supervision_identity implements the
// jamfplatform_pro_supervision_identity resource, data source, and list resource
// backed by the Jamf Pro supervision identities API — the certificate store
// behind Settings > Apple Configurator Enrollment.
//
// Two create paths, one resource (wire-probed 2026-06-12):
//   - certificate_data omitted -> Jamf Pro generates a new self-signed identity.
//   - certificate_data supplied -> the supplied .p12 is imported.
//
// Update is rename-only: display_name is the sole mutable field. The password and
// certificate are write-only secrets — Jamf Pro never echoes them back, and there
// is no API path that accepts a new password or certificate on an existing
// identity. Changing either therefore requires replacing the resource
// (terraform apply -replace); the provider cannot detect drift on write-only
// values, so they carry no RequiresReplace plan modifier (it would never fire).
//
// common_name and expiration_date are baked into the certificate at create and
// are read-only; a rename does not change them (wire-probed).
package supervision_identity

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty string skips the per-resource version check — consistent with
// every other Pro resource. The underlying endpoint carries the
// `supervision-identities-preview` OpenAPI tag; the SDK is pinned and the wire
// contract was probed against Jamf Pro 11.28.1, but a preview contract may change.
const minJamfProVersion = ""

// SupervisionIdentityResource implements the Terraform resource for Jamf Pro
// supervision identities.
type SupervisionIdentityResource struct {
	client *pro.Client
}

var _ resource.Resource = &SupervisionIdentityResource{}
var _ resource.ResourceWithImportState = &SupervisionIdentityResource{}
var _ resource.ResourceWithIdentity = &SupervisionIdentityResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewSupervisionIdentityResource returns a new instance of SupervisionIdentityResource.
func NewSupervisionIdentityResource() resource.Resource {
	return &SupervisionIdentityResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *SupervisionIdentityResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_supervision_identity"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *SupervisionIdentityResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro supervision identity ID used to uniquely reference the identity.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the supervision identity resource.
func (r *SupervisionIdentityResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro supervision identity, the certificate used to supervise and enroll devices through Apple Configurator (Settings → Apple Configurator Enrollment).\n\n" +
			"Provide `certificate_data` to import an existing `.p12` identity, or omit it to have Jamf Pro generate a new identity for you. " +
			"The password and certificate are write-only: they are sent to Jamf Pro but never stored in Terraform state, and Jamf Pro never returns them. " +
			"Only `display_name` can be changed in place; changing the password or certificate replaces the identity." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Supervision identity ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name for the supervision identity. Must not be empty. Can be changed in place to rename the identity.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password that protects the supervision identity's `.p12`. When generating a new identity this is the password Jamf Pro assigns to the minted certificate; when importing an existing `.p12` this is its passphrase. " +
					"`WriteOnly`: sent to Jamf Pro when the identity is created, never stored in Terraform state, and never returned on read. To change it, replace the identity with `terraform apply -replace`.",
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
			},
			"certificate_data": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded `.p12` certificate to import as the supervision identity. Supply with `filebase64(\"identity.p12\")`. " +
					"Omit this to have Jamf Pro generate a new identity instead. " +
					"`WriteOnly`: sent to Jamf Pro when the identity is created, never stored in Terraform state, and never returned on read. To change it, replace the identity with `terraform apply -replace`.",
				Optional:  true,
				Sensitive: true,
				WriteOnly: true,
			},
			"common_name": schema.StringAttribute{
				MarkdownDescription: "Common name of the supervision identity's certificate. Read-only; set by Jamf Pro from the certificate when the identity is created.",
				Computed:            true,
			},
			"expiration_date": schema.StringAttribute{
				MarkdownDescription: "Certificate expiration date (`YYYY-MM-DD`). Read-only; set by Jamf Pro from the certificate when the identity is created.",
				Computed:            true,
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
func (r *SupervisionIdentityResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_supervision_identity")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro supervision identity ID.
func (r *SupervisionIdentityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
