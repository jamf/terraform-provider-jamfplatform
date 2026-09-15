// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package device_group implements the jamfplatform_security_cloud_device_group
// resource, data sources and list resource backed by the Jamf Security Cloud
// device groups API.
//
// A device group is a named bucket of enrolled devices. It is the unit Jamf
// Security Cloud assigns access by: a ZTNA app's assignments name device groups,
// as does UEM Connect's group-membership mapping. The admin UI calls them "Device
// groups" on their own screens and "Jamf Security Cloud group" when one is
// referenced from elsewhere, which is why an attribute on another resource that
// points at one is named `security_cloud_group_id`.
//
// The object is a name and an identifier and nothing else. There is no criteria
// expression, no description, no timestamps, and no membership representation on
// the group — membership is authored entirely by the referrers. A create carrying
// `description` and `criteria` was accepted with both fields silently discarded
// (wire-probed 2026-08-29), which is the confirmation rather than an inference.
// So there is no wire-name mapping table here: `id` and `name` are the wire
// names, and neither is cryptic nor materially different from the UI, so
// STYLE_GUIDE §Attribute names mirror the admin UI prescribes no rename.
//
// Five wire laws drive the code, all probed against the EU sandbox on 2026-08-29
// with raw bodies under local-testing/securitycloud-groups/:
//
// 1. Only the list endpoint has a v2. `GET /v2/groups` is the wrapped successor
// to the deprecated bare-array `GET /v1/groups` and returns byte-identical
// content, but `GET` and `PUT` on `/v2/groups/{id}` are not served at all — so
// reads-by-id, creates, updates and deletes go to v1 and there is no migration
// pending. (The SDK generates UpdateDeviceGroupV2 from the spec; the gateway does
// not route it. Raised upstream; do not call it.) Note that STYLE_GUIDE
// §Endpoint adoption & migration policy is scoped to Jamf Pro and does not
// govern this split — the wire does.
//
// 2. An unmapped Security Cloud route answers 403 BAD_PERMISSIONS, not 404. A
// control probe on a deliberately bogus path returns the identical body to the
// unserved v2 routes above, so that status cannot be read as "the token lacks a
// privilege" and is deliberately left untranslated. See mappings.go.
//
// 3. The server silently trims surrounding whitespace from `name`, and reserves
// "Default Group" case-insensitively after trimming. Both are refused at plan
// time by validators.go rather than absorbed — see that file for why trimming
// provider-side is the wrong fix.
//
// 4. Name uniqueness is per customer and case-sensitive, while the reserved-name
// comparison is case-insensitive. Two different case rules on one field.
//
// 5. Deleting a group that something still references is NOT refused. This is
// the opposite of the Security Cloud shape STYLE_GUIDE documents for ZTNA
// gateways, where a referenced delete is a bare 409 and a Terraform
// destroy-ordering trap. Here the delete succeeds and the referrer is silently
// left pointing at nothing — a ZTNA app was observed dropping to
// `allUsers: false` with an empty group list, a combination the API itself
// rejects on write. There is therefore no destroy-ordering diagnostic to write,
// and nothing for Terraform to sequence; the hazard is on the referrer's side and
// is documented in the resource description instead.
package device_group

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DeviceGroupResource implements the Terraform resource for Jamf Security Cloud
// device groups.
type DeviceGroupResource struct {
	client *securitycloud.Client
}

var (
	_ resource.Resource                = &DeviceGroupResource{}
	_ resource.ResourceWithImportState = &DeviceGroupResource{}
	_ resource.ResourceWithIdentity    = &DeviceGroupResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDeviceGroupResource returns a new instance of DeviceGroupResource.
func NewDeviceGroupResource() resource.Resource {
	return &DeviceGroupResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DeviceGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_device_group"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *DeviceGroupResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Device group ID used to uniquely reference the group.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the device group resource.
//
// `name` is the only writable attribute, and it is Required rather than
// Optional+Computed because a create or update without it is refused — see
// STYLE_GUIDE §Full-replace endpoints, whose Optional+Computed default applies to
// optional scalars and explicitly carves out API-required fields. With one
// required field there is no sparse body to model either.
//
// The absent length validator is deliberate and contradicts the bundled spec, so
// do not "correct" it on an SDK bump. `securitycloud_device_groups_api.json`
// declares maxLength 255 on all four name schemas; the server does not enforce it.
// Wire-probed 2026-08-29: names of 255, 256, 1024 and 65536 characters were each
// accepted with 201 and read back byte-intact. Wire truth beats the spec here, and
// a LengthAtMost(255) would refuse configs the API stores happily. Unlike
// dns_zone, whose 1-to-100 limit the server does enforce.
func (r *DeviceGroupResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Security Cloud device group. Device groups are how access is assigned: " +
			"an app's assignments name the groups whose members may reach it, and UEM Connect maps the groups it " +
			"syncs from Jamf Pro onto them.\n\n" +
			"Destroying a group does not fail when something still points at it. Jamf Security Cloud removes the " +
			"group and quietly drops it from every app assignment and mapping that named it, which can leave those " +
			"objects assigned to nobody. Check what references a group before removing it.\n\n" +
			"The built-in group named \"Default Group\" cannot be managed here: Jamf Security Cloud gives it no " +
			"identifier and reserves its name. Use the `jamfplatform_security_cloud_device_groups` data source to " +
			"see it." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Device group ID assigned by Jamf Security Cloud.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Group name\"** in the Jamf Security Cloud admin UI. Must be unique on the " +
					"tenant. Jamf Security Cloud compares names exactly, so two groups may differ only in " +
					"capitalisation. Leading and trailing whitespace is not accepted, because Jamf Security Cloud " +
					"would silently remove it. There is no length limit.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					GroupName(),
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

// Configure wires the Jamf Security Cloud client into the resource via the shared
// providerdata.ConfigureSecurityCloud helper.
func (r *DeviceGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Security Cloud device group ID.
func (r *DeviceGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
