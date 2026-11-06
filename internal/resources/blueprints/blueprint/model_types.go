// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/blueprints/blueprint/components"
)

// BlueprintResourceModel represents the Terraform resource model for a Jamf Blueprint.
type BlueprintResourceModel struct {
	ID              types.String           `tfsdk:"id"`
	Name            types.String           `tfsdk:"name"`
	Description     types.String           `tfsdk:"description"`
	Deployed        types.Bool             `tfsdk:"deployed"`
	DeviceGroups    types.Set              `tfsdk:"device_groups"`
	ComponentBlocks []ComponentBlockModel  `tfsdk:"component_blocks"`
	Created         types.String           `tfsdk:"created"`
	Updated         types.String           `tfsdk:"updated"`
	DeploymentState types.String           `tfsdk:"deployment_state"`
	Timeouts        resourceTimeouts.Value `tfsdk:"timeouts"`
}

// BlueprintDataSourceModel defines the data structure for the blueprint data source.
type BlueprintDataSourceModel struct {
	ID              types.String                    `tfsdk:"id"`
	Name            types.String                    `tfsdk:"name"`
	BlueprintID     types.String                    `tfsdk:"blueprint_id"`
	Description     types.String                    `tfsdk:"description"`
	Created         types.String                    `tfsdk:"created"`
	Updated         types.String                    `tfsdk:"updated"`
	DeploymentState types.String                    `tfsdk:"deployment_state"`
	DeviceGroups    types.List                      `tfsdk:"device_groups"`
	ComponentBlocks []ComponentBlockDataSourceModel `tfsdk:"component_blocks"`
	Timeouts        datasourceTimeouts.Value        `tfsdk:"timeouts"`
}

// ComponentBlockDataSourceModel is one ordered component block in the data source read model.
type ComponentBlockDataSourceModel struct {
	Name                 types.String     `tfsdk:"name"`
	ActivationConditions types.String     `tfsdk:"activation_conditions"`
	Components           []ComponentModel `tfsdk:"component"`
}

// ComponentModel defines the data structure for a blueprint component.
type ComponentModel struct {
	Identifier    types.String `tfsdk:"identifier"`
	Configuration types.Map    `tfsdk:"configuration"`
}

// ComponentBlockModel represents one ordered component block ("Step N" in the Jamf UI): a named
// group with its own activation condition and its own set of components. It doubles as the
// framework-free carrier the input collector and state mapper read and write.
type ComponentBlockModel struct {
	Name                      types.String                                   `tfsdk:"name"`
	ActivationConditions      types.String                                   `tfsdk:"activation_conditions"`
	Components                []ComponentModel                               `tfsdk:"raw_component"`
	AIGovernance              *components.AIGovernanceComponent              `tfsdk:"ai_governance"`
	AppleDeclarations         []AppleDeclarationModel                        `tfsdk:"apple_declarations"`
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

// AppleDeclarationModel is one Apple declaration in a component block's apple_declarations list.
//
// The list is the component: a block carries its declarations directly, because the
// com.jamf.ddm-strict component has no field other than its declarations. So this one is assembled
// in this package rather than in components/ — see appendAppleDeclarations and
// flattenAppleDeclarations, and legacy_payloads for the same split.
//
// A list rather than a set because the order is part of the meaning: a payload names another
// declaration by its position through the `$PAYLOAD_n` placeholder.
//
// Neither `kind` nor `payloadKey` is a field here. The kind follows from the type's reverse-domain
// prefix and the key from the 1-based index, so both are derived on write. Asking for either would
// only add a value an author can get wrong: the platform accepts any pairing and passes a mismatch
// to the device, making it a silent failure rather than a caught one.
type AppleDeclarationModel struct {
	ChannelType types.String `tfsdk:"channel"`
	Payload     types.String `tfsdk:"payload"`
	Type        types.String `tfsdk:"type"`
}

// BlockLegacyPayloadModel is one legacy configuration profile payload inside a component block.
// The framework forbids a dynamic type inside a collection, so a block carries settings as a
// JSON-encoded string (author with `jsonencode(...)`).
type BlockLegacyPayloadModel struct {
	PayloadType types.String `tfsdk:"payload_type"`
	Settings    types.String `tfsdk:"settings"`
}

// hasLegacyPayloads reports whether the configuration writes a legacy configuration profile
// component, which is the only thing the stored payload identifiers are consumed for. Update reads
// them from the service before building its request, and that read is a whole extra round trip on
// every apply, so it is worth skipping for a blueprint authored entirely with typed components,
// raw_component or apple_declarations — where a failed read would otherwise abort an apply with a
// diagnostic naming a feature the configuration never mentions.
//
// The condition mirrors buildSteps exactly. A gate that disagreed with the consumer would hand
// buildSteps a nil stored value and have the service mint a fresh identifier for every payload,
// which is the churn the read exists to prevent.
func (m *BlueprintResourceModel) hasLegacyPayloads() bool {
	for _, block := range m.ComponentBlocks {
		if len(block.LegacyPayloads) > 0 {
			return true
		}
	}
	return false
}

// blueprintIdentityModel defines the resource identity shared with list results.
type blueprintIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// BlueprintListResourceModel captures supported list filters.
type BlueprintListResourceModel struct {
	Search   types.String       `tfsdk:"search"`
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
