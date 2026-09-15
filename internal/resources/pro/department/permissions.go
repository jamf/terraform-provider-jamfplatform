// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the department resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// r.client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateDepartmentV1",
	"GetDepartmentV1",
	"UpdateDepartmentV1",
	"DeleteDepartmentV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the department resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular department data
// source calls (data_source.go). It documents only the privileges that
// construct needs — a read, not the resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetDepartmentV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular department data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural departments data
// source calls (datasource_plural.go).
var pluralDataSourceSDKMethods = []string{
	"ListDepartmentsV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural departments data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the department list resource
// calls (list_resource.go).
var listResourceSDKMethods = []string{
	"ListDepartmentsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the department list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
