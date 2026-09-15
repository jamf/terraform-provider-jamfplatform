// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_protect

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the Jamf Protect resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"RegisterJamfProtectV1",
	"GetJamfProtectSettingsV1",
	"UpdateJamfProtectSettingsV1",
	"UnregisterJamfProtectV1",
	"SyncJamfProtectPlansV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the Jamf Protect resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the Jamf Protect plans
// plural data source calls. permissions_test.go asserts this list stays in sync
// with the actual client.<Method> calls in data_source.go and with the SDK
// privilege registry.
var pluralDataSourceSDKMethods = []string{
	"ListJamfProtectPlansV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions"
// Markdown section for the Jamf Protect plans plural data source, appended to
// its MarkdownDescription.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)
