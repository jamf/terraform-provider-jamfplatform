// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package class

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the class resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateClassByID",
	"GetClassByID",
	"UpdateClassByID",
	"DeleteClassByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the class resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the class data source calls. A
// data source documents only the read privileges it needs, never the full CRUD
// set.
var dataSourceSDKMethods = []string{
	"GetClassByID",
	"GetClassByName",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the class data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the class list resource calls.
var listResourceSDKMethods = []string{
	"ListClasses",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetClassByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the class list resource, appended to its Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
