// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package app_installer_settings implements the jamfplatform_pro_app_installer_settings
// singleton resource and data source backed by the Jamf Pro App Installer global settings API.
package app_installer_settings

import (
	"context"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// timeOfDayRegexp validates the HH:MM:SSZ format required by the App Installer time window fields.
var timeOfDayRegexp = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]Z$`)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the App Installer global settings endpoint is present at the provider's floor.
const minJamfProVersion = ""

// AppInstallerSettingsResource implements the singleton resource for Jamf Pro App
// Installer global settings.
type AppInstallerSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &AppInstallerSettingsResource{}
var _ resource.ResourceWithImportState = &AppInstallerSettingsResource{}
var _ resource.ResourceWithIdentity = &AppInstallerSettingsResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAppInstallerSettingsResource returns a new instance of AppInstallerSettingsResource.
func NewAppInstallerSettingsResource() resource.Resource {
	return &AppInstallerSettingsResource{}
}

// Metadata sets the resource type name.
func (r *AppInstallerSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_installer_settings"
}

// IdentitySchema defines the identifier used for import.
func (r *AppInstallerSettingsResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\" — App Installer settings are one-per-tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the App Installer settings resource.
func (r *AppInstallerSettingsResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro App Installer global settings. These settings are one per tenant.\n\n" +
			"Import with `terraform import jamfplatform_pro_app_installer_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"deployment_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Controls the batch deployment process for App Installer. Omit to leave the server's existing values untouched. Within this block, omitting a field clears it to null.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"batch_size": schema.Int64Attribute{
						MarkdownDescription: "Number of devices per command batch. The UI presents fixed options (100, 500, 1000, 5000, 10000) but the server accepts any positive integer.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
					"batch_frequency": schema.Int64Attribute{
						MarkdownDescription: "Minutes between batch deployments. Must be between 10 and 1440.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.Between(10, 1440),
						},
					},
					"days": schema.SetAttribute{
						MarkdownDescription: "Days of the week on which deployments run. `null` means no day restriction; `[]` explicitly clears to no days.",
						Optional:            true,
						ElementType:         types.StringType,
						Validators: []validator.Set{
							setvalidator.ValueStringsAre(
								stringvalidator.OneOf(
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekMonday,
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekTuesday,
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekWednesday,
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekThursday,
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekFriday,
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekSaturday,
									pro.AppInstallersDeploymentProcessControlsDaysOfWeekSunday,
								),
							),
						},
					},
					"server_time_from": schema.StringAttribute{
						MarkdownDescription: "Start of the daily deployment window. Format: `HH:MM:SSZ` (UTC, 24-hour). Example: `08:00:00Z`.",
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(
								timeOfDayRegexp,
								"must be in HH:MM:SSZ format with hours 00–23, e.g. 08:00:00Z",
							),
						},
					},
					"server_time_to": schema.StringAttribute{
						MarkdownDescription: "End of the daily deployment window. Format: `HH:MM:SSZ` (UTC, 24-hour). Example: `17:00:00Z`.",
						Optional:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(
								timeOfDayRegexp,
								"must be in HH:MM:SSZ format with hours 00–23, e.g. 17:00:00Z",
							),
						},
					},
				},
			},
			"end_user_experience": schema.SingleNestedAttribute{
				MarkdownDescription: "Controls the end-user notifications and deferral experience. Omit to leave the server's existing values untouched. Within this block, omitting a field clears it to null.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"notification_frequency": schema.Int64Attribute{
						MarkdownDescription: "Hours between repeat notifications while the update is pending.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
					"notification_message": schema.StringAttribute{
						MarkdownDescription: "Message shown when the update notification first appears.",
						Optional:            true,
					},
					"update_deadline": schema.Int64Attribute{
						MarkdownDescription: "Hours the user may defer before the install is forced.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
					"force_quit_message": schema.StringAttribute{
						MarkdownDescription: "Message shown when the user is prompted to quit the app.",
						Optional:            true,
					},
					"force_quit_grace_period": schema.Int64Attribute{
						MarkdownDescription: "Minutes the user is given to quit the app before the install proceeds.",
						Optional:            true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
					"update_complete_message": schema.StringAttribute{
						MarkdownDescription: "Message shown when the installation completes.",
						Optional:            true,
					},
					"relaunch": schema.BoolAttribute{
						MarkdownDescription: "Whether to relaunch the app automatically after the update completes.",
						Optional:            true,
					},
					"suppress": schema.BoolAttribute{
						MarkdownDescription: "Whether to suppress notifications for this update.",
						Optional:            true,
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

// Configure wires the Jamf Pro client into the resource.
func (r *AppInstallerSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_installer_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only "singleton" is accepted.
//
//	terraform import jamfplatform_pro_app_installer_settings.<name> singleton
func (r *AppInstallerSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_app_installer_settings")
}
