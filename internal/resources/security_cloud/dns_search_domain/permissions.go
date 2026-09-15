// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the search domain resource calls.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"GetDnsSearchDomainV1",
	"SetDnsSearchDomainV1",
	"ClearDnsSearchDomainV1",
}

// dataSourceSDKMethods lists the SDK methods the search domain data source calls.
var dataSourceSDKMethods = []string{
	"GetDnsSearchDomainV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the search domain resource.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the search domain data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)
