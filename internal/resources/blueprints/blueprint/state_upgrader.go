// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"

	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

var _ resource.ResourceWithUpgradeState = &BlueprintResource{}

// UpgradeState returns state upgraders for migrating between schema versions.
func (r *BlueprintResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: blueprintSchemaV0(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old blueprintResourceModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}

				upgraded := blueprintResourceModelV4{
					ID:                        old.ID,
					Name:                      old.Name,
					Description:               old.Description,
					Deployed:                  old.Deployed,
					DeviceGroups:              old.DeviceGroups,
					Components:                old.Components,
					AudioAccessorySettings:    sliceToPointer(old.AudioAccessorySettings),
					CustomDeclarations:        sliceToPointer(old.CustomDeclarations),
					DiskManagementSettings:    sliceToPointer(old.DiskManagementSettings),
					MathSettings:              sliceToPointer(old.MathSettings),
					PasscodePolicy:            sliceToPointer(old.PasscodePolicy),
					SafariBookmarks:           sliceToPointer(old.SafariBookmarks),
					SafariExtensions:          sliceToPointer(old.SafariExtensions),
					SafariSettings:            sliceToPointer(old.SafariSettings),
					ServiceBackgroundTasks:    sliceToPointer(old.ServiceBackgroundTasks),
					ServiceConfigurationFiles: sliceToPointer(old.ServiceConfigurationFiles),
					SoftwareUpdate:            sliceToPointer(old.SoftwareUpdate),
					SoftwareUpdateSettings:    upgradeSoftwareUpdateSettingsFromSlice(old.SoftwareUpdateSettings),
					LegacyPayloads:            upgradeLegacyPayloadsFromString(old.LegacyPayloads),
					Created:                   old.Created,
					Updated:                   old.Updated,
					DeploymentState:           old.DeploymentState,
					Timeouts:                  resourceTimeouts.Value{Object: old.Timeouts},
				}

				setUpgradedState(ctx, resp, upgraded)
			},
		},
		1: {
			PriorSchema: blueprintSchemaV1(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old blueprintResourceModelV1
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}

				upgraded := blueprintResourceModelV4{
					ID:                        old.ID,
					Name:                      old.Name,
					Description:               old.Description,
					Deployed:                  old.Deployed,
					DeviceGroups:              old.DeviceGroups,
					Components:                old.Components,
					AudioAccessorySettings:    old.AudioAccessorySettings,
					CustomDeclarations:        old.CustomDeclarations,
					DiskManagementSettings:    old.DiskManagementSettings,
					MathSettings:              old.MathSettings,
					PasscodePolicy:            old.PasscodePolicy,
					SafariBookmarks:           old.SafariBookmarks,
					SafariExtensions:          old.SafariExtensions,
					SafariSettings:            old.SafariSettings,
					ServiceBackgroundTasks:    old.ServiceBackgroundTasks,
					ServiceConfigurationFiles: old.ServiceConfigurationFiles,
					SoftwareUpdate:            old.SoftwareUpdate,
					SoftwareUpdateSettings:    upgradeSoftwareUpdateSettings(old.SoftwareUpdateSettings),
					LegacyPayloads:            upgradeLegacyPayloadsFromString(old.LegacyPayloads),
					Created:                   old.Created,
					Updated:                   old.Updated,
					DeploymentState:           old.DeploymentState,
					Timeouts:                  resourceTimeouts.Value{Object: old.Timeouts},
				}

				setUpgradedState(ctx, resp, upgraded)
			},
		},
		2: {
			PriorSchema: blueprintSchemaV2(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old blueprintResourceModelV2
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}

				upgraded := blueprintResourceModelV4{
					ID:                        old.ID,
					Name:                      old.Name,
					Description:               old.Description,
					Deployed:                  old.Deployed,
					DeviceGroups:              old.DeviceGroups,
					Components:                old.Components,
					AudioAccessorySettings:    old.AudioAccessorySettings,
					CustomDeclarations:        old.CustomDeclarations,
					DiskManagementSettings:    old.DiskManagementSettings,
					MathSettings:              old.MathSettings,
					PasscodePolicy:            old.PasscodePolicy,
					SafariBookmarks:           old.SafariBookmarks,
					SafariExtensions:          old.SafariExtensions,
					SafariSettings:            old.SafariSettings,
					ServiceBackgroundTasks:    old.ServiceBackgroundTasks,
					ServiceConfigurationFiles: old.ServiceConfigurationFiles,
					SoftwareUpdate:            old.SoftwareUpdate,
					SoftwareUpdateSettings:    upgradeSoftwareUpdateSettings(old.SoftwareUpdateSettings),
					LegacyPayloads:            old.LegacyPayloads,
					Created:                   old.Created,
					Updated:                   old.Updated,
					DeploymentState:           old.DeploymentState,
					Timeouts:                  old.Timeouts,
				}

				setUpgradedState(ctx, resp, upgraded)
			},
		},
		3: {
			PriorSchema: blueprintSchemaV3(ctx),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old blueprintResourceModelV3
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}

				setUpgradedState(ctx, resp, *old.upgrade())
			},
		},
		4: {
			PriorSchema: blueprintSchemaV4(ctx),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old blueprintResourceModelV4
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}

				setUpgradedState(ctx, resp, old)
			},
		},
		5: {
			PriorSchema: blueprintSchemaV5(ctx),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old BlueprintResourceModel
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, &old)...)
			},
		},
	}
}

// setUpgradedState rewrites a V4-shaped model for the current schema and stores it. Every upgrader
// ends here, because the framework runs exactly the one upgrader matching the stored version rather
// than chaining them, so each older version first reaches the V4 shape and then takes this step.
func setUpgradedState(ctx context.Context, resp *resource.UpgradeStateResponse, old blueprintResourceModelV4) {
	upgraded, diags := old.upgrade()
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, upgraded)...)
}

// sliceToPointer converts a slice from a ListNestedBlock to a pointer for SingleNestedAttribute.
func sliceToPointer[T any](s []T) *T {
	if len(s) > 0 {
		return &s[0]
	}
	return nil
}

// upgradeLegacyPayloadsFromString converts the v0/v1 JSON string format to the v2 dynamic format.
func upgradeLegacyPayloadsFromString(legacyPayloads types.String) types.Dynamic {
	if legacyPayloads.IsNull() || legacyPayloads.IsUnknown() || legacyPayloads.ValueString() == "" {
		return types.DynamicNull()
	}

	var payloadArray []map[string]any
	if err := json.Unmarshal([]byte(legacyPayloads.ValueString()), &payloadArray); err != nil {
		return types.DynamicNull()
	}

	if len(payloadArray) == 0 {
		return types.DynamicNull()
	}

	var resultItems []any
	for _, payload := range payloadArray {
		payloadType, _ := payload["payloadType"].(string)

		settingsMap := make(map[string]any, len(payload))
		for k, v := range payload {
			if k == "payloadType" || k == "payloadIdentifier" {
				continue
			}
			settingsMap[k] = v
		}

		entry := map[string]any{
			"payload_type": payloadType,
		}
		if len(settingsMap) > 0 {
			entry["settings"] = settingsMap
		}

		resultItems = append(resultItems, entry)
	}

	dynVal, err := helpers.JSONToTerraformDynamic(resultItems)
	if err != nil {
		return types.DynamicNull()
	}

	return dynVal
}

// softwareUpdateSettingsComponentPrior is the v0-v2 software update settings model with string deferral fields.
type softwareUpdateSettingsComponentPrior struct {
	AllowStandardUserOSUpdates           types.Bool                    `tfsdk:"allow_standard_user_os_updates"`
	AutomaticDownload                    types.String                  `tfsdk:"automatic_download"`
	AutomaticInstallOSUpdates            types.String                  `tfsdk:"automatic_install_os_updates"`
	AutomaticInstallSecurityUpdate       types.String                  `tfsdk:"automatic_install_security_updates"`
	BetaProgramEnrollment                types.String                  `tfsdk:"beta_program_enrollment"`
	BetaOfferPrograms                    []components.BetaProgramModel `tfsdk:"beta_offer_programs"`
	BetaRequireProgramToken              types.String                  `tfsdk:"beta_require_program_token"`
	BetaRequireProgramDescription        types.String                  `tfsdk:"beta_require_program_description"`
	DeferralCombinedPeriod               types.String                  `tfsdk:"deferral_combined_period_days"`
	DeferralMajorPeriod                  types.String                  `tfsdk:"deferral_major_period_days"`
	DeferralMinorPeriod                  types.String                  `tfsdk:"deferral_minor_period_days"`
	DeferralSystemPeriod                 types.String                  `tfsdk:"deferral_system_period_days"`
	NotificationsEnabled                 types.Bool                    `tfsdk:"notifications_enabled"`
	RapidSecurityResponseEnabled         types.Bool                    `tfsdk:"rapid_security_response_enabled"`
	RapidSecurityResponseRollbackEnabled types.Bool                    `tfsdk:"rapid_security_response_rollback_enabled"`
	RecommendedCadence                   types.String                  `tfsdk:"recommended_cadence"`
}

// upgradeSoftwareUpdateSettings converts v0-v2 string deferral fields to v3 int64 fields.
func upgradeSoftwareUpdateSettings(old *softwareUpdateSettingsComponentPrior) *components.SoftwareUpdateSettingsComponent {
	if old == nil {
		return nil
	}
	return &components.SoftwareUpdateSettingsComponent{
		AllowStandardUserOSUpdates:           old.AllowStandardUserOSUpdates,
		AutomaticDownload:                    old.AutomaticDownload,
		AutomaticInstallOSUpdates:            old.AutomaticInstallOSUpdates,
		AutomaticInstallSecurityUpdate:       old.AutomaticInstallSecurityUpdate,
		BetaProgramEnrollment:                old.BetaProgramEnrollment,
		BetaOfferPrograms:                    old.BetaOfferPrograms,
		BetaRequireProgramToken:              old.BetaRequireProgramToken,
		BetaRequireProgramDescription:        old.BetaRequireProgramDescription,
		DeferralCombinedPeriod:               stringToInt64(old.DeferralCombinedPeriod),
		DeferralMajorPeriod:                  stringToInt64(old.DeferralMajorPeriod),
		DeferralMinorPeriod:                  stringToInt64(old.DeferralMinorPeriod),
		DeferralSystemPeriod:                 stringToInt64(old.DeferralSystemPeriod),
		NotificationsEnabled:                 old.NotificationsEnabled,
		RapidSecurityResponseEnabled:         old.RapidSecurityResponseEnabled,
		RapidSecurityResponseRollbackEnabled: old.RapidSecurityResponseRollbackEnabled,
		RecommendedCadence:                   old.RecommendedCadence,
	}
}

// upgradeSoftwareUpdateSettingsFromSlice converts a v0 slice to the current pointer type with deferral migration.
func upgradeSoftwareUpdateSettingsFromSlice(old []softwareUpdateSettingsComponentPrior) *components.SoftwareUpdateSettingsComponent {
	if len(old) == 0 {
		return nil
	}
	return upgradeSoftwareUpdateSettings(&old[0])
}

// stringToInt64 converts a types.String to types.Int64, returning null if the string is empty or not parseable.
func stringToInt64(s types.String) types.Int64 {
	if s.IsNull() || s.IsUnknown() || s.ValueString() == "" {
		return types.Int64Null()
	}
	val, err := strconv.ParseInt(s.ValueString(), 10, 64)
	if err != nil {
		return types.Int64Null()
	}
	return types.Int64Value(val)
}

// blueprintResourceModelV0 is the v0 resource model where typed components were ListNestedBlocks.
type blueprintResourceModelV0 struct {
	ID                        types.String                                    `tfsdk:"id"`
	Name                      types.String                                    `tfsdk:"name"`
	Description               types.String                                    `tfsdk:"description"`
	Deployed                  types.Bool                                      `tfsdk:"deployed"`
	DeviceGroups              types.Set                                       `tfsdk:"device_groups"`
	Components                []ComponentModel                                `tfsdk:"raw_component"`
	AudioAccessorySettings    []components.AudioAccessorySettingsComponent    `tfsdk:"audio_accessory_settings"`
	CustomDeclarations        []components.CustomDeclarationsComponent        `tfsdk:"custom_declarations"`
	DiskManagementSettings    []components.DiskManagementPolicyComponent      `tfsdk:"disk_management_settings"`
	MathSettings              []components.MathSettingsComponent              `tfsdk:"math_settings"`
	PasscodePolicy            []components.PasscodePolicyComponent            `tfsdk:"passcode_policy"`
	SafariBookmarks           []components.SafariBookmarksComponent           `tfsdk:"safari_bookmarks"`
	SafariExtensions          []components.SafariExtensionsComponent          `tfsdk:"safari_extensions"`
	SafariSettings            []components.SafariSettingsComponent            `tfsdk:"safari_settings"`
	ServiceBackgroundTasks    []components.ServiceBackgroundTasksComponent    `tfsdk:"service_background_tasks"`
	ServiceConfigurationFiles []components.ServiceConfigurationFilesComponent `tfsdk:"service_configuration_files"`
	SoftwareUpdate            []components.SoftwareUpdateComponent            `tfsdk:"software_update"`
	SoftwareUpdateSettings    []softwareUpdateSettingsComponentPrior          `tfsdk:"software_update_settings"`
	LegacyPayloads            types.String                                    `tfsdk:"legacy_payloads"`
	Created                   types.String                                    `tfsdk:"created"`
	Updated                   types.String                                    `tfsdk:"updated"`
	DeploymentState           types.String                                    `tfsdk:"deployment_state"`
	Timeouts                  types.Object                                    `tfsdk:"timeouts"`
}

// blueprintResourceModelV1 is the v1 resource model where legacy_payloads was a JSON string.
type blueprintResourceModelV1 struct {
	ID                        types.String                                   `tfsdk:"id"`
	Name                      types.String                                   `tfsdk:"name"`
	Description               types.String                                   `tfsdk:"description"`
	Deployed                  types.Bool                                     `tfsdk:"deployed"`
	DeviceGroups              types.Set                                      `tfsdk:"device_groups"`
	Components                []ComponentModel                               `tfsdk:"raw_component"`
	AudioAccessorySettings    *components.AudioAccessorySettingsComponent    `tfsdk:"audio_accessory_settings"`
	CustomDeclarations        *components.CustomDeclarationsComponent        `tfsdk:"custom_declarations"`
	DiskManagementSettings    *components.DiskManagementPolicyComponent      `tfsdk:"disk_management_settings"`
	MathSettings              *components.MathSettingsComponent              `tfsdk:"math_settings"`
	PasscodePolicy            *components.PasscodePolicyComponent            `tfsdk:"passcode_policy"`
	SafariBookmarks           *components.SafariBookmarksComponent           `tfsdk:"safari_bookmarks"`
	SafariExtensions          *components.SafariExtensionsComponent          `tfsdk:"safari_extensions"`
	SafariSettings            *components.SafariSettingsComponent            `tfsdk:"safari_settings"`
	ServiceBackgroundTasks    *components.ServiceBackgroundTasksComponent    `tfsdk:"service_background_tasks"`
	ServiceConfigurationFiles *components.ServiceConfigurationFilesComponent `tfsdk:"service_configuration_files"`
	SoftwareUpdate            *components.SoftwareUpdateComponent            `tfsdk:"software_update"`
	SoftwareUpdateSettings    *softwareUpdateSettingsComponentPrior          `tfsdk:"software_update_settings"`
	LegacyPayloads            types.String                                   `tfsdk:"legacy_payloads"`
	Created                   types.String                                   `tfsdk:"created"`
	Updated                   types.String                                   `tfsdk:"updated"`
	DeploymentState           types.String                                   `tfsdk:"deployment_state"`
	Timeouts                  types.Object                                   `tfsdk:"timeouts"`
}

// blueprintResourceModelV2 is the v2 resource model where deferral fields were strings.
type blueprintResourceModelV2 struct {
	ID                        types.String                                   `tfsdk:"id"`
	Name                      types.String                                   `tfsdk:"name"`
	Description               types.String                                   `tfsdk:"description"`
	Deployed                  types.Bool                                     `tfsdk:"deployed"`
	DeviceGroups              types.Set                                      `tfsdk:"device_groups"`
	Components                []ComponentModel                               `tfsdk:"raw_component"`
	AudioAccessorySettings    *components.AudioAccessorySettingsComponent    `tfsdk:"audio_accessory_settings"`
	CustomDeclarations        *components.CustomDeclarationsComponent        `tfsdk:"custom_declarations"`
	DiskManagementSettings    *components.DiskManagementPolicyComponent      `tfsdk:"disk_management_settings"`
	MathSettings              *components.MathSettingsComponent              `tfsdk:"math_settings"`
	PasscodePolicy            *components.PasscodePolicyComponent            `tfsdk:"passcode_policy"`
	SafariBookmarks           *components.SafariBookmarksComponent           `tfsdk:"safari_bookmarks"`
	SafariExtensions          *components.SafariExtensionsComponent          `tfsdk:"safari_extensions"`
	SafariSettings            *components.SafariSettingsComponent            `tfsdk:"safari_settings"`
	ServiceBackgroundTasks    *components.ServiceBackgroundTasksComponent    `tfsdk:"service_background_tasks"`
	ServiceConfigurationFiles *components.ServiceConfigurationFilesComponent `tfsdk:"service_configuration_files"`
	SoftwareUpdate            *components.SoftwareUpdateComponent            `tfsdk:"software_update"`
	SoftwareUpdateSettings    *softwareUpdateSettingsComponentPrior          `tfsdk:"software_update_settings"`
	LegacyPayloads            types.Dynamic                                  `tfsdk:"legacy_payloads"`
	Created                   types.String                                   `tfsdk:"created"`
	Updated                   types.String                                   `tfsdk:"updated"`
	DeploymentState           types.String                                   `tfsdk:"deployment_state"`
	Timeouts                  resourceTimeouts.Value                         `tfsdk:"timeouts"`
}

// blueprintSchemaV0 returns the v0 schema where components were blocks instead of nested attributes.
func blueprintSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true},
			"name":             schema.StringAttribute{Required: true},
			"description":      schema.StringAttribute{Optional: true},
			"deployed":         schema.BoolAttribute{Required: true},
			"device_groups":    schema.SetAttribute{Required: true, ElementType: types.StringType},
			"legacy_payloads":  schema.StringAttribute{Optional: true},
			"created":          schema.StringAttribute{Computed: true},
			"updated":          schema.StringAttribute{Computed: true},
			"deployment_state": schema.StringAttribute{Computed: true},
			"timeouts":         schema.ObjectAttribute{Optional: true, AttributeTypes: blueprintTimeoutAttributeTypes},
		},
		Blocks: map[string]schema.Block{
			"raw_component": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"identifier":    schema.StringAttribute{Required: true},
						"configuration": schema.MapAttribute{Optional: true, ElementType: types.StringType},
					},
				},
			},
			"audio_accessory_settings": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"temporary_pairing_disabled": schema.BoolAttribute{Required: true},
						"unpairing_time_policy":      schema.StringAttribute{Optional: true, Computed: true},
						"unpairing_time_hour":        schema.Int64Attribute{Optional: true, Computed: true},
					},
				},
			},
			"custom_declarations": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"declaration": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"channel": schema.StringAttribute{Required: true},
									"kind":    schema.StringAttribute{Required: true},
									"payload": schema.StringAttribute{Required: true},
									"type":    schema.StringAttribute{Required: true},
								},
							},
						},
					},
				},
			},
			"disk_management_settings": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"external_storage": schema.StringAttribute{Optional: true},
						"network_storage":  schema.StringAttribute{Optional: true},
					},
				},
			},
			"math_settings": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"calculator_basic_mode_add_square_root":  schema.BoolAttribute{Optional: true},
						"calculator_scientific_mode_enabled":     schema.BoolAttribute{Optional: true},
						"calculator_programmer_mode_enabled":     schema.BoolAttribute{Optional: true},
						"calculator_math_notes_mode_enabled":     schema.BoolAttribute{Optional: true},
						"calculator_input_modes_unit_conversion": schema.BoolAttribute{Optional: true},
						"calculator_input_modes_rpn":             schema.BoolAttribute{Optional: true},
						"system_behavior_keyboard_suggestions":   schema.BoolAttribute{Optional: true},
						"system_behavior_math_notes":             schema.BoolAttribute{Optional: true},
					},
				},
			},
			"passcode_policy": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"change_at_next_auth":              schema.BoolAttribute{Optional: true},
						"failed_attempts_reset_in_minutes": schema.Int64Attribute{Optional: true},
						"maximum_failed_attempts":          schema.Int64Attribute{Optional: true},
						"maximum_grace_period_in_minutes":  schema.Int64Attribute{Optional: true},
						"maximum_inactivity_in_minutes":    schema.Int64Attribute{Optional: true},
						"maximum_passcode_age_in_days":     schema.Int64Attribute{Optional: true},
						"minimum_complex_characters":       schema.Int64Attribute{Optional: true},
						"minimum_length":                   schema.Int64Attribute{Optional: true},
						"passcode_reuse_limit":             schema.Int64Attribute{Optional: true},
						"require_alphanumeric_passcode":    schema.BoolAttribute{Optional: true},
						"require_complex_passcode":         schema.BoolAttribute{Optional: true},
						"require_passcode":                 schema.BoolAttribute{Optional: true},
					},
				},
			},
			"safari_bookmarks": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"managed_bookmarks": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"group_identifier": schema.StringAttribute{Required: true},
									"title":            schema.StringAttribute{Required: true},
								},
								Blocks: map[string]schema.Block{
									"bookmarks": schema.ListNestedBlock{
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"type":  schema.StringAttribute{Optional: true},
												"title": schema.StringAttribute{Required: true},
												"url":   schema.StringAttribute{Optional: true},
											},
											Blocks: map[string]schema.Block{
												"folder": schema.ListNestedBlock{
													NestedObject: schema.NestedBlockObject{
														Attributes: map[string]schema.Attribute{
															"title": schema.StringAttribute{Required: true},
															"url":   schema.StringAttribute{Required: true},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"safari_extensions": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"managed_extensions": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"extension_id":     schema.StringAttribute{Required: true},
									"state":            schema.StringAttribute{Optional: true},
									"private_browsing": schema.StringAttribute{Optional: true},
								},
								Blocks: map[string]schema.Block{
									"allowed_domains": schema.ListNestedBlock{
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"domain": schema.StringAttribute{Required: true},
											},
										},
									},
									"denied_domains": schema.ListNestedBlock{
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"domain": schema.StringAttribute{Required: true},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"safari_settings": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"accept_cookies":                  schema.StringAttribute{Optional: true},
						"allow_disabling_fraud_warning":   schema.BoolAttribute{Optional: true},
						"allow_history_clearing":          schema.BoolAttribute{Optional: true},
						"allow_javascript":                schema.BoolAttribute{Optional: true},
						"allow_private_browsing":          schema.BoolAttribute{Optional: true},
						"allow_popups":                    schema.BoolAttribute{Optional: true},
						"allow_summary":                   schema.BoolAttribute{Optional: true},
						"new_tab_start_page_type":         schema.StringAttribute{Optional: true},
						"new_tab_start_page_homepage_url": schema.StringAttribute{Optional: true},
						"new_tab_start_page_extension_id": schema.StringAttribute{Optional: true},
					},
				},
			},
			"service_background_tasks": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"background_tasks": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"task_type":        schema.StringAttribute{Required: true},
									"task_description": schema.StringAttribute{Optional: true},
								},
								Blocks: map[string]schema.Block{
									"executable_asset_reference": schema.SingleNestedBlock{
										Attributes: map[string]schema.Attribute{
											"data_url":     schema.StringAttribute{Required: true},
											"hash_sha_256": schema.StringAttribute{Optional: true},
											"content_type": schema.StringAttribute{Computed: true},
										},
									},
									"launchd_configurations": schema.ListNestedBlock{
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"context": schema.StringAttribute{Required: true},
											},
											Blocks: map[string]schema.Block{
												"file_asset_reference": schema.SingleNestedBlock{
													Attributes: map[string]schema.Attribute{
														"data_url":     schema.StringAttribute{Required: true},
														"hash_sha_256": schema.StringAttribute{Optional: true},
														"content_type": schema.StringAttribute{Optional: true},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"service_configuration_files": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"service_config_files": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"service_type": schema.StringAttribute{Required: true},
								},
								Blocks: map[string]schema.Block{
									"data_asset_reference": schema.SingleNestedBlock{
										Attributes: map[string]schema.Attribute{
											"data_url":     schema.StringAttribute{Required: true},
											"hash_sha_256": schema.StringAttribute{Optional: true},
											"content_type": schema.StringAttribute{Computed: true},
										},
									},
								},
							},
						},
					},
				},
			},
			"software_update": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"enforcement_type":       schema.StringAttribute{Computed: true},
						"deployment_time":        schema.StringAttribute{Optional: true},
						"enforce_after_days":     schema.Int64Attribute{Optional: true},
						"ignore_major_versions":  schema.BoolAttribute{Optional: true},
						"target_os_version":      schema.StringAttribute{Optional: true},
						"target_local_date_time": schema.StringAttribute{Optional: true},
						"details_url_value":      schema.StringAttribute{Optional: true},
					},
				},
			},
			"software_update_settings": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"allow_standard_user_os_updates":           schema.BoolAttribute{Optional: true},
						"automatic_download":                       schema.StringAttribute{Optional: true},
						"automatic_install_os_updates":             schema.StringAttribute{Optional: true},
						"automatic_install_security_updates":       schema.StringAttribute{Optional: true},
						"beta_program_enrollment":                  schema.StringAttribute{Optional: true},
						"beta_require_program_token":               schema.StringAttribute{Optional: true},
						"beta_require_program_description":         schema.StringAttribute{Optional: true},
						"deferral_combined_period_days":            schema.StringAttribute{Optional: true},
						"deferral_major_period_days":               schema.StringAttribute{Optional: true},
						"deferral_minor_period_days":               schema.StringAttribute{Optional: true},
						"deferral_system_period_days":              schema.StringAttribute{Optional: true},
						"notifications_enabled":                    schema.BoolAttribute{Optional: true},
						"rapid_security_response_enabled":          schema.BoolAttribute{Optional: true},
						"rapid_security_response_rollback_enabled": schema.BoolAttribute{Optional: true},
						"recommended_cadence":                      schema.StringAttribute{Optional: true},
					},
					Blocks: map[string]schema.Block{
						"beta_offer_programs": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"token":       schema.StringAttribute{Required: true},
									"description": schema.StringAttribute{Required: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

// blueprintSchemaV1Transcribed returns the v1 schema where legacy_payloads was a JSON string attribute,
// with the component collections in their live shape; blueprintSchemaV1 restores their set shape.
func blueprintSchemaV1Transcribed() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true},
			"name":             schema.StringAttribute{Required: true},
			"description":      schema.StringAttribute{Optional: true},
			"deployed":         schema.BoolAttribute{Required: true},
			"device_groups":    schema.SetAttribute{Required: true, ElementType: types.StringType},
			"legacy_payloads":  schema.StringAttribute{Optional: true},
			"created":          schema.StringAttribute{Computed: true},
			"updated":          schema.StringAttribute{Computed: true},
			"deployment_state": schema.StringAttribute{Computed: true},
			"raw_component": schema.SetNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"identifier":    schema.StringAttribute{Required: true},
						"configuration": schema.MapAttribute{Optional: true, ElementType: types.StringType},
					},
				},
			},
			"audio_accessory_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.AudioAccessorySettingsComponentSchema(),
			},
			"custom_declarations": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.CustomDeclarationsComponentSchema(),
			},
			"disk_management_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.DiskManagementPolicyComponentSchema(),
			},
			"math_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.MathSettingsComponentSchema(),
			},
			"passcode_policy": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: passcodePolicySchemaPrior(),
			},
			"safari_bookmarks": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SafariBookmarksComponentSchema(),
			},
			"safari_extensions": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SafariExtensionsComponentSchema(),
			},
			"safari_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SafariSettingsComponentSchema(),
			},
			"service_background_tasks": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.ServiceBackgroundTasksComponentSchema(),
			},
			"service_configuration_files": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.ServiceConfigurationFilesComponentSchema(),
			},
			"software_update": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SoftwareUpdateComponentSchema(),
			},
			"software_update_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: softwareUpdateSettingsSchemaV2(),
			},
			"timeouts": schema.ObjectAttribute{Optional: true, AttributeTypes: blueprintTimeoutAttributeTypes},
		},
	}
}

// blueprintSchemaV2Transcribed returns the v2 schema where deferral fields were strings, with the
// component collections in their live shape; blueprintSchemaV2 restores their set shape.
func blueprintSchemaV2Transcribed() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":               schema.StringAttribute{Computed: true},
			"name":             schema.StringAttribute{Required: true},
			"description":      schema.StringAttribute{Optional: true},
			"deployed":         schema.BoolAttribute{Required: true},
			"device_groups":    schema.SetAttribute{Required: true, ElementType: types.StringType},
			"legacy_payloads":  schema.DynamicAttribute{Optional: true},
			"created":          schema.StringAttribute{Computed: true},
			"updated":          schema.StringAttribute{Computed: true},
			"deployment_state": schema.StringAttribute{Computed: true},
			"raw_component": schema.SetNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"identifier":    schema.StringAttribute{Required: true},
						"configuration": schema.MapAttribute{Optional: true, ElementType: types.StringType},
					},
				},
			},
			"audio_accessory_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.AudioAccessorySettingsComponentSchema(),
			},
			"custom_declarations": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.CustomDeclarationsComponentSchema(),
			},
			"disk_management_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.DiskManagementPolicyComponentSchema(),
			},
			"math_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.MathSettingsComponentSchema(),
			},
			"passcode_policy": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: passcodePolicySchemaPrior(),
			},
			"safari_bookmarks": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SafariBookmarksComponentSchema(),
			},
			"safari_extensions": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SafariExtensionsComponentSchema(),
			},
			"safari_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SafariSettingsComponentSchema(),
			},
			"service_background_tasks": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.ServiceBackgroundTasksComponentSchema(),
			},
			"service_configuration_files": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.ServiceConfigurationFilesComponentSchema(),
			},
			"software_update": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: components.SoftwareUpdateComponentSchema(),
			},
			"software_update_settings": schema.SingleNestedAttribute{
				Optional:   true,
				Attributes: softwareUpdateSettingsSchemaV2(),
			},
			"timeouts": schema.ObjectAttribute{Optional: true, AttributeTypes: blueprintTimeoutAttributeTypes},
		},
	}
}

// passcodePolicySchemaPrior returns the prior passcode policy schema without custom regex fields.
func passcodePolicySchemaPrior() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"change_at_next_auth":              schema.BoolAttribute{Optional: true},
		"failed_attempts_reset_in_minutes": schema.Int64Attribute{Optional: true},
		"maximum_failed_attempts":          schema.Int64Attribute{Optional: true},
		"maximum_grace_period_in_minutes":  schema.Int64Attribute{Optional: true},
		"maximum_inactivity_in_minutes":    schema.Int64Attribute{Optional: true},
		"maximum_passcode_age_in_days":     schema.Int64Attribute{Optional: true},
		"minimum_complex_characters":       schema.Int64Attribute{Optional: true},
		"minimum_length":                   schema.Int64Attribute{Optional: true},
		"passcode_reuse_limit":             schema.Int64Attribute{Optional: true},
		"require_alphanumeric_passcode":    schema.BoolAttribute{Optional: true},
		"require_complex_passcode":         schema.BoolAttribute{Optional: true},
		"require_passcode":                 schema.BoolAttribute{Optional: true},
	}
}

// softwareUpdateSettingsSchemaV2 returns the prior schema for software update settings with string deferral attributes.
func softwareUpdateSettingsSchemaV2() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"allow_standard_user_os_updates":           schema.BoolAttribute{Optional: true},
		"automatic_download":                       schema.StringAttribute{Optional: true},
		"automatic_install_os_updates":             schema.StringAttribute{Optional: true},
		"automatic_install_security_updates":       schema.StringAttribute{Optional: true},
		"beta_program_enrollment":                  schema.StringAttribute{Optional: true},
		"beta_require_program_token":               schema.StringAttribute{Optional: true},
		"beta_require_program_description":         schema.StringAttribute{Optional: true},
		"deferral_combined_period_days":            schema.StringAttribute{Optional: true},
		"deferral_major_period_days":               schema.StringAttribute{Optional: true},
		"deferral_minor_period_days":               schema.StringAttribute{Optional: true},
		"deferral_system_period_days":              schema.StringAttribute{Optional: true},
		"notifications_enabled":                    schema.BoolAttribute{Optional: true},
		"rapid_security_response_enabled":          schema.BoolAttribute{Optional: true},
		"rapid_security_response_rollback_enabled": schema.BoolAttribute{Optional: true},
		"recommended_cadence":                      schema.StringAttribute{Optional: true},
		"beta_offer_programs": schema.SetNestedAttribute{
			Optional: true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"token":       schema.StringAttribute{Required: true},
					"description": schema.StringAttribute{Required: true},
				},
			},
		},
	}
}

// blueprintResourceModelV3 is the v3 state model. It differs from BlueprintResourceModel in one
// place: each component block's apple_declarations was an object wrapping a `declaration` list
// rather than the list of declarations itself. Every other field is identical, and the duplication
// is what tfsdk reflection costs — a struct cannot override one field of another.
type blueprintResourceModelV3 struct {
	ID                        types.String                                   `tfsdk:"id"`
	Name                      types.String                                   `tfsdk:"name"`
	Description               types.String                                   `tfsdk:"description"`
	Deployed                  types.Bool                                     `tfsdk:"deployed"`
	DeviceGroups              types.Set                                      `tfsdk:"device_groups"`
	ActivationConditions      types.String                                   `tfsdk:"activation_conditions"`
	ComponentBlocks           []componentBlockModelV3                        `tfsdk:"component_blocks"`
	Components                []ComponentModel                               `tfsdk:"raw_component"`
	AudioAccessorySettings    *components.AudioAccessorySettingsComponent    `tfsdk:"audio_accessory_settings"`
	CustomDeclarations        *components.CustomDeclarationsComponent        `tfsdk:"custom_declarations"`
	DiskManagementSettings    *components.DiskManagementPolicyComponent      `tfsdk:"disk_management_settings"`
	MathSettings              *components.MathSettingsComponent              `tfsdk:"math_settings"`
	PasscodePolicy            *components.PasscodePolicyComponent            `tfsdk:"passcode_policy"`
	SafariBookmarks           *components.SafariBookmarksComponent           `tfsdk:"safari_bookmarks"`
	SafariExtensions          *components.SafariExtensionsComponent          `tfsdk:"safari_extensions"`
	SafariSettings            *components.SafariSettingsComponent            `tfsdk:"safari_settings"`
	ServiceBackgroundTasks    *components.ServiceBackgroundTasksComponent    `tfsdk:"service_background_tasks"`
	ServiceConfigurationFiles *components.ServiceConfigurationFilesComponent `tfsdk:"service_configuration_files"`
	SoftwareUpdate            *components.SoftwareUpdateComponent            `tfsdk:"software_update"`
	SoftwareUpdateSettings    *components.SoftwareUpdateSettingsComponent    `tfsdk:"software_update_settings"`
	LegacyPayloads            types.Dynamic                                  `tfsdk:"legacy_payloads"`
	Created                   types.String                                   `tfsdk:"created"`
	Updated                   types.String                                   `tfsdk:"updated"`
	DeploymentState           types.String                                   `tfsdk:"deployment_state"`
	Timeouts                  resourceTimeouts.Value                         `tfsdk:"timeouts"`
}

// componentBlockModelV3 is one v3 component block.
type componentBlockModelV3 struct {
	Name                      types.String                                   `tfsdk:"name"`
	ActivationConditions      types.String                                   `tfsdk:"activation_conditions"`
	Components                []ComponentModel                               `tfsdk:"raw_component"`
	AIGovernance              *components.AIGovernanceComponent              `tfsdk:"ai_governance"`
	AppleDeclarations         *appleDeclarationsComponentV3                  `tfsdk:"apple_declarations"`
	AudioAccessorySettings    *components.AudioAccessorySettingsComponent    `tfsdk:"audio_accessory_settings"`
	CustomDeclarations        *components.CustomDeclarationsComponent        `tfsdk:"custom_declarations"`
	DiskManagementSettings    *components.DiskManagementPolicyComponent      `tfsdk:"disk_management_settings"`
	MathSettings              *components.MathSettingsComponent              `tfsdk:"math_settings"`
	PasscodePolicy            *components.PasscodePolicyComponent            `tfsdk:"passcode_policy"`
	SafariBookmarks           *components.SafariBookmarksComponent           `tfsdk:"safari_bookmarks"`
	SafariExtensions          *components.SafariExtensionsComponent          `tfsdk:"safari_extensions"`
	SafariSettings            *components.SafariSettingsComponent            `tfsdk:"safari_settings"`
	ServiceBackgroundTasks    *components.ServiceBackgroundTasksComponent    `tfsdk:"service_background_tasks"`
	ServiceConfigurationFiles *components.ServiceConfigurationFilesComponent `tfsdk:"service_configuration_files"`
	SoftwareUpdate            *components.SoftwareUpdateComponent            `tfsdk:"software_update"`
	SoftwareUpdateSettings    *components.SoftwareUpdateSettingsComponent    `tfsdk:"software_update_settings"`
	LegacyPayloads            []BlockLegacyPayloadModel                      `tfsdk:"legacy_payloads"`
}

// appleDeclarationsComponentV3 is the object apple_declarations used to be.
type appleDeclarationsComponentV3 struct {
	Declarations []AppleDeclarationModel `tfsdk:"declaration"`
}

// upgrade rewrites v3 state as v4 by unwrapping each block's declaration list.
// An absent component stays absent, and a component holding no declarations becomes an absent list
// rather than an empty one: an empty list writes no component, so state must not carry one.
func (m blueprintResourceModelV3) upgrade() *blueprintResourceModelV4 {
	upgraded := &blueprintResourceModelV4{
		ID:                        m.ID,
		Name:                      m.Name,
		Description:               m.Description,
		Deployed:                  m.Deployed,
		DeviceGroups:              m.DeviceGroups,
		ActivationConditions:      m.ActivationConditions,
		Components:                m.Components,
		AudioAccessorySettings:    m.AudioAccessorySettings,
		CustomDeclarations:        m.CustomDeclarations,
		DiskManagementSettings:    m.DiskManagementSettings,
		MathSettings:              m.MathSettings,
		PasscodePolicy:            m.PasscodePolicy,
		SafariBookmarks:           m.SafariBookmarks,
		SafariExtensions:          m.SafariExtensions,
		SafariSettings:            m.SafariSettings,
		ServiceBackgroundTasks:    m.ServiceBackgroundTasks,
		ServiceConfigurationFiles: m.ServiceConfigurationFiles,
		SoftwareUpdate:            m.SoftwareUpdate,
		SoftwareUpdateSettings:    m.SoftwareUpdateSettings,
		LegacyPayloads:            m.LegacyPayloads,
		Created:                   m.Created,
		Updated:                   m.Updated,
		DeploymentState:           m.DeploymentState,
		Timeouts:                  m.Timeouts,
	}

	for _, block := range m.ComponentBlocks {
		upgradedBlock := ComponentBlockModel{
			Name:                      block.Name,
			ActivationConditions:      block.ActivationConditions,
			Components:                block.Components,
			AIGovernance:              block.AIGovernance,
			AudioAccessorySettings:    block.AudioAccessorySettings,
			CustomDeclarations:        block.CustomDeclarations,
			DiskManagementSettings:    block.DiskManagementSettings,
			MathSettings:              block.MathSettings,
			PasscodePolicy:            block.PasscodePolicy,
			SafariBookmarks:           block.SafariBookmarks,
			SafariExtensions:          block.SafariExtensions,
			SafariSettings:            block.SafariSettings,
			ServiceBackgroundTasks:    block.ServiceBackgroundTasks,
			ServiceConfigurationFiles: block.ServiceConfigurationFiles,
			SoftwareUpdate:            block.SoftwareUpdate,
			SoftwareUpdateSettings:    block.SoftwareUpdateSettings,
			LegacyPayloads:            block.LegacyPayloads,
		}
		if block.AppleDeclarations != nil && len(block.AppleDeclarations.Declarations) > 0 {
			upgradedBlock.AppleDeclarations = block.AppleDeclarations.Declarations
		}
		upgraded.ComponentBlocks = append(upgraded.ComponentBlocks, upgradedBlock)
	}

	return upgraded
}

// blueprintSchemaV3 returns the v3 schema: the v4 one with apple_declarations in the shape it had
// then. It is derived from the live attribute set rather than transcribed, because one attribute
// changed and a hand-copied component block would duplicate every other. The prior
// schemas above already share the live components.*ComponentSchema() functions for the same reason.
func blueprintSchemaV3(ctx context.Context) *schema.Schema {
	blockAttributes := componentBlockAttributesV5()
	blockAttributes["apple_declarations"] = schema.SingleNestedAttribute{
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"declaration": schema.ListNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"channel": schema.StringAttribute{Required: true},
						"payload": schema.StringAttribute{Required: true},
						"type":    schema.StringAttribute{Required: true},
					},
				},
			},
		},
	}

	attributes := blueprintSchemaV4(ctx).Attributes
	attributes["component_blocks"] = schema.ListNestedAttribute{
		Optional:     true,
		NestedObject: schema.NestedAttributeObject{Attributes: blockAttributes},
	}

	return &schema.Schema{Version: 3, Attributes: attributes}
}

// flatStepNameV4 is the step name v4 wrote for every blueprint authored with the top-level
// component attributes. The upgrader gives the block that replaces them the same name, so the first
// refresh finds state and the platform in agreement and the move to component_blocks changes
// nothing on the service.
const flatStepNameV4 = "Declaration group"

// blueprintResourceModelV4 is the v4 state model, the last to carry the top-level component
// attributes and the top-level activation_conditions. Those attributes held the first component
// block only, and the v5 schema has no field for them.
type blueprintResourceModelV4 struct {
	ID                        types.String                                   `tfsdk:"id"`
	Name                      types.String                                   `tfsdk:"name"`
	Description               types.String                                   `tfsdk:"description"`
	Deployed                  types.Bool                                     `tfsdk:"deployed"`
	DeviceGroups              types.Set                                      `tfsdk:"device_groups"`
	ActivationConditions      types.String                                   `tfsdk:"activation_conditions"`
	ComponentBlocks           []ComponentBlockModel                          `tfsdk:"component_blocks"`
	Components                []ComponentModel                               `tfsdk:"raw_component"`
	AudioAccessorySettings    *components.AudioAccessorySettingsComponent    `tfsdk:"audio_accessory_settings"`
	CustomDeclarations        *components.CustomDeclarationsComponent        `tfsdk:"custom_declarations"`
	DiskManagementSettings    *components.DiskManagementPolicyComponent      `tfsdk:"disk_management_settings"`
	MathSettings              *components.MathSettingsComponent              `tfsdk:"math_settings"`
	PasscodePolicy            *components.PasscodePolicyComponent            `tfsdk:"passcode_policy"`
	SafariBookmarks           *components.SafariBookmarksComponent           `tfsdk:"safari_bookmarks"`
	SafariExtensions          *components.SafariExtensionsComponent          `tfsdk:"safari_extensions"`
	SafariSettings            *components.SafariSettingsComponent            `tfsdk:"safari_settings"`
	ServiceBackgroundTasks    *components.ServiceBackgroundTasksComponent    `tfsdk:"service_background_tasks"`
	ServiceConfigurationFiles *components.ServiceConfigurationFilesComponent `tfsdk:"service_configuration_files"`
	SoftwareUpdate            *components.SoftwareUpdateComponent            `tfsdk:"software_update"`
	SoftwareUpdateSettings    *components.SoftwareUpdateSettingsComponent    `tfsdk:"software_update_settings"`
	LegacyPayloads            types.Dynamic                                  `tfsdk:"legacy_payloads"`
	Created                   types.String                                   `tfsdk:"created"`
	Updated                   types.String                                   `tfsdk:"updated"`
	DeploymentState           types.String                                   `tfsdk:"deployment_state"`
	Timeouts                  resourceTimeouts.Value                         `tfsdk:"timeouts"`
}

// upgrade rewrites v4 state for the current schema. State that already holds component_blocks keeps
// them and drops the top-level attributes, which v4 cleared in that mode anyway. State that holds
// the top-level attributes moves them into one component block, so nothing the practitioner
// managed is dropped: a silent drop would leave the platform holding components that state no
// longer lists, and the next apply would delete them without a plan diff.
//
// The block is named flatStepNameV4 because that is the name the platform stores, and its
// activation condition is the old top-level one. Settings of a legacy payload become the JSON
// strings a block carries.
func (m blueprintResourceModelV4) upgrade() (*BlueprintResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	upgraded := &BlueprintResourceModel{
		ID:              m.ID,
		Name:            m.Name,
		Description:     m.Description,
		Deployed:        m.Deployed,
		DeviceGroups:    m.DeviceGroups,
		ComponentBlocks: m.ComponentBlocks,
		Created:         m.Created,
		Updated:         m.Updated,
		DeploymentState: m.DeploymentState,
		Timeouts:        m.Timeouts,
	}

	if len(m.ComponentBlocks) > 0 {
		return upgraded, diags
	}

	legacyPayloads, err := blockLegacyPayloadsFromDynamic(m.LegacyPayloads)
	if err != nil {
		diags.AddError(
			"Could not upgrade legacy_payloads",
			"The top-level legacy_payloads attribute was removed. Moving it into a component block failed: "+helpers.APIErrorDetail(err)+
				". Write the payloads under component_blocks[].legacy_payloads, with each settings object passed through jsonencode().",
		)
		return nil, diags
	}

	block := ComponentBlockModel{
		Name:                      types.StringValue(flatStepNameV4),
		ActivationConditions:      m.ActivationConditions,
		Components:                m.Components,
		AudioAccessorySettings:    m.AudioAccessorySettings,
		CustomDeclarations:        m.CustomDeclarations,
		DiskManagementSettings:    m.DiskManagementSettings,
		MathSettings:              m.MathSettings,
		PasscodePolicy:            m.PasscodePolicy,
		SafariBookmarks:           m.SafariBookmarks,
		SafariExtensions:          m.SafariExtensions,
		SafariSettings:            m.SafariSettings,
		ServiceBackgroundTasks:    m.ServiceBackgroundTasks,
		ServiceConfigurationFiles: m.ServiceConfigurationFiles,
		SoftwareUpdate:            m.SoftwareUpdate,
		SoftwareUpdateSettings:    m.SoftwareUpdateSettings,
		LegacyPayloads:            legacyPayloads,
	}

	if block.holdsNothing() {
		return upgraded, diags
	}

	upgraded.ComponentBlocks = []ComponentBlockModel{block}
	return upgraded, diags
}

// holdsNothing reports whether the block carries no component, no legacy payload and no activation
// condition, which is what a v4 blueprint with no top-level component attributes upgrades to.
func (b ComponentBlockModel) holdsNothing() bool {
	return len(b.Components) == 0 &&
		b.AudioAccessorySettings == nil &&
		b.CustomDeclarations == nil &&
		b.DiskManagementSettings == nil &&
		b.MathSettings == nil &&
		b.PasscodePolicy == nil &&
		b.SafariBookmarks == nil &&
		b.SafariExtensions == nil &&
		b.SafariSettings == nil &&
		b.ServiceBackgroundTasks == nil &&
		b.ServiceConfigurationFiles == nil &&
		b.SoftwareUpdate == nil &&
		b.SoftwareUpdateSettings == nil &&
		len(b.LegacyPayloads) == 0 &&
		(b.ActivationConditions.IsNull() || b.ActivationConditions.IsUnknown())
}

// blockLegacyPayloadsFromDynamic converts the removed top-level dynamic legacy_payloads value into
// a block's list. A block carries each payload's settings as a JSON object string, which
// json.Marshal renders with sorted keys, the same canonical form jsonencode() produces.
func blockLegacyPayloadsFromDynamic(prior types.Dynamic) ([]BlockLegacyPayloadModel, error) {
	if prior.IsNull() || prior.IsUnknown() || prior.IsUnderlyingValueNull() || prior.IsUnderlyingValueUnknown() {
		return nil, nil
	}

	raw, err := helpers.TerraformDynamicToJSON(prior)
	if err != nil {
		return nil, err
	}

	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list of payload objects, got %T", raw)
	}

	payloads := make([]BlockLegacyPayloadModel, 0, len(items))
	for i, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("payload %d is not an object", i)
		}

		payload := BlockLegacyPayloadModel{Settings: types.StringNull()}
		payloadType, _ := object["payload_type"].(string)
		payload.PayloadType = types.StringValue(payloadType)

		if settings, present := object["settings"]; present && settings != nil {
			encoded, err := json.Marshal(settings)
			if err != nil {
				return nil, fmt.Errorf("settings of payload %d: %w", i, err)
			}
			payload.Settings = types.StringValue(string(encoded))
		}

		payloads = append(payloads, payload)
	}

	return payloads, nil
}

// blueprintSchemaV4 returns the v4 schema: the v5 one plus the top-level component attributes
// and activation_conditions that v5 removed. It is derived from the live attribute set for the
// reason blueprintSchemaV3 gives, and the removed attributes reuse the live component schemas, so
// a future change to a component cannot leave the prior schema decoding state it never held.
func blueprintSchemaV4(ctx context.Context) *schema.Schema {
	attributes := blueprintSchemaV5(ctx).Attributes
	shared := sharedComponentAttributes()
	restoreV5Sets(shared)
	maps.Copy(attributes, shared)
	attributes["activation_conditions"] = schema.StringAttribute{Optional: true}
	attributes["legacy_payloads"] = schema.DynamicAttribute{Optional: true}

	return &schema.Schema{Version: 4, Attributes: attributes}
}

// blueprintSchemaV5 returns the v5 schema: the current one with the collections that v6 made
// ordered lists still in the set shape they had then.
//
// v5 → v6: custom_declarations.declaration, safari_bookmarks.managed_bookmarks and
// its nested bookmarks and folder, safari_extensions' allowed_domains and denied_domains,
// service_background_tasks.background_tasks and its nested launchd_configurations,
// service_configuration_files.service_config_files and software_update_settings.beta_offer_programs
// became lists, because the blueprints service stores each as written, repeats and order included,
// and a set can hold neither (issue #568; the probes are recorded on each component). The element
// types are unchanged, so the upgrade carries every element across in the order state holds it.
// That is the set's own order, not the order configured, so the first plan after upgrading may
// propose one in-place update that writes the configured order. That update is a real change on the
// service, not a no-op: the v5 provider wrote each collection in the set's order, so the service, and
// Safari's bookmark order with it, held that order until now. Like any write it leaves the blueprint
// OUT_OF_DATE, so a blueprint with deployed = true is redeployed by the same apply:
// reconcileBlueprintDeployment sees OUT_OF_DATE and calls deploy. Wire-verified 2026-11-11 on a
// deployed blueprint scoped to 38 computers: the update sent PATCH (204), GET (OUT_OF_DATE), POST
// /deploy (202), and a new deployment job started and reached SUCCEEDED, with every device in scope
// pending again; payloadKey moved from the set's order to the configured one. Wire-verified
// with a v5 build and this one on the EU gateway 2026-11-11: one update, then an empty plan with and
// without refresh, and the v5 build refuses the upgraded state. The bump exists so that a provider
// still on v5 refuses v6 state, which can hold a repeat its sets cannot decode, rather than
// failing to read it.
func blueprintSchemaV5(ctx context.Context) *schema.Schema {
	attributes := blueprintSchemaAttributes(ctx)
	attributes["component_blocks"] = schema.ListNestedAttribute{
		Optional:     true,
		NestedObject: schema.NestedAttributeObject{Attributes: componentBlockAttributesV5()},
	}

	return &schema.Schema{Version: 5, Attributes: attributes}
}

// componentBlockAttributesV5 returns one component block's attributes as v5 declared them.
func componentBlockAttributesV5() map[string]schema.Attribute {
	attributes := componentBlockAttributes()
	restoreV5Sets(attributes)
	return attributes
}

// restoreV5Sets rewrites, in place, every collection that v6 turned from a set into a list back
// into its v5 set shape. attributes holds the shared component attributes, either a component
// block's or the top-level ones v4 carried. The live schema functions build fresh maps on every
// call, so rewriting them cannot reach the current schema.
func restoreV5Sets(attributes map[string]schema.Attribute) {
	withNested(attributes, "custom_declarations", func(component map[string]schema.Attribute) {
		component["declaration"] = listAsSet(component["declaration"])
	})
	withNested(attributes, "safari_bookmarks", func(component map[string]schema.Attribute) {
		groups := component["managed_bookmarks"].(schema.ListNestedAttribute)
		bookmarks := groups.NestedObject.Attributes["bookmarks"].(schema.ListNestedAttribute)
		bookmarks.NestedObject.Attributes["folder"] = listAsSet(bookmarks.NestedObject.Attributes["folder"])
		groups.NestedObject.Attributes["bookmarks"] = listAsSet(bookmarks)
		component["managed_bookmarks"] = listAsSet(groups)
	})
	withNested(attributes, "safari_extensions", func(component map[string]schema.Attribute) {
		extension := component["managed_extensions"].(schema.SetNestedAttribute).NestedObject.Attributes
		extension["allowed_domains"] = listAsSet(extension["allowed_domains"])
		extension["denied_domains"] = listAsSet(extension["denied_domains"])
	})
	withNested(attributes, "service_background_tasks", func(component map[string]schema.Attribute) {
		tasks := component["background_tasks"].(schema.ListNestedAttribute)
		tasks.NestedObject.Attributes["launchd_configurations"] = listAsSet(tasks.NestedObject.Attributes["launchd_configurations"])
		component["background_tasks"] = listAsSet(tasks)
	})
	withNested(attributes, "service_configuration_files", func(component map[string]schema.Attribute) {
		component["service_config_files"] = listAsSet(component["service_config_files"])
	})
	withNested(attributes, "software_update_settings", func(component map[string]schema.Attribute) {
		component["beta_offer_programs"] = listAsSet(component["beta_offer_programs"])
	})
}

// blueprintSchemaV1 returns the v1 schema with every component collection in its set shape.
func blueprintSchemaV1() *schema.Schema {
	s := blueprintSchemaV1Transcribed()
	restoreV5Sets(s.Attributes)
	return s
}

// blueprintSchemaV2 returns the v2 schema with every component collection in its set shape.
func blueprintSchemaV2() *schema.Schema {
	s := blueprintSchemaV2Transcribed()
	restoreV5Sets(s.Attributes)
	return s
}

// withNested applies rewrite to the attributes of the single nested attribute called name.
func withNested(attributes map[string]schema.Attribute, name string, rewrite func(map[string]schema.Attribute)) {
	rewrite(attributes[name].(schema.SingleNestedAttribute).Attributes)
}

// listAsSet returns the set-nested attribute a v6 list-nested attribute was in v5: the same
// element type and the same optionality. Validators are dropped, as a prior schema only decodes.
// An attribute a prior schema transcribes as a set already is returned unchanged.
func listAsSet(attribute schema.Attribute) schema.Attribute {
	list, ok := attribute.(schema.ListNestedAttribute)
	if !ok {
		return attribute
	}
	return schema.SetNestedAttribute{
		NestedObject: list.NestedObject,
		Optional:     list.Optional,
		Required:     list.Required,
		Computed:     list.Computed,
	}
}
