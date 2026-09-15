// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the inventory preload record
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateInventoryPreloadRecordV2",
	"GetInventoryPreloadRecordV2",
	"UpdateInventoryPreloadRecordV2",
	"DeleteInventoryPreloadRecordV2",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the inventory preload record resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-known SDK methods the data source's
// Read path calls. The data source additionally invokes
// ResolveInventoryPreloadRecordV2BySerialNumber, a name-resolver convenience
// that is not a registry entry of its own — it issues the same
// inventory-preload-records:read call as GetInventoryPreloadRecordV2, whose
// privilege already covers it here, so the table is complete. The match test
// filters client calls to registry-known methods to keep the resolver from
// being counted as undeclared.
var dataSourceSDKMethods = []string{
	"GetInventoryPreloadRecordV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the inventory preload record data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource's List path
// calls.
var listResourceSDKMethods = []string{
	"ListInventoryPreloadRecordsV2",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the inventory preload record list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
