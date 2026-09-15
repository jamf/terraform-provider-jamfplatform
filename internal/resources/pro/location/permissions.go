// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package location

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the volume purchasing location
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateVolumePurchasingLocationV1",
	"GetVolumePurchasingLocationV1",
	"UpdateVolumePurchasingLocationV1",
	"DeleteVolumePurchasingLocationV1",
	"ReclaimVolumePurchasingLocationLicensesV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the volume purchasing location resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular volume purchasing
// location data source calls. The data source only reads, so it documents
// fewer privileges than the resource's full CRUD set. The name-lookup wrapper
// ResolveVolumePurchasingLocationV1ByName is intentionally omitted: it is not a
// privilege-bearing SDK registry method (it composes the same read privilege
// the GET already requires).
var dataSourceSDKMethods = []string{
	"GetVolumePurchasingLocationV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the singular volume purchasing location data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the volume purchasing location
// list resource calls. Identity-only listing uses the list endpoint; an
// include_resource query additionally reads each row via the singular GET.
var listResourceSDKMethods = []string{
	"ListVolumePurchasingLocationsV1",
	"GetVolumePurchasingLocationV1",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the volume purchasing location list resource.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
