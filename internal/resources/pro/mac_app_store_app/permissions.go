// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Mac App Store app resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateMacApplicationByID",
	"GetMacApplicationByID",
	"UpdateMacApplicationByID",
	"DeleteMacApplicationByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Mac App Store app resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the data source's Read path calls
// (ID or name lookup). It documents only the privileges the data source needs,
// not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetMacApplicationByID",
	"GetMacApplicationByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Mac App Store app data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource's List path
// calls: the bulk list plus the per-item hydration GET used when
// IncludeResource is requested (config generation).
var listResourceSDKMethods = []string{
	"ListMacApplications",
	"GetMacApplicationByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Mac App Store app list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
