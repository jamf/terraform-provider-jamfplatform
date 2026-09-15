// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package volume_purchasing_notification implements the
// jamfplatform_pro_volume_purchasing_notification resource, data source, and list
// resource. A Volume Purchasing notification (the Jamf Pro admin UI "Notifications"
// tab under Settings → Volume purchasing) emails Jamf Pro accounts and external
// recipients a daily summary when selected volume-purchasing events occur (an item
// is removed from the App Store, or a location runs out of licenses). The endpoint
// and SDK name the feature "subscriptions"; this package uses the UI-aligned
// "notification" vocabulary throughout.
package volume_purchasing_notification

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the Volume Purchasing endpoints predate the provider's overall
// floor (matching every volume_purchasing/ sibling). The provider-level advisory
// still fires through providerdata.ConfigurePro when the tenant is below the floor.
const minJamfProVersion = ""

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// VolumePurchasingNotificationResource implements the Terraform resource for a
// Volume Purchasing notification.
type VolumePurchasingNotificationResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &VolumePurchasingNotificationResource{}
	_ resource.ResourceWithImportState = &VolumePurchasingNotificationResource{}
	_ resource.ResourceWithIdentity    = &VolumePurchasingNotificationResource{}
)

// NewVolumePurchasingNotificationResource returns a new instance.
func NewVolumePurchasingNotificationResource() resource.Resource {
	return &VolumePurchasingNotificationResource{}
}

// Metadata sets the resource type name.
func (r *VolumePurchasingNotificationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_volume_purchasing_notification"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *VolumePurchasingNotificationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Volume Purchasing notification ID used to uniquely reference the notification.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *VolumePurchasingNotificationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Volume Purchasing notification, configured on the **\"Notifications\"** tab under Settings → Volume purchasing in the Jamf Pro admin UI. " +
			"A notification emails the chosen Jamf Pro accounts and external recipients a daily summary when one of the selected events occurs. " +
			"Every apply replaces recipients, triggers and included locations in full: omit one to leave its existing entries untouched, or set it to `[]` to clear it. Set `site_id` to `-1` for no site." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Notification ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display name\"** in the Jamf Pro admin UI. Must not be blank.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enabled\"** in the Jamf Pro admin UI. Whether the notification is active. Defaults to enabled. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"triggers": schema.SetAttribute{
				MarkdownDescription: "Events that send the notification. Any of `REMOVED_FROM_APP_STORE` (an item is removed from the App Store) or `NO_MORE_LICENSES` (a location runs out of licenses). Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` to send the notification for no events.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.OneOf(triggerEnumValues...)),
				},
			},
			"location_ids": schema.SetAttribute{
				MarkdownDescription: "**\"Included locations\"** in the Jamf Pro admin UI. Volume Purchasing location IDs (`jamfplatform_pro_volume_purchasing_location`) the notification covers. Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` for no locations.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"internal_recipients": schema.SetAttribute{
				MarkdownDescription: "**\"Existing Jamf Pro User Accounts\"** in the Jamf Pro admin UI. Jamf Pro account IDs that receive the daily summary email. Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` for no internal recipients.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"external_recipients": schema.SetNestedAttribute{
				MarkdownDescription: "**\"External Recipients\"** in the Jamf Pro admin UI. Email addresses outside Jamf Pro that receive the daily summary. Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` for no external recipients.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"email": schema.StringAttribute{
							MarkdownDescription: "**\"Email Address\"** in the Jamf Pro admin UI.",
							Required:            true,
							Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "**\"Full Name\"** in the Jamf Pro admin UI.",
							Required:            true,
							Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
						},
					},
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "**\"Site\"** in the Jamf Pro admin UI. Jamf Pro site ID scoping the notification. Omit to leave the current value untouched; there is no blank-clear, so set `-1` to remove the site.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
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
func (r *VolumePurchasingNotificationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_volume_purchasing_notification")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro notification ID.
func (r *VolumePurchasingNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
