// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the user-initiated enrollment
// settings resource's CRUD path calls across crud.go, access_groups.go, and
// messaging_languages.go. It drives the "Required Jamf permissions" table
// appended to the resource MarkdownDescription. permissions_test.go asserts
// this list stays in sync with the actual client.<Method> calls in those files
// and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"GetEnrollmentSettingsV4",
	"UpdateEnrollmentSettingsV4",
	"ListEnrollmentAccessGroupsV3",
	"CreateEnrollmentAccessGroupV3",
	"UpdateEnrollmentAccessGroupV3",
	"DeleteEnrollmentAccessGroupV3",
	"ListEnrollmentLanguagesV3",
	"ListEnrollmentLanguageCodesV3",
	"GetEnrollmentLanguageV3",
	"UpdateEnrollmentLanguageV3",
	"DeleteEnrollmentLanguageV3",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user-initiated enrollment settings resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the data source's Read path calls.
// A data source documents only the privileges IT needs, not the resource's full
// CRUD set.
var dataSourceSDKMethods = []string{
	"GetEnrollmentSettingsV4",
	"ListEnrollmentAccessGroupsV3",
	"ListEnrollmentLanguagesV3",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the user-initiated enrollment settings data source, appended to
// its MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
