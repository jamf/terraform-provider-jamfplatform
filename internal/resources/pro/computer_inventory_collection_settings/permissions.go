// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_inventory_collection_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the resource's CRUD path calls. It
// mirrors the "SDK endpoints used" block in crud.go and drives the "Required
// Jamf privileges" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"GetComputerInventoryCollectionSettingsV2",
	"UpdateComputerInventoryCollectionSettingsV2",
	"CreateComputerInventoryCollectionCustomPathV2",
	"DeleteComputerInventoryCollectionCustomPathV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the data source's Read path calls.
// A data source documents only the privileges IT needs, not the resource's
// full CRUD set.
var dataSourceSDKMethods = []string{
	"GetComputerInventoryCollectionSettingsV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
