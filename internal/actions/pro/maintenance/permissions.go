// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package maintenanceactions

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// flushPolicyLogsSDKMethods lists the SDK methods the flush_policy_logs action's
// Invoke path calls. It mirrors the "SDK endpoints used" calls in
// flush_policy_logs.go and drives the "Required Jamf permissions" table appended
// to the action MarkdownDescription. permissions_test.go asserts this list stays
// in sync with the actual classic.<Method> calls in flush_policy_logs.go and
// with the SDK privilege registry.
var flushPolicyLogsSDKMethods = []string{
	"GetPolicyByID",
	"DeleteLogFlushByLogIDInterval",
}

// flushPolicyLogsPrivileges is the rendered "Required Jamf permissions" Markdown
// section for the flush_policy_logs action, appended to its MarkdownDescription.
var flushPolicyLogsPrivileges = permissions.Section(proclassic.Privileges, flushPolicyLogsSDKMethods...)

// redeployManagementFrameworkSDKMethods lists the SDK methods the
// redeploy_management_framework action's Invoke path calls. It mirrors the
// client.<Method> calls in redeploy_management_framework.go and drives the
// "Required Jamf permissions" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in redeploy_management_framework.go and with the SDK
// privilege registry.
var redeployManagementFrameworkSDKMethods = []string{
	"RedeployJamfManagementFrameworkV1",
}

// redeployManagementFrameworkPrivileges is the rendered "Required Jamf
// privileges" Markdown section for the redeploy_management_framework action,
// appended to its MarkdownDescription.
var redeployManagementFrameworkPrivileges = permissions.Section(pro.Privileges, redeployManagementFrameworkSDKMethods...)
