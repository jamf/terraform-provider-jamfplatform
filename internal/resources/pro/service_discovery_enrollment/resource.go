// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package service_discovery_enrollment implements the
// jamfplatform_pro_service_discovery_enrollment singleton resource and data source
// backed by the Jamf Pro service-discovery well-known settings API. These settings
// let Jamf Pro host the .well-known service-discovery redirect for Account-Driven
// (account-driven Device / User) enrollment, per synced Apple Business/School
// Manager (AxM) organization.
package service_discovery_enrollment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// The /v1/service-discovery-enrollment/well-known-settings endpoint (Jamf-hosted
// service discovery for Account-Driven enrollment) was introduced in Jamf Pro
// 11.25.0 (per Jamf Learn: "Prepare for Account-Driven Enrollment with Managed
// Apple IDs and Service Discovery").
const minJamfProVersion = "11.25.0"

// ServiceDiscoveryEnrollmentResource implements the singleton resource for the Jamf
// Pro service-discovery well-known settings. Backed by an Update-only API (no
// Create/Delete on the remote): Create funnels into a PUT; Delete is a no-op that
// only removes the object from Terraform state. The PUT returns 204 No Content with
// no echo, so Create/Update read back via GET for authoritative state.
type ServiceDiscoveryEnrollmentResource struct {
	client *pro.Client
}

var _ resource.Resource = &ServiceDiscoveryEnrollmentResource{}
var _ resource.ResourceWithImportState = &ServiceDiscoveryEnrollmentResource{}
var _ resource.ResourceWithIdentity = &ServiceDiscoveryEnrollmentResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewServiceDiscoveryEnrollmentResource returns a new instance of the resource.
func NewServiceDiscoveryEnrollmentResource() resource.Resource {
	return &ServiceDiscoveryEnrollmentResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ServiceDiscoveryEnrollmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_service_discovery_enrollment"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *ServiceDiscoveryEnrollmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Service-discovery well-known settings are one record per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the service-discovery well-known settings
// resource.
func (r *ServiceDiscoveryEnrollmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro's hosted service-discovery (\"well-known\") settings for Account-Driven enrollment. " +
			"When configured, Jamf Pro hosts the " +
			"`.well-known` redirect that Apple devices fetch during Account-Driven Device (`mdm-adde`) or User (`mdm-byod`) " +
			"enrollment, so you do not have to self-host the service-discovery JSON. Requires Jamf Pro 11.25.0 or later. " +
			"One record per tenant.\n\n" +
			"### Jamf Pro fixes the set of rows\n\n" +
			"Each row corresponds to a synced Apple Business/School Manager " +
			"(AxM) organization, identified by the Server UUID of its Automated Device Enrollment token (Settings > " +
			"Automated Device Enrollment > Server UUID). You can only set `enrollment_type` on a `server_uuid` Jamf Pro " +
			"already knows. A `server_uuid` that does not match a synced AxM org is silently ignored by Jamf Pro, and the " +
			"provider emits a warning when that happens.\n\n" +
			"### Only the rows you declare are managed\n\n" +
			"Rows for AxM orgs you do " +
			"not declare are left untouched. Removing a `well_known_setting` block stops managing that org and leaves its " +
			"current Jamf Pro value unchanged; it does **not** reset it. To turn off Jamf-hosted service discovery for an org, " +
			"set its `enrollment_type = \"none\"` rather than deleting the block.\n\n" +
			"Import with `terraform import jamfplatform_pro_service_discovery_enrollment.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"well_known_setting": schema.ListNestedAttribute{
				MarkdownDescription: "The per-organization service-discovery rows to manage. Each row sets the Account-Driven " +
					"enrollment type for one synced AxM organization, keyed by its `server_uuid`. Only the rows declared here " +
					"are written; undeclared orgs are left untouched (merge). Removing a row stops managing it and preserves its " +
					"current Jamf Pro value: set `enrollment_type = \"none\"` to disable an org. An empty list writes nothing.",
				Required: true,
				NestedObject: schema.NestedAttributeObject{
					// org_name is a plain Computed echo (read-only; the server returns the
					// canonical AxM org display name and ignores any value sent — wire-probed).
					// It carries NO plan modifier: on a changed/new row it goes Unknown and is
					// filled from the post-write GET; a true no-op preserves the prior value.
					// The §UseNonNullStateForUnknown rule applies to Optional+Computed nested
					// fields, not Computed-only echoes (precedent:
					// jamfplatform_pro_macos_onboarding's onboarding_items echoes).
					Attributes: map[string]schema.Attribute{
						"server_uuid": schema.StringAttribute{
							MarkdownDescription: "Server UUID of the synced Automated Device Enrollment (AxM) token this row applies to. " +
								"Find it in Jamf Pro under Settings > Automated Device Enrollment > Server UUID, or read it from " +
								"`jamfplatform_pro_automated_device_enrollment`. A value Jamf Pro does not recognize is silently ignored.",
							Required: true,
						},
						"enrollment_type": schema.StringAttribute{
							MarkdownDescription: "The Account-Driven enrollment type Jamf Pro hosts service discovery for, on this org. " +
								"One of `none` (no Jamf-hosted service discovery), `mdm-byod` (Account-Driven User Enrollment / BYOD), " +
								"or `mdm-adde` (Account-Driven Device Enrollment).",
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf(validEnrollmentTypes...),
							},
						},
						"org_name": schema.StringAttribute{
							MarkdownDescription: "Display name of the Apple Business/School Manager organization, as returned by Jamf Pro. " +
								"Returned by Jamf Pro; not user-settable. Any value supplied is ignored.",
							Computed: true,
						},
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *ServiceDiscoveryEnrollmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_service_discovery_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error.
//
//	terraform import jamfplatform_pro_service_discovery_enrollment.<name> singleton
func (r *ServiceDiscoveryEnrollmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_service_discovery_enrollment")
}
