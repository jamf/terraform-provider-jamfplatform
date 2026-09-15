// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// mergedPrivileges is the union of the two SDK families this package spans:
// every CRUD call and the per-item list read go through ProClassic, while the
// list resource enumerates on the Pro v2 patch-policies collection.
var mergedPrivileges = permissions.Merge(proclassic.Privileges, pro.Privileges)

// resourceSDKMethods lists the SDK methods the patch policy resource's CRUD path
// calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
//
// Note: the resource is bucketed under the pro/ tree to mirror the Jamf Pro
// admin UI, but its CRUD client is *proclassic.Client, so its privileges come
// from the proclassic half of the merged registry.
var resourceSDKMethods = []string{
	"CreatePatchPolicyBySoftwareTitleConfigID",
	"GetPatchPolicyByID",
	"UpdatePatchPolicyByID",
	"DeletePatchPolicyByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch policy resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(mergedPrivileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the patch policy data source calls.
// A data source documents only the privileges IT needs (a single read), not the
// resource's full CRUD set.
var dataSourceSDKMethods = []string{
	"GetPatchPolicyByID",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch policy data source.
var dataSourcePrivileges = permissions.Section(mergedPrivileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the patch policy list resource
// calls: the Pro v2 enumeration plus the ProClassic per-item GET issued during
// config generation (IncludeResource). Both map to patch-policies:read, so the
// rendered table is one row despite spanning two API families.
var listResourceSDKMethods = []string{
	"ListPatchPoliciesV2",
	"GetPatchPolicyByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch policy list resource.
var listResourcePrivileges = permissions.Section(mergedPrivileges, listResourceSDKMethods...)
