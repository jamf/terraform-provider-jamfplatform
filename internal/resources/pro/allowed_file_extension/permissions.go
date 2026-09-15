// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the allowed file extension resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry. There is no
// update write (the record has no PUT; Update only GETs), so no create/update method
// beyond the create-and-read pair is listed.
var resourceSDKMethods = []string{
	"CreateAllowedFileExtensionByID",
	"GetAllowedFileExtensionByID",
	"DeleteAllowedFileExtensionByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section for
// the allowed file extension resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular data source calls. It looks
// up a record by ID or by extension, so it needs the two read methods only.
var dataSourceSDKMethods = []string{
	"GetAllowedFileExtensionByID",
	"GetAllowedFileExtensionByExtension",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section for
// the allowed file extension data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListAllowedFileExtensions",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the allowed file extension list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
