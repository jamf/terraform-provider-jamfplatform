// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_predefined_apps

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the predefined apps data source calls.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in data_source.go and with the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"ListZtnaPredefinedAppsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the predefined apps data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)
