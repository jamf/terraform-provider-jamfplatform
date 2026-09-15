// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_failover_url

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the SSO failover URL resource's CRUD
// path calls. It mirrors the "SDK endpoints used" block in crud.go and drives
// the "Required Jamf permissions" table appended to the resource
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in crud.go and with the SDK privilege
// registry.
var resourceSDKMethods = []string{
	"GenerateSsoFailoverV1",
	"GetSsoFailoverV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the SSO failover URL resource, appended to its
// MarkdownDescription.
var resourcePrivileges = permissions.Section(pro.Privileges, resourceSDKMethods...)

// dataSourceSDKMethods lists the SDK methods the SSO failover URL data source
// calls. A data source documents only the privileges it needs — here just the
// read endpoint.
var dataSourceSDKMethods = []string{
	"GetSsoFailoverV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the SSO failover URL data source, appended to its
// MarkdownDescription.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
