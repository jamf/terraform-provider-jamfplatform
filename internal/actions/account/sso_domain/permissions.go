// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ssodomainaction

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/permissions"
)

// verifySSODomainSDKMethods lists the SDK methods the
// jamfplatform_account_sso_domain_verify action's Invoke path calls. It mirrors the
// "SDK endpoints used" block in verify.go and drives the "Required Jamf
// permissions" table appended to the action MarkdownDescription.
// permissions_test.go asserts this list stays in sync with the actual
// client.<Method> calls and with the SDK privilege registry.
//
// The list read is declared because the action falls back to it whenever a domain
// is named by name rather than by identifier — the common form, since Jamf Account
// never shows the identifier. A caller granted only the update permission can
// still use the `domain_id` form, which is why the attribute's description says so.
var verifySSODomainSDKMethods = []string{
	"VerifyDomain",
	"ListDomains",
}

// verifySSODomainPrivileges is the rendered "Required Jamf permissions" Markdown
// section for the verify action.
var verifySSODomainPrivileges = permissions.Section(account.Privileges, verifySSODomainSDKMethods...)
