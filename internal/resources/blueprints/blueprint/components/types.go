// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"encoding/json"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// ComponentConverter interface defines methods that typed components should implement
// to convert between the user-friendly typed format and the raw API format.
type ComponentConverter interface {
	GetIdentifier() string
	ToRawConfiguration() (json.RawMessage, error)
	FromRawConfiguration(raw json.RawMessage) error
	ToClientComponent() (*blueprints.Component, error)
}

// ComponentRegistry maps component identifiers to their human-readable names for easier management.
type ComponentRegistry struct {
	identifier string
	name       string
}

// CommonComponentRegistries defines all supported strongly-typed components.
var CommonComponentRegistries = []ComponentRegistry{
	{blueprints.AudioAccessorySettingsComponentIdentifierComJamfDdmAudioAccessorySettings, "Audio Accessory Settings"},
	{blueprints.DiskManagementComponentIdentifierComJamfDdmDiskManagement, "Disk Management Settings"},
	{blueprints.MathSettingsComponentIdentifierComJamfDdmMathSettings, "Math Settings"},
	{blueprints.PasscodeSettingsComponentIdentifierComJamfDdmPasscodeSettings, "Passcode Policy"},
	{blueprints.SafariBookmarksComponentIdentifierComJamfDdmSafariBookmarks, "Safari Bookmarks"},
	{blueprints.SafariExtensionsComponentIdentifierComJamfDdmSafariExtensions, "Safari Extensions"},
	{blueprints.SafariSettingsComponentIdentifierComJamfDdmSafariSettings, "Safari Settings"},
	{"com.jamf.ddm.service-background-tasks", "Service Background Tasks"},
	{"com.jamf.ddm.service-configuration-files", "Service Configuration Files"},
	{blueprints.SwUpdateComponentIdentifierComJamfDdmSwUpdates, "Software Update"},
	{blueprints.SoftwareUpdateSettingsComponentIdentifierComJamfDdmSoftwareUpdateSettings, "Software Update Settings"},
	{blueprints.ConfigurationProfileIdentifierComJamfDdmConfigurationProfile, "Legacy Payloads"},
	{"com.jamf.ai-governance", "AI Governance"},
}
