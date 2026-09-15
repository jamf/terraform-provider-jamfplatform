// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the printer resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreatePrinterByID",
	"GetPrinterByID",
	"UpdatePrinterByID",
	"DeletePrinterByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the printer resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the printer data source calls.
// Lookup is by ID or by exact name, so it requires only read access.
var dataSourceSDKMethods = []string{
	"GetPrinterByID",
	"GetPrinterByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the printer data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the printer list resource calls.
// It lists identities, then optionally fetches the full record per item when
// include_resource is set.
var listResourceSDKMethods = []string{
	"ListPrinters",
	"GetPrinterByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the printer list resource, appended to its description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
