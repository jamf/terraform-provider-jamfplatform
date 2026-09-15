// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the activation code resource's CRUD
// path calls. Read calls GetActivationCode directly; Create/Update funnel
// through applyActivationCode (helpers.go), which calls UpdateActivationCode
// then GetActivationCode to read back the authoritative state. It drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go + helpers.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"GetActivationCode",
	"UpdateActivationCode",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the activation code resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the activation code data source
// calls. The data source is read-only — GetActivationCode is its entire SDK
// surface. permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in data_source.go and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetActivationCode",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the activation code data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)
