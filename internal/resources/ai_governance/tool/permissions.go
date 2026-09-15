// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tool

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the singular tool data source calls.
// permissions_test.go asserts this list stays in sync with the actual client.<Method> calls in the
// package and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetTool",
	"GetToolSchema",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section for the singular
// tool data source.
var dataSourcePrivileges = permissions.Section(aigovernance.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural tools data source calls.
var pluralDataSourceSDKMethods = []string{
	"ListTools",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section for the
// plural tools data source.
var pluralDataSourcePrivileges = permissions.Section(aigovernance.Privileges, pluralDataSourceSDKMethods...)
