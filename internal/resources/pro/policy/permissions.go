// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the policy resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreatePolicyByID",
	"GetPolicyByID",
	"UpdatePolicyByID",
	"DeletePolicyByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the policy resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the policy data source calls. The
// data source resolves a policy by ID or by exact name, so it requires only the
// read privileges its two GET calls need.
var dataSourceSDKMethods = []string{
	"GetPolicyByID",
	"GetPolicyByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the policy data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the policy list resource calls.
// It lists policy identities and, when IncludeResource is requested (config
// generation), hydrates each policy via a per-item GET.
var listResourceSDKMethods = []string{
	"ListPolicies",
	"GetPolicyByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the policy list resource, appended to its schema description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
