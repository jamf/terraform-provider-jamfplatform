// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_external_source

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the patch external source resource's
// CRUD path calls. It mirrors the "SDK endpoints used" block in crud.go and
// drives the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"CreatePatchExternalSourceByID",
	"GetPatchExternalSourceByID",
	"UpdatePatchExternalSourceByID",
	"DeletePatchExternalSourceByID",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch external source resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(proclassic.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singular data source calls:
// lookup by ID or by name, plus the available-titles catalog fetch.
var dataSourceSDKMethods = []string{
	"GetPatchExternalSourceByID",
	"GetPatchExternalSourceByName",
	"ListPatchAvailableTitlesBySourceID",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch external source data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(proclassic.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the list resource calls.
var listResourceSDKMethods = []string{
	"ListPatchExternalSources",
	// The per-item hydration GET the list resource issues when
	// IncludeResource asks for full resource state.
	"GetPatchExternalSourceByID",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the patch external source list resource, appended to its
// schema Description.
var listResourcePrivileges = permissions.Section(proclassic.Privileges, listResourceSDKMethods...)
