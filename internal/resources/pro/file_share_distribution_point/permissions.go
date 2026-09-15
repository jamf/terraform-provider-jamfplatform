// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the file share distribution point
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateDistributionPointV1",
	"GetDistributionPointV1",
	"PatchDistributionPointV1",
	"DeleteDistributionPointV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the file share distribution point resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the registry-backed SDK methods the data source's
// Read path calls. The name lookup also calls ResolveDistributionPointV1ByName,
// a resolver wrapper that is not a distinct privilege registry entry (it
// resolves by name then reads, requiring only the read privilege
// GetDistributionPointV1 already covers), so it is intentionally omitted here;
// the match test filters it out.
var dataSourceSDKMethods = []string{
	"GetDistributionPointV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the file share distribution point data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource's List path
// calls.
var listResourceSDKMethods = []string{
	"ListDistributionPointsV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the file share distribution point list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
