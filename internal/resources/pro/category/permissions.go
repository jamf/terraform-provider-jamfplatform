// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package category

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the category resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateCategoryV1",
	"GetCategoryV1",
	"UpdateCategoryV1",
	"DeleteCategoryV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the category resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular category data source
// calls. The data source only reads, so it documents fewer privileges than the
// resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetCategoryV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular category data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the plural categories data
// source calls.
var pluralDataSourceSDKMethods = []string{
	"ListCategoriesV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the plural categories data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the category list resource
// calls.
var listResourceSDKMethods = []string{
	"ListCategoriesV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the category list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
