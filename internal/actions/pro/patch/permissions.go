// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patchactions

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// retryPatchPolicyLogsSDKMethods lists the SDK methods the
// jamfplatform_pro_retry_patch_policy_logs action's Invoke path calls. It
// mirrors the client.<Method> calls in retry_patch_policy_logs.go and drives the
// "Required Jamf permissions" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls and with the SDK privilege registry.
var retryPatchPolicyLogsSDKMethods = []string{
	"RetryPatchPolicyLogsV2",
	"RetryAllPatchPolicyLogsV2",
}

// retryPatchPolicyLogsPrivileges is the rendered "Required Jamf permissions"
// Markdown section for the retry patch policy logs action, appended to its
// MarkdownDescription.
var retryPatchPolicyLogsPrivileges = permissions.Section(pro.Privileges, retryPatchPolicyLogsSDKMethods...)
