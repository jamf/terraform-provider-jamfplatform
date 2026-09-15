// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package licensed_software

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the licensed software resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateLicensedSoftwareByID",
	"GetLicensedSoftwareByID",
	"UpdateLicensedSoftwareByID",
	"DeleteLicensedSoftwareByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the licensed software resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the licensed software data source
// calls (lookup by ID or by exact name).
var dataSourceSDKMethods = []string{
	"GetLicensedSoftwareByID",
	"GetLicensedSoftwareByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the licensed software data source.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the licensed software list
// resource calls.
var listResourceSDKMethods = []string{
	"ListLicensedSoftware",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetLicensedSoftwareByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the licensed software list resource.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
