// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_adcs

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the AD CS resource's CRUD path calls.
// It mirrors the "SDK endpoints used" block in crud.go and drives the "Required
// Jamf privileges" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"CreateAdcsSettingsV1",
	"GetAdcsSettingsV1",
	"UpdateAdcsSettingsV1",
	"DeleteAdcsSettingsV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the AD CS resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the AD CS data source's Read path
// calls. A data source documents only the privileges it needs (a read), not the
// resource's full CRUD set. permissions_test.go keeps this in sync with
// data_source.go and the SDK privilege registry.
var dataSourceSDKMethods = []string{
	"GetAdcsSettingsV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the AD CS data source, appended to its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
