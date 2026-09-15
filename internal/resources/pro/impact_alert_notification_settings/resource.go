// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package impact_alert_notification_settings implements the
// jamfplatform_pro_impact_alert_notification_settings singleton resource and data source
// backed by the Jamf Pro Impact Alert Notification settings API.
package impact_alert_notification_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the Impact Alert Notification settings endpoint is present at the provider's
// overall floor, so no per-resource gate is needed.
const minJamfProVersion = ""

// ImpactAlertNotificationSettingsResource implements the singleton resource for Jamf Pro
// Impact Alert Notification settings. Backed by an Update-only API (no Create/Delete on
// the remote): Create funnels into Update; Delete is a no-op that only removes the object
// from Terraform state.
type ImpactAlertNotificationSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &ImpactAlertNotificationSettingsResource{}
var _ resource.ResourceWithImportState = &ImpactAlertNotificationSettingsResource{}
var _ resource.ResourceWithIdentity = &ImpactAlertNotificationSettingsResource{}
var _ resource.ResourceWithConfigValidators = &ImpactAlertNotificationSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewImpactAlertNotificationSettingsResource returns a new instance of the resource.
func NewImpactAlertNotificationSettingsResource() resource.Resource {
	return &ImpactAlertNotificationSettingsResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ImpactAlertNotificationSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_impact_alert_notification_settings"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *ImpactAlertNotificationSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Impact Alert Notification settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// alertToggleAttribute returns the schema for one of the two alert toggles.
func alertToggleAttribute(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

// Schema returns the Terraform schema for the Impact Alert Notification settings resource.
func (r *ImpactAlertNotificationSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro Impact Alert Notification settings (Settings > System > Impact alert notifications). " +
			"One record per tenant. " +
			"A toggle you omit keeps its current Jamf Pro value, including on the first apply: this resource adopts the existing settings and changes only the toggles you declare. A boolean has no \"unset\", so omit it to preserve the value or set `true`/`false` to change it. " +
			"A confirmation-code toggle requires its matching alert toggle to be `true`. Jamf Pro rejects `*_confirmation_code_enabled = true` while the matching `*_alert_enabled` is `false`. **To turn an alert off, set its matching `*_confirmation_code_enabled = false` in the same apply.** Omitting the confirmation-code toggle preserves the prior `true` and Jamf Pro rejects the apply. " +
			"Import with `terraform import jamfplatform_pro_impact_alert_notification_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"deployable_objects_alert_enabled": alertToggleAttribute(
				"Display deployment impact alert on Save for deployable objects (policies, configuration profiles, apps, managed software updates). " +
					"Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
			),
			"deployable_objects_confirmation_code_enabled": alertToggleAttribute(
				"Require Jamf Pro users to type a confirmation code (COMMIT) to acknowledge edits to deployable objects before saving. " +
					"Requires `deployable_objects_alert_enabled = true`. " +
					"Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
			),
			"scopeable_objects_alert_enabled": alertToggleAttribute(
				"Display criteria impact alert on Save for scopeable object edits (smart groups, static groups, classes). " +
					"Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
			),
			"scopeable_objects_confirmation_code_enabled": alertToggleAttribute(
				"Require Jamf Pro users to type a confirmation code (COMMIT) to acknowledge edits to scopeable objects before saving. " +
					"Requires `scopeable_objects_alert_enabled = true`. " +
					"Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
			),
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// ConfigValidators returns the cross-field validators evaluated at plan time. The single
// validator enforces the confirmation-code ↔ alert dependency that Jamf Pro enforces on
// the wire (wire-probed 2026-06-09: a confirmation code without its alert returns HTTP
// 400). It catches only the case where both fields are explicitly declared in config;
// preserved (omitted) values fall through to the server 400 — see validators.go.
func (r *ImpactAlertNotificationSettingsResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		confirmationCodeRequiresAlertValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *ImpactAlertNotificationSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_impact_alert_notification_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error so users do
// not accidentally end up with mis-keyed state that the resource silently normalizes
// on the next Read.
//
//	terraform import jamfplatform_pro_impact_alert_notification_settings.<name> singleton
func (r *ImpactAlertNotificationSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_impact_alert_notification_settings")
}
