// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the enrollment customization
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateEnrollmentCustomizationV2",
	"GetEnrollmentCustomizationV2",
	"UpdateEnrollmentCustomizationV2",
	"DeleteEnrollmentCustomizationV2",
	"UploadEnrollmentCustomizationImageV2",
	"ListEnrollmentCustomizationPanelsV1",
	"CreateEnrollmentCustomizationTextPanelV1",
	"GetEnrollmentCustomizationTextPanelV1",
	"UpdateEnrollmentCustomizationTextPanelV1",
	"CreateEnrollmentCustomizationLdapPanelV1",
	"GetEnrollmentCustomizationLdapPanelV1",
	"UpdateEnrollmentCustomizationLdapPanelV1",
	"CreateEnrollmentCustomizationSsoPanelV1",
	"GetEnrollmentCustomizationSsoPanelV1",
	"UpdateEnrollmentCustomizationSsoPanelV1",
	"DeleteEnrollmentCustomizationPanelV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the enrollment customization resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the enrollment customization data
// source calls (data_source.go). The name-lookup path resolves via
// ResolveEnrollmentCustomizationV2ByName, which is a synthetic resolver absent
// from the SDK privilege registry; the privilege it requires
// (enrollment-customization:read) is already covered by
// GetEnrollmentCustomizationV2.
var dataSourceSDKMethods = []string{
	"GetEnrollmentCustomizationV2",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the enrollment customization data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// listResourceSDKMethods lists the SDK methods the enrollment customization
// list resource's List path calls. Drives the privileges table appended to the
// list resource Description.
var listResourceSDKMethods = []string{
	"ListEnrollmentCustomizationsV2",
}

// listResourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the enrollment customization list resource, appended to its
// Description.
var listResourcePrivileges = permissions.Section(pro.Privileges, listResourceSDKMethods...)
