// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the activation profile resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
//
// Pause and resume are both here because the resource asserts the profile's
// paused state on create and on update, and the two operations share the
// activation-profiles:update privilege.
var resourceSDKMethods = []string{
	"CreateActivationProfileV1",
	"GetActivationProfileV1",
	"PauseActivationProfileV1",
	"ResumeActivationProfileV1",
	"DeleteActivationProfilesV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the activation profile resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)
