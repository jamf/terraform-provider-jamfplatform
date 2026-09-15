// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the iBeacon resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateIBeaconByID",
	"GetIBeaconByID",
	"UpdateIBeaconByID",
	"DeleteIBeaconByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the iBeacon resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the iBeacon data source calls.
// The singular data source looks up by ID or by exact name.
var dataSourceSDKMethods = []string{
	"GetIBeaconByID",
	"GetIBeaconByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the iBeacon data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the iBeacon list resource calls.
var listResourceSDKMethods = []string{
	"ListIBeacons",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the iBeacon list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
