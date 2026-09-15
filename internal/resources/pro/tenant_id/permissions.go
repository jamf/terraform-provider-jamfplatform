// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenant_id

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// dataSourceSDKMethods lists the SDK methods the tenant identifier data source's
// Read path calls. It mirrors the "SDK endpoints used" block in data_source.go
// and drives the "Required Jamf permissions" table appended to the data source
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls in data_source.go and with the SDK privilege
// registry.
//
// The registry records no privileges for this method, so the rendered section is
// the "None — any authenticated integration may call it" variant rather than a
// table. That is deliberate on Jamf's side: the identifier is the value an
// integration needs before it can be pointed at anything, so gating the read on
// a privilege would be circular.
var dataSourceSDKMethods = []string{
	"GetCsaTenantIdV1",
}

// dataSourcePrivileges is the rendered "Required Jamf permissions" Markdown
// section for the tenant identifier data source.
var dataSourcePrivileges = permissions.Section(pro.Privileges, dataSourceSDKMethods...)
