// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the cloud distribution point
// resource's CRUD path calls. It mirrors the "SDK endpoints used" block in
// crud.go and drives the "Required Jamf permissions" table appended to the
// resource MarkdownDescription. permissions_test.go asserts this list stays in
// sync with the actual client.<Method> calls in crud.go and with the SDK
// privilege registry.
var resourceSDKMethods = []string{
	"CreateCloudDistributionPointV1",
	"GetCloudDistributionPointV1",
	"UpdateCloudDistributionPointV1",
	"DeleteCloudDistributionPointV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the cloud distribution point resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the cloud distribution point data
// source's Read path calls — only the privileges the data source itself needs.
var dataSourceSDKMethods = []string{
	"GetCloudDistributionPointV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the cloud distribution point data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
