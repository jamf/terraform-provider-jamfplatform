// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the supervision identity resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreateSupervisionIdentityV1",
	"UploadSupervisionIdentityV1",
	"GetSupervisionIdentityV1",
	"UpdateSupervisionIdentityV1",
	"DeleteSupervisionIdentityV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the supervision identity resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the privilege-bearing SDK methods the data source
// calls. data_source.go also calls ResolveSupervisionIdentityV1ByName, but that
// resolver is a name-lookup wrapper that delegates to the same
// apple-configurator-enrollment:read endpoint and is not itself a key in
// the SDK privilege registry, so only the GET is listed here.
var dataSourceSDKMethods = []string{
	"GetSupervisionIdentityV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the supervision identity data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListSupervisionIdentitiesV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the supervision identity list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
