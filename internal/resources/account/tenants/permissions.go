// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenants

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the tenants data source calls. It
// mirrors the call in data_source.go and drives the "Required Jamf permissions"
// table appended to the MarkdownDescription.
//
// The privilege comes from the gateway's own authorization policy rather than the
// published specification, which declares none for this namespace. The SDK
// records that provenance per method, so a spec-declared empty set is never
// rendered as "no permission needed".
var dataSourceSDKMethods = []string{
	"ListTenants",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the tenants data source.
var dataSourcePrivileges = permissions.Section(account.Privileges, dataSourceSDKMethods...)
