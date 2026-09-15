// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the macOS Onboarding resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives the
// "Required Jamf permissions" table appended to the resource MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"GetOnboardingV1",
	"UpdateOnboardingV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the macOS Onboarding resource, appended to its MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the singleton data source's Read
// path calls. A data source documents only the privileges IT needs — a read of
// the onboarding configuration.
var dataSourceSDKMethods = []string{
	"GetOnboardingV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the macOS Onboarding data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)

// pluralDataSourceSDKMethods lists the SDK methods the eligible-items discovery
// data source's Read path calls. It fans the single entity_type argument out to
// one of the eligible-* list endpoints, so it may call any of these three.
var pluralDataSourceSDKMethods = []string{
	"ListOnboardingEligiblePoliciesV1",
	"ListOnboardingEligibleConfigurationProfilesV1",
	"ListOnboardingEligibleAppsV1",
}

// pluralDataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the macOS Onboarding eligible-items data source.
var pluralDataSourcePrivileges = permissions.Section(pro.Privileges, pluralDataSourceSDKMethods...)
