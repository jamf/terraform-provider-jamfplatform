// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package deviceactions

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/deviceactions"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// eraseDeviceSDKMethods lists the SDK methods the device erase action's Invoke
// path calls. It mirrors the a.actions.<Method> call in erase.go and drives the
// "Required Jamf permissions" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// a.actions.<Method> calls in erase.go and with the SDK privilege registry.
var eraseDeviceSDKMethods = []string{
	"EraseDevice",
}

// eraseDevicePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device erase action, appended to its MarkdownDescription.
var eraseDevicePrivileges = permissions.Section(deviceactions.Privileges, eraseDeviceSDKMethods...)

// restartDeviceSDKMethods lists the SDK methods the device restart action's
// Invoke path calls. Mirrors the a.actions.<Method> call in restart.go.
var restartDeviceSDKMethods = []string{
	"RestartDevice",
}

// restartDevicePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device restart action.
var restartDevicePrivileges = permissions.Section(deviceactions.Privileges, restartDeviceSDKMethods...)

// shutdownDeviceSDKMethods lists the SDK methods the device shutdown action's
// Invoke path calls. Mirrors the a.actions.<Method> call in shutdown.go.
var shutdownDeviceSDKMethods = []string{
	"ShutdownDevice",
}

// shutdownDevicePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device shutdown action.
var shutdownDevicePrivileges = permissions.Section(deviceactions.Privileges, shutdownDeviceSDKMethods...)

// unmanageDeviceSDKMethods lists the SDK methods the device unmanage action's
// Invoke path calls. Mirrors the a.actions.<Method> call in unmanage.go.
var unmanageDeviceSDKMethods = []string{
	"UnmanageDevice",
}

// unmanageDevicePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the device unmanage action.
var unmanageDevicePrivileges = permissions.Section(deviceactions.Privileges, unmanageDeviceSDKMethods...)
