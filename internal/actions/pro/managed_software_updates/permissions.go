// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// abandonFeatureToggleSDKMethods lists the SDK methods the abandon-feature-toggle
// action's Invoke path calls. It mirrors the "SDK endpoints used" block in
// abandon.go and drives the "Required Jamf permissions" table appended to the
// action MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in abandon.go and with the SDK
// privilege registry.
var abandonFeatureToggleSDKMethods = []string{
	"AbandonManagedSoftwareUpdateFeatureToggleV1",
}

// abandonFeatureTogglePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the abandon-feature-toggle action, appended to its
// MarkdownDescription.
var abandonFeatureTogglePrivileges = permissions.Section(pro.Privileges, abandonFeatureToggleSDKMethods...)

// planSDKMethods lists the SDK methods the plan action's Invoke path calls. It
// mirrors the "SDK endpoints used" block in plan.go and drives the "Required
// Jamf privileges" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in plan.go and with the SDK privilege registry.
var planSDKMethods = []string{
	"CreateManagedSoftwareUpdateGroupPlanV1",
}

// planPrivileges is the rendered "Required Jamf permissions" Markdown section for
// the plan action, appended to its MarkdownDescription.
var planPrivileges = permissions.Section(pro.Privileges, planSDKMethods...)
