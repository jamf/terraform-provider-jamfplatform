// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package devices

import (
	devSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devices"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the devices data source's Read path
// calls. It mirrors the "SDK endpoints used" block in data_source.go and drives
// the "Required Jamf permissions" table appended to the data source
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in data_source.go and with the SDK privilege
// registry.
var dataSourceSDKMethods = []string{
	"ListDevices",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the devices data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(devSDK.Privileges, dataSourceSDKMethods...)
