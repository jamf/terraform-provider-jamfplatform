// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// resourceSDKMethods lists the SDK methods the hostname mappings resource calls.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls in crud.go and with the SDK privilege registry.
var resourceSDKMethods = []string{
	"GetDnsCustomHostnameMappingsV1",
	"ReplaceDnsCustomHostnameMappingsV1",
	"ClearDnsCustomHostnameMappingsV1",
}

// dataSourceSDKMethods lists the SDK methods the hostname mappings data source
// calls.
var dataSourceSDKMethods = []string{
	"GetDnsCustomHostnameMappingsV1",
}

// resourcePrivileges is the rendered "Required Jamf permissions" Markdown section for
// the hostname mappings resource.
var resourcePrivileges = permissions.Section(securitycloud.Privileges, resourceSDKMethods...)

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown section
// for the hostname mappings data source.
var dataSourcePrivileges = permissions.Section(securitycloud.Privileges, dataSourceSDKMethods...)
