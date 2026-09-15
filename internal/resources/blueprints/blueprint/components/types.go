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
	{"com.jamf.ddm.audio-accessory-settings", "Audio Accessory Settings"},
	{"com.jamf.ddm.disk-management", "Disk Management Settings"},
	{"com.jamf.ddm.math-settings", "Math Settings"},
	{"com.jamf.ddm.passcode-settings", "Passcode Policy"},
	{"com.jamf.ddm.safari-bookmarks", "Safari Bookmarks"},
	{"com.jamf.ddm.safari-extensions", "Safari Extensions"},
	{"com.jamf.ddm.safari-settings", "Safari Settings"},
	{"com.jamf.ddm.service-background-tasks", "Service Background Tasks"},
	{"com.jamf.ddm.service-configuration-files", "Service Configuration Files"},
	{"com.jamf.ddm.sw-updates", "Software Update"},
	{"com.jamf.ddm.software-update-settings", "Software Update Settings"},
	{"com.jamf.ddm-configuration-profile", "Legacy Payloads"},
	{"com.jamf.ai-governance", "AI Governance"},
}
