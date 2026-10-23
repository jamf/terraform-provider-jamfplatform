// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platformenvironmentaction

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// assignTenantsSDKMethods lists the SDK methods the
// jamfplatform_account_platform_environment_assign_tenants action's Invoke path
// calls. It mirrors the "SDK endpoints used" block in assign_tenants.go and
// drives the "Required Jamf permissions" table appended to the action
// MarkdownDescription. permissions_test.go asserts this list stays in sync with
// the actual client.<Method> calls and with the SDK privilege registry.
//
// The two list reads are declared because the action checks the platform
// environment and every tenant before it sends anything, and reads the tenants
// back afterwards to confirm the assignment took. Both need the read action in
// addition to the update action the assignment itself needs.
var assignTenantsSDKMethods = []string{
	"AssignTenants",
	"ListEnvironments",
	"ListTenants",
}

// assignTenantsPrivileges is the rendered "Required Jamf permissions" Markdown
// section for the assign tenants action.
var assignTenantsPrivileges = permissions.Section(account.Privileges, assignTenantsSDKMethods...)
