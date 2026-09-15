// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appinstalleractions

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// retryInstallationsSDKMethods lists the SDK methods the
// jamfplatform_pro_retry_app_installer_installations action's Invoke path calls.
// It mirrors the client.<Method> calls in retry_installations.go and drives the
// "Required Jamf permissions" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls and with the SDK privilege registry.
var retryInstallationsSDKMethods = []string{
	"RetryAppInstallerDeploymentInstallationsV1",
	"RetryAppInstallerDeploymentComputerInstallationV1",
}

// retryInstallationsPrivileges is the rendered "Required Jamf permissions"
// Markdown section for the per-deployment retry action.
var retryInstallationsPrivileges = permissions.Section(pro.Privileges, retryInstallationsSDKMethods...)

// retryAllInstallationsSDKMethods lists the SDK methods the
// jamfplatform_pro_retry_all_app_installer_installations action's Invoke path
// calls. It mirrors the client.<Method> calls in retry_all_installations.go.
var retryAllInstallationsSDKMethods = []string{
	"RetryAppInstallerInstallationsV1",
}

// retryAllInstallationsPrivileges is the rendered "Required Jamf permissions"
// Markdown section for the tenant-wide retry action.
var retryAllInstallationsPrivileges = permissions.Section(pro.Privileges, retryAllInstallationsSDKMethods...)

// updateVersionSDKMethods lists the SDK methods the
// jamfplatform_pro_update_app_installer_version action's Invoke path calls. It
// mirrors the client.<Method> calls in update_version.go.
var updateVersionSDKMethods = []string{
	"UpdateAppInstallerDeploymentVersionV1",
}

// updateVersionPrivileges is the rendered "Required Jamf permissions" Markdown
// section for the version-update action.
var updateVersionPrivileges = permissions.Section(pro.Privileges, updateVersionSDKMethods...)
