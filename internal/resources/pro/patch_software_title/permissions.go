// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_software_title

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// mergedPrivileges is the union of the two SDK families every construct in this
// package spans: the Pro v3 configuration endpoints do the work, and ProClassic
// supplies the id-minting create plus the patch-source catalogues source_id is
// resolved from.
var mergedPrivileges = permissions.Merge(proclassic.Privileges, pro.Privileges)

// resourceSDKMethods lists the SDK methods the patch software title resource's
// CRUD path calls. Read/update/delete and the extension-attribute side-channel
// run through the Pro v3 client (crud.go, extension_attributes.go); ProClassic
// contributes the create that mints the id and the two source catalogues
// source_id is resolved from on import (patch_sources.go). It drives the
// "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual <client>.<Method> calls in those files and with the SDK privilege
// registries.
var resourceSDKMethods = []string{
	"CreatePatchSoftwareTitleByID",
	"GetPatchSoftwareTitleConfigurationV3",
	"UpdatePatchSoftwareTitleConfigurationV3",
	"DeletePatchSoftwareTitleConfigurationV3",
	"ListPatchSoftwareTitleDefinitionsV3",
	"ListPatchSoftwareTitleExtensionAttributesV3",
	"ListPatchInternalSources",
	"ListPatchExternalSources",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the patch software title resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(mergedPrivileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the patch software title data
// source calls (data_source.go, patch_sources.go). Lookup is read-only, by ID
// or by exact name — the v3 configurations list returns whole objects, so a
// name lookup needs no follow-up get.
var dataSourceSDKMethods = []string{
	"GetPatchSoftwareTitleConfigurationV3",
	"ListPatchSoftwareTitleConfigurationsV3",
	"ListPatchSoftwareTitleDefinitionsV3",
	"ListPatchInternalSources",
	"ListPatchExternalSources",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch software title data source.
var dataSourcePrivileges = permissions.Section(mergedPrivileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the patch software title list
// resource calls (list_resource.go).
var listResourceSDKMethods = []string{
	"ListPatchSoftwareTitleConfigurationsV3",
	"ListPatchInternalSources",
	"ListPatchExternalSources",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch software title list resource.
var listResourcePrivileges = permissions.Section(mergedPrivileges, listResourceSDKMethods...)
