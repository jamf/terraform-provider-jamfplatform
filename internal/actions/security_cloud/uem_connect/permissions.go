// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// synchronizeSDKMethods lists the SDK methods the
// jamfplatform_security_cloud_uem_connect_synchronize action's Invoke path calls.
// It mirrors the "SDK endpoints used" block in synchronize.go and drives the
// "Required Jamf permissions" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls and with the SDK privilege registry.
//
// The list read is declared because the action falls back to it to find the
// tenant's only integration when no ID is configured — so a caller granted only
// the update privilege would still fail on the common, ID-less form.
var synchronizeSDKMethods = []string{
	"TriggerUemConnectorSyncV1",
	"ListUemConnectorsV1",
}

// synchronizePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the synchronize action.
var synchronizePrivileges = permissions.Section(securitycloud.Privileges, synchronizeSDKMethods...)

// deployActivationProfileSDKMethods lists the SDK methods the
// jamfplatform_security_cloud_activation_profile_deploy action's Invoke path
// calls. It mirrors the "SDK endpoints used" block in
// deploy_activation_profile.go and drives the "Required Jamf permissions" table
// appended to the action MarkdownDescription. permissions_test.go asserts this
// list stays in sync with the actual client.<Method> calls and with the SDK
// privilege registry.
//
// One method only: unlike synchronize, this action needs no connector lookup —
// the route takes the activation profile code rather than a connector ID, and
// Jamf Security Cloud resolves the tenant's connector itself.
var deployActivationProfileSDKMethods = []string{
	"DeployActivationProfileToUemV1",
}

// deployActivationProfilePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the deploy action.
var deployActivationProfilePrivileges = permissions.Section(securitycloud.Privileges, deployActivationProfileSDKMethods...)
