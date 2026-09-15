// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the policy resource's CRUD path calls, including the
// plan-time validation path — an operator whose integration cannot read the tool catalogue gets no
// validation, so the read privilege belongs in the table even though nothing fails without it.
// permissions_test.go asserts this list stays in sync with the actual client.<Method> calls in the
// package and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreatePolicy",
	"GetPolicy",
	"UpdatePolicy",
	"PublishPolicy",
	"ArchivePolicy",
	"ListTools",
	"GetToolSchema",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section for the policy
// resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(aigovernance.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular policy data source calls.
var dataSourceSDKMethods = []string{
	"GetPolicy",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section for the singular
// policy data source.
var dataSourcePrivileges = permissions.Section(aigovernance.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural policies data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListPolicies",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section for the
// plural policies data source.
var pluralDataSourcePrivileges = permissions.Section(aigovernance.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the policy list resource calls. GetPolicy is only
// reached when a query asks for full resource state: the listing omits the settings, so there is no
// way to answer that from the list alone.
var listResourceSDKMethods = []string{
	"ListPolicies",
	"GetPolicy",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown section for the policy
// list resource.
var listResourcePrivileges = permissions.Section(aigovernance.Privileges, listResourceSDKMethods...)
