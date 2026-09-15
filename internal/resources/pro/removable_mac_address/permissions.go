// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the removable MAC address resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateRemovableMacAddressByID",
	"GetRemovableMacAddressByID",
	"UpdateRemovableMacAddressByID",
	"DeleteRemovableMacAddressByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the removable MAC address resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular data source calls
// (lookup by ID or by MAC address). It documents only the privileges the data
// source needs, not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetRemovableMacAddressByID",
	"GetRemovableMacAddressByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the removable MAC address data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListRemovableMacAddresses",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the removable MAC address list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
